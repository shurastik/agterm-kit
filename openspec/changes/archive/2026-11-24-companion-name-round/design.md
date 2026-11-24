# Design

## Context

Both workers name their companion through one helper, `companionName(change, harness, kind, num)` in
`internal/cli/worker.go`, giving `<harness>-<kind> #<num>` with an optional `<change> · ` prefix; the reviewer
picks the kind (`plan` or `review`) in `reviewCompanionName`. `num` comes from `rundir.Number(dir)`, the
position of the run directory among its siblings. Besides the name, the reviewer uses the number in
`review.Label` (info lines `codex #8 r1: ...`) and in the two inline log lines of `runRound`. Nothing else
reads it: run directories are addressed by path everywhere.

The companion is a regular agterm session with a parent, and `agtermctl session rename <name> --target <id>`
works on companions as on any session (agterm reference: "Other session commands (status, notify, type,
split, rename, flag, close) act on a companion as on any session").

`agk review send` and `agk apply send` both go through `typeWhenIdle` (`internal/cli/worker.go`): wait for the
turn to be over, type one line, confirm the start. A refusal there (busy, permission prompt, gone) rolls the
round back with `rollbackRound`; an unconfirmed start keeps the round recorded and returns `started: false`.

## Goals / Non-Goals

**Goals:**
- One naming scheme for both workers: `[<change> · ]<harness>-<kind> r<k>`.
- The name follows the recorded round, and never shows a round that was rolled back.

**Non-Goals:**
- Renaming on `agk review wait` / `agk apply wait` (a round's outcome does not change its number).
- Keeping run numbers anywhere, including `agk config` or JSON output (they never appeared there).
- Renaming companions of runs started by an older agk on start; they get the new name on their next `send`.

## Decisions

### Round in the name, computed by the same helper

`companionName(change, harness, kind string, round int)` returns `<harness>-<kind> r<round>` with the
optional prefix. `ApplyStart` and `startInteractive` pass `1`; the send commands pass `k+1`. The send commands
rebuild the name from the run's recorded state (change and worker harness from `worker.json`; for a review,
`state.Change` picks `plan`, else `review` with `state.For`), so no name is stored on disk.

Alternative: store the name in the run directory and substitute the round. Rejected: one more file to keep in
sync, and the state needed to rebuild the name is already recorded.

### Rename after the line is typed, not before

The send commands rename only after `typeWhenIdle` returned without error, that is, after the line was typed
and the round stays recorded, whether or not the start was confirmed. A refusal rolls the round back and skips
the rename, so the name never runs ahead of the recorded round.

Alternative: rename right after `initRound`. Rejected: a busy or blocked worker would leave the companion
named after a round that `rollbackRound` then removed.

### A failed rename is a warning

The name is cosmetic; the round already reached the worker. A rename error (agterm refuses, companion gone in
between) prints `agk: warning: rename <role> <id>: <err>` on stderr and the command carries on with its normal
output and exit code. Failing the command would make the main session believe the round did not start.

### `Ctl.Rename`

`agterm.Ctl` gains `Rename(ctx, target, name string) error`, implemented as
`agtermctl session rename <name> --target <id> --socket ...` through the existing `runOK`, so `--target` and
the socket are passed like every other call. The name reaches agtermctl as one argv element (no shell), the
same as `--name` at creation, so the ` · ` separator needs no quoting. The fake records it like the other
methods and can be told to fail it.

### Number removed outright

`review.Label` drops `Num`; `Label.String` becomes `<harness> r<k>` (or `<harness>` when the round is
unknown). The inline log lines become `agk: review round <k> started, run dir <dir>` and
`agk: review round <k>: ...`. `reviewRun.num`, the `rundir.Number` calls and `rundir.Number` itself (with its
test) are removed. Keeping a dead `Number` "for later" was rejected: nothing reads it.

## Risks / Trade-offs

- [Two diff reviews without `--for` running at once are both `codex-review r1`] → Rare (the skills always
  pass `--for` or `--change`); their info lines and run dirs still tell them apart.
- [A user renamed a companion by hand] → `send` overwrites the name with agk's. Acceptable: agk owns the names
  of the companions it creates.
- [Rename runs while the worker's turn is starting] → `session rename` touches only the sidebar label, not the
  pane, so it cannot interfere with the typed line or the harness's status hooks.
