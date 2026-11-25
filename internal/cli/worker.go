package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/fsx"
	"github.com/shurastik/agterm-kit/internal/rundir"
	"github.com/shurastik/agterm-kit/internal/termsafe"
)

// Plumbing shared by the two interactive workers (applier and reviewer): the
// start handshake, the round-started marker, sliced waits and typing a line
// once the worker's turn is over.

const (
	startedFile    = "started"        // in rounds/<k>/: the round was seen active
	startHandshake = 60 * time.Second // companion must report a status by then
	handshakeLines = 20               // screen lines shown when it does not
	sendHandshake  = 30 * time.Second // a typed line must turn the status active by then

	// sendFirstHandshake is how long a typed line gets to turn the status
	// active before agk looks at the screen and, if the line sits unsent in
	// the input box, types one Enter on its own.
	sendFirstHandshake = 10 * time.Second
	// statusCheck bounds the "what is the status now" query before that Enter.
	statusCheck = time.Second
)

// workerRole names a worker in messages.
type workerRole struct {
	name    string // "applier"
	restart string // what to do when its companion is gone
}

var (
	applierWorker  = workerRole{"applier", "start a new apply run"}
	reviewerWorker = workerRole{"reviewer", "start a new review"}
)

// companionName is the sidebar name of a worker companion: <harness>-<kind> r<round>,
// prefixed with "<change> · " when the change is known. The name reaches
// agtermctl as its own argv element, so the separator needs no quoting.
func companionName(change, harness, kind string, round int) string {
	name := fmt.Sprintf("%s-%s r%d", harness, kind, round)
	if change == "" {
		return name
	}
	return change + " · " + name
}

// renameCompanion renames a worker's companion after a round was typed into
// it. The name is cosmetic and the round already reached the worker, so a
// failure is a warning and changes nothing else.
func (e *Env) renameCompanion(ctx context.Context, w workerRole, id, name string) {
	if err := e.Ctl.Rename(ctx, id, name); err != nil {
		fmt.Fprintf(e.Stderr, "agk: warning: rename %s %s: %v\n", w.name, id, err)
	}
}

// markStarted records that round k was seen active, so a later idle status
// means its turn is over rather than not begun.
func markStarted(dir string, k int) {
	_ = fsx.ReplaceFile(filepath.Join(rundir.RoundDir(dir, k), startedFile), nil, 0o644)
}

func roundStarted(dir string, k int) bool {
	_, err := os.Stat(filepath.Join(rundir.RoundDir(dir, k), startedFile))
	return err == nil
}

// handshake waits for the new worker to report any agent status. When none
// comes (a trust prompt, a login screen, a bad flag), it shows the last
// screen lines and leaves the companion open for the user.
func (e *Env) handshake(ctx context.Context, w workerRole, id string) bool {
	_, err := e.Ctl.Wait(ctx, id, []string{"active", "completed", "blocked"}, startHandshake)
	if err == nil {
		return true
	}
	fmt.Fprintf(e.Stderr, "agk: warning: %s %s did not start (%v); it is waiting in its companion:\n", w.name, id, err)
	if !errors.Is(err, agterm.ErrGone) {
		if screen, terr := e.Ctl.Text(ctx, id, handshakeLines); terr == nil {
			tw := termsafe.NewWriter(e.Stderr)
			io.WriteString(tw, strings.TrimRight(screen, "\n")+"\n")
		}
	}
	return false
}

// pollSlice is how long one status wait lasts before the state on disk is
// checked again: files decide even when a harness's agent-status hooks never
// fire.
var pollSlice = 5 * time.Second

// waitPolling waits up to left for one of until, in pollSlice slices,
// returning early with found when done reports true between slices. It
// returns the unused budget; the time done takes, read off Env.Now, is spent
// like a slice. The budget is checked before done: a file that already exists
// must not hide a spent budget from the caller.
func (e *Env) waitPolling(ctx context.Context, companion string, until []string, left time.Duration, done func() bool) (status string, err error, found bool, rest time.Duration) {
	for {
		slice := min(pollSlice, left)
		status, err = e.Ctl.Wait(ctx, companion, until, slice)
		if !errors.Is(err, agterm.ErrTimeout) {
			return status, err, false, left
		}
		left -= slice
		if left <= 0 {
			return "", err, false, 0
		}
		// done may take real time (a screen read): it spends the budget too.
		t0 := e.Now()
		ok := done()
		left -= e.Now().Sub(t0)
		if ok {
			return "", nil, true, max(left, 0)
		}
		if left <= 0 {
			return "", err, false, 0
		}
	}
}

