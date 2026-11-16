package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/agterm/agtermtest"
	"github.com/shurastik/agterm-kit/internal/apply"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// Tests of `agk apply wait` on a claude run with turn-state hooks. Time passes
// only through scripted Ctl.Wait calls: a timed-out wait spends its slice.

func writeTurn(t *testing.T, dir string, tn apply.Turn) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, apply.TurnFile), tn.Marshal(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setTurnHooks flips the turn_hooks flag of a started run.
func setTurnHooks(t *testing.T, dir string, on bool) {
	t.Helper()
	s := readApplyState(t, dir)
	s.TurnHooks = on
	if err := rundir.WriteJSON(dir, workerFile, s); err != nil {
		t.Fatal(err)
	}
}

// hookedWait is a started run at round k plus a harness whose Ctl times out
// every wait, with 5-second slices.
func hookedWait(t *testing.T, k int) (*tenv, string) {
	t.Helper()
	h := newHarness(t, true)
	dir := startedApply(t, h, k)
	h.ctl.WaitErr = agterm.ErrTimeout
	pollSlice = 5 * time.Second
	return h, dir
}

// hookedWaitOneShot is hookedWait with slices longer than any budget: every
// scripted Wait answers exactly one phase of the wait.
func hookedWaitOneShot(t *testing.T, k int) (*tenv, string) {
	t.Helper()
	h, dir := hookedWait(t, k)
	pollSlice = 24 * time.Hour
	return h, dir
}

func waitOut(t *testing.T, h *tenv, dir string, args ...string) string {
	t.Helper()
	if code := runApply(h, append([]string{"wait", dir}, args...)...); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err)
	}
	return strings.TrimSpace(h.out.String())
}

// waitCalls lists "<until> <timeout>" of every recorded Wait.
func waitCalls(h *tenv) []string {
	var out []string
	for _, c := range h.ctl.Calls {
		if c.Method == "Wait" {
			out = append(out, c.Args[1]+" "+c.Args[2])
		}
	}
	return out
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func eqStrings(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

const statusCheckCall = "active,completed,idle,blocked 1s"

func TestHookedWaitPausedThenDone(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 3 {
			writeReport(t, dir, 1, doneReport)
		}
	}
	got := waitOut(t, h, dir, "--timeout", "600")
	if !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("stdout = %s", got)
	}
	if want := repeat("blocked 5s", 3); !eqStrings(waitCalls(h), want) {
		t.Fatalf("waits = %v, want %v: a pause waits for blocked only", waitCalls(h), want)
	}
}

func TestHookedWaitPausedPastTheTimeout(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 2})
	h.ctl.WaitStatus = "" // the status is not consulted while paused
	got := waitOut(t, h, dir, "--timeout", "60")
	if want := `{"status":"running","round":1,"reason":"background","background":2}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
	if want := repeat("blocked 5s", 12); !eqStrings(waitCalls(h), want) {
		t.Fatalf("waits = %v", waitCalls(h))
	}
}

func TestHookedWaitCountsWakeupsToo(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1, Wakeups: 2})
	got := waitOut(t, h, dir, "--timeout", "10")
	if want := `{"status":"running","round":1,"reason":"background","background":3}`; got != want {
		t.Fatalf("stdout = %s", got)
	}
}

func TestHookedWaitResumedTurnEndsWithAQuestion(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})
	n := 0
	h.ctl.OnWait = func() {
		switch n++; n {
		case 2:
			writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})
		case 4:
			writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped})
		}
	}
	got := waitOut(t, h, dir, "--timeout", "600")
	if want := `{"status":"needs-human","round":1,"reason":"no-report"}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
	calls := waitCalls(h)
	if last := calls[len(calls)-1]; last != statusCheckCall {
		t.Fatalf("last wait = %q, want the status check %q", last, statusCheckCall)
	}
	if want := append(repeat("blocked 5s", 4), statusCheckCall); !eqStrings(calls, want) {
		t.Fatalf("waits = %v, want %v", calls, want)
	}
}

