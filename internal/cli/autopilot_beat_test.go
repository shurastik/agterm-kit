package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// beatClock gives h a clock that every Wait moves by one 5-second slice, and
// records the clock at every autopilot line set.
type beatClock struct {
	now   time.Time
	beats []time.Time
}

func fakeBeatClock(h *tenv) *beatClock {
	c := &beatClock{now: h.env.Now()}
	h.env.Now = func() time.Time { return c.now }
	pollSlice = 5 * time.Second
	h.ctl.WaitErr = agterm.ErrTimeout
	h.ctl.OnWait = func() { c.now = c.now.Add(5 * time.Second) }
	h.ctl.OnInfo = func() error {
		c.beats = append(c.beats, c.now)
		return h.ctl.InfoErr
	}
	return c
}

// lineSets counts the autopilot lines set so far.
func lineSets(h *tenv) int {
	n := 0
	for _, c := range h.ctl.Calls {
		if c.Method == "Info" && c.Args[0] == "SELF" && c.Args[1] == autopilotLine {
			n++
		}
	}
	return n
}

func checkGaps(t *testing.T, beats []time.Time) {
	t.Helper()
	for i := 1; i < len(beats); i++ {
		if gap := beats[i].Sub(beats[i-1]); gap < 60*time.Second {
			t.Errorf("beats %d and %d are %v apart, want at least 60s", i-1, i, gap)
		}
	}
}

// openRound drops the round-1 result of an autopilot review run so a wait has
// to wait.
func openReviewRound(t *testing.T, dir string) {
	t.Helper()
	if err := os.Remove(filepath.Join(rundir.RoundDir(dir, 1), "result.json")); err != nil {
		t.Fatal(err)
	}
}

