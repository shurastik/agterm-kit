package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/apply"
)

// turnFallback is how long a wait gives the hooks to record a turn state after
// the agent status shows the turn over, before it reads the turn as ended with
// nothing in flight.
const turnFallback = 30 * time.Second

// readTurnOf returns the raw turn.json and, when it is valid and of round k,
// its state. A turn state of any other round counts as missing: a late hook of
// an earlier round must not decide the current one.
func readTurnOf(dir string, k int) (raw []byte, t apply.Turn, ok bool) {
	raw, _ = os.ReadFile(filepath.Join(dir, apply.TurnFile))
	t, err := apply.ParseTurn(raw)
	return raw, t, err == nil && t.Round == k
}

// waitHooked is `agk apply wait` for a run whose claude applier has turn-state
// hooks (applyState.TurnHooks). The turn state tells a pause (the turn ended
// with background work in flight) from the end of the round; the files decide
// and the agent status is only a wake-up signal:
//
//   - the round has not started: wait for it, ending the wait early on a turn
//     state of the round, which proves its start;
//   - the turn runs or is paused: wait for a permission prompt only, in slices
//     that end when the report appears or turn.json changes;
//   - the turn ended with nothing in flight: no report, ask the companion once
//     more and decide;
//   - no turn state yet although the round started: wait for the status to show
//     the turn over, then give the hooks turnFallback and decide.
func (e *Env) waitHooked(ctx context.Context, run applyRun, companion string, left time.Duration) int {
	k := run.round
	// finish prints the outcome. The report and the turn state are re-read
	// first: the hooks may land after the wait last looked, and an outcome must
	// carry what is on disk now (a pause must never read as no-report).
	finish := func(status string, werr error) int {
		report, err := readReport(run.dir, k)
		if err != nil {
			return e.errorf("%v", err)
		}
		var tp *apply.Turn
		if _, t, ok := readTurnOf(run.dir, k); ok {
			tp = &t
		}
		return e.printJSON(apply.Decide(k, report, status, werr, tp))
	}
	// live reports a turn that is running or paused on disk now.
	live := func() bool {
		_, t, ok := readTurnOf(run.dir, k)
		return ok && (t.State == apply.TurnRunning || t.Paused())
	}
	// failed is a wait error that is neither a gone companion nor a timeout.
	failed := func(werr error) bool {
		return werr != nil && !errors.Is(werr, agterm.ErrGone) && !errors.Is(werr, agterm.ErrTimeout)
	}
	fail := func(werr error) int { return e.errorf("wait for applier %s: %v", companion, werr) }
	started := roundStarted(run.dir, k)
	for {
		report, err := readReport(run.dir, k)
		if err != nil {
			return e.errorf("%v", err)
		}
		raw, turn, have := readTurnOf(run.dir, k)
		if report != nil {
			return finish("", nil)
		}
		if have && !started {
			markStarted(run.dir, k)
			started = true
		}
		if left <= 0 {
			return finish("", agterm.ErrTimeout)
		}
		// changed ends a slice: the report appeared or the turn state moved.
		changed := func() bool {
			if _, err := os.Stat(reportPath(run.dir, k)); err == nil {
				return true
			}
			now, _ := os.ReadFile(filepath.Join(run.dir, apply.TurnFile))
			return !bytes.Equal(now, raw)
		}
		switch {
		case !started:
			// Unconfirmed start (a trust prompt, a slow login): a fresh session's
			// idle would read as "turn over". A first round may also be over already.
			until := []string{"active", "blocked"}
			if k == 1 {
				until = append(until, "completed")
			}
			status, werr, found, rest := e.waitPolling(ctx, companion, until, left, changed)
			left = rest
			switch {
			case failed(werr):
				return fail(werr)
			case werr != nil:
				return finish("", werr)
			case found:
			case status == "active":
				markStarted(run.dir, k)
				started = true
			case status == "blocked":
				return finish(status, nil)
			case status == "completed":
				started = true // round 1 ended before it was seen; not recorded
			}
		case have && (turn.State == apply.TurnRunning || turn.Paused()):
			// Running or paused: not over, whatever the status says.
			status, werr, found, rest := e.waitPolling(ctx, companion, []string{"blocked"}, left, changed)
			left = rest
			switch {
			case failed(werr):
				return fail(werr)
			case werr != nil:
				return finish("", werr)
			case found:
			case status == "blocked":
				return finish(status, nil)
			}
		case have:
			// The turn ended with nothing in flight.
			if code, again := e.noReport(ctx, companion, finish, fail, live); !again {
				return code
			}
		default:
			// The round started and no turn state is recorded: wait for the
			// status to show the turn over.
			status, werr, found, rest := e.waitPolling(ctx, companion, []string{"completed", "blocked", "idle"}, left, changed)
			left = rest
			switch {
			case failed(werr):
				return fail(werr)
			case werr != nil:
				return finish("", werr)
			case found:
				continue
			case status == "blocked":
				return finish(status, nil)
			case changed():
				continue // the report or the turn state landed with the status
			}
			// The status shows the turn over: give the hooks time to say how.
			fb := min(turnFallback, left)
			status, werr, found, rest = e.waitPolling(ctx, companion, []string{"active", "blocked"}, fb, changed)
			left -= fb - rest
			switch {
			case failed(werr):
				return fail(werr)
			case errors.Is(werr, agterm.ErrGone):
				return finish("", werr)
			case found || status == "active":
				continue
			case status == "blocked":
				return finish(status, nil)
			case live():
				continue // a hook landed after the last slice's budget check
			case left <= 0:
				return finish("", agterm.ErrTimeout)
			}
			if code, again := e.noReport(ctx, companion, finish, fail, live); !again {
				return code
			}
		}
	}
}

// noReport is the end of a turn that left no report: one more look at the
// companion first, a closed one is cancelled and one that asks for permission
// is not a missing report. The look takes time: if the turn is live on disk by
// then (it started again or paused), again is true and the wait carries on.
func (e *Env) noReport(ctx context.Context, companion string, finish func(string, error) int, fail func(error) int, live func() bool) (code int, again bool) {
	status, err := e.Ctl.Wait(ctx, companion, []string{"active", "completed", "idle", "blocked"}, statusCheck)
	switch {
	case errors.Is(err, agterm.ErrGone):
		return finish("", err), false
	case err != nil && !errors.Is(err, agterm.ErrTimeout):
		return fail(err), false
	case status == "blocked":
		return finish(status, nil), false
	case live():
		return 0, true
	}
	return finish("completed", nil), false
}
