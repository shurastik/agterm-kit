# Design

## Context

Every line agk types into a live worker goes through one function, `typeWhenIdle` in
`internal/cli/worker.go`. It waits until the turn is over (`completed`/`idle`, refusing `blocked`),
types `line + "\r"` with one `agtermctl session type`, then waits up to 30 s (`sendHandshake`,
capped by an optional deadline) for `active`. Its callers are `review send`, `apply send` and the
review fix-ups typed by `review wait` (`typeFixup`). Round 1 of both workers is not typed: the
kickoff is the harness's command-line prompt, so `start` is not affected.

The codex argv comes from `internal/harness/codex.go`: `Interactive` (applier) and
`InteractiveReviewer`. Both append `codexTuning(w)` (model, effort, `extra_args`). The reviewer then
adds its mandatory sandbox `-c` flags after the tuning; the applier puts its fixed flags before it.

`agterm.Ctl` already has everything the fallback needs: `Wait` (returns at once when the current
status is in `until`), `Type` and `Text`. The test fake scripts successive waits (`WaitSeq`) and the
screen (`TextOut`), so no new Ctl method and no fake change are needed.

See proposal.md for the root cause and the smoke-test numbers.

## Goals / Non-Goals

**Goals:**
- A typed round line reaches a codex worker as a submitted message.
- When it still does not, agk recovers once without risking a stray keystroke, and tells the caller
  through `started` instead of only a stderr warning.

**Non-Goals:**
- Changing how the first round is delivered (argv), or the `start` handshake.
- Detecting a stuck line in claude beyond the generic retry: claude has not shown the bug.
- Rolling back a round whose line was typed but not confirmed. The line may still be in the input
  box, and a later Enter would run a round agk had deleted.

## Decisions

### 1. Disable paste-burst detection with `-c tui.disable_paste_burst=true`

Add the flag to both codex interactive argvs, after `codexTuning(w)`:
- reviewer: after the tuning, next to the mandatory sandbox flags;
- applier: after the tuning, before the kickoff. The applier's other fixed flags stay where they
  are, because users may override those on purpose.

The `[tui]` key is the current one. The top-level `disable_paste_burst` is a legacy fallback that
the `[tui]` key overrides, so setting the `[tui]` key wins over either one in the user's config.

Alternatives:
- *Type the Enter separately after a pause of ≥ 200 ms* (longer than codex's 120 ms Enter
  suppression window plus its 60 ms idle flush). This works for any harness, but the delay is tuned
  to codex internals that can change, and it adds a fixed sleep to every send. Kept only in its
  conditional form, as decision 2.
- *Bracketed paste* (wrap the text in `ESC[200~ … ESC[201~` and send Enter outside it): codex
  would take the text as an explicit paste. This depends on how `agtermctl session type` passes
  escape sequences through, which is untested, and a bracketed paste in claude behaves differently.
  Rejected.

### 2. Two-step handshake with one conditional Enter

After `Type(line+"\r")`:

```
end   = min(now + 30s, deadline)        # absolute; deadline only when the caller passes one
left() = end - Now()                    # recomputed from the injected clock before every step
first = min(10s, left())
Wait(active, first)                     -> active: started
left() <= 0                             -> not started (no retry)
status = Wait([active, completed, idle, blocked], min(1s, left()))  # current status, returns at once
  active                 -> started
  completed | idle       -> screen = Text(last 20 lines)
                            left() <= 0       -> not started (no keystroke)
                            line visible?     -> Type("\r"); left() > 0 ? Wait([active, blocked], left()) : not started
                                                 (blocked after the Enter -> not started, permission hint)
                            else              -> Wait([active, blocked], left())
  blocked | timeout | err -> not started (no keystroke)
```

Each `agtermctl` call is a separate process and takes real time, so the remaining budget is never
carried over as a precomputed number: it is recomputed from `Env.Now` before the status check,
before the Enter and before the last wait. Once the end is reached nothing more is typed.

