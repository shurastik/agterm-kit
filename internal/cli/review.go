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

	"github.com/shurastik/agterm-kit/assets"
	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/fsx"
	"github.com/shurastik/agterm-kit/internal/harness"
	"github.com/shurastik/agterm-kit/internal/review"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// reporter sets a worker companion's sidebar info line. It never touches the
// agent status: an interactive harness reports its own turns through its
// hooks, and a status set by agk could mark a turn done or blocked that is not.
type reporter struct {
	env    *Env
	target string
}

// statusTimeout bounds each info update, which runs detached from the caller's
// context so a cancelled run still reports.
const statusTimeout = 5 * time.Second

// statusAttempts and statusRetryDelay govern final-line retries: the outcome
// is on disk either way, but the sidebar should show it.
var (
	statusAttempts   = 3
	statusRetryDelay = 500 * time.Millisecond
)

// set updates the info line, retrying the final states ("completed",
// "blocked"); state only picks the retry policy. It reports whether agterm
// took the update.
func (r reporter) set(ctx context.Context, state, line string) bool {
	if r.target == "" {
		return false
	}
	attempts := 1
	if state == "completed" || state == "blocked" {
		attempts = statusAttempts
	}
	for i := 1; ; i++ {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusTimeout)
		err := r.env.Ctl.Info(cctx, r.target, line)
		cancel()
		if err == nil {
			return true
		}
		// The line is best effort: a broken socket must not abort the review.
		fmt.Fprintf(r.env.Stderr, "agk: warning: %v\n", err)
		if i >= attempts {
			return false
		}
		time.Sleep(statusRetryDelay)
	}
}

// Run-dir files of a review.
const (
	workerFile = "worker.json" // reviewState
	reviewLog  = "reviewer.log"
)

// reviewState is worker.json: the configuration a review is pinned to.
// A diff review records its base, a plan review its change.
type reviewState struct {
	Worker  config.Worker `json:"worker"`
	Base    string        `json:"base,omitempty"`
	Change  string        `json:"change,omitempty"`
	Session string        `json:"session,omitempty"`
	// Interactive marks a reviewer that runs as a harness TUI in its
	// companion. A companion without it was started by an older agk.
	Interactive bool `json:"interactive,omitempty"`
}

// Files of an interactive review in the run directory.
const (
	outDirName = "out"         // the only place the reviewer may write
	schemaName = "schema.json" // the review output schema, for the reviewer to read
)

// prepareOut creates <run>/out/ and writes the embedded review schema to
// <run>/schema.json.
func prepareOut(dir string) (out, schema string, err error) {
	out = filepath.Join(dir, outDirName)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", "", err
	}
	schema = filepath.Join(dir, schemaName)
	if err := fsx.ReplaceFile(schema, assets.ReviewSchema, 0o644); err != nil {
		return "", "", err
	}
	return out, schema, nil
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

// reviewRun is a verified run directory of a review.
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

