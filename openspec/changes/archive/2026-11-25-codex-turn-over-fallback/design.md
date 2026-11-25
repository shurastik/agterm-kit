# Design

## Context

See proposal.md (Why) for the incident. Relevant current state:

- `typeWhenIdle` (`internal/cli/worker.go`) is the only gate before a line is typed into a live
  worker. It makes one `Ctl.Wait` call for `completed|blocked|idle` that lasts the whole budget, and
  it has three callers: `ReviewSend` (round k+1), `ApplySend` (round k+1) and `typeFixup` in
  `internal/cli/wait.go` (a reviewer fix-up after an invalid output file).
- `wait` already polls in `pollSlice` (5 s) slices through `waitPolling`, and files on disk decide
  the outcome. That is why round 1 completed in the incident while `send` hung.
- Codex workers learn about turn ends only from the user's codex hooks (agterm's
  `agterm-codex-status.sh` / `agterm-agent-status.sh`). Claude workers have agk-owned hooks: the
  restricted reviewer gets `--settings` hooks that set the status, and the applier gets `turn.json`.
- Spike results (codex 0.171.0, this repo's scratchpad):
  - The hook-trust screen is not the cause. After "Trust all", the newly trusted hooks ran in the
    same session, and with agk's exact reviewer argv the status went `idle → active → completed`.
  - A codex TUI with no `-c` overrides attaches to a running shared app-server daemon, and the
    hooks then run in the daemon with the daemon's `AGTERM_SESSION_ID`, which belonged to a session
    that no longer exists. Any `-c` other than `features.*` / `tui.fullscreen_transcript` keeps the
    TUI embedded, and that is the only reason agk's workers are embedded today.
  - The incident session and the run that worked right after it used the same codex (0.169.0). Its
    exact cause cannot be recovered: codex logs for that day were rotated.
- codex rejects a repeated `--no-daemon` (`the argument '--no-daemon' cannot be used multiple
  times`, exit 2) and an unknown flag (exit 2). `--no-daemon` first shipped in codex 0.156.0 (PR
  #46088).

## Goals / Non-Goals

**Goals:**
- A codex worker's turn end is detected even when its agterm status never moves, without typing
  into a running turn or into an open codex dialog.
- Codex workers never route their hooks through a shared daemon.

**Non-Goals:**
- Finding the exact cause of the 2026-11-17 incident, or fixing codex or agterm's hook scripts.
- Changing claude workers or inline/headless runs (`codex exec`, `claude -p`).
- Changing how `agk review wait` / `agk apply wait` decide an outcome. The one exception is the
  reviewer fix-up decision (decision 3a): it is the only `wait` path that types into a worker.
- Detecting the installed codex version.
- Making the start handshake (`confirmStart`) independent of the status.

## Decisions

### 1. `--no-daemon` always, deduplicated

Both codex interactive argv builders append `--no-daemon` after the tuning (model, effort,
`extra_args`), next to `-c tui.disable_paste_burst=true`. If `extra_args` already holds the exact
token `--no-daemon`, it is not appended again, because codex exits on a repeated flag. The user
chose to always pass it.
- Alternative: probe `codex --help` at start and pass the flag only when it is listed. This needs a
  new driver seam and one more exec per start. Rejected: codex older than 0.156.0 is two months old,
  and a failed start is visible in the companion (the handshake warning prints its screen).
- Alternative: a config opt-out key. Rejected: more config surface for a short-lived compatibility
  case.
- README states codex ≥ 0.156.0 under the requirements.

### 2. The fallback lives in `typeWhenIdle`, behind an optional settle check

The settle check is one small stateful helper, a `settler`. It holds the evidence predicate and the
previous screen, and every call takes one `Ctl.Text` read and answers settled or not (steps 1 to 3
below). `typeWhenIdle` gets an optional `*settler` (nil for no fallback), whose `evidence func()
bool` says the round's outcome is on disk. Callers pass it only for codex workers (`worker.json`
harness).
With the argument set, the single long `Wait` becomes `waitPolling` over the same budget and the
same `completed|blocked|idle` set. The status therefore still decides first: a `completed` returns
from the first `Wait` at once, and `blocked` still refuses. Between slices, `done` runs the settle
check:
1. If `evidence()` is false, the previous screen is forgotten and the check reports not settled.
2. Read `Ctl.Text(companion, settleLines)`. On an error, the previous screen is forgotten and the
   check reports not settled, so the status rule stays.
3. If the read equals the previous read (one slice earlier) and its last 12 lines contain no codex
   dialog marker, the check reports settled. Otherwise the read is kept as the previous one.

When the check reports settled, agk prints the fixed warning `agk: warning: the <worker> <id> still
reports <status>, but round <k> is on disk and its screen has not changed for 5 s; treating its turn
as over`, types the line, and runs `confirmStart` as usual. The status in that warning is the last
one known, or "no finished turn" when the status is unknown. The fallback therefore needs at least
two slices, about 10 s.
- Alternative: a separate pre-check in each caller. Rejected: three callers would repeat the budget
  and error handling that `typeWhenIdle` already owns.
- Alternative: recognise codex's idle composer text ("Ask Codex to do anything"). Rejected: the
  placeholder is codex UI copy and can change between releases, while "unchanged for 5 s" holds as
  long as a running turn shows its live timer (`Working (Ns • esc to interrupt)`).

### 3. Evidence per caller

- `ReviewSend`: round k's `result.json`. The command already refuses without it, so this is always
  true there, and it is passed for clarity.
- `typeFixup`: the round's output file (`out/<k>.json`) exists. It is invalid, which is why a
  fix-up is being sent. That alone is not enough, see 3a.
- `ApplySend`: the current round's `report.json` exists. Without a report, an applier that went
  quiet may be stuck or asking, so the status rule stays.

### 3a. The fix-up decision counts a settled screen as a turn end

`typeFixup` runs only when `review.Decide` returns `Fixup`, and `Decide` asks for a turn end
(`completed`/`idle`) before it does. With a status stuck at `active`, it returns `KeepWaiting`
until the deadline and the fix-up is never typed. So `Decide` gets one more input, `settled bool`,
and `turnOver` becomes `status == completed || status == idle || settled`. All its other rules
stay: a valid output completes, gone/timeout/`blocked` come first, no second fix-up before the
previous one started, and at most `MaxFixups`.

`waitInteractive` owns a `settler` for codex runs (nil otherwise), whose evidence is "`out/<k>.json`
exists". In each loop iteration, before `Decide`, it calls the settler only when the output exists
and does not parse and the status is not `completed`/`idle`/`blocked`. Otherwise it resets the
settler. The loop already wakes every slice while the file exists, because `waitSliced` returns
`found` once it does, so successive calls are one slice apart. When a fix-up is typed, the settler
is reset, so the screen of the next turn has to settle again. `typeFixup` hands the same settler to
`typeWhenIdle`, which then needs one more slice to confirm.
- Alternative: drop the fix-up path from the fallback. Rejected: the incident pattern (status
  stuck, file on disk) leaves an invalid first output stuck for the whole `wait` budget too, and
  the reviewer would still have to be restarted by hand.

### 4. Dialog markers

The same set agterm's codex hook watches, matched case-insensitively in the last 12 screen lines:
`press enter to confirm`, `enter to submit answer`, `enter to submit all`, `allow command?`. Codex
workers run with `--ask-for-approval never`, so approval prompts should not appear. The check
guards against question dialogs and anything a future codex adds under the same wording.

### 5. Codex only

Claude statuses come from agk's own hooks, and a claude applier can be paused with background work
in flight: its screen can be still while its turn is not over. The fallback is keyed on the run's
recorded harness (`worker.json`), not the current config, consistent with "a run is pinned to the
configuration it started with".

## Risks / Trade-offs

- [An idle codex screen that animates, such as a rotating placeholder or a ticking clock, never
  settles] → The fallback does not fire and behaviour is as today (busy after the timeout). A
  manual check on the spike stand confirms that two reads 5 s apart match on an idle codex 0.171.
- [A turn that shows no change for 5 s while still running, such as a long silent tool call
  without a live timer] → Codex shows the elapsed-time spinner during a turn. The fallback also
  needs the outcome on disk, which a worker writes at the end of its round. The worst case is a
  line typed while codex is still finishing, which codex queues as a steer of the current turn.
- [With a status stuck at `active`, `confirmStart` sees `active` at once and reports
  `started: true` even if the line sits unsent] → `wait` decides rounds on files, so the round
  still completes. The Enter retry is lost only in this degraded mode. Accepted; see Non-Goals.
- [codex < 0.156.0 cannot start as a worker] → The error is visible in the companion; README names
  the minimum version.

## Migration Plan

No data migration. Runs started by an older agk have no `--no-daemon` in their recorded argv: their
companion keeps running as it was started. The fallback applies to them because it reads only the
harness from `worker.json`. Rollback: revert the change, nothing on disk depends on it.
