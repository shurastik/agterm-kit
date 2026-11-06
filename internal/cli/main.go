package cli

import (
	"context"
	"fmt"
)

const usage = `agk - agterm workflow kit

Usage:
  agk review start [--base REF] [--reviewer H[:M]]
                                  Review changes since the merge base of REF (default main) in a
                                  <harness>-review companion; prints {"companion","dir"}.
                                  Outside agterm the review runs inline (companion null).
  agk review run <run-dir>        The reviewer loop inside the companion (runs rounds on request).
  agk review wait <run-dir> [--timeout SECONDS]
                                  Wait for the current round's outcome (default 1800 s) and print it
                                  as {"status":"completed|blocked|cancelled|running","round":K,...}.
  agk review send <run-dir> [--note FILE]
                                  Start the next round: re-review the current diff in the same
                                  reviewer session; prints {"companion","dir","round"}.
  agk review close <run-dir>      Close the reviewer companion (undo restores it).
  agk triage <review.json>        Triage findings in an agterm overlay; prints decisions JSON.
  agk config [--applier H[:M]] [--reviewer H[:M]]
                                  Print the resolved worker configuration (harness, model, effort,
                                  extra args, permission mode) and where each value comes from:
                                  flag, .agk.toml in the repo root, ~/.config/agk/config.toml
                                  ($XDG_CONFIG_HOME/agk/config.toml), or default.
  agk setup [--skills-dir DIR] [--dry-run]
                                  Link the agk-* skills for Claude Code and Codex, add .agk/ to the
                                  global git ignore, check dependencies. Rerun after every upgrade.
  agk version                     Print the agk version.

Exit codes: 0 ok, 1 error, 2 dismissed by the user, 3 no agterm UI (fallback output printed).
`

const reviewUsage = `Usage:
  agk review start [--base REF] [--reviewer H[:M]]   REF defaults to main
  agk review run <run-dir>
  agk review wait <run-dir> [--timeout SECONDS]   SECONDS defaults to 1800
  agk review send <run-dir> [--note FILE]
  agk review close <run-dir>
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
	case "config":
		return e.Config(ctx, args[1:])
	case "setup":
		return e.Setup(args[1:])
	case "version":
		return e.Version(args[1:])
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
	case "send":
		return e.ReviewSend(ctx, args[1:])
	case "close":
		return e.ReviewClose(ctx, args[1:])
	}
	fmt.Fprintf(e.Stderr, "agk: unknown review command %q\n\n%s", args[0], reviewUsage)
	return ExitError
}
