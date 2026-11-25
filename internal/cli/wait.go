package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/fsx"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// defaultWaitTimeout bounds `agk review wait` unless --timeout says otherwise.
const defaultWaitTimeout = 1800

// companionID is the shape of an agterm session or pane id (a UUID, or a prefix of one).
var companionID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// waitOutcome is what `agk review wait` prints when result.json has no say.
type waitOutcome struct {
	Status string `json:"status"`
	Round  int    `json:"round"`
	Reason string `json:"reason,omitempty"`
}

// ReviewWait implements `agk review wait <run-dir> [--timeout SECONDS]`.
func (e *Env) ReviewWait(ctx context.Context, args []string) int {
	dir, timeout, code, ok := e.parseWaitArgs("agk review wait", rundir.Reviews, defaultWaitTimeout, args)
	if !ok {
		return code
	}
	run, err := loadReviewRun(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	k, err := rundir.Round(run.dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	rd := rundir.RoundDir(run.dir, k)
	companion, err := readCompanion(run.dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	res, have, err := readResult(rd)
	if err != nil {
		return e.errorf("%v", err)
	}
	switch {
	case have:
		return e.printJSON(res) // result.json decides, whatever the reviewer shows now
	case companion == "":
		// Inline run: it finished before `review start`/`send` returned.
		return e.errorf("%s has no result.json and no companion: the round did not record an outcome", rd)
	case !run.state.Interactive:
		return e.errorf("%s was started by an older agk and has no outcome for round %d; start a new review", run.dir, k)
	}
	return e.waitInteractive(ctx, run, k, companion, time.Duration(timeout)*time.Second)
}

// Files of an interactive round in rounds/<k>/.
const (
	fixupsFile = "fixups" // how many fix-ups were typed
	outputKept = "output.json"
)

func outputPath(dir string, k int) string {
	return filepath.Join(dir, outDirName, fmt.Sprintf("%d.json", k))
}

func fixupFile(rd string, n int) string { return filepath.Join(rd, fmt.Sprintf("fixup-%d.md", n)) }

// readFixups returns the number of fix-ups typed for the round.
func readFixups(rd string) (int, error) {
	b, err := os.ReadFile(filepath.Join(rd, fixupsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s: not a count: %q", filepath.Join(rd, fixupsFile), strings.TrimSpace(string(b)))
	}
	return n, nil
}

func writeFixups(rd string, n int) error {
	return fsx.ReplaceFile(filepath.Join(rd, fixupsFile), []byte(strconv.Itoa(n)+"\n"), 0o644)
}

// fixupStartedFile marks fix-up n as seen active.
func fixupStartedFile(rd string, n int) string { return fixupFile(rd, n) + ".started" }

// validOutput reports whether raw parses as a review.
func validOutput(raw []byte) bool {
	_, err := review.Parse(raw)
	return err == nil
}

// readOutput returns the reviewer's output file of the round, if any.
func readOutput(path string) (raw []byte, exists bool, err error) {
	raw, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return raw, err == nil, err
}

// waitInteractive waits for the outcome of round k of an interactive review.
// Under one deadline it loops: decide from the output file and the reviewer's
// agent status, act, and otherwise wait for the status in slices (the file
// decides even when a hook never fires). It records an outcome only from a
// valid output file or a spent fix-up budget.
func (e *Env) waitInteractive(ctx context.Context, run reviewRun, k int, companion string, left time.Duration) int {
	rd := rundir.RoundDir(run.dir, k)
	out := outputPath(run.dir, k)
	rep := reporter{env: e, target: companion}
	status, werr := "", error(nil)
	// A codex reviewer's status can stay wrong for good: its settled screen,
	// with an invalid output file on disk, ends its turn for the fix-up
	// decision. A claude reviewer's status comes from agk's own hooks.
	settle := codexSettler(run.state.Worker.Harness, "its output file", func() bool { return fileExists(out) })
	// One deadline for the whole wait: what is left is the smaller of the
	// budget the slices still count and the clock's remainder, so waits that
	// succeeded (and took time) spend it too.
	deadline := e.Now().Add(left)
	for {
		left = min(left, deadline.Sub(e.Now()))
		raw, exists, err := readOutput(out)
		if err != nil {
			return e.errorf("read %s: %v", out, err)
		}
		fixups, err := readFixups(rd)
		if err != nil {
			return e.errorf("%v", err)
		}
		started := fixups == 0
		if !started {
			_, serr := os.Stat(fixupStartedFile(rd, fixups))
			started = serr == nil
		}
		settled := false
		if settle != nil {
			// A typed fix-up that has not started closes Decide's gate, so
			// a settled screen could change nothing.
			if started && exists && !validOutput(raw) && status != "completed" && status != "idle" && status != "blocked" {
				// The screen read stays inside the wait's deadline.
				if rem := deadline.Sub(e.Now()); rem > 0 {
					cctx, cancel := context.WithTimeout(ctx, rem)
					settled = settle.check(cctx, e.Ctl, companion)
					cancel()
				}
				left = min(left, deadline.Sub(e.Now()))
			} else {
				settle.reset()
			}
		}
		act := review.Decide(k, raw, exists, status, werr, fixups, started, settled)
		switch act.Kind {
		case review.Completed:
			return e.recordCompleted(ctx, run, k, act.Review, rep)
		case review.RecordBlocked:
			return e.recordBlocked(ctx, run, k, act.Reason, raw, rep)
		case review.NeedsHuman:
			return e.printJSON(waitOutcome{Status: "needs-human", Round: k, Reason: act.Reason})
		case review.Cancelled:
			return e.printJSON(waitOutcome{Status: "cancelled", Round: k})
		case review.Running:
			return e.printJSON(waitOutcome{Status: "running", Round: k})
		}
		if left <= 0 {
			return e.printJSON(waitOutcome{Status: "running", Round: k})
		}
		if act.Kind == review.Fixup {
			if stop := e.typeFixup(ctx, run, k, companion, fixups+1, act.Err, left, deadline, settle); stop != nil {
				return *stop
			}
			if settle != nil {
				settle.reset() // the next turn's screen has to settle again
			}
			status, werr = "", nil
			continue
		}
		status, werr, left = e.waitStep(ctx, run, k, companion, fixups, started, left)
		if werr != nil && !errors.Is(werr, agterm.ErrGone) && !errors.Is(werr, agterm.ErrTimeout) {
			return e.errorf("wait for reviewer %s: %v", companion, werr)
		}
	}
}

// waitStep waits for the next agent status that can move the round on, in
// slices that also end when the output file appears. Until a typed fix-up has
// started, or the round itself, only `active` (or a permission prompt) counts:
// the status shown may still be the previous turn's.
func (e *Env) waitStep(ctx context.Context, run reviewRun, k int, companion string, fixups int, fixupStarted bool, left time.Duration) (string, error, time.Duration) {
	rd := rundir.RoundDir(run.dir, k)
	until := []string{"completed", "blocked", "idle"}
	var onActive func()
	switch {
	case !fixupStarted:
		until = []string{"active", "blocked"}
		onActive = func() {
			markStarted(run.dir, k)
			_ = fsx.ReplaceFile(fixupStartedFile(rd, fixups), nil, 0o644)
		}
	case !roundStarted(run.dir, k):
		// A fresh session's idle must not read as "turn over". A first round may
		// also have ended already.
		until = []string{"active", "blocked"}
		if k == 1 {
			until = append(until, "completed")
		}
		onActive = func() { markStarted(run.dir, k) }
	}
	status, err, _, left := e.waitSliced(ctx, companion, until, left, outputPath(run.dir, k))
	if err == nil && status == "active" {
		onActive()
		status = ""
	}
	return status, err, left
}

// typeFixup writes fix-up n, types its line into the session and records the
// attempt. It returns the exit code to stop with, or nil to carry on waiting.
func (e *Env) typeFixup(ctx context.Context, run reviewRun, k int, companion string, n int, verr error, left time.Duration, deadline time.Time, settle *settler) *int {
	rd := rundir.RoundDir(run.dir, k)
	prompt, err := review.FixupPrompt(k, outputPath(run.dir, k), filepath.Join(run.dir, schemaName), verr)
	if err != nil {
		return ptr(e.errorf("render fix-up prompt: %v", err))
	}
	path := fixupFile(rd, n)
	if err := fsx.ReplaceFile(path, []byte(prompt), 0o644); err != nil {
		return ptr(e.errorf("write fix-up prompt: %v", err))
	}
	if err := writeFixups(rd, n); err != nil {
		return ptr(e.errorf("record fix-up: %v", err))
	}
	started, err := e.typeWhenIdle(ctx, reviewerWorker, companion, kickoff(path), fmt.Sprintf("fix-up %d", n), left, deadline, settle)
	if err != nil {
		// Nothing was typed: the attempt did not happen.
		_ = writeFixups(rd, n-1)
		os.Remove(path)
		switch {
		case errors.Is(err, agterm.ErrGone):
			return ptr(e.printJSON(waitOutcome{Status: "cancelled", Round: k}))
		case errors.Is(err, errPrompt):
			return ptr(e.printJSON(waitOutcome{Status: "needs-human", Round: k, Reason: review.ReasonPermission}))
		case errors.Is(err, agterm.ErrTimeout):
			return ptr(e.printJSON(waitOutcome{Status: "running", Round: k}))
		}
		return ptr(e.errorf("%v", err))
	}
	if started {
		markStarted(run.dir, k)
		_ = fsx.ReplaceFile(fixupStartedFile(rd, n), nil, 0o644)
	}
	return nil
}

func ptr[T any](v T) *T { return &v }

// recordCompleted writes review.json and result.json of round k and the
// summary info line, then prints the outcome. The reviewer's turn may still
// run: only the info line is touched, never the agent status.
func (e *Env) recordCompleted(ctx context.Context, run reviewRun, k int, r review.Review, rep reporter) int {
	rd := rundir.RoundDir(run.dir, k)
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return e.errorf("%v", err)
	}
	if err := fsx.ReplaceFile(filepath.Join(rd, "review.json"), append(b, '\n'), 0o644); err != nil {
		return e.errorf("write review.json: %v", err)
	}
	res := runResult{Status: "completed", Round: k, Findings: len(r.Findings), Verdict: r.Verdict}
	if err := writeResult(rd, res); err != nil {
		return e.errorf("write result.json: %v", err)
	}
	rep.set(ctx, "completed", review.SummaryLine(run.label(k), r))
	return e.printJSON(res)
}

// recordBlocked fails round k with reason, keeping the raw output.
func (e *Env) recordBlocked(ctx context.Context, run reviewRun, k int, reason string, raw []byte, rep reporter) int {
	rd := rundir.RoundDir(run.dir, k)
	if err := fsx.ReplaceFile(filepath.Join(rd, outputKept), raw, 0o644); err != nil {
		fmt.Fprintf(e.Stderr, "agk: warning: keep the raw output: %v\n", err)
	}
	res := runResult{Status: "blocked", Round: k, Reason: reason}
	if err := writeResult(rd, res); err != nil {
		return e.errorf("write result.json: %v", err)
	}
	rep.set(ctx, "blocked", review.FailureLine(run.label(k), reason))
	return e.printJSON(res)
}

// parseInterleaved parses fs over args with positional arguments allowed
// before, between and after flags.
func parseInterleaved(fs *flag.FlagSet, args []string) (pos []string, code int, ok bool) {
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, ExitOK, false
			}
			return nil, ExitError, false
		}
		if fs.NArg() == 0 {
			return pos, ExitOK, true
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// parseWaitArgs accepts the run dir before or after --timeout.
func (e *Env) parseWaitArgs(name, kind string, def int, args []string) (dir string, timeout int, code int, ok bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.IntVar(&timeout, "timeout", def, "seconds to wait for the outcome")
	pos, code, ok := parseInterleaved(fs, args)
	if !ok {
		return "", 0, code, false
	}
	if len(pos) != 1 {
		return "", 0, e.errorf("usage: %s <run-dir> [--timeout SECONDS]", name), false
	}
	if timeout <= 0 {
		return "", 0, e.errorf("--timeout must be a positive number of seconds, got %d", timeout), false
	}
	dir, err := filepath.Abs(pos[0])
	if err != nil {
		return "", 0, e.errorf("%v", err), false
	}
	if _, err := rundir.Root(dir, kind); err != nil {
		return "", 0, e.errorf("%v", err), false
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", 0, e.errorf("%s is not a run directory", dir), false
	}
	return dir, timeout, ExitOK, true
}

// readCompanion returns the recorded companion id, or "" for an inline run.
func readCompanion(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, CompanionFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(b))
	if !companionID.MatchString(id) {
		return "", fmt.Errorf("%s: not a session id: %q", filepath.Join(dir, CompanionFile), id)
	}
	return id, nil
}

// readResult reads rd/result.json; have is false when it does not exist yet.
func readResult(rd string) (res runResult, have bool, err error) {
	p := filepath.Join(rd, "result.json")
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return runResult{}, false, nil
	}
	if err != nil {
		return runResult{}, false, err
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return runResult{}, false, fmt.Errorf("%s: %v", p, err)
	}
	if res.Status != "completed" && res.Status != "blocked" {
		return runResult{}, false, fmt.Errorf("%s: unknown status %q", p, res.Status)
	}
	return res, true, nil
}
