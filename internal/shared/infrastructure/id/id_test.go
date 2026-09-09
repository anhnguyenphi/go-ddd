package id_test

import (
	"regexp"
	"testing"

	"github.com/example/myapp/internal/shared/infrastructure/id"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNew_ShapeAndUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		v := id.New()
		if !uuidV4.MatchString(v) {
			t.Fatalf("%q is not a v4 UUID", v)
		}
		if _, dup := seen[v]; dup {
			t.Fatalf("collision on %q", v)
		}
		seen[v] = struct{}{}
	}
}

func TestGenerators(t *testing.T) {
	if got := (id.Fixed{Value: "x"}).NewID(); got != "x" {
		t.Fatalf("Fixed.NewID() = %q", got)
	}
	if !uuidV4.MatchString(id.UUID{}.NewID()) {
		t.Fatal("UUID.NewID() did not return a v4 UUID")
	}
}
