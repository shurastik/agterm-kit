package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/harness"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

const threeFindings = `{"verdict":"needs-attention","summary":"s","findings":[
 {"severity":"high","title":"a","body":"b","file":"f.go","line_start":1,"line_end":2,"confidence":0.9,"recommendation":"r"},
 {"severity":"low","title":"a","body":"b","file":"f.go","line_start":1,"line_end":2,"confidence":0.9,"recommendation":"r"},
 {"severity":"medium","title":"a","body":"b","file":"f.go","line_start":1,"line_end":2,"confidence":0.9,"recommendation":"r"}
],"next_steps":[]}`

const approve = `{"verdict":"approve","summary":"fine","findings":[],"next_steps":[]}`

var errMailboxClosed = errors.New("mailbox closed")

// mailbox scripts the reviewer loop's MsgRead: each step runs (e.g. to
// simulate `review send`) and delivers its message; then the mailbox fails,
// which ends the loop.
func mailbox(h *tenv, steps ...func() string) {
	i := 0
	h.ctl.OnMsgRead = func() ([]agterm.Message, error) {
		if i >= len(steps) {
			return nil, errMailboxClosed
		}
		body := steps[i]()
		i++
		return []agterm.Message{{ID: "M", Body: body}}, nil
	}
}

// nextRound records round k with prompt like `review send` does.
func nextRound(t *testing.T, dir string, k int, prompt string) {
	t.Helper()
	rd, err := rundir.MakeRound(dir, k)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(rd, "prompt.md"), []byte(prompt), 0o644)
	if err := rundir.SetRound(dir, k); err != nil {
		t.Fatal(err)
	}
}

func readState(t *testing.T, dir string) reviewState {
	t.Helper()
	var s reviewState
	if have, err := rundir.ReadJSON(dir, workerFile, &s); !have || err != nil {
		t.Fatalf("worker.json: %v %v", have, err)
	}
	return s
}

func TestReviewRunFindings(t *testing.T) {
	h := newHarness(t, true)
	h.vars["AGTERM_PANE_ID"] = "PANE-1"
	root, dir := runDir(t)
	codex := h.drivers["codex"]
	codex.outputs = []string{threeFindings}
	codex.sessions = []string{"T-1"}
	mailbox(h)

	h.env.ReviewRun(context.Background(), []string{dir})
	rd := filepath.Join(dir, "rounds", "1")
	want := harness.Turn{Worker: config.Worker{Harness: "codex"}, Root: root, Dir: rd, Log: "reviewer.log", Prompt: "THE PROMPT"}
	if len(codex.turns) != 1 || !reflect.DeepEqual(codex.turns[0], want) {
		t.Fatalf("turns = %+v", codex.turns)
	}
	wantCalls := []agtermtest.Call{
		{Method: "Status", Args: []string{"SELF", "active"}},
		{Method: "Info", Args: []string{"SELF", "codex #1 r1: reviewing"}},
		{Method: "Status", Args: []string{"SELF", "completed"}},
		{Method: "Info", Args: []string{"SELF", `codex #1 r1: **3** findings (^[1 high](color: "orange"))`}},
		{Method: "MsgRead", Args: []string{"0s"}},
	}
	if !reflect.DeepEqual(h.ctl.Calls, wantCalls) {
		t.Fatalf("ctl calls = %v", h.ctl.Calls)
	}
	b, err := os.ReadFile(filepath.Join(rd, "review.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r, err := review.Parse(b); err != nil || len(r.Findings) != 3 {
		t.Fatalf("review.json invalid: %v", err)
	}
	assertResult(t, rd, "completed", 3)
	if !strings.Contains(h.out.String(), "codex streaming") {
		t.Fatal("reviewer output must stream to the companion terminal")
	}
	if s := readState(t, dir); s.Session != "T-1" {
		t.Fatalf("session not recorded: %+v", s)
	}
	if !exists(filepath.Join(rd, startedFile)) {
		t.Fatal("the loop marks the round started")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, paneFile)); string(b) != "PANE-1\n" {
		t.Fatalf("pane file = %q", b)
	}
}

