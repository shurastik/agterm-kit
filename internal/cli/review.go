package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/fsx"
	"github.com/shurastik/agterm-kit/internal/harness"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// reporter mirrors review progress into agterm status; a no-op outside agterm.
type reporter struct {
	env    *Env
	target string
}

// statusTimeout bounds each status update, which runs detached from the
// review's context so a cancelled run still reports blocked.
const statusTimeout = 5 * time.Second

// statusAttempts and statusRetryDelay govern final-status retries: a lost
// completed/blocked would leave `session wait` hanging.
var (
	statusAttempts   = 3
	statusRetryDelay = 500 * time.Millisecond
)

// set reports state and line, retrying final states; it reports whether
// agterm took the update.
func (r reporter) set(ctx context.Context, state, line string) bool {
	if r.target == "" {
		return false
	}
	attempts := 1
	if state == "completed" || state == "blocked" {
		attempts = statusAttempts
	}
	for i := 1; ; i++ {
		err := r.update(ctx, state, line)
		if err == nil {
			return true
		}
		// Status is best effort: a broken socket must not abort the review.
		fmt.Fprintf(r.env.Stderr, "agk: warning: %v\n", err)
		if i >= attempts {
			return false
		}
		time.Sleep(statusRetryDelay)
	}
}

func (r reporter) update(ctx context.Context, state, line string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusTimeout)
	defer cancel()
	if err := r.env.Ctl.Status(ctx, r.target, state); err != nil {
		return err
	}
	return r.env.Ctl.Info(ctx, r.target, line)
}

// Run-dir files of a review.
const (
	workerFile = "worker.json" // reviewState
	paneFile   = "pane"        // the reviewer loop's own pane id
	reviewLog  = "reviewer.log"
)

// reviewState is worker.json: the configuration a review run is pinned to.
// A diff review records its base, a plan review its change.
type reviewState struct {
	Worker  config.Worker `json:"worker"`
	Base    string        `json:"base,omitempty"`
	Change  string        `json:"change,omitempty"`
	Session string        `json:"session,omitempty"`
}

// runResult is rounds/<k>/result.json, the round's outcome on disk for
// callers whose `session wait` timed out.
type runResult struct {
	Status   string `json:"status"`
	Round    int    `json:"round"`
	Reason   string `json:"reason,omitempty"`
	Findings int    `json:"findings"`
	Verdict  string `json:"verdict,omitempty"`
}

func writeResult(rd string, r runResult) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return fsx.ReplaceFile(filepath.Join(rd, "result.json"), append(b, '\n'), 0o644)
}

// reviewRun is a verified review run directory.
type reviewRun struct {
	dir, root string
	num       int
	state     reviewState
}

func loadReviewRun(dir string) (reviewRun, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return reviewRun{}, err
	}
	root, err := rundir.Root(dir, rundir.Reviews)
	if err != nil {
		return reviewRun{}, err
	}
	num, err := rundir.Number(dir)
	if err != nil {
		return reviewRun{}, err
	}
	r := reviewRun{dir: dir, root: root, num: num}
	have, err := rundir.ReadJSON(dir, workerFile, &r.state)
	if err != nil {
		return reviewRun{}, err
	}
	if !have || r.state.Worker.Harness == "" {
		return reviewRun{}, fmt.Errorf("%s has no %s: not a round-based run directory", dir, workerFile)
	}
	if _, err := rundir.Round(dir); err != nil {
		return reviewRun{}, err
	}
	return r, nil
}

func (r reviewRun) label(k int) review.Label {
	return review.Label{Harness: r.state.Worker.Harness, Num: r.num, Round: k}
}

