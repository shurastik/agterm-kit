# Proposal

## Why

`agk review send` and `agk apply send` type the next round into a worker only once agterm reports
that its turn is over. For a codex worker that status comes from codex's own hooks, and it can stay
wrong for good. A codex reviewer finished round 1 (its output file was valid and `agk review wait`
completed), but its status stayed `active`, so `agk review send` timed out after 600 s with "the
reviewer … is busy". The only way out was to close the reviewer and start a new one. `wait` already
decides on files and is not affected; `send` trusts the status alone.

A spike found one more way the status goes wrong. When codex's shared app-server daemon is running,
a codex TUI with no `-c` overrides attaches to it, and the hooks run inside the daemon with the
daemon's environment. They report to whatever agterm session the daemon was started from, often a
session that no longer exists. agk's codex workers avoid the daemon today only by accident: codex
refuses to reuse it when the command line has `-c` overrides, and agk happens to pass some.

## What Changes

- The interactive codex reviewer and the interactive codex applier run with `--no-daemon`, so codex
  runs its hooks in the worker's own process, with the worker's `AGTERM_*` variables. agk adds the
  flag once, after the user's tuning, and does not add it again if `extra_args` already has it.
  This needs codex 0.156.0 or newer, the first release with `--no-daemon`. An older codex exits on
  the unknown flag, and the companion shows that error.
- `agk review send`, `agk apply send` and the reviewer fix-up typed by `agk review wait` get a
  fallback for codex workers when the status is wrong. If agterm does not report the turn as over,
  but the current round's outcome is already on disk and the companion's screen has not changed for
  one poll interval (5 s) and shows no codex dialog, agk treats the turn as over, types the line and
  warns on stderr. A status that does report the turn as over still wins, as before. A `blocked`
  status still refuses.
- Claude workers do not change: their turn state comes from agk's own hooks (the reviewer's
  `--settings` hooks and the applier's `turn.json`), and a paused claude applier must not read as
  idle.
- README: the requirements list gives codex 0.156.0 as the minimum, and the `send` docs describe the
  fallback.
- `BACKLOG.md`: the entry "Codex reviewer stuck `active` after the hooks-trust prompt" is removed.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `review-companion`: the codex reviewer's command line gets `--no-daemon`. The re-review round and
  the fix-up accept a settled screen as the end of a codex reviewer's turn once the round's outcome
  is on disk.
- `apply-worker`: the codex applier's command line gets `--no-daemon`. The follow-up round accepts a
  settled screen as the end of a codex applier's turn once the current round's report is on disk.

## Impact

- `internal/harness/codex.go`: `Interactive` and `InteractiveReviewer` argv (`--no-daemon`).
- `internal/cli/worker.go`: `typeWhenIdle` waits in slices and gains the screen fallback for codex
  workers; new helpers that compare screens and detect dialogs.
- `internal/cli/review.go` (`ReviewSend`), `internal/cli/apply.go` (`ApplySend`),
  `internal/cli/wait.go` (`waitInteractive`, `typeFixup`): pass the harness and the evidence that
  the round's outcome is on disk.
- `internal/review/decide.go` (`Decide`): a settled screen counts as a turn end for the fix-up
  decision.
- Tests: `internal/harness` argv tests; `internal/cli` send, fix-up and worker tests against the
  agterm fake (`agtermtest`), including `Text` sequences.
- README.md, BACKLOG.md.
- Users: codex older than 0.156.0 no longer starts as a worker.