func TestReviewRunApprove(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	h.drivers["codex"].outputs = []string{approve}
	mailbox(h)
	h.env.ReviewRun(context.Background(), []string{dir})
	c := h.ctl.Calls
	if c[2].Args[1] != "completed" || !strings.Contains(c[3].Args[1], "approve") {
		t.Fatalf("calls = %v", c)
	}
}

func TestReviewRunReviewerFailsKeepsWaiting(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	h.drivers["codex"].errs = []error{errors.New("codex failed: exit status 1")}
	mailbox(h)
	h.env.ReviewRun(context.Background(), []string{dir})
	assertBlockedBefore(t, h, review.Label{Harness: "codex", Num: 1, Round: 1}, "codex failed")
	rd := filepath.Join(dir, "rounds", "1")
	if exists(filepath.Join(rd, "review.json")) {
		t.Fatal("review.json must not be written")
	}
	assertResult(t, rd, "blocked", 0)
	if !strings.Contains(h.err.String(), "exit status 1") {
		t.Fatalf("stderr lacks detail: %s", h.err)
	}
	if m := h.ctl.Methods(); m[len(m)-1] != "MsgRead" {
		t.Fatalf("a failed round must keep the companion waiting: %v", m)
	}
}

func TestReviewRunInvalidOutput(t *testing.T) {
	for name, out := range map[string]string{"garbage": "not json", "no output": ""} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			_, dir := runDir(t)
			h.drivers["codex"].outputs = []string{out}
			mailbox(h)
			h.env.ReviewRun(context.Background(), []string{dir})
			assertBlockedBefore(t, h, review.Label{Harness: "codex", Num: 1, Round: 1}, review.ReasonInvalidOutput)
			rd := filepath.Join(dir, "rounds", "1")
			if exists(filepath.Join(rd, "review.json")) {
				t.Fatal("review.json must not be written")
			}
			if out != "" && !exists(filepath.Join(rd, harness.OutputFile)) {
				t.Fatal("raw output must be kept")
			}
		})
	}
}

func TestReviewRunBadDir(t *testing.T) {
	for name, mk := range map[string]func(t *testing.T) string{
		"outside .agk":   func(t *testing.T) string { return t.TempDir() },
		"no worker.json": func(t *testing.T) string { _, d := runDir(t); os.Remove(filepath.Join(d, workerFile)); return d },
		"no round file":  func(t *testing.T) string { _, d := runDir(t); os.Remove(filepath.Join(d, rundir.RoundFile)); return d },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, true)
			if code := h.env.ReviewRun(context.Background(), []string{mk(t)}); code == 0 {
				t.Fatal("want non-zero exit")
			}
			c := h.ctl.Calls
			if len(c) < 2 || c[len(c)-2].Args[1] != "blocked" || !strings.HasSuffix(c[len(c)-1].Args[1], review.ReasonSetupFailed) {
				t.Fatalf("calls = %v", c)
			}
		})
	}
}

