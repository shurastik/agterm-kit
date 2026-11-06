# Design

## Context

See proposal.md for motivation. Current state:

- `agk review start` creates the companion with `agtermctl session new --parent … --command … --wait --no-select` (`internal/agterm/ctl.go`, `NewCompanion`). `--wait` keeps the session after the command exits ("press any key to close"). Nothing calls `session close`.
- `agk review run` writes `result.json` (`{"status","reason","findings","verdict"}`) before it sets the final agterm status (`completed`/`blocked`). Setup failures before the run dir is verified set `blocked` without a `result.json`.
- The skill waits with a raw `agtermctl session wait --until completed --until blocked --timeout 1800` and interprets exit 0/2/124 and `result.json` itself.

agterm 0.40 behaviour observed while designing (relevant to the decisions below):

- `session wait` returns at once when the current status already matches; exit 2 when the session leaves the tree *during* the wait; exit 124 on timeout; on a session that is already gone it exits **1** with `Error: no such session: <id>`.
- A `completed` status that agterm auto-reset before the wait started is gone, so a late wait on a finished companion can block until its timeout.
- `session close --target X --json` on a missing session prints `{"error":"no such session: X","ok":false}` and exits 1. `--undoable` gives the close the ⌘W undo grace.

## Goals / Non-Goals

**Goals:**
- Close a companion only after the agent has collected a `completed` outcome, with no race against the wait.
- Move the outcome interpretation out of the skill into tested Go code.

**Non-Goals:**
- Closing companions of earlier runs on the next `review start` (each run's own `wait` closes it; old runs from before this change are left alone).
- Closing `blocked`, `cancelled` or `running` companions.
- Changing `agk triage` or the `findings-triage` capability.

## Decisions

### D1. The close lives in `agk review wait`, not in `agk review run`
The agent already has to wait for the outcome; the command that observes the outcome is the only place that can close the companion without racing that observation. Alternatives: drop `--wait` or let `review run` close itself — both make the session disappear right after the final status, so a wait that starts late gets `no such session` (exit 1) or exit 2 and the agent sees "cancelled"; a failed review would also lose its visible output. Rejected.

### D2. Companion id in `<run dir>/companion`
`review start` writes the id (one line, trailing newline) with `fsx.ReplaceFile` right after `NewCompanion` returns and before printing its JSON. If that write fails, `start` closes the companion (best effort) and exits 1, so no review runs that `wait` could not find. `wait` reads the file, trims it and accepts only a non-empty single token without whitespace; anything else is an error (exit 1). No file means the review ran inline. Alternative: `wait --companion <id>` from the agent. Rejected: one more thing for the agent to carry, and the id would not survive a lost conversation context.

### D3. Ctl gains `Wait` and `Close`
```go
// Wait blocks until target's status is one of until or timeout passes.
Wait(ctx context.Context, target string, until []string, timeout time.Duration) (string, error)
// Close closes target with undo grace.
Close(ctx context.Context, target string) error

var ErrGone    = errors.New("session gone")      // exit 2, or exit 1 "no such session"
var ErrTimeout = errors.New("wait timed out")    // exit 124
```
`Wait` runs `session wait --target T --until … --timeout <seconds> --json` and parses `{"id","status"}`. `Close` runs `session close --target T --undoable --json`; a `no such session` response maps to `ErrGone`. Both get `--socket` like every other call; the agk process context carries no deadline of its own, so agtermctl's `--timeout` governs. The fake records both calls and scripts their results.

### D4. Outcome resolution in `review wait`
```
validate dir (runRoot) + --timeout  --> error: exit 1, no agterm calls
read companion file
  none  --> result.json ? print it : exit 1        (inline run, no agterm calls)
  C     --> result.json exists?
             yes --> outcome = result.json; open = unknown
             no  --> Wait(C, completed|blocked, timeout)
                       status     --> open = true
                       ErrGone    --> open = false
                       ErrTimeout --> open = true
                     re-read result.json
                       present  --> outcome = result.json
                       absent   --> blocked (status) | cancelled (gone) | running (timeout)
             outcome completed and open != false --> Close(C)
                       ErrGone   --> silent
                       other err --> warning on stderr
print outcome JSON, exit 0
```
Reading `result.json` first avoids blocking on a completed status that agterm already auto-reset (agk writes `result.json` before setting the status). Re-reading after the wait makes `result.json` win over the agterm status, as the skill does today. A `result.json` with a status other than `completed`/`blocked`, or one that does not parse, is an error (exit 1).

### D5. Exit 0 for every determined outcome
The agent branches on `status` in the JSON; exit 1 means agk itself failed. Alternative: distinct exit codes per outcome. Rejected: they would collide with agterm's own 2/124 meanings that the skill used to handle, and the JSON already carries `reason`, `findings` and `verdict`.

### D6. Skill flow
Step 2 becomes `agk review wait <dir>` (run in the background, no polling) with one branch per `status`: `completed` → act on findings; `blocked` → read `codex.log`, report, fix our side, restart; `cancelled` → ask before restarting; `running` → tell the user, wait again. Inline runs (`companion: null`) also go through `wait`, which just reads `result.json`. Triage becomes an optional step taken only when the user asks for it; by default the agent works through `review.json` findings directly, raising in chat any finding it disagrees with instead of silently skipping it.

## Risks / Trade-offs

- [The `companion` file is edited to another session id] → `wait` would close that session. The run dir lives in the user's own repo under gitignored `.agk/`; the close is `--undoable`, and only a `completed` outcome triggers it.
- [Undo is disabled in agterm Settings] → `--undoable` closes at once; the review output is still in `codex.log`.
- [agterm changes the `no such session` text] → `Close`/`Wait` would report a generic error instead of `ErrGone`; for `Close` that is only a warning, for `Wait` an exit 1 the skill reports. Tests pin the current text.
- [Companions left open by reviews whose outcome nobody collects] → unchanged from today; the user closes them by hand.
