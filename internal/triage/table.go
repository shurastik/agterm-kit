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

func cell(s string) string { return strings.TrimSpace(cellReplacer.Replace(s)) }