// ReviewRun implements `agk review run <dir>`: the process inside the
// companion. It runs the recorded round, then waits in its mailbox for
// `agk review send` to request the next one; outside agterm it runs one round.
func (e *Env) ReviewRun(ctx context.Context, args []string) int {
	rep := reporter{env: e, target: e.session()}
	setupFailed := func(err error) int {
		rep.set(ctx, "blocked", review.FailureLine(review.Label{}, review.ReasonSetupFailed))
		return e.errorf("%s: %v", review.ReasonSetupFailed, err)
	}
	if len(args) != 1 {
		return setupFailed(errors.New("usage: agk review run <run-dir>"))
	}
	run, err := loadReviewRun(args[0])
	if err != nil {
		return setupFailed(err)
	}
	if rep.target != "" {
		if pane := e.Getenv("AGTERM_PANE_ID"); pane != "" {
			if err := fsx.ReplaceFile(filepath.Join(run.dir, paneFile), []byte(pane+"\n"), 0o644); err != nil {
				return setupFailed(err)
			}
		}
	}
	k, _ := rundir.Round(run.dir)
	for {
		code := e.runRound(ctx, run.dir, k, e.Stdout, rep)
		if rep.target == "" || ctx.Err() != nil {
			return code
		}
		next, err := e.nextRound(ctx, run.dir, k)
		if err != nil {
			if ctx.Err() != nil {
				return code
			}
			return e.errorf("wait for the next round: %v", err)
		}
		k = next
	}
}

