package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/harness"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

const threeFindings = `{"verdict":"needs-attention","summary":"s","findings":[
 {"severity":"high","title":"a","body":"b","file":"f.go","line_start":1,"line_end":2,"confidence":0.9,"recommendation":"r"},
 {"severity":"low","title":"a","body":"b","file":"f.go","line_start":1,"line_end":2,"confidence":0.9,"recommendation":"r"},
 {"severity":"medium","title":"a","body":"b","file":"f.go","line_start":1,"line_end":2,"confidence":0.9,"recommendation":"r"}
],"next_steps":[]}`

const approve = `{"verdict":"approve","summary":"fine","findings":[],"next_steps":[]}`

func readState(t *testing.T, dir string) reviewState {
	t.Helper()
	var s reviewState
	if have, err := rundir.ReadJSON(dir, workerFile, &s); !have || err != nil {
		t.Fatalf("worker.json: %v %v", have, err)
	}
	return s
}

func assertResult(t *testing.T, rd, status string, findings int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(rd, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Status   string `json:"status"`
		Round    int    `json:"round"`
		Reason   string `json:"reason"`
		Findings int    `json:"findings"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.Status != status || r.Findings != findings || r.Round != roundOf(rd) {
		t.Fatalf("result.json = %s (%v)", b, err)
	}
	if status == "blocked" && r.Reason == "" {
		t.Fatal("blocked result needs a reason")
	}
}

func roundOf(rd string) int {
	var k int
	json.Unmarshal([]byte(filepath.Base(rd)), &k)
	return k
}

// The inline review (outside agterm) runs the reviewer headless in the
// foreground; these cover its round bookkeeping.

func inlineStart(t *testing.T, h *tenv, args ...string) (root, dir string, code int) {
	t.Helper()
	root, _ = filepath.EvalSymlinks(t.TempDir())
	scriptScope(h, root)
	code = h.env.ReviewStart(context.Background(), args)
	return root, filepath.Join(root, ".agk", "reviews", "20261105-100000"), code
}

func TestInlineReviewRecordsFindings(t *testing.T) {
	h := newHarness(t, false)
	h.drivers["codex"].outputs = []string{threeFindings}
	_, dir, code := inlineStart(t, h)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	rd := filepath.Join(dir, "rounds", "1")
	assertResult(t, rd, "completed", 3)
	if !exists(filepath.Join(rd, "review.json")) {
		t.Fatal("review.json missing")
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls expected, got %v", h.ctl.Calls)
	}
}

func TestInlineReviewInvalidOutput(t *testing.T) {
	for name, out := range map[string]string{"garbage": "not json", "no output": ""} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, false)
			h.drivers["codex"].outputs = []string{out}
			_, dir, code := inlineStart(t, h)
			if code == 0 {
				t.Fatal("want non-zero exit")
			}
			rd := filepath.Join(dir, "rounds", "1")
			assertResult(t, rd, "blocked", 0)
			if !strings.Contains(h.err.String(), "invalid review output") {
				t.Fatalf("stderr = %q", h.err)
			}
			if exists(filepath.Join(rd, "review.json")) {
				t.Fatal("review.json must not be written")
			}
			if out != "" && !exists(filepath.Join(rd, harness.OutputFile)) {
				t.Fatal("raw output must be kept")
			}
		})
	}
}

func TestInlineReviewerFailure(t *testing.T) {
	h := newHarness(t, false)
	h.drivers["codex"].errs = []error{errors.New("codex failed: boom")}
	_, dir, code := inlineStart(t, h)
	if code == 0 {
		t.Fatal("want non-zero exit")
	}
	assertResult(t, filepath.Join(dir, "rounds", "1"), "blocked", 0)
	if !strings.Contains(h.err.String(), "codex failed") {
		t.Fatalf("stderr = %q", h.err)
	}
}

func TestInlineReviewPrintsTimestampedHeader(t *testing.T) {
	h := newHarness(t, false)
	h.drivers["codex"].outputs = []string{threeFindings}
	_, dir, _ := inlineStart(t, h)
	// Reviewer output goes to stderr inline so stdout stays JSON.
	first := strings.SplitN(h.err.String(), "\n", 2)[0]
	if want := "[2026-11-05 10:00:00] agk: review round 1 started, run dir " + dir; first != want {
		t.Fatalf("header = %q, want %q", first, want)
	}
	if !strings.Contains(h.err.String(), "[2026-11-05 10:00:00] agk: review round 1: 3 finding(s)") {
		t.Fatalf("footer missing:\n%s", h.err)
	}
}

func TestInlineClaudeReviewer(t *testing.T) {
	h := newHarness(t, false)
	h.drivers["claude"].outputs = []string{approve}
	_, _, code := inlineStart(t, h, "--reviewer", "claude:claude-opus-5-5")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if tr := h.drivers["claude"].turns; len(tr) != 1 || tr[0].Worker.Model != "claude-opus-5-5" {
		t.Fatalf("claude turns = %+v", tr)
	}
}
