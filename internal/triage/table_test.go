package triage

import (
	"strings"
	"testing"
)

func TestTable(t *testing.T) {
	r := sample()
	r.Findings[0].Title = "a | b\nc"
	r.Findings[1].Recommendation = "x|y\r\nz"
	out := Table(r)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2+len(r.Findings) {
		t.Fatalf("want header, separator and one row per finding, got %d lines:\n%s", len(lines), out)
	}
	if lines[0] != "| # | Severity | Title | Location | Recommendation |" {
		t.Fatalf("header = %q", lines[0])
	}
	for i, l := range lines {
		if n := strings.Count(strings.ReplaceAll(l, `\|`, ""), "|"); n != 6 {
			t.Errorf("line %d has %d unescaped pipes: %q", i, n, l)
		}
	}
	if !strings.Contains(lines[2], `| 1 | high | a \| b c | internal/cache.go:40-52 | Guard with sync.Mutex. |`) {
		t.Errorf("row 1 = %q", lines[2])
	}
	if !strings.Contains(lines[3], `x\|y z`) {
		t.Errorf("row 2 = %q", lines[3])
	}
}

// Finding text is untrusted: terminal control sequences (ANSI CSI, OSC 52
// clipboard writes) and bidi overrides must reach stdout only as visible escapes.
func TestTableNeutralizesControlCharacters(t *testing.T) {
	r := sample()
	r.Findings[0].Title = "\x1b[2J\x1b[31mred"
	r.Findings[0].Recommendation = "\x1b]52;c;dGVzdA==\x07copy"
	r.Findings[1].Title = "evil\u202egnp.exe\u009b31m\x7f"
	r.Findings[1].Recommendation = "raw\x9bbyte" // invalid UTF-8 C1 byte
	out := Table(r)
	for _, b := range []rune{0x1b, 0x07, 0x7f, 0x9b, 0x202e} {
		if strings.ContainsRune(out, b) {
			t.Errorf("raw control %U in table output", b)
		}
	}
	if strings.IndexByte(out, 0x9b) >= 0 {
		t.Error("raw 0x9b byte in table output")
	}
	for _, want := range []string{`\u001b[2J`, `\u001b]52;c;dGVzdA==\u0007copy`, `evil\u202egnp.exe\u009b31m\u007f`} {
		if !strings.Contains(out, want) {
			t.Errorf("visible escape %q missing from:\n%s", want, out)
		}
	}
}