// The agent status can read "completed" before the Stop hook wrote its state:
// while the turn state says running, the wait goes on.
func TestHookedWaitHookLagsTheStatus(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 3 {
			writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped})
		}
	}
	got := waitOut(t, h, dir, "--timeout", "600")
	if want := `{"status":"needs-human","round":1,"reason":"no-report"}`; got != want {
		t.Fatalf("stdout = %s", got)
	}
	if want := append(repeat("blocked 5s", 3), statusCheckCall); !eqStrings(waitCalls(h), want) {
		t.Fatalf("waits = %v", waitCalls(h))
	}
}

func TestHookedWaitRunningPastTheTimeout(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})
	if got := waitOut(t, h, dir, "--timeout", "10"); got != `{"status":"running","round":1}` {
		t.Fatalf("stdout = %s", got)
	}
}

func TestHookedWaitStoppedWithNothingInFlight(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped})
	got := waitOut(t, h, dir)
	if want := `{"status":"needs-human","round":1,"reason":"no-report"}`; got != want {
		t.Fatalf("stdout = %s", got)
	}
	if !eqStrings(waitCalls(h), []string{statusCheckCall}) {
		t.Fatalf("waits = %v: only the status check", waitCalls(h))
	}
}

func TestHookedWaitReportWinsOverTheStatusCheck(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped})
	h.ctl.OnWait = func() { writeReport(t, dir, 1, doneReport) } // flushed after the hook
	if got := waitOut(t, h, dir); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("stdout = %s", got)
	}
}

