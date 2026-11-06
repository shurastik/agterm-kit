package review

import (
	"fmt"
	"strings"
)

// Label identifies a review round in info lines: "codex #4 r2".
type Label struct {
	Harness    string
	Num, Round int // 0 when unknown
}

// String is the info-line prefix; unknown parts are left out, and an unknown
// harness reads "review".
func (l Label) String() string {
	s := l.Harness
	if s == "" {
		s = "review"
	}
	if l.Num > 0 {
		s += fmt.Sprintf(" #%d", l.Num)
		if l.Round > 0 {
			s += fmt.Sprintf(" r%d", l.Round)
		}
	}
	return s
}

func label(l Label) string { return l.String() }

// ReviewingLine is the info line while a round runs.
func ReviewingLine(l Label) string { return label(l) + ": reviewing" }

// Failure reasons shown in the info line. Fixed strings keep reviewer output
// out of the sidebar markdown; details go to the terminal and codex.log.
const (
	ReasonInvalidOutput = "invalid review output"
	ReasonSetupFailed   = "setup failed"
)

// ReasonFailed is the reason for a reviewer process that failed.
func ReasonFailed(harness string) string { return harness + " failed" }

// FailureLine is the info line for a failed round.
func FailureLine(l Label, reason string) string { return label(l) + ": " + reason }

// SummaryLine is the info line for a finished round, as agterm inline markdown.
func SummaryLine(num Label, r Review) string {
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
