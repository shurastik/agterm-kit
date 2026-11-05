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
