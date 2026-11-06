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
	"github.com/shurastik/agterm-kit/internal/termsafe"
)

// Run-dir files of an apply run.
const (
	settingsFile = "claude-settings.json"
	rulesFile    = "applier-rules.md"
	applierLog   = "applier.log"
	startedFile  = "started" // in rounds/<k>/: the round was seen active
)

// Timeouts of the applier commands.
const (
	defaultApplyWait = 3600             // apply wait, seconds
	defaultApplySend = 600              // apply send waiting for the turn to end, seconds
	startHandshake   = 60 * time.Second // companion must report a status by then
	handshakeLines   = 20               // screen lines shown when it does not
)

// applyState is worker.json of an apply run.
type applyState struct {
	Worker  config.Worker `json:"worker"`
	Change  string        `json:"change"`
	Session string        `json:"session,omitempty"`
}

var changeName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type applyStart struct {
	Companion *string `json:"companion"`
	Dir       string  `json:"dir"`
	Started   bool    `json:"started"`
}

const applyUsage = `Usage:
  agk apply start <change> [--applier H[:M]]
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
	}
	fmt.Fprintf(e.Stderr, "agk: unknown apply command %q\n\n%s", args[0], applyUsage)
	return ExitError
}

// kickoff is the one line that starts a round: the prompt itself lives in a
// file, because typed input and the companion command line are single lines.
func kickoff(prompt string) string { return "Read " + prompt + " and follow it." }

// applierFiles writes the claude applier's settings and rules into dir; a
// codex applier gets its rules inside each prompt instead.
func applierFiles(dir string, w config.Worker) (harness.ApplierFiles, error) {
	if w.Harness != config.Claude {
		return harness.ApplierFiles{}, nil
	}
	f := harness.ApplierFiles{Settings: filepath.Join(dir, settingsFile), Rules: filepath.Join(dir, rulesFile)}
	if err := fsx.ReplaceFile(f.Settings, apply.ClaudeSettings(), 0o644); err != nil {
		return f, err
	}
	return f, fsx.ReplaceFile(f.Rules, []byte(apply.Rules()), 0o644)
}

// inlineRules reports whether the harness takes the rules in the prompt.
func inlineRules(w config.Worker) bool { return w.Harness != config.Claude }

// ApplyStart implements `agk apply start <change> [--applier H[:M]]`.
func (e *Env) ApplyStart(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk apply start", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	applier := fs.String("applier", "", "applier HARNESS[:MODEL] for this run")
	pos, code, ok := parseInterleaved(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return e.errorf("usage: agk apply start <change> [--applier H[:M]]")
	}
	change := pos[0]
	if !changeName.MatchString(change) {
		return e.errorf("change name %q is not kebab-case", change)
	}
	root, err := e.RepoRoot(ctx, e.Cwd)
	if err != nil {
		return e.errorf("not in a git repository: %v", err)
	}
	state, msg, err := e.ApplyState(ctx, root, change)
	switch {
	case err != nil:
		return e.errorf("%v", err)
	case state == "all_done":
		return e.errorf("nothing to apply: every task of %s is done", change)
	case state != "ready":
		return e.errorf("change %s is not ready to apply (%s): %s", change, state, msg)
	}
	cfg, err := e.resolveConfig(root, config.Flags{Applier: *applier})
	if err != nil {
		return e.errorf("%v", err)
	}
	e.warnSame(cfg)
	w := cfg.Applier
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
	if w.Harness == config.Claude {
		st.Session = e.NewSessionID()
	}
	files, err := applierFiles(dir, w)
	if err != nil {
		return e.errorf("write applier files: %v", err)
	}
	report := filepath.Join(rundir.RoundDir(dir, 1), apply.ReportFile)
	prompt, err := apply.FirstPrompt(change, report, inlineRules(w))
	if err != nil {
		return e.errorf("render prompt: %v", err)
	}
	if err := initRound(dir, 1, prompt); err != nil {
		return e.errorf("%v", err)
	}
	if err := rundir.WriteJSON(dir, workerFile, st); err != nil {
		return e.errorf("record applier: %v", err)
	}
	first := kickoff(filepath.Join(rundir.RoundDir(dir, 1), "prompt.md"))
	if parent == "" {
		code := e.headlessRound(ctx, driver, dir, root, 1, st, files, first)
		if c := e.printJSON(applyStart{Dir: dir, Started: true}); c != ExitOK {
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
	id, err := e.startCompanion(ctx, parent, fmt.Sprintf("%s-apply #%d", w.Harness, num), cmd, root, dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	started := e.handshake(ctx, id)
	if started {
		markStarted(dir, 1)
	}
	return e.printJSON(applyStart{Companion: &id, Dir: dir, Started: started})
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

// handshake waits for the new applier to report any agent status. When none
// comes (a trust prompt, a login screen, a bad flag), it shows the last
// screen lines and leaves the companion open for the user.
func (e *Env) handshake(ctx context.Context, id string) bool {
	_, err := e.Ctl.Wait(ctx, id, []string{"active", "completed", "blocked"}, startHandshake)
	if err == nil {
		return true
	}
	fmt.Fprintf(e.Stderr, "agk: warning: applier %s did not start (%v); it is waiting in its companion:\n", id, err)
	if !errors.Is(err, agterm.ErrGone) {
		if screen, terr := e.Ctl.Text(ctx, id, handshakeLines); terr == nil {
			w := termsafe.NewWriter(e.Stderr)
			io.WriteString(w, strings.TrimRight(screen, "\n")+"\n")
		}
	}
	return false
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

// readReport returns round k's report bytes, or nil when there is none yet.
func readReport(dir string, k int) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(rundir.RoundDir(dir, k), apply.ReportFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// pollSlice is how long one status wait lasts before the state on disk is
// checked again: files decide even when a harness's agent-status hooks never
// fire.
var pollSlice = 5 * time.Second

// waitPolling waits up to left for one of until, in pollSlice slices,
// returning early with found when done reports true between slices. It
// returns the unused budget.
func (e *Env) waitPolling(ctx context.Context, companion string, until []string, left time.Duration, done func() bool) (status string, err error, found bool, rest time.Duration) {
	for {
		slice := min(pollSlice, left)
		status, err = e.Ctl.Wait(ctx, companion, until, slice)
		if !errors.Is(err, agterm.ErrTimeout) {
			return status, err, false, left
		}
		left -= slice
		if done() {
			return "", nil, true, left
		}
		if left <= 0 {
			return "", err, false, 0
		}
	}
}

// waitSliced is waitPolling until round k's report appears.
func (e *Env) waitSliced(ctx context.Context, companion string, until []string, left time.Duration, dir string, k int) (string, error, bool, time.Duration) {
	return e.waitPolling(ctx, companion, until, left, func() bool {
		r, _ := readReport(dir, k)
		return r != nil
	})
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
		return e.printJSON(apply.Decide(run.round, report, "completed", nil))
	}
	left := time.Duration(timeout) * time.Second
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
		status, werr, found, left = e.waitSliced(ctx, companion, until, left, run.dir, run.round)
		if status == "active" {
			markStarted(run.dir, run.round)
			status = ""
		}
	}
	if !found && werr == nil && status == "" {
		status, werr, _, _ = e.waitSliced(ctx, companion, []string{"completed", "blocked", "idle"}, left, run.dir, run.round)
	}
	if werr != nil && !errors.Is(werr, agterm.ErrGone) && !errors.Is(werr, agterm.ErrTimeout) {
		return e.errorf("wait for applier %s: %v", companion, werr)
	}
	// The hooks may report the end of the turn before the file is flushed.
	if report, err = readReport(run.dir, run.round); err != nil {
		return e.errorf("%v", err)
	}
	return e.printJSON(apply.Decide(run.round, report, status, werr))
}

type sendRound struct {
	Round int `json:"round"`
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
	if companion != "" {
		status, err := e.Ctl.Wait(ctx, companion, []string{"completed", "blocked", "idle"}, time.Duration(*timeout)*time.Second)
		switch {
		case errors.Is(err, agterm.ErrGone):
			return e.errorf("the applier %s is gone; start a new apply run", companion)
		case errors.Is(err, agterm.ErrTimeout):
			return e.errorf("the applier %s is busy: its turn did not end within %d s", companion, *timeout)
		case err != nil:
			return e.errorf("wait for applier %s: %v", companion, err)
		case status == "blocked":
			// A permission prompt is open: typed text would answer it.
			return e.errorf("the applier %s waits for the user (permission prompt); answer it in the companion first", companion)
		}
	}
	k := run.round + 1
	report := filepath.Join(rundir.RoundDir(run.dir, k), apply.ReportFile)
	prompt, err := apply.FixPrompt(k, string(msg), report, inlineRules(run.state.Worker))
	if err != nil {
		return e.errorf("render prompt: %v", err)
	}
	if err := initRound(run.dir, k, prompt); err != nil {
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
	if err := e.Ctl.Type(ctx, companion, line+"\r"); err != nil {
		if rerr := rundir.SetRound(run.dir, run.round); rerr != nil {
			fmt.Fprintf(e.Stderr, "agk: warning: %v\n", rerr)
		}
		os.RemoveAll(rundir.RoundDir(run.dir, k))
		if errors.Is(err, agterm.ErrGone) {
			return e.errorf("the applier %s is gone; start a new apply run", companion)
		}
		return e.errorf("type into applier %s: %v", companion, err)
	}
	if _, err := e.Ctl.Wait(ctx, companion, []string{"active"}, sendHandshake); err != nil {
		fmt.Fprintf(e.Stderr, "agk: warning: applier %s did not report round %d as started: %v\n", companion, k, err)
	} else {
		markStarted(run.dir, k)
	}
	return e.printJSON(sendRound{Round: k})
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
