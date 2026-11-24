# Proposal

## Why

A worker companion is named `<harness>-review #N`, `<harness>-plan #N` or `<harness>-apply #N`, where N is
the position of the run among all runs of the repository (`#15` today). It reads like a pass number, never
changes while the run goes through its rounds, and says nothing the user needs: which round the worker is on
is the useful number, and the kind (plan or code) is already in the name.

## What Changes

- **BREAKING** (display only): the run number N is dropped everywhere it is shown. Companion names, reviewer
  info lines and the inline review log no longer carry `#N`.
- A companion is named after its current round: `<change> · <harness>-plan r<k>`,
  `[<change> · ]<harness>-review r<k>` and `<change> · <harness>-apply r<k>`. `agk review start` and
  `agk apply start` create it as `r1`.
- `agk review send` and `agk apply send` rename the companion to `r<k+1>` once the new round was typed into
  it. A failed rename is a warning on stderr; the command's output and exit code stay what they would be
  without it.
- Reviewer info lines read `<harness> r<k>: ...` (was `<harness> #N r<k>: ...`); the inline review log
  reads `agk: review round <k> started, run dir <dir>` and `agk: review round <k>: ...`.
- `agk --help`, README and the `agk-review` / `agk-apply` skills name companions with the new scheme.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `review-companion`: the companion name carries the round instead of the review number and is renamed on
  `agk review send`; the "Review numbering" requirement is removed; info lines and the inline log drop the
  number.
- `apply-worker`: the applier companion carries the round instead of the run number and is renamed on
  `agk apply send`; apply run directories are no longer numbered.

## Impact

- `internal/agterm`: `Ctl` gains a rename of a session (`agtermctl session rename <name> --target <id>`),
  with the fake.
- `internal/cli`: companion naming (`companionName`, `reviewCompanionName`), `ReviewStart`, `ReviewSend`,
  `ApplyStart`, `ApplySend`, the inline review log lines, `--help` text.
- `internal/review`: `Label` loses its run number.
- `internal/rundir`: `Number` loses its last caller and is removed.
- Docs: `README.md`, `skills/agk-review/SKILL.md`, `skills/agk-apply/SKILL.md`.
- `BACKLOG.md`: the "Review companion name" entry is removed (it becomes this change).