// waitSliced is waitPolling until the file at path exists.
func (e *Env) waitSliced(ctx context.Context, companion string, until []string, left time.Duration, path string) (string, error, bool, time.Duration) {
	return e.waitPolling(ctx, companion, until, left, func() bool {
		_, err := os.Stat(path)
		return err == nil
	})
}

// errPrompt marks a typeWhenIdle refusal because a permission prompt is open.
var errPrompt = errors.New("permission prompt open")

// idleError is a typeWhenIdle refusal: msg for the user, kind (agterm.ErrGone,
// agterm.ErrTimeout or errPrompt) for callers that act on it.
type idleError struct {
	kind error
	msg  string
}

func (e *idleError) Error() string { return e.msg }
func (e *idleError) Unwrap() error { return e.kind }

// settleLines is how many screen lines a settle check reads; dialogLines is
// how many of them, counted from the last non-blank one, are searched for a
// codex dialog.
const (
	settleLines = 40
	dialogLines = 12
)

// codexDialogMarkers are the footers of the codex dialogs that wait for the
// user (the same set agterm's codex hook watches), matched case-insensitively.
// Typing into one would answer it.
var codexDialogMarkers = []string{
	"press enter to confirm",
	"enter to submit answer",
	"enter to submit all",
	"allow command?",
}

// settler decides, for a codex worker whose agterm status never reports the
// end of its turn, that the turn is over anyway: the round's outcome is on
// disk (evidence), the screen has not changed since the previous check and it
// shows no codex dialog. It holds the previous screen, so successive checks
// must be one poll slice apart.
type settler struct {
	what     string      // the evidence, named for the warning ("round 1")
	evidence func() bool // the round's outcome is on disk
	prev     string
	havePrev bool
}

func newSettler(what string, evidence func() bool) *settler {
	return &settler{what: what, evidence: evidence}
}

// reset forgets the previous screen: the next check only records one.
func (s *settler) reset() { s.prev, s.havePrev = "", false }

// check reports whether the turn looks over. Without evidence, or when the
// screen cannot be read, it forgets the previous screen and reports false, so
// the status rule stays.
func (s *settler) check(ctx context.Context, ctl agterm.Ctl, companion string) bool {
	if !s.evidence() {
		s.reset()
		return false
	}
	screen, err := ctl.Text(ctx, companion, settleLines)
	if err != nil {
		s.reset()
		return false
	}
	same := s.havePrev && screen == s.prev
	s.prev, s.havePrev = screen, true
	return same && !hasDialog(screen)
}

