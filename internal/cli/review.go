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

	"github.com/shurastik/agterm-kit/internal/fsx"
	"github.com/shurastik/agterm-kit/internal/review"
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

func (r reporter) set(ctx context.Context, state, line string) {
	if r.target == "" {
		return
	}
	attempts := 1
	if state == "completed" || state == "blocked" {
		attempts = statusAttempts
	}
	for i := 1; ; i++ {
		err := r.update(ctx, state, line)
		if err == nil {
			return
		}
		// Status is best effort: a broken socket must not abort the review.
		fmt.Fprintf(r.env.Stderr, "agk: warning: %v\n", err)
		if i >= attempts {
			return
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

// runResult is <run dir>/result.json, the outcome on disk for callers whose
// `session wait` timed out.
type runResult struct {
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	Findings int    `json:"findings"`
	Verdict  string `json:"verdict,omitempty"`
}

func writeResult(dir string, r runResult) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return fsx.ReplaceFile(filepath.Join(dir, "result.json"), append(b, '\n'), 0o644)
}

// ReviewRun implements `agk review run <dir>`: the process inside the companion.
func (e *Env) ReviewRun(ctx context.Context, args []string) int {
	rep := reporter{env: e, target: e.session()}
	if len(args) != 1 {
		rep.set(ctx, "blocked", review.FailureLine(0, review.ReasonSetupFailed))
		return e.errorf("usage: agk review run <run-dir>")
	}
	return e.runReview(ctx, args[0], e.Stdout, rep)
}

// runReview runs codex for a prepared run dir and records the outcome.
func (e *Env) runReview(ctx context.Context, dir string, term io.Writer, rep reporter) int {
	num := 0 // review number; 0 until known
	recordable := false
	fail := func(reason string, err error) int {
		if recordable {
			if werr := writeResult(dir, runResult{Status: "blocked", Reason: reason}); werr != nil {
				fmt.Fprintf(e.Stderr, "agk: warning: %v\n", werr)
			}
		}
		rep.set(ctx, "blocked", review.FailureLine(num, reason))
		return e.errorf("%s: %v", reason, err)
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	root, err := runRoot(dir)
	if err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	if num, err = review.RunNumber(dir); err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	recordable = true // dir is a verified run dir from here on
	prompt, err := os.ReadFile(filepath.Join(dir, "prompt.md"))
	if err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	stamped := review.NewStampWriter(term, e.Now)
	fmt.Fprintf(stamped, "agk: review #%d started, run dir %s\n", num, dir)
	rep.set(ctx, "active", review.ReviewingLine(num))
	if err := e.Runner.Run(ctx, root, dir, string(prompt), term); err != nil {
		return fail(review.ReasonCodexFailed, err)
	}
	raw, err := os.ReadFile(e.Runner.OutputPath(dir))
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
	if err := fsx.ReplaceFile(filepath.Join(dir, "review.json"), append(b, '\n'), 0o644); err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	if err := writeResult(dir, runResult{Status: "completed", Findings: len(r.Findings), Verdict: r.Verdict}); err != nil {
		return fail(review.ReasonSetupFailed, err)
	}
	rep.set(ctx, "completed", review.SummaryLine(num, r))
	fmt.Fprintf(review.NewStampWriter(term, e.Now), "agk: review #%d: %d finding(s), verdict %s -> %s\n",
		num, len(r.Findings), r.Verdict, filepath.Join(dir, "review.json"))
	return ExitOK
}

// runRoot maps <root>/.agk/reviews/<ts> to <root>.
func runRoot(dir string) (string, error) {
	reviews := filepath.Dir(dir)
	agk := filepath.Dir(reviews)
	if filepath.Base(reviews) != "reviews" || filepath.Base(agk) != ".agk" {
		return "", errors.New(dir + " is not a .agk/reviews/<run> directory")
	}
	return filepath.Dir(agk), nil
}

// forwardedEnv lists the caller variables the companion command carries.
var forwardedEnv = []string{"PATH", "CODEX_HOME", "AGTERMCTL"}

// CompanionName is the sidebar name of the review companion, suffixed " #N".
const CompanionName = "codex-review"

type startResult struct {
	Companion *string `json:"companion"`
	Dir       string  `json:"dir"`
}

// ReviewStart implements `agk review start [--base REF]`.
func (e *Env) ReviewStart(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("agk review start", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	base := fs.String("base", "main", "review changes since the merge base of this ref and HEAD")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitError
	}
	if fs.NArg() != 0 {
		return e.errorf("unexpected arguments %q", fs.Args())
	}
	scope, err := e.Collect(ctx, e.Cwd, *base)
	if err != nil {
		return e.errorf("%v", err)
	}
	prompt, err := review.Prompt(scope)
	if err != nil {
		return e.errorf("render prompt: %v", err)
	}
	parent := e.session()
	var cmdExe string
	if parent != "" {
		// Refuse unquotable paths before leaving a run dir behind. The run dir
		// lives under the root, so checking the root covers it.
		exe, err := e.Executable()
		if err != nil {
			return e.errorf("locate agk binary: %v", err)
		}
		if cmdExe, err = shellQuote(exe); err != nil {
			return e.errorf("%v", err)
		}
		// Forward the caller's PATH (so codex and agtermctl resolve even when
		// agterm runs the command without a login shell), CODEX_HOME (codex
		// config and auth) and AGTERMCTL (the same agtermctl as the caller). Never secrets: the command line is visible in ps.
		var envArgs []string
		for _, k := range forwardedEnv {
			if v := e.Getenv(k); v != "" {
				q, err := shellQuote(k + "=" + v)
				if err != nil {
					return e.errorf("%v", err)
				}
				envArgs = append(envArgs, q)
			}
		}
		if len(envArgs) > 0 {
			cmdExe = "env " + strings.Join(envArgs, " ") + " " + cmdExe
		}
		if _, err := shellQuote(scope.Root); err != nil {
			return e.errorf("%v", err)
		}
	}
	dir, err := review.NewRunDir(scope.Root, e.Now())
	if err != nil {
		return e.errorf("create run dir: %v", err)
	}
	if err := fsx.ReplaceFile(filepath.Join(dir, "prompt.md"), []byte(prompt), 0o644); err != nil {
		return e.errorf("write prompt: %v", err)
	}
	if parent == "" {
		// Outside agterm: review in the foreground. Codex output goes to stderr
		// so stdout carries only the result object.
		code := e.runReview(ctx, dir, e.Stderr, reporter{env: e})
		if c := e.printJSON(startResult{Dir: dir}); c != ExitOK {
			return c
		}
		return code
	}
	cmdDir, err := shellQuote(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	num, err := review.RunNumber(dir)
	if err != nil {
		return e.errorf("%v", err)
	}
	name := fmt.Sprintf("%s #%d", CompanionName, num)
	id, err := e.Ctl.NewCompanion(ctx, parent, name, cmdExe+" review run "+cmdDir, scope.Root)
	if err != nil {
		return e.errorf("start companion: %v", err)
	}
	return e.printJSON(startResult{Companion: &id, Dir: dir})
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