func TestBeatReviewWaitOncePerMinute(t *testing.T) {
	h := newHarness(t, true)
	dir := autopilotReviewRun(t, h, false, 1)
	openReviewRound(t, dir)
	c := fakeBeatClock(h)
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.Calls, c.beats = nil, nil
	if code := h.env.Main(context.Background(), []string{"review", "wait", dir, "--timeout", "300"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	// On entry, then at 60, 120, 180 and 240 s.
	if n := lineSets(h); n != 5 {
		t.Fatalf("%d beats in a 300 s wait, want 5: %v", n, c.beats)
	}
	checkGaps(t, c.beats)
	if got := strings.TrimSpace(h.out.String()); got != `{"status":"running","round":1}` {
		t.Fatalf("stdout = %q", got)
	}
}

func TestBeatHookedApplyWaitBeats(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	if code := runApply(h, "start", "add-auth", "--autopilot"); code != 0 {
		t.Fatalf("start: %d %s", code, h.err)
	}
	dir := applyDir(root)
	if !readApplyState(t, dir).TurnHooks {
		t.Fatal("the run must be a hooked one")
	}
	*h = *newHarnessLike(t, h)
	c := fakeBeatClock(h)
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.Calls, c.beats = nil, nil
	if code := runApply(h, "wait", dir, "--timeout", "300"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if n := lineSets(h); n != 5 {
		t.Fatalf("%d beats in a 300 s hooked wait, want 5: %v", n, c.beats)
	}
	checkGaps(t, c.beats)
}

func TestBeatSendBeatsOnEntry(t *testing.T) {
	h := newHarness(t, true)
	dir := autopilotReviewRun(t, h, false, 1)
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.Calls = nil
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if n := lineSets(h); n != 1 {
		t.Fatalf("%d beats, want 1: %v", n, h.ctl.Calls)
	}
	if h.ctl.Calls[0].Method != "Info" {
		t.Fatalf("the first call must be the beat on entry: %v", h.ctl.Calls)
	}
}

func TestBeatApplySendBeatsOnEntry(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	runApply(h, "start", "add-auth", "--autopilot")
	dir := applyDir(root)
	writeReport(t, dir, 1, doneReport)
	*h = *newHarnessLike(t, h)
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.Calls = nil
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	msg := filepath.Join(t.TempDir(), "m.md")
	os.WriteFile(msg, []byte("m"), 0o644)
	if code := runApply(h, "send", dir, msg); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if n := lineSets(h); n != 1 || h.ctl.Calls[0].Method != "Info" {
		t.Fatalf("%d beats, calls %v", n, h.ctl.Calls)
	}
}

// claudeReviewer switches an autopilot review run to a claude reviewer, whose
// send waits for the status alone (no codex settle check).
func claudeReviewer(t *testing.T, dir string) {
	t.Helper()
	var st reviewState
	if _, err := rundir.ReadJSON(dir, workerFile, &st); err != nil {
		t.Fatal(err)
	}
	st.Worker.Harness = "claude"
	if err := rundir.WriteJSON(dir, workerFile, st); err != nil {
		t.Fatal(err)
	}
}

func TestBeatLongSendBeatsOncePerMinute(t *testing.T) {
	h := newHarness(t, true)
	dir := autopilotReviewRun(t, h, false, 1)
	claudeReviewer(t, dir)
	c := fakeBeatClock(h)
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.Calls, c.beats = nil, nil
	if code := runSend(h, dir, "--timeout", "3600"); code != 1 {
		t.Fatalf("exit %d, want 1 (busy): %s", code, h.err)
	}
	if !strings.Contains(h.err.String(), "is busy") {
		t.Fatalf("stderr = %q", h.err)
	}
	if n := lineSets(h); n != 60 {
		t.Fatalf("%d beats in a 3600 s send, want 60", n)
	}
	checkGaps(t, c.beats)
}

// A run without the beat keeps its one Ctl.Wait for the whole budget, even
// with short slices.
func TestBeatUnarmedSendKeepsOneWait(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	runApply(h, "start", "add-auth")
	dir := applyDir(root)
	writeReport(t, dir, 1, doneReport)
	*h = *newHarnessLike(t, h)
	pollSlice = 5 * time.Second
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	msg := filepath.Join(t.TempDir(), "m.md")
	os.WriteFile(msg, []byte("m"), 0o644)
	if code := runApply(h, "send", dir, msg); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if got := waitCalls(h); len(got) == 0 || got[0] != "completed,blocked,idle 10m0s" {
		t.Fatalf("waits = %v, want one wait of the whole budget first", got)
	}
}

func TestBeatArmedSendSlicesTheWait(t *testing.T) {
	h := newHarness(t, true)
	dir := autopilotReviewRun(t, h, false, 1)
	claudeReviewer(t, dir)
	pollSlice = 5 * time.Second
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.Calls = nil
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, {Status: "completed"}, {Status: "active"}}
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	got := waitCalls(h)
	want := []string{"completed,blocked,idle 5s", "completed,blocked,idle 5s", "completed,blocked,idle 5s", "active 10s"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("waits = %v, want %v", got, want)
	}
}

// An armed codex send keeps its settle check: a status stuck "active" with
// round 1 on disk and a still screen types the round, and the wait still beats.
func TestBeatArmedCodexSendKeepsSettle(t *testing.T) {
	h := newHarness(t, true)
	dir := autopilotReviewRun(t, h, false, 1)
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.Calls = nil
	stuckStatus(h)
	h.ctl.TextOut = "idle composer\n"
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Err: agterm.ErrTimeout}, {Err: agterm.ErrTimeout}, // two slices
		{Status: "active"}, // status check
		{Status: "active"}, // the line was picked up
	}
	if code := runSend(h, dir, "--timeout", "30"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	prompt := filepath.Join(dir, "rounds", "2", "prompt.md")
	if got := types(h); len(got) != 1 || got[0] != "Read "+prompt+" and follow it.\r" {
		t.Fatalf("typed %q", got)
	}
	if !strings.Contains(h.err.String(), "treating its turn as over") {
		t.Fatalf("stderr = %q", h.err)
	}
	if n := lineSets(h); n < 1 {
		t.Fatalf("no beat in an armed codex send: %v", h.ctl.Calls)
	}
}

func TestBeatNoneAfterOff(t *testing.T) {
	h := newHarness(t, true)
	dir := autopilotReviewRun(t, h, false, 1)
	h.env.autopilotOn(context.Background(), "SELF")
	h.env.autopilotOff(context.Background(), "SELF")
	h.ctl.Calls = nil
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if n := lineSets(h); n != 0 {
		t.Fatalf("beat after off: %v", h.ctl.Calls)
	}
}

func TestBeatNoneOnceUntilPassed(t *testing.T) {
	h := newHarness(t, true)
	dir := autopilotReviewRun(t, h, false, 1)
	c := fakeBeatClock(h)
	h.ctl.WaitErr = nil
	h.env.autopilotOn(context.Background(), "SELF")
	c.now = c.now.Add(autopilotExpiry + time.Second)
	h.ctl.Calls = nil
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	if code := runSend(h, dir); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if n := lineSets(h); n != 0 {
		t.Fatalf("beat after the live period ran out: %v", h.ctl.Calls)
	}
}

func TestBeatNoneWithoutTheFlag(t *testing.T) {
	h := newHarness(t, true)
	root := repoWith(t, h, "")
	h.ctl.WaitStatus = "active"
	runApply(h, "start", "add-auth")
	dir := applyDir(root)
	*h = *newHarnessLike(t, h)
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.Calls = nil
	h.ctl.WaitErr = agterm.ErrTimeout
	if code := runApply(h, "wait", dir, "--timeout", "10"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if n := lineSets(h); n != 0 {
		t.Fatalf("beat for a run without the flag: %v", h.ctl.Calls)
	}
}

func TestBeatRefusedLineKeepsOutput(t *testing.T) {
	h := newHarness(t, true)
	dir := autopilotReviewRun(t, h, false, 1)
	openReviewRound(t, dir)
	c := fakeBeatClock(h)
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.InfoErr = errors.New("agterm said no")
	h.ctl.Calls, c.beats = nil, nil
	if code := h.env.Main(context.Background(), []string{"review", "wait", dir, "--timeout", "20"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	if !strings.Contains(h.err.String(), "warning") || !strings.Contains(h.err.String(), "agterm said no") {
		t.Fatalf("stderr = %q", h.err)
	}
	if got := strings.TrimSpace(h.out.String()); got != `{"status":"running","round":1}` {
		t.Fatalf("stdout = %q", got)
	}
}

func TestBeatInlineAutopilotRunNeverBeats(t *testing.T) {
	h := newHarness(t, true)
	dir := autopilotReviewRun(t, h, false, 1)
	st := readState(t, dir)
	st.AutopilotSession = ""
	rundir.WriteJSON(dir, workerFile, st)
	h.env.autopilotOn(context.Background(), "SELF")
	h.ctl.Calls = nil
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "active"}}
	runSend(h, dir)
	if n := lineSets(h); n != 0 {
		t.Fatalf("beat without a recorded session: %v", h.ctl.Calls)
	}
}
