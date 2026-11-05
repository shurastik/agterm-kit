package triage

import (
	"fmt"
	"strings"

	"github.com/shurastik/agterm-kit/internal/review"
)

// Table renders the findings as a markdown table for terminals without agterm.
func Table(r review.Review) string {
	var b strings.Builder
	b.WriteString("| # | Severity | Title | Location | Recommendation |\n")
	b.WriteString("|---|----------|-------|----------|----------------|\n")
	for i, f := range r.Findings {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			i+1, cell(f.Severity), cell(f.Title), cell(location(f)), cell(f.Recommendation))
	}
	return b.String()
}

var cellReplacer = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "|", `\|`)

// cell makes untrusted text safe for one table cell on a terminal: line breaks
// become spaces, pipes are escaped, and control characters (C0, DEL, C1) and
// bidi overrides are shown as \uXXXX instead of reaching the terminal.
func cell(s string) string {
	s = cellReplacer.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if unsafeRune(r) {
			fmt.Fprintf(&b, `\u%04x`, r)
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func unsafeRune(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x2028, r == 0x2029:
		return true
	}
	return false
}
