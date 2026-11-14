# Proposal

## Why

When `agk review send`, `agk apply send` or a review fix-up types its line into an interactive codex
worker, codex often leaves the line unsent in its input box (GitHub issue #1). The round then never
starts. `agk` only prints a warning and exits 0, and the next `agk ... wait` hangs until someone
presses Enter in the companion.

The cause is confirmed. Codex's TUI detects paste bursts in terminals without bracketed paste
(`tui/src/bottom_pane/paste_burst.rs`): chars that arrive less than 8 ms apart count as a paste,
and an Enter within the next 120 ms is inserted as a newline instead of submitting the message.
`agtermctl session type` injects `line + "\r"` in one burst, so the Enter lands inside that window.

A live A/B test with codex-cli 0.167.0 gave these results:
- without the fix, 0 of 10 typed lines were submitted;
- with `-c tui.disable_paste_burst=true`, 10 of 10 were submitted;
- a separate `"\r"` typed afterwards submitted the stuck line 10 of 10 times.

## What Changes

- Both interactive codex workers, the reviewer (`agk review start`) and the applier
  (`agk apply start`), run with `-c tui.disable_paste_burst=true`. The flag comes after the user's
  model, effort and `extra_args`, so the user's tuning cannot turn it back on.
- When a typed line is not reported as started within a short first wait, the turn is still over
  and the line is visible on the companion's screen, agk types one separate Enter and waits for
  the rest of the 30 s handshake. This applies to every typed line: `review send`, `apply send` and
  the review fix-ups typed by `review wait`. It also protects harnesses and future codex versions
  that the flag does not cover.
- `agk review send` and `agk apply send` inside agterm add `"started": true|false` to their JSON,
  as `start` already does. `false` means the line was typed but the worker never reported the
  round as started, most likely because the line is still in its input box. The round stays
  recorded and the exit code stays 0. Inline output (outside agterm) is unchanged.
- The `agk-review`, `agk-apply` and `agk-propose` skills handle `"started": false` on send: tell
  the user to press Enter in the companion, then wait as usual.
- README documents the new field. The matching BACKLOG.md entry is removed, since issue #1 now
  tracks the bug.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `review-companion`: the interactive codex reviewer disables paste-burst detection; `review send`
  reports `started`; a typed round or fix-up line that is not picked up gets one separate Enter.
- `apply-worker`: the interactive codex applier disables paste-burst detection; `apply send`
  reports `started` and retries the Enter the same way.

## Impact

- `internal/harness/codex.go`: argv of `Interactive` and `InteractiveReviewer`.
- `internal/cli/worker.go`: `typeWhenIdle` gets a two-step handshake and the Enter retry.
- `internal/cli/review.go`, `internal/cli/apply.go`: the send results gain `started`.
- `skills/agk-review/SKILL.md`, `skills/agk-apply/SKILL.md`, `skills/agk-propose/SKILL.md`,
  `README.md`, `BACKLOG.md`.
- No new dependency. The flag needs a codex-cli that knows `tui.disable_paste_burst` (0.167.0 does).
  Codex accepts unknown `-c` keys silently, so an older codex keeps working, just without the fix,
  and the Enter retry still covers it.
