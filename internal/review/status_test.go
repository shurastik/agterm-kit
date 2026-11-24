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
	l := Label{Harness: "codex", Round: 1}
	cases := []struct {
		r    Review
		want string
	}{
		{Review{Verdict: "needs-attention", Findings: findings("high", "low", "medium")}, "codex r1: 3 findings (1 high)"},
		{Review{Verdict: "approve", Findings: nil}, "codex r1: approve, no findings"},
		{Review{Verdict: "needs-attention", Findings: findings("medium")}, "codex r1: 1 finding"},
		{Review{Verdict: "needs-attention", Findings: findings("critical", "high", "high", "low")}, "codex r1: 4 findings (1 critical, 2 high)"},
		{Review{Verdict: "needs-attention"}, "codex r1: needs attention, no findings"},
	}
	for _, c := range cases {
		if got := visible(SummaryLine(l, c.r)); got != c.want {
			t.Errorf("SummaryLine = %q (raw %q), want %q", got, SummaryLine(l, c.r), c.want)
		}
	}
	if got := visible(SummaryLine(Label{Harness: "claude", Round: 2}, cases[1].r)); got != "claude r2: approve, no findings" {
		t.Errorf("claude round 2 = %q", got)
	}
	if got := visible(SummaryLine(Label{Harness: "codex"}, cases[2].r)); got != "codex: 1 finding" {
		t.Errorf("no round = %q", got)
	}
}

func TestReviewingAndFailureLines(t *testing.T) {
	l := Label{Harness: "codex", Round: 2}
	if got := visible(ReviewingLine(l)); got != "codex r2: reviewing" {
		t.Errorf("reviewing = %q", got)
	}
	if got := visible(FailureLine(l, ReasonInvalidOutput)); got != "codex r2: invalid review output" {
		t.Errorf("failure = %q", got)
	}
	if got := visible(FailureLine(Label{}, ReasonSetupFailed)); got != "review: setup failed" {
		t.Errorf("failure = %q", got)
	}
	if got := ReasonFailed("claude"); got != "claude failed" {
		t.Errorf("reason = %q", got)
	}
}
