// Package arch enforces the dependency rules from PROJECT.md §12 by inspecting
// the real import graph via `go list`. It runs with the normal unit-test suite
// (no build tag) so a boundary violation fails CI immediately.
package arch

import (
	"context"
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

type pkg struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

func loadGraph(t *testing.T) []pkg { return loadGraphTags(t, "") }

// loadGraphTags is loadGraph with build tags, for packages whose files are all
// behind a tag (tests/e2e, tests/integration).
func loadGraphTags(t *testing.T, tags string) []pkg {
	t.Helper()
	// Use the absolute module pattern so this works regardless of the test's
	// working directory (which is this package's dir, not the module root).
	args := []string{"list", "-json"}
	if tags != "" {
		args = append(args, "-tags="+tags)
	}
	args = append(args, mod+"...")
	out, err := exec.CommandContext(context.Background(), "go", args...).Output()
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

// isTestOnly reports whether importPath is a test-helper package: the
// internal/testsupport tree, or a conformance-suite package such as
// persistence/repotest (last path element ending in "test").
func isTestOnly(importPath string) bool {
	if !strings.HasPrefix(importPath, mod) {
		return false
	}
	if strings.Contains(importPath, "/internal/testsupport") {
		return true
	}
	last := importPath[strings.LastIndex(importPath, "/")+1:]
	return strings.HasSuffix(last, "test")
}

// Test helpers pull in testing, testcontainers and the like; production code
// must never link them. Only _test files, tests/... and other helpers may.
func TestTestOnlyPackagesStayOutOfProduction(t *testing.T) {
	for _, p := range loadGraph(t) {
		if isTestOnly(p.ImportPath) || strings.HasPrefix(p.ImportPath, mod+"tests/") {
			continue
		}
		for _, imp := range p.Imports {
			if isTestOnly(imp) {
				t.Errorf("%s imports test-only package %s (only _test.go files may)", p.ImportPath, imp)
			}
		}
	}
}

// The e2e suite and the helpers it uses are black-box: they may only know the
// public contract (api/proto) so they can run against any deployment.
func TestE2EIsBlackBox(t *testing.T) {
	blackBox := []string{mod + "tests/e2e", mod + "internal/testsupport/api", mod + "internal/testsupport/scenarios"}
	allowed := []string{mod + "api/proto/", mod + "internal/testsupport/api", mod + "internal/testsupport/scenarios"}

	seen := 0
	for _, p := range loadGraphTags(t, "e2e") {
		if !slices.Contains(blackBox, p.ImportPath) {
			continue
		}
		seen++
		imports := slices.Concat(p.Imports, p.TestImports, p.XTestImports)
		for _, imp := range imports {
			if !strings.HasPrefix(imp, mod) {
				continue
			}
			ok := slices.ContainsFunc(allowed, func(a string) bool { return strings.HasPrefix(imp, a) })
			if !ok {
				t.Errorf("%s imports %s; black-box test code may only import api/proto and testsupport/{api,scenarios}", p.ImportPath, imp)
			}
		}
	}
	if seen != len(blackBox) {
		t.Fatalf("found %d of %d black-box packages; did one move?", seen, len(blackBox))
	}
}
