package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Domain memory: every bounded context keeps what an agent (or a new human)
// cannot cheaply re-derive from the code in internal/<ctx>/CONTEXT.md: the
// language, the invariants, the events that cross its boundary, and why.
// These tests keep that file from drifting away from the code. The reference
// is internal/customer/CONTEXT.md; the template is
// .agents/skills/update-memory/context-template.md.

const (
	contextFile     = "CONTEXT.md"
	contextMapFile  = "docs/agent/context-map.md"
	contextMaxLines = 200

	secAggregates = "Aggregates"
	secPublished  = "Events published"
	secConsumed   = "Events consumed"
)

// contextSections are the headings every CONTEXT.md must have, so an agent
// always finds the same thing in the same place.
var contextSections = []string{
	"Language", secAggregates, secPublished, secConsumed,
	"Synchronous dependencies", "Decisions", "Open questions",
}

var (
	// An integration event name: <context>.v<N>.<event>.
	eventNameRE = regexp.MustCompile(`^[a-z_]+\.v\d+\.[a-z_]+$`)
	backtickRE  = regexp.MustCompile("`([^`]+)`")
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s has no go.mod: %v", root, err)
	}
	return root
}

// boundedContexts lists every internal/<name> that has a domain/ directory,
// except shared (the shared kernel is not a context).
func boundedContexts(t *testing.T) []string {
	t.Helper()
	internal := filepath.Join(repoRoot(t), "internal")
	entries, err := os.ReadDir(internal)
	if err != nil {
		t.Fatalf("read internal/: %v", err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "shared" {
			continue
		}
		if st, err := os.Stat(filepath.Join(internal, e.Name(), "domain")); err == nil && st.IsDir() {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		t.Fatal("found no bounded context (internal/<name>/domain); did the layout change?")
	}
	return out
}

// contextCode is what the code of one bounded context says about itself.
type contextCode struct {
	aggregates []string // types embedding AggregateRoot in <ctx>/domain
	events     []string // integration event names used as string literals
}

func scanContext(t *testing.T, dir string) contextCode {
	t.Helper()
	var code contextCode
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "mocks" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		inDomain := strings.HasPrefix(filepath.ToSlash(rel), "domain/")
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.TypeSpec:
				if inDomain && embedsAggregateRoot(n) {
					code.aggregates = append(code.aggregates, n.Name.Name)
				}
			case *ast.BasicLit:
				if n.Kind != token.STRING {
					return true
				}
				if s, err := strconv.Unquote(n.Value); err == nil && eventNameRE.MatchString(s) {
					code.events = append(code.events, s)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s: %v", dir, err)
	}
	slices.Sort(code.aggregates)
	slices.Sort(code.events)
	code.events = slices.Compact(code.events)
	return code
}

func embedsAggregateRoot(spec *ast.TypeSpec) bool {
	st, ok := spec.Type.(*ast.StructType)
	if !ok {
		return false
	}
	for _, f := range st.Fields.List {
		if len(f.Names) != 0 {
			continue
		}
		if sel, ok := f.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "AggregateRoot" {
			return true
		}
	}
	return false
}

// markdownSections splits a document on its "## " headings.
func markdownSections(doc string) map[string]string {
	out := map[string]string{}
	name := ""
	var body []string
	flush := func() {
		if name != "" {
			out[name] = strings.Join(body, "\n")
		}
	}
	for line := range strings.SplitSeq(doc, "\n") {
		if h, ok := strings.CutPrefix(line, "## "); ok {
			flush()
			name, body = strings.TrimSpace(h), nil
			continue
		}
		body = append(body, line)
	}
	flush()
	return out
}

// backticked returns every `code span` in a markdown fragment.
func backticked(md string) []string {
	var out []string
	for _, m := range backtickRE.FindAllStringSubmatch(md, -1) {
		out = append(out, m[1])
	}
	return out
}

// eventsIn returns the integration event names written as code spans.
func eventsIn(md string) []string {
	var out []string
	for _, s := range backticked(md) {
		if eventNameRE.MatchString(s) {
			out = append(out, s)
		}
	}
	return out
}

func eventOwner(event string) string {
	owner, _, _ := strings.Cut(event, ".")
	return owner
}

func TestEveryContextHasDomainMemory(t *testing.T) {
	root := repoRoot(t)
	for _, ctx := range boundedContexts(t) {
		path := filepath.Join("internal", ctx, contextFile)
		raw, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Errorf("%s is missing: every bounded context records its language, invariants, "+
				"events and decisions there (copy .agents/skills/update-memory/context-template.md; "+
				"internal/customer/CONTEXT.md is the reference)", path)
			continue
		}
		doc := string(raw)
		if want := "# " + ctx + " context"; !strings.HasPrefix(doc, want+"\n") {
			t.Errorf("%s must start with %q", path, want)
		}
		sections := markdownSections(doc)
		for _, s := range contextSections {
			if _, ok := sections[s]; !ok {
				t.Errorf("%s has no %q section", path, "## "+s)
			}
		}
		if n := strings.Count(doc, "\n"); n > contextMaxLines {
			t.Errorf("%s is %d lines (max %d): it is read before every task in this context, "+
				"so keep the rules and drop the narrative", path, n, contextMaxLines)
		}
	}
}

func TestDomainMemoryMatchesCode(t *testing.T) {
	root := repoRoot(t)
	aggregates, events := 0, 0
	for _, ctx := range boundedContexts(t) {
		code := scanContext(t, filepath.Join(root, "internal", ctx))
		aggregates += len(code.aggregates)
		events += len(code.events)

		path := filepath.Join("internal", ctx, contextFile)
		raw, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			continue // reported by TestEveryContextHasDomainMemory
		}
		sections := markdownSections(string(raw))

		documented := backticked(sections[secAggregates])
		for _, agg := range code.aggregates {
			if !slices.Contains(documented, agg) {
				t.Errorf("%s: aggregate `%s` (internal/%s/domain) is not under %q; "+
					"add it with the invariants it guards", path, agg, ctx, "## "+secAggregates)
			}
		}

		published, consumed := eventsIn(sections[secPublished]), eventsIn(sections[secConsumed])
		for _, ev := range code.events {
			section, listed := secConsumed, consumed
			if eventOwner(ev) == ctx {
				section, listed = secPublished, published
			}
			if !slices.Contains(listed, ev) {
				t.Errorf("%s: the code uses integration event `%s` but %q does not list it",
					path, ev, "## "+section)
			}
		}
		for _, ev := range published {
			switch {
			case eventOwner(ev) != ctx:
				t.Errorf("%s: `%s` under %q belongs to context %q; a context publishes only its own events",
					path, ev, "## "+secPublished, eventOwner(ev))
			case !slices.Contains(code.events, ev):
				t.Errorf("%s: `%s` is listed under %q but nothing in internal/%s names it; "+
					"remove it, or move a planned event to \"## Open questions\"", path, ev, "## "+secPublished, ctx)
			}
		}
		for _, ev := range consumed {
			switch {
			case eventOwner(ev) == ctx:
				t.Errorf("%s: `%s` under %q is this context's own event; list it under %q",
					path, ev, "## "+secConsumed, "## "+secPublished)
			case !slices.Contains(code.events, ev):
				t.Errorf("%s: `%s` is listed under %q but nothing in internal/%s subscribes to it; "+
					"remove it, or move a planned event to \"## Open questions\"", path, ev, "## "+secConsumed, ctx)
			}
		}
	}
	// Exit 0 is not proof that something was compared.
	if aggregates == 0 || events == 0 {
		t.Fatalf("scanned %d aggregates and %d integration events; the scanner no longer matches the code",
			aggregates, events)
	}
}

func TestContextMapIndexesEveryContext(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, contextMapFile))
	if err != nil {
		t.Fatalf("%s is missing: it is the index of every bounded context and how they relate", contextMapFile)
	}
	for _, ctx := range boundedContexts(t) {
		if link := "internal/" + ctx + "/" + contextFile; !strings.Contains(string(raw), link) {
			t.Errorf("%s does not link %s", contextMapFile, link)
		}
	}
}
