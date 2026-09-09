// Package arch enforces the dependency rules from PROJECT.md §12 by inspecting
// the real import graph via `go list`. It runs with the normal unit-test suite
// (no build tag) so a boundary violation fails CI immediately.
package arch

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

type pkg struct {
	ImportPath string
	Imports    []string
}

func loadGraph(t *testing.T) []pkg {
	t.Helper()
	// Use the absolute module pattern so this works regardless of the test's
	// working directory (which is this package's dir, not the module root).
	out, err := exec.CommandContext(context.Background(), "go", "list", "-json", mod+"...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var pkgs []pkg
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Generated test doubles (mockery) live in `mocks/` sub-packages and are
		// not subject to the production layering rules.
		if strings.Contains(p.ImportPath, "/mocks") {
			continue
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

const mod = "github.com/example/myapp/"

func TestDomainLayerIsPure(t *testing.T) {
	// Any package whose path contains "/domain" must not import infrastructure,
	// transport, the event bus, or another bounded context's non-domain code.
	forbidden := []string{
		"/infrastructure", "/interfaces", "internal/eventbus",
		"internal/platform", "database/sql", "net/http",
	}
	for _, p := range loadGraph(t) {
		if !strings.Contains(p.ImportPath, "/domain") {
			continue
		}
		for _, imp := range p.Imports {
			for _, bad := range forbidden {
				if strings.Contains(imp, bad) {
					t.Errorf("%s imports %s (domain layer must stay pure)", p.ImportPath, imp)
				}
			}
		}
	}
}

func TestApplicationLayerAvoidsConcreteInfra(t *testing.T) {
	for _, p := range loadGraph(t) {
		if !strings.Contains(p.ImportPath, "/application") {
			continue
		}
		for _, imp := range p.Imports {
			if strings.Contains(imp, "/infrastructure/") || strings.Contains(imp, "/interfaces/") {
				t.Errorf("%s imports %s (application must depend on ports, not adapters)", p.ImportPath, imp)
			}
			if strings.Contains(imp, "net/http") {
				t.Errorf("%s imports net/http (transport leak into application layer)", p.ImportPath)
			}
		}
	}
}

func TestBoundedContextsDoNotImportEachOther(t *testing.T) {
	contexts := []string{"customer", "notification", "order", "payment", "identity"}
	for _, p := range loadGraph(t) {
		self := contextOf(p.ImportPath, contexts)
		if self == "" {
			continue
		}
		for _, imp := range p.Imports {
			other := contextOf(imp, contexts)
			if other != "" && other != self {
				t.Errorf("%s imports %s: bounded context %q must not import %q "+
					"(communicate via events/commands/queries)", p.ImportPath, imp, self, other)
			}
		}
	}
}

func contextOf(importPath string, contexts []string) string {
	rest, ok := strings.CutPrefix(importPath, mod+"internal/")
	if !ok {
		return ""
	}
	name, _, _ := strings.Cut(rest, "/")
	for _, c := range contexts {
		if name == c {
			return c
		}
	}
	return ""
}
