package review

import (
	"regexp"
	"testing"
)

// visible strips agterm's ^[text](attrs) markup and **bold** to the shown text.
func visible(md string) string {
	md = regexp.MustCompile(`\^\[([^\]]*)\]\([^)]*\)`).ReplaceAllString(md, "$1")
	return regexp.MustCompile(`\*\*`).ReplaceAllString(md, "")
}

func findings(sev ...string) []Finding {
	out := make([]Finding, len(sev))
	for i, s := range sev {
		out[i] = Finding{Severity: s}
	}
	return out
}

func TestSummaryLine(t *testing.T) {
	cases := []struct {
		r    Review
		want string
	}{
		{Review{Verdict: "needs-attention", Findings: findings("high", "low", "medium")}, "codex #3: 3 findings (1 high)"},
		{Review{Verdict: "approve", Findings: nil}, "codex #3: approve, no findings"},
		{Review{Verdict: "needs-attention", Findings: findings("medium")}, "codex #3: 1 finding"},
		{Review{Verdict: "needs-attention", Findings: findings("critical", "high", "high", "low")}, "codex #3: 4 findings (1 critical, 2 high)"},
		{Review{Verdict: "needs-attention"}, "codex #3: needs attention, no findings"},
	}
	for _, c := range cases {
		if got := visible(SummaryLine(3, c.r)); got != c.want {
			t.Errorf("SummaryLine = %q (raw %q), want %q", got, SummaryLine(3, c.r), c.want)
		}
	}
	if got := visible(SummaryLine(0, cases[2].r)); got != "codex: 1 finding" {
		t.Errorf("unnumbered = %q", got)
	}
}

func TestReviewingAndFailureLines(t *testing.T) {
	if got := visible(ReviewingLine(8)); got != "codex #8: reviewing" {
		t.Errorf("reviewing = %q", got)
	}
	if got := visible(FailureLine(8, ReasonInvalidOutput)); got != "codex #8: invalid review output" {
		t.Errorf("failure = %q", got)
	}
	if got := visible(FailureLine(0, ReasonSetupFailed)); got != "codex: setup failed" {
		t.Errorf("failure = %q", got)
	}
}
