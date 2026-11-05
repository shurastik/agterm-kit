package review

import (
	"fmt"
	"strings"
)

// ReviewingLine is the info line while codex runs.
const ReviewingLine = "codex: reviewing"

// Failure reasons shown in the info line. Fixed strings keep reviewer output
// out of the sidebar markdown; details go to the terminal and codex.log.
const (
	ReasonCodexFailed   = "codex failed"
	ReasonInvalidOutput = "invalid review output"
	ReasonSetupFailed   = "setup failed"
)

// FailureLine is the info line for a failed review.
func FailureLine(reason string) string { return "codex: " + reason }

// SummaryLine is the info line for a finished review, as agterm inline markdown.
func SummaryLine(r Review) string {
	n := len(r.Findings)
	if n == 0 {
		if r.Verdict == "approve" {
			return "codex: ^[approve](color: \"green\"), no findings"
		}
		return "codex: needs attention, no findings"
	}
	counts := map[string]int{}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	noun := "findings"
	if n == 1 {
		noun = "finding"
	}
	line := fmt.Sprintf("codex: **%d** %s", n, noun)
	var parts []string
	for _, s := range []struct{ sev, color string }{{"critical", "red"}, {"high", "orange"}} {
		if c := counts[s.sev]; c > 0 {
			parts = append(parts, fmt.Sprintf("^[%d %s](color: \"%s\")", c, s.sev, s.color))
		}
	}
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, ", ") + ")"
	}
	return line
}
