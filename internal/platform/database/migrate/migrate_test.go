package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFiles(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func versions(ms []Migration) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Version)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLoad_PairsAndSortsNumerically(t *testing.T) {
	dir := writeFiles(t,
		"10_ten.up.sql", "10_ten.down.sql",
		"2_two.up.sql", "2_two.down.sql",
		"0001_one.up.sql",
		"README.md",
	)
	ms, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := versions(ms); !equal(got, []string{"0001", "2", "10"}) {
		t.Fatalf("order = %v", got)
	}
	if ms[0].DownPath != "" || ms[0].UpPath == "" {
		t.Fatalf("0001 paths = %+v", ms[0])
	}
	if ms[2].Path(Down) == "" || ms[2].Name != "ten" {
		t.Fatalf("10 = %+v", ms[2])
	}
}

func TestPendingUp(t *testing.T) {
	ms := []Migration{{Version: "1"}, {Version: "2"}, {Version: "3"}}
	applied := map[string]bool{"1": true}

	if got := versions(pendingUp(ms, applied, 0)); !equal(got, []string{"2", "3"}) {
		t.Fatalf("all = %v", got)
	}
	if got := versions(pendingUp(ms, applied, 1)); !equal(got, []string{"2"}) {
		t.Fatalf("steps=1 = %v", got)
	}
}

func TestPendingDown(t *testing.T) {
	ms := []Migration{{Version: "1"}, {Version: "2"}, {Version: "3"}}
	applied := map[string]bool{"1": true, "2": true}

	got, err := pendingDown(ms, applied, 0)
	if err != nil || !equal(versions(got), []string{"2"}) {
		t.Fatalf("default = %v err=%v", versions(got), err)
	}
	got, err = pendingDown(ms, applied, 5)
	if err != nil || !equal(versions(got), []string{"2", "1"}) {
		t.Fatalf("steps=5 = %v err=%v", versions(got), err)
	}
	if _, err := pendingDown(ms, map[string]bool{"9": true}, 1); err == nil {
		t.Fatal("want error for applied version without a file")
	}
}
