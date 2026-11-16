# Design

## Context

See proposal.md - Why. Current state:

- `agk apply wait` (`internal/cli/apply.go`) waits for the companion's agent status in 5-second
  slices (`waitSliced`), polling for `rounds/<k>/report.json` between them, and hands the status to
  `apply.Decide` (`internal/apply/report.go`), which maps `completed`/`idle` without a report to
  `needs-human` / `no-report`.
- The interactive claude applier keeps the user's settings, so agterm's status hooks
  (`UserPromptSubmit`/`PostToolUse` → `active`, `Stop` → `completed --auto-reset`) set its status.
  agk adds one settings layer through `--settings`: `<run>/claude-settings.json`, a copy of the
  embedded deny rules (`assets/claude-applier-settings.json`), written by `applierFiles`.
- `agtermctl wait` returns at once when the current status is already one of the awaited ones, so
  during a pause the status `completed` wakes every wait immediately.
- The reviewer already uses agk-written hooks (`reviewerSettings`, hidden `agk review hook`): the
  pattern of a quoted agk path plus a subcommand in a settings file exists and is tested.
- Spike (claude 2.1.325, `-p`, shell and Monitor): the first `Stop` input had
  `background_tasks: [{"id","type":"shell","status":"running","description","command"}]`, the resume
  fired `UserPromptSubmit` with a `<task-notification>` prompt, the last `Stop` had
  `background_tasks: []`, `session_crons: []`.

## Goals / Non-Goals

**Goals:**
- Tell a pause from a finished turn from Claude Code's own data, not from the model's discipline.
- Keep the agent status untouched by agk, as the conventions require for the interactive applier.
- Keep every existing outcome for runs without the hooks.

**Non-Goals:**
- The codex applier (not used with background work) and the reviewer (read-only, never pauses).
- Guarding `agk apply send` against typing into a paused applier: a typed line simply starts a new
  turn, which claude handles.
- Showing task commands or descriptions anywhere agk prints: they are model-chosen text.

## Decisions

### D1. Signal: `background_tasks` and `session_crons` of the `Stop` hook input

In-flight count = `len(background_tasks) + len(session_crons)`; a missing field counts as empty, so
an older Claude Code behaves as today. Every entry counts: the docs define both arrays as what is
in flight or scheduled.

Alternatives: a rule in `applier-rules.md` ("don't end the turn with background work") fights
Claude Code's own instructions to background long work and end the turn, and fails silently; a
marker file written by the model has the same weakness; a quiet-screen grace period delays every
honest `no-report`; walking the companion's process tree cannot tell MCP servers from background
shells.

### D2. One run-level turn-state file that carries its round, written at both turn edges

`<run>/turn.json` is `{"round":k,"state":"running"}` (written by the `UserPromptSubmit` hook) or
`{"round":k,"state":"stopped","background":N,"wakeups":M}` (written by the `Stop` hook). Every write
is atomic (`fsx.ReplaceFile`). A session's turns are sequential, so the hooks' read-modify-writes of
one session never race each other.

The round is never read from `<run>/round`, which `agk apply send` changes from outside the
applier: it publishes round k+1 (`initRound` → `rundir.SetRound`) before `typeWhenIdle` waits for
round k's turn to end, and puts the round back when the send gives up. A hook reading it could land
round k's turn on k+1 at either edge (a late `Stop`, or a resumed turn whose `UserPromptSubmit` hook
runs after the publish). Instead the round is bound to what the applier actually received:

- `UserPromptSubmit`: if the hook input's `prompt`, trimmed, is exactly agk's kickoff line for
  `<run>/rounds/<k>/prompt.md` (`kickoff()`, the same function `apply start`/`send` use), the
  round is k. Any other prompt (a `<task-notification>`, a line the user typed) keeps the round of
  the existing file; without a file it records nothing.
- `Stop`: keeps the round of the existing file; without a file it records nothing.

`wait` reads the file only when its `round` equals the run's current round; any other round counts
as missing.

Writing `running` instead of deleting the file lets `wait` tell "the turn is on" from "the hook has
not run yet". agterm's and agk's hooks for the same event run in parallel, so the status may say
`completed` before `turn.json` says `stopped`; while it says `running`, wait keeps going. The file
holds the round and counts only, no model text.

Alternatives: a file per round dir, chosen by `<run>/round` at hook time: rejected for the round
switch above. Matching `Stop` to its start by the hook input's `prompt_id`: fixes only the `Stop`
edge; the start of a resumed turn still has to find its round. Deleting the file on `UserPromptSubmit`: a missing file is
then ambiguous between a running turn and a lagging `Stop` hook.