// runRound runs round k of the inline review in dir (outside agterm, the
// reviewer headless in the foreground) and records its outcome.
func (e *Env) runRound(ctx context.Context, dir string, k int, term io.Writer) int {
	run, err := loadReviewRun(dir)
	if err != nil {
		return e.errorf("%s: %v", review.ReasonSetupFailed, err)
	}
	rd := rundir.RoundDir(run.dir, k)
	fail := func(reason string, err error) int {
		if werr := writeResult(rd, runResult{Status: "blocked", Round: k, Reason: reason}); werr != nil {
			fmt.Fprintf(e.Stderr, "agk: warning: %v\n", werr)
		}
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
	fmt.Fprintf(harness.NewStampWriter(term, e.Now), "agk: review #%d round %d: %d finding(s), verdict %s -> %s\n",
		run.num, k, len(r.Findings), r.Verdict, filepath.Join(rd, "review.json"))
	return ExitOK
}

// forwardedEnv lists the caller variables a companion command carries.
var forwardedEnv = []string{"PATH", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "AGTERMCTL"}

// CompanionFile is the run-dir file holding a worker companion's session id.
const CompanionFile = "companion"

// startResult is what an inline `review start` prints; inside agterm it is
// applyStart (with started).
type startResult struct {
	Companion *string `json:"companion"`
	Dir       string  `json:"dir"`
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
	root, fingerprint, render, err := e.reviewScope(ctx, e.Cwd, state)
	if err != nil {
		return e.errorf("%v", err)
	}
	cfg, err := e.resolveConfig(root, config.Flags{Reviewer: *reviewer})
	if err != nil {
		return e.errorf("%v", err)
	}
	e.warnSame(cfg)
	w := cfg.Reviewer
	if err := harness.CheckReviewerArgs(w.Harness, w.ExtraArgs); err != nil {
		return e.errorf("%v", err)
	}
	parent := e.session()
	if parent != "" {
		// Refuse unquotable paths before leaving a run dir behind. The run dir
		// lives under the root, so checking the root covers it; the claude
		// hooks run the agk binary through the shell too.
		if _, err := e.envCommand(root); err != nil {
			return e.errorf("%v", err)
		}
		if w.Harness == config.Claude {
			if err := e.checkReviewerSettings(); err != nil {
				return e.errorf("%v", err)
			}
			// The reviewer's out path goes into an Edit permission rule.
			if err := harness.CheckRulePath(root); err != nil {
				return e.errorf("%v", err)
			}
		}
	}
	dir, err := rundir.New(root, rundir.Reviews, e.Now())
	if err != nil {
		return e.errorf("create run dir: %v", err)
	}
	state.Worker = w
	if parent == "" {
		// Outside agterm: review in the foreground. Reviewer output goes to
		// stderr so stdout carries only the result object.
		prompt, err := render(review.Output{})
		if err != nil {
			return e.errorf("render prompt: %v", err)
		}
		if err := initRound(dir, 1, prompt, fingerprint); err != nil {
			return e.errorf("%v", err)
		}
		if err := rundir.WriteJSON(dir, workerFile, state); err != nil {
			return e.errorf("record reviewer: %v", err)
		}
		code := e.runRound(ctx, dir, 1, e.Stderr)
		if c := e.printJSON(startResult{Dir: dir}); c != ExitOK {
			return c
		}
		return code
	}
	return e.startInteractive(ctx, parent, root, dir, fingerprint, state, render)
}

// startInteractive runs the reviewer as a harness TUI in a companion of
// parent: round 1's prompt is written with the output-file contract and a
// single typed-in line points the reviewer at it.
func (e *Env) startInteractive(ctx context.Context, parent, root, dir, fingerprint string, state reviewState, render func(review.Output) (string, error)) int {
	w := state.Worker
	driver, err := e.Driver(w.Harness)
	if err != nil {
		return e.errorf("%v", err)
	}
	out, schema, err := prepareOut(dir)
	if err != nil {
		return e.errorf("prepare reviewer output: %v", err)
	}
	prompt, err := render(reviewOutput(w.Harness, root, schema, outputPath(dir, 1)))
	if err != nil {
		return e.errorf("render prompt: %v", err)
	}
	if err := initRound(dir, 1, prompt, fingerprint); err != nil {
		return e.errorf("%v", err)
	}
	state.Interactive = true
	var files harness.ReviewerFiles
	if w.Harness == config.Claude {
		state.Session = e.NewSessionID()
		if files.Settings, err = e.writeReviewerSettings(dir); err != nil {
			return e.errorf("write reviewer settings: %v", err)
		}
	}
	if err := rundir.WriteJSON(dir, workerFile, state); err != nil {
		return e.errorf("record reviewer: %v", err)
	}
	first := kickoff(filepath.Join(rundir.RoundDir(dir, 1), "prompt.md"))
	cmd, err := e.envCommand(driver.InteractiveReviewer(w, root, out, files, state.Session, first)...)
	if err != nil {
		return e.errorf("%v", err)
	}
	num, err := rundir.Number(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	id, err := e.startCompanion(ctx, parent, fmt.Sprintf("%s-review #%d", w.Harness, num), cmd, root, dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	started := e.handshake(ctx, reviewerWorker, id)
	if started {
		markStarted(dir, 1)
	}
	reporter{env: e, target: id}.set(ctx, "active", review.ReviewingLine(review.Label{Harness: w.Harness, Num: num, Round: 1}))
	return e.printJSON(applyStart{Companion: &id, Dir: dir, Started: started})
}

// reviewOutput is the output contract of an interactive round: the reviewer
// writes file, a codex reviewer works from out/ and so needs git -C root.
func reviewOutput(harnessName, root, schema, file string) review.Output {
	return review.Output{Schema: schema, File: file, Root: root, GitC: harnessName == config.Codex}
}

// reviewScope collects the scope of a run with state from dir and returns the
// repository root, the fingerprint of the scope (empty for a plan review) and a
// renderer of the first-round prompt for an Output. A plan review (state.Change
// set) reads the change from disk; a diff review diffs against state.Base.
func (e *Env) reviewScope(ctx context.Context, dir string, state reviewState) (string, string, func(review.Output) (string, error), error) {
	if state.Change != "" {
		scope, err := e.CollectPlan(ctx, dir, state.Change)
		if err != nil {
			return "", "", nil, err
		}
		return scope.Root, "", func(o review.Output) (string, error) { return review.PlanPrompt(scope, o) }, nil
	}
	scope, err := e.Collect(ctx, dir, state.Base)
	if err != nil {
		return "", "", nil, err
	}
	return scope.Root, scope.Fingerprint, func(o review.Output) (string, error) { return review.Prompt(scope, o) }, nil
}

// reviewPrompt renders the prompt of round k of a run: the first-round prompt,
// wrapped for a re-review when the reviewer has a live session to continue
// (always, after round 1, in an interactive run; with a recorded session id
// inline). It returns the repository root and the scope fingerprint (empty for
// a plan review) with the prompt.
func (e *Env) reviewPrompt(ctx context.Context, dir string, state reviewState, k int, note string, o review.Output) (root, fingerprint, prompt string, err error) {
	root, fingerprint, render, err := e.reviewScope(ctx, dir, state)
	if err != nil {
		return "", "", "", err
	}
	body, err := render(o)
	if err != nil {
		return "", "", "", fmt.Errorf("render prompt: %v", err)
	}
	resume := state.Session != ""
	if state.Interactive {
		resume = k > 1
	}
	if !resume {
		return root, fingerprint, body, nil // nothing to resume: start over
	}
	prompt, err = review.Rereview(body, k, note, o)
	if err != nil {
		return "", "", "", fmt.Errorf("render prompt: %v", err)
	}
	return root, fingerprint, prompt, nil
}

// scopeFile is the round file holding the fingerprint of the scope its prompt
// was built from; only diff rounds have one.
const scopeFile = "scope.sha256"

// initRound creates rounds/<k>/prompt.md and records k as the current round. A
// non-empty fingerprint (a diff round) goes to rounds/<k>/scope.sha256 first,
// so it exists before any reviewer hears of the round.
func initRound(dir string, k int, prompt, fingerprint string) error {
	rd, err := rundir.MakeRound(dir, k)
	if err != nil {
		return fmt.Errorf("create round dir: %v", err)
	}
	if fingerprint != "" {
		if err := fsx.ReplaceFile(filepath.Join(rd, scopeFile), []byte(fingerprint+"\n"), 0o644); err != nil {
			return fmt.Errorf("write scope fingerprint: %v", err)
		}
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
	// Started is set only inside agterm: false means the line was typed but the
	// reviewer never reported the round as started. Inline output has none.
	Started *bool `json:"started,omitempty"`
}

// defaultReviewSend is how long `review send` waits for the reviewer's turn to
// end, seconds.
const defaultReviewSend = 600

// ReviewSend implements `agk review send <run-dir> [--note FILE] [--timeout SECONDS]`.
func (e *Env) ReviewSend(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk review send", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	notePath := fs.String("note", "", "file with a note for the reviewer (what was fixed)")
	timeout := fs.Int("timeout", defaultReviewSend, "seconds to wait for the reviewer's turn to end")
	pos, code, ok := parseInterleaved(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return e.errorf("usage: agk review send <run-dir> [--note FILE] [--timeout SECONDS]")
	}
	if *timeout <= 0 {
		return e.errorf("--timeout must be a positive number of seconds, got %d", *timeout)
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
	companion, err := readCompanion(run.dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	if companion != "" && !run.state.Interactive {
		return e.errorf("%s was started by an older agk and cannot take another round; start a new review", run.dir)
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
	var o review.Output
	if companion != "" {
		o = reviewOutput(run.state.Worker.Harness, run.root, filepath.Join(run.dir, schemaName), outputPath(run.dir, k+1))
	}
	_, fingerprint, prompt, err := e.reviewPrompt(ctx, run.root, run.state, k+1, note, o)
	if err != nil {
		return e.errorf("%v", err)
	}
	if err := initRound(run.dir, k+1, prompt, fingerprint); err != nil {
		return e.errorf("%v", err)
	}
	if companion == "" {
		code := e.runRound(ctx, run.dir, k+1, e.Stderr)
		if c := e.printJSON(sendResult{Dir: run.dir, Round: k + 1}); c != ExitOK {
			return c
		}
		return code
	}
	line := kickoff(filepath.Join(rundir.RoundDir(run.dir, k+1), "prompt.md"))
	started, err := e.typeWhenIdle(ctx, reviewerWorker, companion, line, fmt.Sprintf("round %d", k+1), time.Duration(*timeout)*time.Second, time.Time{})
	if err != nil {
		// The reviewer never heard of the round: take it back.
		rollbackRound(run.dir, k, k+1, e.Stderr)
		return e.errorf("%v", err)
	}
	if started {
		markStarted(run.dir, k+1)
	}
	reporter{env: e, target: companion}.set(ctx, "active", review.ReviewingLine(run.label(k+1)))
	return e.printJSON(sendResult{Companion: &companion, Dir: run.dir, Round: k + 1, Started: &started})
}

type freshResult struct {
	Fresh bool `json:"fresh"`
	Round int  `json:"round"`
}

// ReviewFresh implements `agk review fresh <run-dir>`: whether the current diff
// scope still matches the fingerprint recorded for the run's current round. It
// reads the repository and the run directory and writes nothing.
func (e *Env) ReviewFresh(ctx context.Context, args []string) int {
	if len(args) != 1 {
		return e.errorf("usage: agk review fresh <run-dir>")
	}
	run, err := loadReviewRun(args[0])
	if err != nil {
		return e.errorf("%v", err)
	}
	if run.state.Change != "" {
		return e.errorf("%s is a plan review of %s: fresh covers diff reviews only", run.dir, run.state.Change)
	}
	k, err := rundir.Round(run.dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	recorded, err := os.ReadFile(filepath.Join(rundir.RoundDir(run.dir, k), scopeFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return e.errorf("round %d of %s has no %s: it was written by an older agk; run a new review round", k, run.dir, scopeFile)
		}
		return e.errorf("%v", err)
	}
	scope, err := e.Collect(ctx, run.root, run.state.Base)
	switch {
	case errors.Is(err, review.ErrNothingToReview):
		return e.printJSON(freshResult{Fresh: false, Round: k})
	case err != nil:
		return e.errorf("%v", err)
	}
	return e.printJSON(freshResult{Fresh: scope.Fingerprint == strings.TrimSpace(string(recorded)), Round: k})
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
