// Command mockapi is a small, configurable HTTP server that stands in for an
// external API (a payment gateway, an identity provider, ...) in end-to-end
// tests. It is a generic double, not tied to any one bounded context: point a
// client at it instead of the real provider, and it answers with canned
// responses read from a JSON routes file.
//
// Every request it receives is recorded and can be inspected or cleared
// through its own introspection endpoints, so a test (or an agent driving one)
// can assert "was this called, with what body" without a second capture
// mechanism:
//
//	mockapi -addr :9999 -routes configs/mockapi.routes.example.json
//
//	GET  /__health            -> 200 once the server is up
//	GET  /__requests          -> JSON array of every request received so far
//	POST /__reset             -> clears the recorded requests
//	POST /__routes            -> replaces the route table (body: same shape as -routes file)
//
// Anything else is matched against the configured routes and answered with
// the configured status/body; an unmatched request gets a 404 that lists the
// routes currently configured, to make a wiring mistake obvious.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// route is one canned response. Path must match exactly unless it ends in
// "*", in which case it matches by prefix (e.g. "/v1/charges/*" matches
// "/v1/charges/ch_123").
type route struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
	DelayMs int               `json:"delay_ms,omitempty"`
}

// recordedRequest is what /__requests reports back.
type recordedRequest struct {
	At      time.Time         `json:"at"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   string            `json:"query,omitempty"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

type server struct {
	mu       sync.Mutex
	routes   []route
	requests []recordedRequest
}

const maxRecordedRequests = 2000

func main() {
	addr := flag.String("addr", ":9999", "address to listen on")
	routesPath := flag.String("routes", "", "path to a JSON routes file (see configs/mockapi.routes.example.json); omit for an empty route table")
	flag.Parse()

	srv := &server{}
	if *routesPath != "" {
		routes, err := loadRoutes(*routesPath)
		if err != nil {
			log.Fatalf("mockapi: %v", err)
		}
		srv.routes = routes
		log.Printf("mockapi: loaded %d route(s) from %s", len(routes), *routesPath)
	}

	log.Printf("mockapi: listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		log.Fatalf("mockapi: %v", err)
	}
}

func loadRoutes(path string) ([]route, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read routes: %w", err)
	}
	return parseRoutes(b)
}

func parseRoutes(b []byte) ([]route, error) {
	var routes []route
	if err := json.Unmarshal(b, &routes); err != nil {
		return nil, fmt.Errorf("parse routes: %w", err)
	}
	for i, r := range routes {
		if r.Method == "" || r.Path == "" {
			return nil, fmt.Errorf("route %d: method and path are required", i)
		}
		if r.Status == 0 {
			routes[i].Status = http.StatusOK
		}
	}
	return routes, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/__health":
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case r.URL.Path == "/__requests" && r.Method == http.MethodGet:
		s.handleListRequests(w)
	case r.URL.Path == "/__reset" && r.Method == http.MethodPost:
		s.handleReset(w)
	case r.URL.Path == "/__routes" && r.Method == http.MethodPost:
		s.handleSetRoutes(w, r)
	default:
		s.handleMocked(w, r)
	}
}

func (s *server) handleListRequests(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.requests)
}

func (s *server) handleReset(w http.ResponseWriter) {
	s.mu.Lock()
	s.requests = nil
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (s *server) handleSetRoutes(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	routes, err := parseRoutes(b)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.mu.Lock()
	s.routes = routes
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "routes": len(routes)})
}

func (s *server) handleMocked(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.record(r, body)

	route, ok := s.match(r.Method, r.URL.Path)
	if !ok {
		s.mu.Lock()
		available := make([]string, len(s.routes))
		for i, rt := range s.routes {
			available[i] = rt.Method + " " + rt.Path
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":     fmt.Sprintf("mockapi: no route for %s %s", r.Method, r.URL.Path),
			"available": available,
		})
		return
	}

	if route.DelayMs > 0 {
		time.Sleep(time.Duration(route.DelayMs) * time.Millisecond)
	}
	for k, v := range route.Headers {
		w.Header().Set(k, v)
	}
	if _, set := route.Headers["Content-Type"]; !set && len(route.Body) > 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(route.Status)
	if len(route.Body) > 0 {
		_, _ = w.Write(route.Body)
	}
}

func (s *server) match(method, path string) (route, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rt := range s.routes {
		if !strings.EqualFold(rt.Method, method) {
			continue
		}
		if strings.HasSuffix(rt.Path, "*") {
			if strings.HasPrefix(path, strings.TrimSuffix(rt.Path, "*")) {
				return rt, true
			}
			continue
		}
		if rt.Path == path {
			return rt, true
		}
	}
	return route{}, false
}

func (s *server) record(r *http.Request, body []byte) {
	headers := make(map[string]string, len(r.Header))
	for k := range r.Header {
		headers[k] = r.Header.Get(k)
	}
	rec := recordedRequest{
		At:      time.Now().UTC(),
		Method:  r.Method,
		Path:    r.URL.Path,
		Query:   r.URL.RawQuery,
		Headers: headers,
	}
	if json.Valid(body) {
		rec.Body = body
	} else if len(body) > 0 {
		b, _ := json.Marshal(string(body))
		rec.Body = b
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, rec)
	if len(s.requests) > maxRecordedRequests {
		s.requests = s.requests[len(s.requests)-maxRecordedRequests:]
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