Recomputing between calls does not stop a call that is already running: `Text` and `Type` take no
timeout, and `Wait` rounds its timeout up to whole seconds. So every call after the line is typed
(the first wait, the status check, `Text`, the retry `Type`, the last wait) runs under one context
derived from the caller's with `context.WithTimeout(ctx, end - Env.Now())` (a duration, because `Env.Now` is a fake clock in tests). `RunProcess` already kills the
`agtermctl` process when its context ends. A call cut off this way, like any other error after the
line was typed, means "not started": the round stays recorded and the send reports
`"started": false`. It is never the refusal path, which would roll the round back. This keeps
`review wait`'s overall deadline (fix-ups) and the 30 s cap of a send, both to within process
teardown time.

- **10 s first step.** In the smoke test a submitted line turned `active` within 10 s every time,
  and in normal sends it turns `active` within a second or two. This leaves 20 s for the retry.
- **Status check before the Enter.** If a permission prompt opened (`blocked`), an Enter would
  answer it. Unknown status (timeout or error) also means no keystroke.
- **Screen check before the Enter.** "Line visible" means: the typed line, with all whitespace
  removed, is a substring of the last 20 screen lines with all whitespace removed. Removing
  whitespace makes the check immune to codex wrapping a long path across rows with indentation.
  The screen is read only to make this decision. It is never printed or parsed further, so the
  `termsafe` rule does not apply.
- **Exactly one retry.** In the smoke test a second Enter always worked. More retries would only
  hide a different failure.

Alternative: an unconditional Enter after the first step. It is simpler, but it could press Enter
in a dialog that opened in the meantime. Rejected.

### 3. `started` in the send JSON

- `sendResult` (review) and `sendRound` (apply) gain `Started *bool`, serialized as
  `json:"started,omitempty"`. It is set only on the companion path, so inline output stays
  byte-for-byte the same.
- A pointer is used because `false` must still be printed.
- The warning on stderr names why the start was not confirmed and a recovery hint that depends on
  what agk saw: the line on screen with status `completed`/`idle` means "the line may still be in its
  input box: press Enter in the companion" (only while no retry Enter was typed since the screen
  check); status `blocked`, before or after the retry Enter, means "answer the permission prompt
  deliberately"; anything else, including a retry Enter that was typed but never confirmed, means
  "look at the companion: if the line is still in its input box press Enter, if it asks for
  permission answer it deliberately". A bare Enter advice is never given once the screen snapshot
  may be stale, because in a permission prompt Enter could approve an action.
- Exit code stays 0, and the round stays recorded. `wait` already handles a round whose start was
  not seen (`roundStarted`): once the user presses Enter, `active` arrives and the round runs
  normally.

Alternative: exit 1 with an error. Rejected in explore: exit 1 already means "refused, nothing
typed", and here something was typed. `start` already reports `started` the same way.

### 4. Skills

`agk-review` step 4 and `agk-apply` step 3 add a branch for `"started": false`: tell the user the
line is waiting in the companion's input box and to press Enter there, then continue with the wait
step as usual.

`agk-propose` runs its own `agk review send` (step 2, item 4) and goes straight back to the wait,
so it gets the same branch there: on `"started": false`, tell the user to press Enter in the
reviewer companion, then wait as usual. An inline send has no `started` field and needs no
handling. `agk-archive` and `agk-explore` never call `send` and stay unchanged.

## Risks / Trade-offs

- [A turn that starts and ends within the first 10 s is never seen as `active`, and its line is on
  screen as the echoed user message] → agk types one extra Enter into an empty composer. Codex and
  claude are expected to ignore Enter on an empty input; the live smoke task checks this for codex.
  The same race already exists today, as a false warning.
- [A future codex renames or drops `tui.disable_paste_burst`] → codex ignores unknown `-c` keys
  (checked with 0.167.0), so nothing breaks, and decision 2 still recovers the round.
- [The kickoff line scrolls out of the last 20 screen lines] → this cannot happen for a line still
  in the input box: the composer is at the bottom of the screen.
- [`send` now takes up to 10 s longer when the worker is slow to report `active`] → only in the
  failure path. The total handshake stays capped at 30 s, as today.
