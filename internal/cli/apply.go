package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shurastik/agterm-kit/internal/agterm"
	"github.com/shurastik/agterm-kit/internal/apply"
	"github.com/shurastik/agterm-kit/internal/config"
	"github.com/shurastik/agterm-kit/internal/fsx"
	"github.com/shurastik/agterm-kit/internal/harness"
	"github.com/shurastik/agterm-kit/internal/rundir"
)

// Run-dir files of an apply run.
const (
	settingsFile = "claude-settings.json"
	rulesFile    = "applier-rules.md"
	applierLog   = "applier.log"
)

// Timeouts of the applier commands.
const (
	defaultApplyWait = 3600 // apply wait, seconds
	defaultApplySend = 600  // apply send waiting for the turn to end, seconds
)

// applyState is worker.json of an apply run.
type applyState struct {
	Worker  config.Worker `json:"worker"`
	Change  string        `json:"change"`
	Session string        `json:"session,omitempty"`
	// TurnHooks is set when the run's settings carry the turn-state hooks:
	// only then does `agk apply wait` read turn.json. Codex runs and runs of
	// an older agk decide from the agent status alone.
	TurnHooks bool `json:"turn_hooks,omitempty"`
}

var changeName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// workerStart is what a worker's start prints inside agterm.
type workerStart struct {
	Companion *string `json:"companion"`
	Dir       string  `json:"dir"`
	Started   bool    `json:"started"`
}

// applyStart is the stdout of `agk apply start`: the worker's start plus the
// worktree it runs in.
type applyStart struct {
	workerStart
	// Worktree is the change's worktree the applier runs in, or null.
	Worktree *string `json:"worktree"`
	// Pulled says whether a `git pull --ff-only` of the base branch ran.
	Pulled bool `json:"pulled"`
}

const applyUsage = `Usage:
  agk apply start <change> [--applier H[:M]] [--worktree | --no-worktree] [--no-pull]
  agk apply wait <run-dir> [--timeout SECONDS]   SECONDS defaults to 3600
  agk apply send <run-dir> (<file> | --stdin) [--timeout SECONDS]   SECONDS defaults to 600
  agk apply close <run-dir>
`

func (e *Env) applyCmd(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(e.Stderr, applyUsage)
		return ExitError
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(e.Stdout, applyUsage)
		return ExitOK
	case "start":
		return e.ApplyStart(ctx, args[1:])
	case "wait":
		return e.ApplyWait(ctx, args[1:])
	case "send":
		return e.ApplySend(ctx, args[1:])
	case "close":
		return e.ApplyClose(ctx, args[1:])
	case "hook":
		return e.ApplyHook(ctx, args[1:])
	}
	fmt.Fprintf(e.Stderr, "agk: unknown apply command %q\n\n%s", args[0], applyUsage)
	return ExitError
}

// kickoff is the one line that starts a round: the prompt itself lives in a
// file, because typed input and the companion command line are single lines.
func kickoff(prompt string) string { return "Read " + prompt + " and follow it." }

// applierFiles writes the claude applier's settings (deny rules and the
// turn-state hooks) and rules into dir; a codex applier gets its rules inside
// each prompt instead.
func (e *Env) applierFiles(dir string, w config.Worker) (harness.ApplierFiles, error) {
	if w.Harness != config.Claude {
		return harness.ApplierFiles{}, nil
	}
	exe, err := e.Executable()
	if err != nil {
		return harness.ApplierFiles{}, fmt.Errorf("locate agk binary: %v", err)
	}
	settings, err := applierSettings(exe, dir)
	if err != nil {
		return harness.ApplierFiles{}, err
	}
	f := harness.ApplierFiles{Settings: filepath.Join(dir, settingsFile), Rules: filepath.Join(dir, rulesFile)}
	if err := fsx.ReplaceFile(f.Settings, settings, 0o644); err != nil {
		return f, err
	}
	return f, fsx.ReplaceFile(f.Rules, []byte(apply.Rules()), 0o644)
}