// nextRound blocks in the companion's mailbox until the run's round file
// moves past k.
func (e *Env) nextRound(ctx context.Context, dir string, k int) (int, error) {
	for {
		_, err := e.Ctl.MsgRead(ctx, 0)
		if err != nil && !errors.Is(err, agterm.ErrTimeout) {
			return 0, err
		}
		if cur, err := rundir.Round(dir); err == nil && cur > k {
			return cur, nil
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
	}
}

// runRound runs round k of the review in dir and records its outcome.
func (e *Env) runRound(ctx context.Context, dir string, k int, term io.Writer, rep reporter) int {
	run, err := loadReviewRun(dir)
	if err != nil {
		rep.set(ctx, "blocked", review.FailureLine(review.Label{}, review.ReasonSetupFailed))
		return e.errorf("%s: %v", review.ReasonSetupFailed, err)
	}
	rd := rundir.RoundDir(run.dir, k)
	l := run.label(k)
	fail := func(reason string, err error) int {
		if werr := writeResult(rd, runResult{Status: "blocked", Round: k, Reason: reason}); werr != nil {
			fmt.Fprintf(e.Stderr, "agk: warning: %v\n", werr)
		}
		rep.set(ctx, "blocked", review.FailureLine(l, reason))
		return e.errorf("%s: %v", reason, err)
	}
	prompt, err := os.ReadFile(filepath.Join(rd, "prompt.md"))
	if err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	driver, err := e.Driver(run.state.Worker.Harness)
	if err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	fmt.Fprintf(harness.NewStampWriter(term, e.Now), "agk: review #%d round %d started, run dir %s\n", run.num, k, run.dir)
	if rep.set(ctx, "active", review.ReviewingLine(l)) {
		// Only a confirmed active replaces the last round's status, so only
		// then may `review wait` trust the statuses that follow.
		markStarted(run.dir, k)
	}
	turn := harness.Turn{Worker: run.state.Worker, Root: run.root, Dir: rd, Log: reviewLog, Prompt: string(prompt),
		Session: run.state.Session, Resume: run.state.Session != ""}
	session, rerr := driver.Review(ctx, turn, term)
	if session != "" && session != run.state.Session {
		run.state.Session = session
		if err := rundir.WriteJSON(run.dir, workerFile, run.state); err != nil {
			fmt.Fprintf(e.Stderr, "agk: warning: record reviewer session: %v\n", err)
		}
	}
	if rerr != nil {
		return fail(review.ReasonFailed(run.state.Worker.Harness), rerr)
	}
	raw, err := os.ReadFile(filepath.Join(rd, harness.OutputFile))
	if err != nil {
		return fail(review.ReasonInvalidOutput, err)
	}
	r, err := review.Parse(raw)
	if err != nil {
		return fail(review.ReasonInvalidOutput, err)
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	if err := fsx.ReplaceFile(filepath.Join(rd, "review.json"), append(b, '\n'), 0o644); err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	if err := writeResult(rd, runResult{Status: "completed", Round: k, Findings: len(r.Findings), Verdict: r.Verdict}); err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	rep.set(ctx, "completed", review.SummaryLine(l, r))
	fmt.Fprintf(harness.NewStampWriter(term, e.Now), "agk: review #%d round %d: %d finding(s), verdict %s -> %s\n",
		run.num, k, len(r.Findings), r.Verdict, filepath.Join(rd, "review.json"))
	return ExitOK
}

// forwardedEnv lists the caller variables a companion command carries.
var forwardedEnv = []string{"PATH", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "AGTERMCTL"}

// CompanionFile is the run-dir file holding a worker companion's session id.
const CompanionFile = "companion"

type startResult struct {
	Companion *string `json:"companion"`
	Dir       string  `json:"dir"`
}

// companionCommand builds `[env K=V…] '<agk>' <sub> '<path>'` for a companion,
// forwarding PATH (so harnesses and agtermctl resolve even without a login
// shell), CODEX_HOME, CLAUDE_CONFIG_DIR and AGTERMCTL. Never secrets: the
// command line is visible in ps. sub is plain words.
func (e *Env) companionCommand(sub, path string) (string, error) {
	exe, err := e.Executable()
	if err != nil {
		return "", fmt.Errorf("locate agk binary: %v", err)
	}
	prefix, err := e.envCommand(exe)
	if err != nil {
		return "", err
	}
	qp, err := shellQuote(path)
	if err != nil {
		return "", err
	}
	return prefix + " " + sub + " " + qp, nil
}

// envCommand quotes argv for the login shell, prefixed with the forwarded
// environment.
func (e *Env) envCommand(argv ...string) (string, error) {
	var parts []string
	for _, k := range forwardedEnv {
		if v := e.Getenv(k); v != "" {
			q, err := shellQuote(k + "=" + v)
			if err != nil {
				return "", err
			}
			parts = append(parts, q)
		}
	}
	if len(parts) > 0 {
		parts = append([]string{"env"}, parts...)
	}
	for _, a := range argv {
		q, err := shellQuote(a)
		if err != nil {
			return "", err
		}
		parts = append(parts, q)
	}
	return strings.Join(parts, " "), nil
}

// ReviewStart implements `agk review start [--base REF | --change NAME] [--reviewer H[:M]]`.
func (e *Env) ReviewStart(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk review start", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	base := fs.String("base", "main", "review changes since the merge base of this ref and HEAD")
	change := fs.String("change", "", "review the planning artifacts of this OpenSpec change instead of a diff (excludes --base)")
	reviewer := fs.String("reviewer", "", "reviewer HARNESS[:MODEL] for this run")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitError
	}
	if fs.NArg() != 0 {
		return e.errorf("unexpected arguments %q", fs.Args())
	}
	state := reviewState{Base: *base}
	// Mode follows the flags given, not their values: an empty --change must
	// not fall back to a diff review.
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if given["change"] {
		if given["base"] {
			return e.errorf("--change and --base exclude each other")
		}
		if !changeName.MatchString(*change) {
			return e.errorf("invalid change name %q: want kebab-case, like add-dark-mode", *change)
		}
		state = reviewState{Change: *change}
	}
	root, prompt, err := e.reviewPrompt(ctx, e.Cwd, state, 1, "")
	if err != nil {
		return e.errorf("%v", err)
	}
	cfg, err := e.resolveConfig(root, config.Flags{Reviewer: *reviewer})
	if err != nil {
		return e.errorf("%v", err)
	}
	e.warnSame(cfg)
	parent := e.session()
	if parent != "" {
		// Refuse unquotable paths before leaving a run dir behind. The run dir
		// lives under the root, so checking the root covers it.
		if _, err := e.companionCommand("review run", root); err != nil {
			return e.errorf("%v", err)
		}
	}
	dir, err := rundir.New(root, rundir.Reviews, e.Now())
	if err != nil {
		return e.errorf("create run dir: %v", err)
	}
	if err := initRound(dir, 1, prompt); err != nil {
		return e.errorf("%v", err)
	}
	state.Worker = cfg.Reviewer
	if err := rundir.WriteJSON(dir, workerFile, state); err != nil {
		return e.errorf("record reviewer: %v", err)
	}
	if parent == "" {
		// Outside agterm: review in the foreground. Reviewer output goes to
		// stderr so stdout carries only the result object.
		code := e.runRound(ctx, dir, 1, e.Stderr, reporter{env: e})
		if c := e.printJSON(startResult{Dir: dir}); c != ExitOK {
			return c
		}
		return code
	}
	cmd, err := e.companionCommand("review run", dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	num, err := rundir.Number(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	name := fmt.Sprintf("%s-review #%d", cfg.Reviewer.Harness, num)
	id, err := e.startCompanion(ctx, parent, name, cmd, root, dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	return e.printJSON(startResult{Companion: &id, Dir: dir})
}

// reviewPrompt collects the scope of a run with state from dir and renders
// the prompt of round k: the first-round prompt, wrapped for a re-review when
// the reviewer has a session to resume. A plan review (state.Change set)
// reads the change from disk; a diff review diffs against state.Base. It
// returns the repository root with the prompt.
func (e *Env) reviewPrompt(ctx context.Context, dir string, state reviewState, k int, note string) (string, string, error) {
	var root, body string
	if state.Change != "" {
		scope, err := e.CollectPlan(ctx, dir, state.Change)
		if err != nil {
			return "", "", err
		}
		root = scope.Root
		if body, err = review.PlanPrompt(scope); err != nil {
			return "", "", fmt.Errorf("render prompt: %v", err)
		}
	} else {
		scope, err := e.Collect(ctx, dir, state.Base)
		if err != nil {
			return "", "", err
		}
		root = scope.Root
		if body, err = review.Prompt(scope); err != nil {
			return "", "", fmt.Errorf("render prompt: %v", err)
		}
	}
	if state.Session == "" {
		return root, body, nil // nothing to resume: start over
	}
	prompt, err := review.Rereview(body, k, note)
	if err != nil {
		return "", "", fmt.Errorf("render prompt: %v", err)
	}
	return root, prompt, nil
}

// initRound creates rounds/<k>/prompt.md and records k as the current round.
func initRound(dir string, k int, prompt string) error {
	rd, err := rundir.MakeRound(dir, k)
	if err != nil {
		return fmt.Errorf("create round dir: %v", err)
	}
	if err := fsx.ReplaceFile(filepath.Join(rd, "prompt.md"), []byte(prompt), 0o644); err != nil {
		return fmt.Errorf("write prompt: %v", err)
	}
	if err := rundir.SetRound(dir, k); err != nil {
		return fmt.Errorf("record round: %v", err)
	}
	return nil
}

// startCompanion creates a worker companion and records its id in dir. A
// companion whose id cannot be recorded would never be found again, so it is
// closed.
func (e *Env) startCompanion(ctx context.Context, parent, name, cmd, cwd, dir string) (string, error) {
	id, err := e.Ctl.NewCompanion(ctx, parent, name, cmd, cwd)
	if err != nil {
		return "", fmt.Errorf("start companion: %v", err)
	}
	if err := fsx.ReplaceFile(filepath.Join(dir, CompanionFile), []byte(id+"\n"), 0o644); err != nil {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusTimeout)
		defer cancel()
		if cerr := e.Ctl.Close(cctx, id); cerr != nil {
			fmt.Fprintf(e.Stderr, "agk: warning: close companion %s: %v\n", id, cerr)
		}
		return "", fmt.Errorf("record companion id: %v", err)
	}
	return id, nil
}

type sendResult struct {
	Companion *string `json:"companion"`
	Dir       string  `json:"dir"`
	Round     int     `json:"round"`
}

// sendHandshake bounds how long `review send` waits for the reviewer to pick
// up the round, so a following `review wait` does not see the last round's
// stale completed status.
const sendHandshake = 30 * time.Second

// ReviewSend implements `agk review send <run-dir> [--note FILE]`.
func (e *Env) ReviewSend(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk review send", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	notePath := fs.String("note", "", "file with a note for the reviewer (what was fixed)")
	pos, code, ok := parseInterleaved(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return e.errorf("usage: agk review send <run-dir> [--note FILE]")
	}
	run, err := loadReviewRun(pos[0])
	if err != nil {
		return e.errorf("%v", err)
	}
	note := ""
	if *notePath != "" {
		b, err := os.ReadFile(*notePath)
		if err != nil {
			return e.errorf("read note: %v", err)
		}
		note = string(b)
	}
	k, _ := rundir.Round(run.dir)
	var last runResult
	have, err := rundir.ReadJSON(rundir.RoundDir(run.dir, k), "result.json", &last)
	if err != nil {
		return e.errorf("%v", err)
	}
	if !have {
		return e.errorf("round %d of %s is still running; wait for it first", k, run.dir)
	}
	companion, err := readCompanion(run.dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	pane := ""
	if companion != "" {
		b, err := os.ReadFile(filepath.Join(run.dir, paneFile))
		if err != nil {
			return e.errorf("reviewer pane unknown (%v); start a new review", err)
		}
		pane = strings.TrimSpace(string(b))
		if !companionID.MatchString(pane) {
			return e.errorf("%s: not a pane id: %q", filepath.Join(run.dir, paneFile), pane)
		}
	}
	_, prompt, err := e.reviewPrompt(ctx, run.root, run.state, k+1, note)
	if err != nil {
		return e.errorf("%v", err)
	}
	if err := initRound(run.dir, k+1, prompt); err != nil {
		return e.errorf("%v", err)
	}
	if companion == "" {
		code := e.runRound(ctx, run.dir, k+1, e.Stderr, reporter{env: e})
		if c := e.printJSON(sendResult{Dir: run.dir, Round: k + 1}); c != ExitOK {
			return c
		}
		return code
	}
	if err := e.Ctl.MsgSend(ctx, companion+":"+pane, fmt.Sprintf("round %d", k+1)); err != nil {
		// The reviewer never heard of the round: take it back.
		if rerr := rundir.SetRound(run.dir, k); rerr != nil {
			fmt.Fprintf(e.Stderr, "agk: warning: %v\n", rerr)
		}
		os.RemoveAll(rundir.RoundDir(run.dir, k+1))
		if errors.Is(err, agterm.ErrGone) {
			return e.errorf("the reviewer is gone (%v); start a new review", err)
		}
		return e.errorf("%v", err)
	}
	if _, err := e.Ctl.Wait(ctx, companion, []string{"active"}, sendHandshake); err != nil {
		fmt.Fprintf(e.Stderr, "agk: warning: reviewer %s did not report round %d as started: %v\n", companion, k+1, err)
	}
	return e.printJSON(sendResult{Companion: &companion, Dir: run.dir, Round: k + 1})
}

// ReviewClose implements `agk review close <run-dir>`.
func (e *Env) ReviewClose(ctx context.Context, args []string) int {
	if len(args) != 1 {
		return e.errorf("usage: agk review close <run-dir>")
	}
	run, err := loadReviewRun(args[0])
	if err != nil {
		return e.errorf("%v", err)
	}
	return e.closeCompanion(ctx, run.dir)
}

// closeCompanion closes the companion recorded in dir, if any and still open.
func (e *Env) closeCompanion(ctx context.Context, dir string) int {
	companion, err := readCompanion(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	if companion == "" {
		return ExitOK
	}
	if err := e.Ctl.Close(ctx, companion); err != nil && !errors.Is(err, agterm.ErrGone) {
		return e.errorf("close companion %s: %v", companion, err)
	}
	return ExitOK
}

// shellQuote single-quotes s for the user's login shell (sh-family or fish).
// fish treats backslashes inside single quotes as escapes, so paths with a
// backslash or a line break are refused rather than quoted.
func shellQuote(s string) (string, error) {
	if strings.ContainsAny(s, "\\\n\r") {
		return "", fmt.Errorf("path %q contains a backslash or line break; move it to a plain path", s)
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'", nil
}

func (e *Env) printJSON(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return e.errorf("%v", err)
	}
	fmt.Fprintln(e.Stdout, string(b))
	return ExitOK
}