func TestHookedWaitStatusCheckBeforeNoReport(t *testing.T) {
	cases := map[string]struct {
		check agtermtest.WaitResult
		want  string
	}{
		"gone":       {agtermtest.WaitResult{Err: agterm.ErrGone}, `{"status":"cancelled","round":1}`},
		"permission": {agtermtest.WaitResult{Status: "blocked"}, `{"status":"needs-human","round":1,"reason":"permission"}`},
		"completed":  {agtermtest.WaitResult{Status: "completed"}, `{"status":"needs-human","round":1,"reason":"no-report"}`},
		"idle":       {agtermtest.WaitResult{Status: "idle"}, `{"status":"needs-human","round":1,"reason":"no-report"}`},
		"unknown":    {agtermtest.WaitResult{Err: agterm.ErrTimeout}, `{"status":"needs-human","round":1,"reason":"no-report"}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h, dir := hookedWait(t, 1)
			writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped})
			h.ctl.WaitSeq = []agtermtest.WaitResult{c.check}
			if got := waitOut(t, h, dir); got != c.want {
				t.Fatalf("stdout = %s, want %s", got, c.want)
			}
		})
	}
}

func TestHookedWaitCompanionClosedWhilePaused(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}, {Err: agterm.ErrGone}}
	if got := waitOut(t, h, dir); got != `{"status":"cancelled","round":1}` {
		t.Fatalf("stdout = %s", got)
	}
}

func TestHookedWaitPermissionPromptWhilePausedOrRunning(t *testing.T) {
	for name, tn := range map[string]apply.Turn{
		"paused":  {Round: 1, State: apply.TurnStopped, Background: 1},
		"running": {Round: 1, State: apply.TurnRunning},
	} {
		t.Run(name, func(t *testing.T) {
			h, dir := hookedWait(t, 1)
			writeTurn(t, dir, tn)
			h.ctl.WaitSeq = []agtermtest.WaitResult{{Err: agterm.ErrTimeout}, {Status: "blocked"}}
			got := waitOut(t, h, dir)
			if want := `{"status":"needs-human","round":1,"reason":"permission"}`; got != want {
				t.Fatalf("stdout = %s", got)
			}
		})
	}
}

func TestHookedWaitTurnStateNeverRecorded(t *testing.T) {
	h, dir := hookedWaitOneShot(t, 2)
	markStarted(dir, 2) // seen active, no hook ever ran for round 2
	h.ctl.WaitSeq = []agtermtest.WaitResult{
		{Status: "completed"},    // the status shows the turn over
		{Err: agterm.ErrTimeout}, // 30 s, no turn state
		{Status: "completed"},    // the status check
	}
	got := waitOut(t, h, dir)
	if want := `{"status":"needs-human","round":2,"reason":"no-report"}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
	want := []string{"completed,blocked,idle 1h0m0s", "active,blocked 30s", statusCheckCall}
	if !eqStrings(waitCalls(h), want) {
		t.Fatalf("waits = %v, want %v", waitCalls(h), want)
	}
}

func TestHookedWaitTurnStateShowsUpDuringTheFallback(t *testing.T) {
	h, dir := hookedWait(t, 2)
	markStarted(dir, 2)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}}
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 3 { // second slice of the 30 s fallback
			writeTurn(t, dir, apply.Turn{Round: 2, State: apply.TurnStopped, Background: 1})
		}
	}
	got := waitOut(t, h, dir, "--timeout", "100")
	if want := `{"status":"running","round":2,"reason":"background","background":1}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
}

func TestHookedWaitFallbackIsCappedByTheBudget(t *testing.T) {
	h, dir := hookedWait(t, 2)
	markStarted(dir, 2)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}}
	got := waitOut(t, h, dir, "--timeout", "20")
	if got != `{"status":"running","round":2}` {
		t.Fatalf("stdout = %s: a spent budget is running, not no-report", got)
	}
}

// A late Stop hook of round 1 after round 2 was published records round 1:
// round 2's wait must not read it.
func TestHookedWaitIgnoresAnEarlierRoundsState(t *testing.T) {
	for name, tn := range map[string]apply.Turn{
		"stopped":       {Round: 1, State: apply.TurnStopped},
		"paused":        {Round: 1, State: apply.TurnStopped, Background: 1},
		"running":       {Round: 1, State: apply.TurnRunning},
		"a later round": {Round: 3, State: apply.TurnStopped},
	} {
		t.Run(name, func(t *testing.T) {
			h, dir := hookedWait(t, 2)
			writeTurn(t, dir, tn)
			got := waitOut(t, h, dir, "--timeout", "10")
			if got != `{"status":"running","round":2}` {
				t.Fatalf("stdout = %s", got)
			}
			if want := repeat("active,blocked 5s", 2); !eqStrings(waitCalls(h), want) {
				t.Fatalf("waits = %v: round 2 has not started, wait for it", waitCalls(h))
			}
			if exists(filepath.Join(rundir.RoundDir(dir, 2), startedFile)) {
				t.Fatal("another round's state must not mark round 2 started")
			}
		})
	}
}

func TestHookedWaitGarbageTurnFileCountsAsMissing(t *testing.T) {
	h, dir := hookedWaitOneShot(t, 1)
	os.WriteFile(filepath.Join(dir, apply.TurnFile), []byte("{nope"), 0o644)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "idle"}, {Err: agterm.ErrTimeout}, {Status: "idle"}}
	if got := waitOut(t, h, dir); got != `{"status":"needs-human","round":1,"reason":"no-report"}` {
		t.Fatalf("stdout = %s", got)
	}
}

// Round 2 was typed but never seen active; its hooks ran.
func TestHookedWaitRoundStartedUnseen(t *testing.T) {
	t.Run("paused", func(t *testing.T) {
		h, dir := hookedWait(t, 2)
		writeTurn(t, dir, apply.Turn{Round: 2, State: apply.TurnStopped, Background: 1})
		got := waitOut(t, h, dir, "--timeout", "60")
		if want := `{"status":"running","round":2,"reason":"background","background":1}`; got != want {
			t.Fatalf("stdout = %s, want %s", got, want)
		}
		if !exists(filepath.Join(rundir.RoundDir(dir, 2), startedFile)) {
			t.Fatal("a turn state of the round proves it started")
		}
		if want := repeat("blocked 5s", 12); !eqStrings(waitCalls(h), want) {
			t.Fatalf("waits = %v", waitCalls(h))
		}
	})
	t.Run("stopped with nothing in flight", func(t *testing.T) {
		h, dir := hookedWait(t, 2)
		writeTurn(t, dir, apply.Turn{Round: 2, State: apply.TurnStopped})
		got := waitOut(t, h, dir)
		if want := `{"status":"needs-human","round":2,"reason":"no-report"}`; got != want {
			t.Fatalf("stdout = %s", got)
		}
		if !eqStrings(waitCalls(h), []string{statusCheckCall}) {
			t.Fatalf("waits = %v: no-report at once", waitCalls(h))
		}
	})
}

func TestHookedWaitStartPhaseEndsOnATurnState(t *testing.T) {
	h, dir := hookedWait(t, 1)
	os.Remove(filepath.Join(rundir.RoundDir(dir, 1), startedFile)) // handshake never confirmed it
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 2 {
			writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})
		}
	}
	got := waitOut(t, h, dir, "--timeout", "30")
	if want := `{"status":"running","round":1,"reason":"background","background":1}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
	want := append([]string{"active,blocked,completed 5s", "active,blocked,completed 5s"}, repeat("blocked 5s", 4)...)
	if !eqStrings(waitCalls(h), want) {
		t.Fatalf("waits = %v, want %v", waitCalls(h), want)
	}
	if !exists(filepath.Join(rundir.RoundDir(dir, 1), startedFile)) {
		t.Fatal("round 1 not marked started")
	}
}