// codexSettler is the settler of a worker started with harnessName, or nil for
// every harness but codex: claude workers learn their turn ends from agk's own
// hooks, and a claude applier paused with background work in flight can have a
// still screen while its turn is not over.
func codexSettler(harnessName, what string, evidence func() bool) *settler {
	if harnessName != config.Codex {
		return nil
	}
	return newSettler(what, evidence)
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// hasDialog reports whether the last dialogLines lines of screen, trailing
// blank lines not counted, carry a codex dialog marker.
func hasDialog(screen string) bool {
	lines := strings.Split(strings.TrimRight(screen, " \t\r\n"), "\n")
	if len(lines) > dialogLines {
		lines = lines[len(lines)-dialogLines:]
	}
	tail := strings.ToLower(strings.Join(lines, "\n"))
	for _, m := range codexDialogMarkers {
		if strings.Contains(tail, m) {
			return true
		}
	}
	return false
}

// knownStatuses are the agent statuses a warning may name.
var knownStatuses = []string{"active", "completed", "idle", "blocked"}

// currentStatus asks agterm for the worker's status right now. It returns ""
// when there is none to report (a timeout, an error, an unknown word); only a
// gone companion is an error.
func (e *Env) currentStatus(ctx context.Context, companion string) (string, error) {
	status, err := e.Ctl.Wait(ctx, companion, knownStatuses, statusCheck)
	if errors.Is(err, agterm.ErrGone) {
		return "", err
	}
	if err != nil || !slices.Contains(knownStatuses, status) {
		return "", nil
	}
	return status, nil
}

// typeWhenIdle types line into the worker's companion once its turn is over
// and no permission prompt is open, waiting up to timeout. what names the
// typed work ("round 2") for the warning when the worker does not report it
// as started. started tells whether the active handshake saw the worker pick
// the line up; a non-zero deadline caps that handshake at what is left of it.
//
// A nil settle waits for the status alone. With one (codex workers only), the
// wait runs in pollSlice slices over the same budget and the same statuses, so
// the status still decides first, and settle is checked between slices: when
// it reports the turn over, the line is typed anyway, with a warning. A
// refusal returns an error and types nothing.
func (e *Env) typeWhenIdle(ctx context.Context, w workerRole, companion, line, what string, timeout time.Duration, deadline time.Time, settle *settler) (started bool, err error) {
	until := []string{"completed", "blocked", "idle"}
	busy := &idleError{agterm.ErrTimeout, fmt.Sprintf("the %s %s is busy: its turn did not end within %d s", w.name, companion, int(timeout/time.Second))}
	var status string
	var settled bool
	// wctx bounds every agtermctl call of the fallback (the waits, the screen
	// reads, the last status query) by the effective deadline: start + timeout,
	// or the caller's deadline when that is earlier.
	wctx, end := ctx, time.Time{}
	if settle == nil {
		status, err = e.Ctl.Wait(ctx, companion, until, timeout)
	} else {
		end = e.Now().Add(timeout)
		if !deadline.IsZero() && deadline.Before(end) {
			end = deadline
		}
		budget := end.Sub(e.Now())
		if budget <= 0 {
			return false, busy
		}
		var cancel context.CancelFunc
		wctx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
		status, err, settled, _ = e.waitPolling(wctx, companion, until, budget, func() bool {
			return settle.check(wctx, e.Ctl, companion)
		})
		if err != nil && ctx.Err() == nil && wctx.Err() != nil && !errors.Is(err, agterm.ErrGone) {
			err = agterm.ErrTimeout // the deadline cut a call short
		}
	}
	switch {
	case errors.Is(err, agterm.ErrGone):
		return false, &idleError{agterm.ErrGone, fmt.Sprintf("the %s %s is gone; %s", w.name, companion, w.restart)}
	case errors.Is(err, agterm.ErrTimeout):
		return false, busy
	case err != nil:
		return false, fmt.Errorf("wait for %s %s: %v", w.name, companion, err)
	case status == "blocked":
		// A permission prompt is open: typed text would answer it.
		return false, &idleError{errPrompt, fmt.Sprintf("the %s %s waits for the user (permission prompt); answer it in the companion first", w.name, companion)}
	}
	if settled {
		// The status never said the turn was over. One last look at it: a
		// prompt that opened meanwhile still refuses, a turn end that arrived
		// is the ordinary path.
		cur, err := e.currentStatus(wctx, companion)
		switch {
		case err != nil:
			return false, &idleError{agterm.ErrGone, fmt.Sprintf("the %s %s is gone; %s", w.name, companion, w.restart)}
		case !e.Now().Before(end):
			return false, busy // the budget ran out while the screen was read
		case cur == "blocked":
			return false, &idleError{errPrompt, fmt.Sprintf("the %s %s waits for the user (permission prompt); answer it in the companion first", w.name, companion)}
		case cur != "completed" && cur != "idle":
			if cur == "" {
				cur = "no finished turn"
			}
			fmt.Fprintf(e.Stderr, "agk: warning: the %s %s still reports %s, but %s is on disk and its screen has not changed for %d s; treating its turn as over\n",
				w.name, companion, cur, settle.what, int(pollSlice/time.Second))
		}
		settle.reset()
	}
	if err := e.Ctl.Type(ctx, companion, line+"\r"); err != nil {
		if errors.Is(err, agterm.ErrGone) {
			return false, &idleError{agterm.ErrGone, fmt.Sprintf("the %s %s is gone; %s", w.name, companion, w.restart)}
		}
		return false, fmt.Errorf("type into %s %s: %v", w.name, companion, err)
	}
	startBy := e.Now().Add(sendHandshake)
	if !deadline.IsZero() && deadline.Before(startBy) {
		startBy = deadline
	}
	if why, hint := e.confirmStart(ctx, companion, line, startBy); why != "" {
		fmt.Fprintf(e.Stderr, "agk: warning: %s %s did not report %s as started: %s; %s\n", w.name, companion, what, why, hint)
		return false, nil
	}
	return true, nil
}

// What the warning tells the user to do, by why the line was not confirmed.
// Enter is advised only when the line is known to sit in the input box: with a
// permission prompt open it could approve the prompt. Once an Enter has been
// typed after the screen was read, the screen no longer proves an unsent line,
// so only the conditional hintLook is left.
const (
	hintInputBox = "the line may still be in its input box: press Enter in the companion"
	hintPrompt   = "the worker shows a permission prompt: look at the companion and answer it deliberately"
	hintLook     = "look at the companion: if the line is still in its input box, press Enter there; if it asks for permission, answer it deliberately"
)

// confirmStart waits until end for the worker to report the line it was just
// typed as started, and returns "" when it did, else why not and a hint for
// the user. It is two steps: a first wait for active, then, only when the turn
// is still over and the line is visible on the screen (so it sits unsent in
// the input box), one Enter typed on its own and a wait for the rest of the
// budget. Nothing is typed when the status is anything else, as an Enter could
// answer a permission prompt.
//
// Every agtermctl call is its own process and takes real time, so the budget
// is read off Env.Now before each step, and all calls run under one context
// that ends with the budget: Text and Type take no timeout of their own and
// Wait rounds its up. Any error here means "not started", never a refusal,
// because the line is typed and the round stays recorded.
func (e *Env) confirmStart(ctx context.Context, companion, line string, end time.Time) (why, hint string) {
	left := func() time.Duration { return end.Sub(e.Now()) }
	if left() <= 0 {
		return "no time left to wait", hintLook
	}
	ctx, cancel := context.WithTimeout(ctx, left())
	defer cancel()

	first := min(sendFirstHandshake, left())
	_, err := e.Ctl.Wait(ctx, companion, []string{"active"}, first)
	if err == nil {
		return "", ""
	}
	if !errors.Is(err, agterm.ErrTimeout) || left() <= 0 {
		return err.Error(), hintLook
	}
	status, err := e.Ctl.Wait(ctx, companion, []string{"active", "completed", "idle", "blocked"}, min(statusCheck, left()))
	if err != nil {
		return err.Error(), hintLook
	}
	switch status {
	case "active":
		return "", ""
	case "completed", "idle":
	case "blocked":
		return "the status is \"blocked\"", hintPrompt
	default:
		return "the status is " + strconv.Quote(status), hintLook
	}
	if left() <= 0 {
		return "no time left to wait", hintLook
	}
	screen, err := e.Ctl.Text(ctx, companion, handshakeLines)
	if err != nil {
		return err.Error(), hintLook
	}
	hint = hintLook
	if lineOnScreen(screen, line) {
		hint = hintInputBox // known to be unsent, until an Enter is typed
		if left() <= 0 {
			return "no time left to wait", hint
		}
		hint = hintLook // the Enter changes what the screen showed
		if err := e.Ctl.Type(ctx, companion, "\r"); err != nil {
			return err.Error(), hint
		}
	}
	if left() <= 0 {
		return "no time left to wait", hint
	}
	status, err = e.Ctl.Wait(ctx, companion, []string{"active", "blocked"}, left())
	switch {
	case err != nil:
		return err.Error(), hint
	case status == "blocked":
		return "the status is \"blocked\"", hintPrompt
	}
	return "", ""
}

// lineOnScreen reports whether line is among the screen's text, ignoring all
// whitespace: codex wraps a long path across rows and indents the next one.
// The screen is read only for this decision and never shown.
func lineOnScreen(screen, line string) bool {
	return strings.Contains(stripSpace(screen), stripSpace(line))
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// rollbackRound takes back round k after its line was never delivered:
// the run's current round returns to prev and the round directory goes.
func rollbackRound(dir string, prev, k int, stderr io.Writer) {
	if err := rundir.SetRound(dir, prev); err != nil {
		fmt.Fprintf(stderr, "agk: warning: %v\n", err)
	}
	os.RemoveAll(rundir.RoundDir(dir, k))
}