func TestReviewRunOutsideAgtermRunsOneRound(t *testing.T) {
	h := newHarness(t, false)
	_, dir := runDir(t)
	h.drivers["codex"].outputs = []string{threeFindings}
	if code := h.env.ReviewRun(context.Background(), []string{dir}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(h.ctl.Calls) != 0 {
		t.Fatalf("no agterm calls expected, got %v", h.ctl.Calls)
	}
}

// assertBlockedBefore checks the blocked status and info line right before the
// loop's first MsgRead.
func assertBlockedBefore(t *testing.T, h *tenv, l review.Label, reason string) {
	t.Helper()
	m := h.ctl.Methods()
	i := len(m)
	for j, name := range m {
		if name == "MsgRead" {
			i = j
			break
		}
	}
	if i < 2 {
		t.Fatalf("calls = %v", h.ctl.Calls)
	}
	want := []agtermtest.Call{
		{Method: "Status", Args: []string{"SELF", "blocked"}},
		{Method: "Info", Args: []string{"SELF", review.FailureLine(l, reason)}},
	}
	if !reflect.DeepEqual(h.ctl.Calls[i-2:i], want) {
		t.Fatalf("calls = %v, want %v before MsgRead", h.ctl.Calls[i-2:i], want)
	}
}

func TestReviewRunReportsBlockedAfterCancel(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the companion is closed: SIGHUP before or during the run
	h.drivers["codex"].errs = []error{context.Canceled}
	if code := h.env.ReviewRun(ctx, []string{dir}); code == 0 {
		t.Fatal("want non-zero exit")
	}
	c := h.ctl.Calls
	n := len(c)
	if c[n-2].Args[1] != "blocked" || c[n-1].Args[1] != "codex #1 r1: codex failed" {
		t.Fatalf("calls = %v", c)
	}
	if h.ctl.CtxErrs[n-2] != nil || h.ctl.CtxErrs[n-1] != nil {
		t.Fatalf("final status sent with a cancelled context: %v", h.ctl.CtxErrs)
	}
	for _, m := range h.ctl.Methods() {
		if m == "MsgRead" {
			t.Fatal("a cancelled run must not wait for another round")
		}
	}
}

func TestReviewRunUsesItsOwnNumber(t *testing.T) {
	h := newHarness(t, true)
	root, dir := runDir(t) // 20261105-100000 -> #1
	later := filepath.Join(root, ".agk", "reviews", "20261105-110000")
	os.MkdirAll(filepath.Join(later, "rounds", "1"), 0o755)
	b, _ := os.ReadFile(filepath.Join(dir, workerFile))
	os.WriteFile(filepath.Join(later, workerFile), b, 0o644)
	rundir.SetRound(later, 1)
	os.WriteFile(filepath.Join(later, "rounds", "1", "prompt.md"), []byte("P"), 0o644)
	h.drivers["codex"].outputs = []string{threeFindings}
	mailbox(h)
	h.env.ReviewRun(context.Background(), []string{later})
	if got := h.ctl.Calls[1].Args[1]; got != "codex #2 r1: reviewing" {
		t.Fatalf("info = %q", got)
	}
}

func TestReviewRunPrintsTimestampedHeader(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	h.drivers["codex"].outputs = []string{threeFindings}
	mailbox(h)
	h.env.ReviewRun(context.Background(), []string{dir})
	first := strings.SplitN(h.out.String(), "\n", 2)[0]
	if want := "[2026-11-05 10:00:00] agk: review #1 round 1 started, run dir " + dir; first != want {
		t.Fatalf("header = %q, want %q", first, want)
	}
	if !strings.Contains(h.out.String(), "[2026-11-05 10:00:00] agk: review #1 round 1: 3 finding(s)") {
		t.Fatalf("footer missing:\n%s", h.out)
	}
}

// A lost final status would leave `session wait` hanging: retry it, and record
// the outcome on disk so a timed-out wait can still learn it.
func TestReviewRunRetriesFinalStatusAndRecordsResult(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	h.drivers["codex"].outputs = []string{threeFindings}
	mailbox(h)
	fails := 2
	h.ctl.OnStatus = func(state string) error {
		if state == "completed" && fails > 0 {
			fails--
			return errors.New("socket hiccup")
		}
		return nil
	}
	h.env.ReviewRun(context.Background(), []string{dir})
	n := 0
	for _, c := range h.ctl.Calls {
		if c.Method == "Status" && c.Args[1] == "completed" {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("completed attempts = %d, want 3", n)
	}
	assertResult(t, filepath.Join(dir, "rounds", "1"), "completed", 3)
}

func TestReviewRunSecondRoundResumesSession(t *testing.T) {
	h := newHarness(t, true)
	root, dir := runDir(t)
	codex := h.drivers["codex"]
	codex.outputs = []string{threeFindings, approve}
	codex.sessions = []string{"T-1", "T-1"}
	mailbox(h, func() string { nextRound(t, dir, 2, "ROUND TWO"); return "round 2" })

	h.env.ReviewRun(context.Background(), []string{dir})
	if len(codex.turns) != 2 {
		t.Fatalf("turns = %d", len(codex.turns))
	}
	want := harness.Turn{Worker: config.Worker{Harness: "codex"}, Root: root, Dir: filepath.Join(dir, "rounds", "2"),
		Log: "reviewer.log", Prompt: "ROUND TWO", Session: "T-1", Resume: true}
	if !reflect.DeepEqual(codex.turns[1], want) {
		t.Fatalf("round 2 turn = %+v", codex.turns[1])
	}
	var infos []string
	for _, c := range h.ctl.Calls {
		if c.Method == "Info" {
			infos = append(infos, c.Args[1])
		}
	}
	if infos[2] != "codex #1 r2: reviewing" || !strings.Contains(infos[3], "codex #1 r2: ") || !strings.Contains(infos[3], "approve") {
		t.Fatalf("infos = %q", infos)
	}
	assertResult(t, filepath.Join(dir, "rounds", "2"), "completed", 0)
}

func TestReviewRunFailedRoundThenSuccess(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	codex := h.drivers["codex"]
	codex.errs = []error{errors.New("codex failed: boom")}
	codex.outputs = []string{"", approve}
	codex.sessions = []string{"", "T-2"}
	mailbox(h, func() string { nextRound(t, dir, 2, "AGAIN"); return "round 2" })
	h.env.ReviewRun(context.Background(), []string{dir})
	if codex.turns[1].Resume || codex.turns[1].Session != "" {
		t.Fatalf("no session to resume after a failed first round: %+v", codex.turns[1])
	}
	assertResult(t, filepath.Join(dir, "rounds", "1"), "blocked", 0)
	assertResult(t, filepath.Join(dir, "rounds", "2"), "completed", 0)
}

func TestReviewRunIgnoresStaleWakeUp(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	h.drivers["codex"].outputs = []string{approve}
	mailbox(h, func() string { return "round 1" }) // round file not advanced
	h.env.ReviewRun(context.Background(), []string{dir})
	if n := len(h.drivers["codex"].turns); n != 1 {
		t.Fatalf("turns = %d, want 1", n)
	}
}

func TestReviewRunClaudeReviewer(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDirWith(t, config.Worker{Harness: "claude", Model: "claude-opus-5-5"}, "")
	h.drivers["claude"].outputs = []string{approve}
	mailbox(h)
	h.env.ReviewRun(context.Background(), []string{dir})
	if got := h.ctl.Calls[1].Args[1]; got != "claude #1 r1: reviewing" {
		t.Fatalf("info = %q", got)
	}
	if tr := h.drivers["claude"].turns; len(tr) != 1 || tr[0].Worker.Model != "claude-opus-5-5" {
		t.Fatalf("claude turns = %+v", tr)
	}
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

// The started marker vouches that the companion's status is this round's:
// when setting active fails, the old status may still show, so no marker.
func TestReviewRunNoStartedMarkerWhenActiveFails(t *testing.T) {
	h := newHarness(t, true)
	_, dir := runDir(t)
	h.drivers["codex"].outputs = []string{approve}
	mailbox(h)
	h.ctl.OnStatus = func(state string) error {
		if state == "active" {
			return errors.New("socket hiccup")
		}
		return nil
	}
	h.env.ReviewRun(context.Background(), []string{dir})
	rd := filepath.Join(dir, "rounds", "1")
	if exists(filepath.Join(rd, startedFile)) {
		t.Fatal("unconfirmed active must not mark the round started")
	}
	assertResult(t, rd, "completed", 0)
}
