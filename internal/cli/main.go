package cli

import (
	"context"
	"fmt"
)

const usage = `agk - agterm workflow kit

Usage:
  agk review start [--base REF]   Review changes since the merge base of REF (default main)
                                  in a codex-review companion; prints {"companion","dir"}.
                                  Outside agterm the review runs inline (companion null).
  agk review run <run-dir>        Run the review for a prepared run dir (the companion's command).
  agk review wait <run-dir> [--timeout SECONDS]
                                  Wait for the review's outcome (default 1800 s) and print it as
                                  {"status":"completed|blocked|cancelled|running",...}; closes the
                                  companion after a completed review.
  agk triage <review.json>        Triage findings in an agterm overlay; prints decisions JSON.

Exit codes: 0 ok, 1 error, 2 dismissed by the user, 3 no agterm UI (fallback output printed).
`

const reviewUsage = `Usage:
  agk review start [--base REF]   REF defaults to main
  agk review run <run-dir>
  agk review wait <run-dir> [--timeout SECONDS]   SECONDS defaults to 1800
`

// Main dispatches agk subcommands and returns the process exit code.
func (e *Env) Main(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(e.Stderr, usage)
		return ExitError
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(e.Stdout, usage)
		return ExitOK
	case "review":
		return e.reviewCmd(ctx, args[1:])
	case "triage":
		return e.Triage(ctx, args[1:])
	}
	fmt.Fprintf(e.Stderr, "agk: unknown command %q\n\n%s", args[0], usage)
	return ExitError
}

func (e *Env) reviewCmd(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(e.Stderr, reviewUsage)
		return ExitError
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(e.Stdout, reviewUsage)
		return ExitOK
	case "start":
		return e.ReviewStart(ctx, args[1:])
	case "run":
		return e.ReviewRun(ctx, args[1:])
	case "wait":
		return e.ReviewWait(ctx, args[1:])
	}
	fmt.Fprintf(e.Stderr, "agk: unknown review command %q\n\n%s", args[0], reviewUsage)
	return ExitError
}
