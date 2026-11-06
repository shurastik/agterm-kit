package triage

import (
	"fmt"
	"strings"

	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/termsafe"
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

// Backslashes are doubled first so an escaped pipe cannot be un-escaped by
// text that already ends in a backslash.
var cellReplacer = strings.NewReplacer(`\`, `\\`, "\r\n", " ", "\n", " ", "\r", " ", "|", `\|`)

// cell makes untrusted text safe for one table cell on a terminal: line breaks
// become spaces, backslashes and pipes are escaped, and control characters (C0, DEL, C1) and
// bidi overrides are shown as \uXXXX instead of reaching the terminal.
func cell(s string) string {
	s = cellReplacer.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if termsafe.Unsafe(r) {
			fmt.Fprintf(&b, `\u%04x`, r)
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