func TestHookedWaitStartPhaseSeesActive(t *testing.T) {
	h, dir := hookedWait(t, 1)
	os.Remove(filepath.Join(rundir.RoundDir(dir, 1), startedFile))
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "active"}}
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 2 {
			writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnRunning})
		}
		if n == 4 {
			writeReport(t, dir, 1, doneReport)
		}
	}
	if got := waitOut(t, h, dir, "--timeout", "600"); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("stdout = %s", got)
	}
	if !exists(filepath.Join(rundir.RoundDir(dir, 1), startedFile)) {
		t.Fatal("seeing active marks the round started")
	}
}

func TestHookedWaitRoundOneAlreadyOverInTheStartPhase(t *testing.T) {
	h, dir := hookedWaitOneShot(t, 1)
	os.Remove(filepath.Join(rundir.RoundDir(dir, 1), startedFile))
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}, {Status: "completed"}, {Err: agterm.ErrTimeout}, {Status: "completed"}}
	if got := waitOut(t, h, dir); got != `{"status":"needs-human","round":1,"reason":"no-report"}` {
		t.Fatalf("stdout = %s", got)
	}
	want := []string{"active,blocked,completed 1h0m0s", "completed,blocked,idle 1h0m0s", "active,blocked 30s", statusCheckCall}
	if !eqStrings(waitCalls(h), want) {
		t.Fatalf("waits = %v, want %v", waitCalls(h), want)
	}
}

// agk apply send published round 2, was refused (the applier is busy) and put
// round 1 back, all while round 1 resumed from a pause. The hooks never read
// the run's current round, so the wait still sees round 1 paused.
func TestHookedWaitRoundOneResumedWhileABusySendRolledBack(t *testing.T) {
	h, dir := hookedWait(t, 1)
	hook := func(event, stdin string) {
		t.Helper()
		runHook(t, event, dir, stdin)
	}
	hook("turn", promptInput(kickoffOf(dir, 1)))
	hook("stop", stopWithShell)
	// round 1 resumes by itself; its start hook runs after the send published round 2
	rundir.MakeRound(dir, 2)
	if err := rundir.SetRound(dir, 2); err != nil {
		t.Fatal(err)
	}
	hook("turn", promptInput("<task-notification>done</task-notification>"))
	rollbackRound(dir, 1, 2, os.Stderr) // the send gave up: busy
	hook("stop", stopWithShell)
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})

	got := waitOut(t, h, dir, "--timeout", "10")
	if want := `{"status":"running","round":1,"reason":"background","background":1}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
}

func TestHookedWaitWithoutTurnHooksKeepsTheOldDecision(t *testing.T) {
	h := newHarness(t, true)
	dir := startedApply(t, h, 1)
	setTurnHooks(t, dir, false)
	// A stale paused state must not matter to a run that never had the hooks.
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})
	h.ctl.WaitStatus = "completed"
	got := waitOut(t, h, dir)
	if want := `{"status":"needs-human","round":1,"reason":"no-report"}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
	assertCalls(t, h.ctl, agtermtest.Call{Method: "Wait", Args: []string{"C-1", "completed,blocked,idle", "1h0m0s"}})
}

