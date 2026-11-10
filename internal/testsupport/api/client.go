// Package api is a small black-box REST client for tests. It knows only the
// public HTTP contract (paths and snake_case JSON from api/proto), never the
// application's internals, so the same helpers serve in-process integration
// tests and tests/e2e against a deployed environment.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Client issues requests against BaseURL.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a client with a sane timeout.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Response is a fully read HTTP response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// JSON decodes the body into v, failing the test on error.
func (r Response) JSON(t testing.TB, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("decode %s: %v", r.Body, err)
	}
}

// Map decodes the body into a generic map.
func (r Response) Map(t testing.TB) map[string]any {
	t.Helper()
	var m map[string]any
	r.JSON(t, &m)
	return m
}

// Expect fails the test unless the status matches.
func (r Response) Expect(t testing.TB, status int) Response {
	t.Helper()
	if r.Status != status {
		t.Fatalf("status = %d, want %d; body: %s", r.Status, status, r.Body)
	}
	return r
}

// Do sends a request. body may be nil, a string/[]byte (sent as-is) or any
// value (JSON-encoded). Transport errors fail the test.
func (c *Client) Do(t testing.TB, method, path string, body any) Response {
	t.Helper()
	res, err := c.Try(method, path, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return res
}

// Try is Do without failing the test, for use inside Eventually. Each request
// is bounded by the client's timeout.
func (c *Client) Try(method, path string, body any) (Response, error) {
	var rdr io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rdr = strings.NewReader(b)
	case []byte:
		rdr = bytes.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			return Response{}, err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.BaseURL+path, rdr)
	if err != nil {
		return Response{}, err
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return Response{}, err
	}
	return Response{Status: res.StatusCode, Header: res.Header, Body: data}, nil
}

// Get is Do(GET).
func (c *Client) Get(t testing.TB, path string) Response {
	t.Helper()
	return c.Do(t, http.MethodGet, path, nil)
}

// Post is Do(POST).
func (c *Client) Post(t testing.TB, path string, body any) Response {
	t.Helper()
	return c.Do(t, http.MethodPost, path, body)
}

// Patch is Do(PATCH).
func (c *Client) Patch(t testing.TB, path string, body any) Response {
	t.Helper()
	return c.Do(t, http.MethodPatch, path, body)
}