### D3. Hidden `agk apply hook <turn|stop> <run-dir>`

`stop` handles every turn end: `Stop` and `StopFailure` (a final API error ends the turn with
`StopFailure`, whose input has no `background_tasks`, so it records `stopped` with nothing in flight).

The settings command is `<quoted agk path> apply hook turn|stop <quoted run dir>`, quoted with the
existing `shellQuote` (which refuses `\` and line breaks: `agk apply start` exits 1 before creating
the companion). The hooks take the round from the prompt or the existing file (D2), so one
settings file serves every round. It reads at most 1 MiB of stdin, writes nothing on any error, and
always exits 0 with empty stdout and stderr: `UserPromptSubmit` stdout would land in the applier's
context, `Stop` stdout is parsed as a decision, and a failing hook must never stop the applier. The
run dir argument is checked to be an apply run directory (`rundir.Root` with `rundir.Applies`).

### D4. Generated applier settings

`applierFiles` stops copying the asset verbatim: it unmarshals the deny-rule asset, adds
`hooks.UserPromptSubmit`, `hooks.Stop` and `hooks.StopFailure` entries, and writes the merged JSON. Claude merges hook
arrays across settings sources, so the user's agterm status hooks still run. The headless (inline)
claude applier gets the same file; its hooks write `turn.json`, which inline `wait` ignores (the
report or its absence decides there, as now). The resume path (`apply send` after a restart rebuilds
`ApplierFiles` from the run dir) reuses the file already written.

### D5. Runs opt in through `worker.json`

`applyState` gains `TurnHooks bool` (`json:"turn_hooks,omitempty"`), set when the claude settings
with hooks were written. `agk apply wait` uses turn state only when it is set; codex runs and runs
from an older agk decide as before, so no run changes behavior mid-flight.

### D6. Wait loop for a hooked run

"Turn state" below means `turn.json` with `round` equal to the run's current round; anything else
counts as missing.

Start phase (round not yet marked started): as today, wait for `active`/`blocked` (plus
`completed` in round 1), but a slice also ends when the report or a turn state appears. A turn state
proves the round's `UserPromptSubmit` ran, so it marks the round started (`markStarted`) even when
`active` was never seen (`apply send` returned `"started": false` for a short turn). Then the loop:

1. Report present → `apply.Decide` as now.
2. By turn state:
   - `running`, or `stopped` with in-flight > 0 (paused): wait for `blocked` only, in slices,
     ending a slice early when the report appears or `turn.json` changes (content compare).
   - `stopped` with 0 in flight: go to step 3.
   - missing: wait for `completed|blocked|idle`; once the status shows the turn over, poll up to
     30 s for a turn state or the report, then go to step 3.
3. Before `no-report`: re-read the report, then one status check bounded by the existing
   `statusCheck` (1 s): `ErrGone` → `cancelled`, `blocked` → `permission`, else `no-report`.
4. In every wait: `blocked` → `needs-human` / `permission`; `ErrGone` → `cancelled`.
5. Budget spent → `running`; with `reason` `background` and `background` = in-flight count when the
   last turn state read was a pause.

`apply.Decide` gains the turn state as an input so the mapping stays one pure, table-tested function;
`Outcome` gains `Background int` (`json:"background,omitempty"`). The polling reuses `waitPolling`
with a `done` func that compares report existence and `turn.json` bytes.

## Risks / Trade-offs

- [Interactive claude may differ from `-p`] → an e2e check on a live companion is a task; the docs
  describe the fields for every `Stop`.
- [A background task that never ends (`tail -f`, a dev server)] → wait returns `running` /
  `background` on every timeout; the `agk-apply` skill tells the main session to show the user after
  a second such result.
- [Claude passes the typed kickoff to `UserPromptSubmit` altered (wrapped, prefixed)] → the round
  never moves past the recorded one, so a new round's turn state counts as missing and wait falls
  back to today's decision after 30 s; the e2e task checks a round-2 kickoff on a live companion.
- [User settings disable hooks (`disableAllHooks`)] → no `turn.json`; wait falls back after 30 s to
  `no-report`, a 30-second delay against today.
- [A permission prompt that the user already answered keeps the status `blocked` until the next
  tool] → same as today; not changed here.

## Migration Plan

None: new runs get the hooks; running and old runs keep the old decision (D5). Rollback is a
revert; leftover `turn.json` files are ignored.
