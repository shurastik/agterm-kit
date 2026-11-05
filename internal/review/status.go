package review

import (
	"fmt"
	"strings"
)

// label is the info-line prefix: "codex #N", or "codex" when N is unknown (0).
func label(n int) string {
	if n > 0 {
		return fmt.Sprintf("codex #%d", n)
	}
	return "codex"
}

// ReviewingLine is the info line while review n runs.
func ReviewingLine(n int) string { return label(n) + ": reviewing" }

// Failure reasons shown in the info line. Fixed strings keep reviewer output
// out of the sidebar markdown; details go to the terminal and codex.log.
const (
	ReasonCodexFailed   = "codex failed"
	ReasonInvalidOutput = "invalid review output"
	ReasonSetupFailed   = "setup failed"
)

// FailureLine is the info line for failed review n.
func FailureLine(n int, reason string) string { return label(n) + ": " + reason }

// SummaryLine is the info line for finished review num, as agterm inline markdown.
func SummaryLine(num int, r Review) string {
	n := len(r.Findings)
	if n == 0 {
		if r.Verdict == "approve" {
			return label(num) + `: ^[approve](color: "green"), no findings`
		}
		return label(num) + ": needs attention, no findings"
	}
	counts := map[string]int{}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	noun := "findings"
	if n == 1 {
		noun = "finding"
	}
	line := fmt.Sprintf("%s: **%d** %s", label(num), n, noun)
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
