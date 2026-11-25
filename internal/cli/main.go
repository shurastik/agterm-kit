package cli

import (
	"context"
	"fmt"
)

const usage = `agk - agterm workflow kit

Usage:
  agk review start [--base REF [--for NAME] | --change NAME] [--reviewer H[:M]] [--autopilot]
                                  Review changes since the merge base of REF (default main) in a
                                  <harness>-review companion, an interactive reviewer session you can
                                  talk to; prints {"companion","dir","started"}.
                                  --for names the OpenSpec change NAME the diff implements: the
                                  companion becomes "NAME · <harness>-review r<k>" (k is the round; send
                                  renames it); the review is the same.
                                  --change reviews the planning artifacts of OpenSpec change NAME
                                  (and the main specs its deltas touch) instead of a diff.
                                  Outside agterm the review runs inline (companion null).
  agk review wait <run-dir> [--timeout SECONDS]
                                  Wait for the current round's outcome (default 1800 s) and print it
                                  as {"status":"completed|blocked|needs-human|cancelled|running","round":K,...}.
                                  needs-human has a reason: no-output (the reviewer ended its turn
                                  without writing its review) or permission (it asks for an answer).
  agk review send <run-dir> [--note FILE] [--timeout SECONDS]
                                  Start the next round once the reviewer's turn is over (default
                                  600 s): re-review the current diff (or the change's artifacts) in
                                  the same reviewer session; prints {"companion","dir","round"}.
  agk review fresh <run-dir>      Read-only: whether the current diff scope still matches the
                                  current round's fingerprint; prints {"fresh":true|false,"round":K}.
                                  A diff review only; an empty scope is not fresh.
  agk review close <run-dir>      Close the reviewer companion (undo restores it).
  agk apply start <change> [--applier H[:M]] [--worktree | --no-worktree] [--no-pull] [--autopilot]
                                  Start the applier for an OpenSpec change in a <harness>-apply
                                  companion (interactive, autonomous); prints
                                  {"companion","dir","started","worktree","pulled"}. Outside agterm
                                  it runs headless. With [applier] worktree = true (or --worktree)
                                  it first pulls the base branch (--ff-only; --no-pull skips it),
                                  commits the change's artifacts there and runs the applier in the
                                  new worktree .worktrees/<change> on branch <change>; "worktree"
                                  is its path (null otherwise), "pulled" whether a pull ran.
  agk apply wait <run-dir> [--timeout SECONDS]
                                  Wait for the current round (default 3600 s); prints
                                  {"status":"done|blocked|needs-human|cancelled|running","round":K,...}.
                                  running with "reason":"background" and a "background" count: a claude
                                  applier ended its turn with background work in flight and will resume
                                  by itself; wait again.
  agk apply send <run-dir> (<file> | --stdin) [--timeout SECONDS]
                                  Start the next round with findings or notes once the applier's
                                  turn is over (default 600 s); prints {"round":K}.
  agk apply close <run-dir>       Close the applier companion (undo restores it).
  agk autopilot on|off            Mark the calling agterm session as run by the autopilot: "on" sets
                                  its info line (🤖 autopilot, hides itself after 20 minutes) and
                                  "off" clears it; prints {"session":"<id>"|null,"marked":<bool>}.
                                  The --autopilot flag of review start and apply start puts the same
                                  mark on a companion (🤖 name prefix, purple icon) and lets the
                                  waits of that run refresh the line. A no-op outside agterm.
  agk sync <change> [--no-pull]   In worktree mode: pull the base branch (--ff-only, skipped with
                                  --no-pull or without an upstream) and rebase the change's branch
                                  onto it, in .worktrees/<change>; needs a clean worktree. Prints
                                  {"change","base","pulled","rebased","head"}. A conflict aborts the
                                  rebase, leaves the branch as it was and exits 1.
  agk finish <change>             Merge a finished worktree change and clean up: needs a clean
                                  worktree, the change archived on its branch and the branch synced
                                  onto the base. Fast-forwards the base in the main checkout (no
                                  pull, no rebase), closes the companions of the worktree's runs,
                                  removes the worktree, deletes the branch; prints
                                  {"change","base","head","closed"}. A rerun finishes what is left.
  agk config [--applier H[:M]] [--reviewer H[:M]]
                                  Print the resolved worker configuration (harness, model, effort,
                                  extra args, permission mode) and where each value comes from:
                                  flag, .agk.toml in the repo root, ~/.config/agk/config.toml
                                  ($XDG_CONFIG_HOME/agk/config.toml), or default.
  agk setup [--skills-dir DIR] [--dry-run]
                                  Link the agk-* skills for Claude Code and Codex, add .agk/ to the
                                  global git ignore, check dependencies. Rerun after every upgrade.
  agk version                     Print the agk version.

Exit codes: 0 ok, 1 error (2 and 3 are reserved).
`

const reviewUsage = `Usage:
  agk review start [--base REF [--for NAME] | --change NAME] [--reviewer H[:M]] [--autopilot]   REF defaults to main
  agk review wait <run-dir> [--timeout SECONDS]   SECONDS defaults to 1800
  agk review send <run-dir> [--note FILE] [--timeout SECONDS]   SECONDS defaults to 600
  agk review fresh <run-dir>
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
	case "apply":
		return e.applyCmd(ctx, args[1:])
	case "autopilot":
		return e.Autopilot(ctx, args[1:])
	case "sync":
		return e.Sync(ctx, args[1:])
	case "finish":
		return e.Finish(ctx, args[1:])
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
	case "hook":
		return e.ReviewHook(ctx, args[1:])
	case "wait":
		return e.ReviewWait(ctx, args[1:])
	case "send":
		return e.ReviewSend(ctx, args[1:])
	case "fresh":
		return e.ReviewFresh(ctx, args[1:])
	case "close":
		return e.ReviewClose(ctx, args[1:])
	}
	fmt.Fprintf(e.Stderr, "agk: unknown review command %q\n\n%s", args[0], reviewUsage)
	return ExitError
}