func TestHookedWaitReportAlreadyThereSkipsEverything(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})
	writeReport(t, dir, 1, doneReport)
	if got := waitOut(t, h, dir); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("stdout = %s", got)
	}
	assertCalls(t, h.ctl)
}

func TestHookedWaitHardErrorsStillFail(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})
	h.ctl.WaitErr = os.ErrPermission
	if code := h.env.Main(context.Background(), []string{"apply", "wait", dir}); code != 1 || h.out.Len() != 0 {
		t.Fatalf("exit %d stdout %q", code, h.out)
	}
}

// The Stop hook can land in the very last slice of the 30 s fallback, after the
// slice's budget check: the wait must read the file once more before it calls
// the turn over.
func TestHookedWaitPauseLandsInTheLastFallbackSlice(t *testing.T) {
	h, dir := hookedWait(t, 2)
	markStarted(dir, 2)
	h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}} // the status shows the turn over
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 7 { // 1 status wait + 6 slices of 5 s: the last slice
			writeTurn(t, dir, apply.Turn{Round: 2, State: apply.TurnStopped, Background: 1})
		}
	}
	got := waitOut(t, h, dir, "--timeout", "100")
	if want := `{"status":"running","round":2,"reason":"background","background":1}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
	for _, c := range waitCalls(h) {
		if c == statusCheckCall {
			t.Fatalf("waits = %v: a pause is on disk, no status check", waitCalls(h))
		}
	}
}

// The status check before no-report takes time too: a turn that started or
// paused meanwhile is not a missing report.
func TestHookedWaitTurnStateChangesDuringTheStatusCheck(t *testing.T) {
	for name, tn := range map[string]apply.Turn{
		"paused":  {Round: 1, State: apply.TurnStopped, Background: 2},
		"running": {Round: 1, State: apply.TurnRunning},
	} {
		t.Run(name, func(t *testing.T) {
			h, dir := hookedWait(t, 1)
			writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped})
			n := 0
			h.ctl.OnWait = func() {
				if n++; n == 1 { // the status check
					writeTurn(t, dir, tn)
				}
			}
			h.ctl.WaitSeq = []agtermtest.WaitResult{{Status: "completed"}}
			got := waitOut(t, h, dir, "--timeout", "20")
			want := `{"status":"running","round":1}`
			if tn.Paused() {
				want = `{"status":"running","round":1,"reason":"background","background":2}`
			}
			if got != want {
				t.Fatalf("stdout = %s, want %s", got, want)
			}
		})
	}
}

// On the overall timeout the outcome carries the turn state on disk now, not
// the one read before the last slice.
func TestHookedWaitTimeoutReportsTheLastTurnState(t *testing.T) {
	h, dir := hookedWait(t, 1)
	writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 1})
	n := 0
	h.ctl.OnWait = func() {
		if n++; n == 12 { // the last slice of 60 s
			writeTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped, Background: 3})
		}
	}
	got := waitOut(t, h, dir, "--timeout", "60")
	if want := `{"status":"running","round":1,"reason":"background","background":3}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
}

// A final API error fires StopFailure, not Stop: the turn is over, nothing in
// flight, and no report was written.
func TestHookedWaitTurnEndedByAnAPIError(t *testing.T) {
	h, dir := hookedWait(t, 1)
	runHook(t, "turn", dir, promptInput(kickoffOf(dir, 1)))
	runHook(t, "stop", dir, `{"hook_event_name":"StopFailure","error":"rate_limit","error_details":"429"}`)
	wantTurn(t, dir, apply.Turn{Round: 1, State: apply.TurnStopped})
	got := waitOut(t, h, dir)
	if want := `{"status":"needs-human","round":1,"reason":"no-report"}`; got != want {
		t.Fatalf("stdout = %s, want %s", got, want)
	}
	if !eqStrings(waitCalls(h), []string{statusCheckCall}) {
		t.Fatalf("waits = %v: no-report at once, not after the timeout", waitCalls(h))
	}
}
