package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/review"
)

const threeFindings = `{"verdict":"needs-attention","summary":"s","findings":[
 {"severity":"high","title":"a","body":"b","file":"f.go","line_start":1,"line_end":2,"confidence":0.9,"recommendation":"r"},
 {"severity":"low","title":"a","body":"b","file":"f.go","line_start":1,"line_end":2,"confidence":0.9,"recommendation":"r"},
 {"severity":"medium","title":"a","body":"b","file":"f.go","line_start":1,"line_end":2,"confidence":0.9,"recommendation":"r"}
],"next_steps":[]}`

func TestReviewRunFindings(t *testing.T) {
	h := newHarness(t, true)
	root, dir := runDir(t)
	h.runner.output = threeFindings

	if code := h.env.ReviewRun(context.Background(), []string{dir}); code != 0 {
		t.Fatalf("exit %d, stderr %s", code, h.err)
	}
	if !reflect.DeepEqual(h.runner.calls, []string{root, dir, "THE PROMPT"}) {
		t.Fatalf("runner calls = %q", h.runner.calls)
	}
	want := []agtermtest.Call{
		{Method: "Status", Args: []string{"SELF", "active"}},
		{Method: "Info", Args: []string{"SELF", review.ReviewingLine}},
		{Method: "Status", Args: []string{"SELF", "completed"}},
		{Method: "Info", Args: []string{"SELF", `codex: **3** findings (^[1 high](color: "orange"))`}},
	}
	if !reflect.DeepEqual(h.ctl.Calls, want) {
		t.Fatalf("ctl calls = %v", h.ctl.Calls)
	}
	b, err := os.ReadFile(filepath.Join(dir, "review.json"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := review.Parse(b)
	if err != nil || len(r.Findings) != 3 {
		t.Fatalf("review.json invalid: %v", err)
	}
	if !strings.Contains(h.out.String(), "codex streaming") {
		t.Fatal("codex output must stream to the companion terminal")
	}
}

func TestReviewRunApprove(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	h.runner.output = `{"verdict":"approve","summary":"fine","findings":[],"next_steps":[]}`
	if code := h.env.ReviewRun(context.Background(), []string{dir}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	last := h.ctl.Calls[len(h.ctl.Calls)-2:]
	if last[0].Args[1] != "completed" || !strings.Contains(last[1].Args[1], "approve") {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
}

func TestReviewRunCodexFails(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	h.runner.err = errors.New("codex failed: exit status 1")
	if code := h.env.ReviewRun(context.Background(), []string{dir}); code == 0 {
		t.Fatal("want non-zero exit")
	}
	assertBlocked(t, h, review.ReasonCodexFailed)
	if exists(filepath.Join(dir, "review.json")) {
		t.Fatal("review.json must not be written")
	}
	if !strings.Contains(h.err.String(), "exit status 1") {
		t.Fatalf("stderr lacks detail: %s", h.err)
	}
}

func TestReviewRunInvalidOutput(t *testing.T) {
	for name, out := range map[string]string{
		"garbage":   "not json",
		"no output": "",
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			_, dir := runDir(t)
			h.runner.output = out
			if code := h.env.ReviewRun(context.Background(), []string{dir}); code == 0 {
				t.Fatal("want non-zero exit")
			}
			assertBlocked(t, h, review.ReasonInvalidOutput)
			if exists(filepath.Join(dir, "review.json")) {
				t.Fatal("review.json must not be written")
			}
			if out != "" && !exists(filepath.Join(dir, "codex-output.json")) {
				t.Fatal("raw output must be kept")
			}
		})
	}
}

func TestReviewRunBadDir(t *testing.T) {
	h := newHarness(t, true)
	if code := h.env.ReviewRun(context.Background(), []string{t.TempDir()}); code == 0 {
		t.Fatal("dir outside .agk/reviews must be rejected")
	}
	assertBlocked(t, h, review.ReasonSetupFailed)
}

func TestReviewRunOutsideAgterm(t *testing.T) {
	h := newHarness(t, false)
	_, dir := runDir(t)
	h.runner.output = threeFindings
	if code := h.env.ReviewRun(context.Background(), []string{dir}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls expected, got %v", h.ctl.Calls)
	}
}

func assertBlocked(t *testing.T, h *harness, reason string) {
	t.Helper()
	n := len(h.ctl.Calls)
	if n < 2 {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
	want := []agtermtest.Call{
		{Method: "Status", Args: []string{"SELF", "blocked"}},
		{Method: "Info", Args: []string{"SELF", review.FailureLine(reason)}},
	}
	if !reflect.DeepEqual(h.ctl.Calls[n-2:], want) {
		t.Fatalf("last calls = %v, want %v", h.ctl.Calls[n-2:], want)
	}
}