// inlineRules reports whether the harness takes the rules in the prompt.
func inlineRules(w config.Worker) bool { return w.Harness != config.Claude }

// ApplyStart implements `agk apply start <change> [--applier H[:M]]
// [--worktree | --no-worktree] [--no-pull]`.
func (e *Env) ApplyStart(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk apply start", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	applier := fs.String("applier", "", "applier HARNESS[:MODEL] for this run")
	worktreeOn := fs.Bool("worktree", false, "implement in a git worktree of its own (overrides the config)")
	worktreeOff := fs.Bool("no-worktree", false, "implement in the current checkout (overrides the config)")
	noPull := fs.Bool("no-pull", false, "worktree mode: do not pull the base branch first")
	pos, code, ok := parseInterleaved(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return e.errorf("usage: agk apply start <change> [--applier H[:M]] [--worktree | --no-worktree] [--no-pull]")
	}
	change := pos[0]
	if !changeName.MatchString(change) {
		return e.errorf("change name %q is not kebab-case", change)
	}
	if *worktreeOn && *worktreeOff {
		return e.errorf("--worktree and --no-worktree cannot be used together")
	}
	root, err := e.RepoRoot(ctx, e.Cwd)
	if err != nil {
		return e.errorf("not in a git repository: %v", err)
	}
	flags := config.Flags{Applier: *applier}
	switch {
	case *worktreeOn:
		on := true
		flags.Worktree = &on
	case *worktreeOff:
		off := false
		flags.Worktree = &off
	}
	cfg, err := e.resolveConfig(root, flags)
	if err != nil {
		return e.errorf("%v", err)
	}
	e.warnSame(cfg)
	w := cfg.Applier
	var wt *string
	pulled := false
	if w.Worktree {
		var code int
		if root, wt, pulled, code, ok = e.worktreeRoot(ctx, change, *noPull); !ok {
			return code
		}
	} else if code, ok := e.checkReady(ctx, root, change); !ok {
		return code
	}
	driver, err := e.Driver(w.Harness)
	if err != nil {
		return e.errorf("%v", err)
	}
	parent := e.session()
	if parent != "" {
		if _, err := e.envCommand(root); err != nil {
			return e.errorf("%v", err)
		}
	}
	dir, err := rundir.New(root, rundir.Applies, e.Now())
	if err != nil {
		return e.errorf("create run dir: %v", err)
	}
	st := applyState{Worker: w, Change: change}
	files, err := e.applierFiles(dir, w)
	if err != nil {
		return e.errorf("write applier files: %v", err)
	}
	if w.Harness == config.Claude {
		st.Session = e.NewSessionID()
		st.TurnHooks = true
	}
	report := filepath.Join(rundir.RoundDir(dir, 1), apply.ReportFile)
	prompt, err := apply.FirstPrompt(change, report, inlineRules(w))
	if err != nil {
		return e.errorf("render prompt: %v", err)
	}
	if err := initRound(dir, 1, prompt, "", ""); err != nil {
		return e.errorf("%v", err)
	}
	if err := rundir.WriteJSON(dir, workerFile, st); err != nil {
		return e.errorf("record applier: %v", err)
	}
	first := kickoff(filepath.Join(rundir.RoundDir(dir, 1), "prompt.md"))
	if parent == "" {
		code := e.headlessRound(ctx, driver, dir, root, 1, st, files, first)
		if c := e.printJSON(applyStart{workerStart{Dir: dir, Started: true}, wt, pulled}); c != ExitOK {
			return c
		}
		return code
	}
	cmd, err := e.envCommand(driver.Interactive(w, root, files, st.Session, first)...)
	if err != nil {
		return e.errorf("%v", err)
	}
	num, err := rundir.Number(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	id, err := e.startCompanion(ctx, parent, companionName(change, w.Harness, "apply", num), cmd, root, dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	started := e.handshake(ctx, applierWorker, id)
	if started {
		markStarted(dir, 1)
	}
	return e.printJSON(applyStart{workerStart{Companion: &id, Dir: dir, Started: started}, wt, pulled})
}

// headlessRound runs round k of an apply run in the foreground (outside
// agterm), streaming to stderr, and records the harness session.
func (e *Env) headlessRound(ctx context.Context, d harness.Driver, dir, root string, k int, st applyState, files harness.ApplierFiles, line string) int {
	turn := harness.Turn{Worker: st.Worker, Root: root, Dir: rundir.RoundDir(dir, k), Log: applierLog,
		Prompt: line, Session: st.Session, Resume: k > 1 && st.Session != ""}
	session, err := d.Headless(ctx, turn, files, e.Stderr)
	if session != st.Session { // "" when a first round never got a session
		if session == "" && k > 1 {
			session = st.Session // a failed resume keeps the session
		}
		st.Session = session
		if werr := rundir.WriteJSON(dir, workerFile, st); werr != nil {
			fmt.Fprintf(e.Stderr, "agk: warning: record applier session: %v\n", werr)
		}
	}
	if err != nil {
		return e.errorf("applier round %d: %v", k, err)
	}
	return ExitOK
}

// applyRun is a verified apply run directory.
type applyRun struct {
	dir, root string
	state     applyState
	round     int
}

func loadApplyRun(dir string) (applyRun, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return applyRun{}, err
	}
	root, err := rundir.Root(dir, rundir.Applies)
	if err != nil {
		return applyRun{}, err
	}
	r := applyRun{dir: dir, root: root}
	have, err := rundir.ReadJSON(dir, workerFile, &r.state)
	if err != nil {
		return applyRun{}, err
	}
	if !have || r.state.Worker.Harness == "" {
		return applyRun{}, fmt.Errorf("%s has no %s: not an apply run directory", dir, workerFile)
	}
	if r.round, err = rundir.Round(dir); err != nil {
		return applyRun{}, err
	}
	return r, nil
}

// reportPath is where the applier writes round k's report.
func reportPath(dir string, k int) string {
	return filepath.Join(rundir.RoundDir(dir, k), apply.ReportFile)
}

// readReport returns round k's report bytes, or nil when there is none yet.
func readReport(dir string, k int) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(rundir.RoundDir(dir, k), apply.ReportFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// ApplyWait implements `agk apply wait <run-dir> [--timeout SECONDS]`.
func (e *Env) ApplyWait(ctx context.Context, args []string) int {
	dir, timeout, code, ok := e.parseWaitArgs("agk apply wait", rundir.Applies, defaultApplyWait, args)
	if !ok {
		return code
	}
	run, err := loadApplyRun(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	companion, err := readCompanion(run.dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	report, err := readReport(run.dir, run.round)
	if err != nil {
		return e.errorf("%v", err)
	}
	if report != nil || companion == "" {
		// Inline runs finished before start/send returned.
		return e.printJSON(apply.Decide(run.round, report, "completed", nil, nil))
	}
	left := time.Duration(timeout) * time.Second
	if run.state.TurnHooks {
		return e.waitHooked(ctx, run, companion, left)
	}
	var status string
	var werr error
	found := false
	if !roundStarted(run.dir, run.round) {
		// Unconfirmed start (a trust prompt, a slow login): a fresh session's
		// idle would read as "turn over". Wait for the round to begin; a first
		// round may also already have ended (completed) or be asking (blocked).
		until := []string{"active", "blocked"}
		if run.round == 1 {
			until = append(until, "completed")
		}
		status, werr, found, left = e.waitSliced(ctx, companion, until, left, reportPath(run.dir, run.round))
		if status == "active" {
			markStarted(run.dir, run.round)
			status = ""
		}
	}
	if !found && werr == nil && status == "" {
		status, werr, _, _ = e.waitSliced(ctx, companion, []string{"completed", "blocked", "idle"}, left, reportPath(run.dir, run.round))
	}
	if werr != nil && !errors.Is(werr, agterm.ErrGone) && !errors.Is(werr, agterm.ErrTimeout) {
		return e.errorf("wait for applier %s: %v", companion, werr)
	}
	// The hooks may report the end of the turn before the file is flushed.
	if report, err = readReport(run.dir, run.round); err != nil {
		return e.errorf("%v", err)
	}
	return e.printJSON(apply.Decide(run.round, report, status, werr, nil))
}

// sendRound is the stdout of `apply send`. Started is set only inside agterm,
// where false means the line was typed but the applier never reported the round
// as started (most likely it sits in the input box); inline output has no
// such field.
type sendRound struct {
	Round   int   `json:"round"`
	Started *bool `json:"started,omitempty"`
}

// ApplySend implements `agk apply send <run-dir> (<file> | --stdin) [--timeout SECONDS]`.
func (e *Env) ApplySend(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk apply send", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	stdin := fs.Bool("stdin", false, "read the message from stdin")
	timeout := fs.Int("timeout", defaultApplySend, "seconds to wait for the applier's turn to end")
	pos, code, ok := parseInterleaved(fs, args)
	if !ok {
		return code
	}
	if (*stdin && len(pos) != 1) || (!*stdin && len(pos) != 2) {
		return e.errorf("usage: agk apply send <run-dir> (<file> | --stdin) [--timeout SECONDS]")
	}
	if *timeout <= 0 {
		return e.errorf("--timeout must be a positive number of seconds, got %d", *timeout)
	}
	run, err := loadApplyRun(pos[0])
	if err != nil {
		return e.errorf("%v", err)
	}
	var msg []byte
	if *stdin {
		msg, err = io.ReadAll(e.Stdin)
	} else {
		msg, err = os.ReadFile(pos[1])
	}
	if err != nil {
		return e.errorf("read message: %v", err)
	}
	if strings.TrimSpace(string(msg)) == "" {
		return e.errorf("the message is empty")
	}
	companion, err := readCompanion(run.dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	k := run.round + 1
	report := filepath.Join(rundir.RoundDir(run.dir, k), apply.ReportFile)
	prompt, err := apply.FixPrompt(k, string(msg), report, inlineRules(run.state.Worker))
	if err != nil {
		return e.errorf("render prompt: %v", err)
	}
	if err := initRound(run.dir, k, prompt, "", ""); err != nil {
		return e.errorf("%v", err)
	}
	line := kickoff(filepath.Join(rundir.RoundDir(run.dir, k), "prompt.md"))
	if companion == "" {
		driver, err := e.Driver(run.state.Worker.Harness)
		if err != nil {
			return e.errorf("%v", err)
		}
		files := harness.ApplierFiles{}
		if run.state.Worker.Harness == config.Claude {
			files = harness.ApplierFiles{Settings: filepath.Join(run.dir, settingsFile), Rules: filepath.Join(run.dir, rulesFile)}
		}
		code := e.headlessRound(ctx, driver, run.dir, run.root, k, run.state, files, line)
		if c := e.printJSON(sendRound{Round: k}); c != ExitOK {
			return c
		}
		return code
	}
	started, err := e.typeWhenIdle(ctx, applierWorker, companion, line, fmt.Sprintf("round %d", k), time.Duration(*timeout)*time.Second, time.Time{})
	if err != nil {
		rollbackRound(run.dir, run.round, k, e.Stderr)
		return e.errorf("%v", err)
	}
	if started {
		markStarted(run.dir, k)
	}
	return e.printJSON(sendRound{Round: k, Started: &started})
}

// ApplyClose implements `agk apply close <run-dir>`.
func (e *Env) ApplyClose(ctx context.Context, args []string) int {
	if len(args) != 1 {
		return e.errorf("usage: agk apply close <run-dir>")
	}
	run, err := loadApplyRun(args[0])
	if err != nil {
		return e.errorf("%v", err)
	}
	return e.closeCompanion(ctx, run.dir)
}
