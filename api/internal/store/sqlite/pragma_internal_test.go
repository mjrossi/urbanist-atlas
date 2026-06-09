package sqlite

import "testing"

// TestEscapePragmas_EscapesBothHalves pins issue #29: escapePragmas must
// percent-escape both the key and the value of each key=value pragma,
// not just the value. The production keys are a fixed `_pragma` set
// (unchanged by escaping), but the function must stay correct for any
// input — a key carrying a reserved character must not slip through raw.
func TestEscapePragmas_EscapesBothHalves(t *testing.T) {
	// Real pragmas: key `_pragma` is unreserved (passes through), value's
	// parens are escaped. This is the byte-for-byte contract the modernc
	// DSN relies on.
	got := escapePragmas([]string{"_pragma=journal_mode(WAL)"})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0] != "_pragma=journal_mode%28WAL%29" {
		t.Fatalf("real pragma = %q, want _pragma=journal_mode%%28WAL%%29", got[0])
	}

	// A key with a reserved character must be escaped too (the #29 gap:
	// previously the key half was emitted raw). `&` would otherwise start
	// a new query parameter and corrupt the DSN.
	got = escapePragmas([]string{"a&b=c d"})
	if got[0] != "a%26b=c+d" {
		t.Fatalf("reserved-key pragma = %q, want a%%26b=c+d", got[0])
	}

	// No '=' at all: the whole token is escaped rather than passed raw.
	got = escapePragmas([]string{"no equals&here"})
	if got[0] != "no+equals%26here" {
		t.Fatalf("no-eq pragma = %q, want no+equals%%26here", got[0])
	}
}
