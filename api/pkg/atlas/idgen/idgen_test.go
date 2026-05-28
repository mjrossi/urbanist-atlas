package idgen

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewUUIDv7_producesParseableV7(t *testing.T) {
	g := NewUUIDv7()
	for range 5 {
		got, err := g()
		if err != nil {
			t.Fatalf("generator returned error: %v", err)
		}
		parsed, err := uuid.Parse(got)
		if err != nil {
			t.Fatalf("returned value is not a valid uuid: %q: %v", got, err)
		}
		if parsed.Version() != 7 {
			t.Fatalf("expected uuid version 7, got %d (id=%s)", parsed.Version(), got)
		}
		// UUIDv7 string form is 36 chars with the canonical 8-4-4-4-12.
		if len(got) != 36 || strings.Count(got, "-") != 4 {
			t.Fatalf("returned value not in canonical uuid string form: %q", got)
		}
	}
}

func TestNewUUIDv7_isMonotonicAcrossClose(t *testing.T) {
	g := NewUUIDv7()
	prev, err := g()
	if err != nil {
		t.Fatalf("first generation: %v", err)
	}
	for i := range 20 {
		next, err := g()
		if err != nil {
			t.Fatalf("generation %d: %v", i, err)
		}
		if next <= prev {
			t.Fatalf("UUIDv7 strings expected to sort monotonically; got %s then %s", prev, next)
		}
		prev = next
	}
}

func TestMustGenerate_propagates(t *testing.T) {
	got := MustGenerate(NewUUIDv7())
	if _, err := uuid.Parse(got); err != nil {
		t.Fatalf("MustGenerate returned unparseable id: %q: %v", got, err)
	}
}

func TestMustGenerate_panicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	failing := Generator(func() (string, error) { return "", errBoom })
	_ = MustGenerate(failing)
}

type sentinelErr string

func (e sentinelErr) Error() string { return string(e) }

var errBoom sentinelErr = "boom"
