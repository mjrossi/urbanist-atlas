package etl

import (
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestTOMLString_Escapes(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", `"plain"`},
		{"Montréal", `"Montréal"`},
		{`say "hi"`, `"say \"hi\""`},
		{`C:\path`, `"C:\\path"`},
		{"a\tb\nc\rd", `"a\tb\nc\rd"`},
		{"\b\f", `"\b\f"`},
		{"\x00", `"\u0000"`},
		{"\x1f", `"\u001F"`},
		{"\x7f", `"\u007F"`},
		{"\u0080", "\"\u0080\""},
	}
	for _, c := range cases {
		if got := tomlString(c.in); got != c.want {
			t.Errorf("tomlString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Every control character must survive a real TOML parser, which
// rejects them raw inside a basic string.
func TestTOMLString_RoundTrip(t *testing.T) {
	inputs := []string{
		"",
		"Montréal",
		"Saint-Jean-sur-Richelieu–Iberville 🚇",
		`quote " and backslash \ together \"`,
		`trailing backslash \`,
	}
	var all strings.Builder
	for r := rune(0); r < 0x20; r++ {
		all.WriteRune(r)
		inputs = append(inputs, "x"+string(r)+"y")
	}
	all.WriteRune(0x7f)
	inputs = append(inputs, all.String(), "x\x7fy")

	for _, in := range inputs {
		doc := "k = " + tomlString(in) + "\n"
		var out struct{ K string }
		if err := toml.Unmarshal([]byte(doc), &out); err != nil {
			t.Errorf("tomlString(%q) produced unparseable TOML %q: %v", in, doc, err)
			continue
		}
		if out.K != in {
			t.Errorf("round trip of %q = %q", in, out.K)
		}
	}
}
