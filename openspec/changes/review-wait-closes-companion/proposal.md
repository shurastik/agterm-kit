# Proposal

## Why

Every `codex-review #N` companion stays open after its review finishes (`session new --wait`, and nothing ever calls `session close`), so the review/fix loop leaves a pile of finished companions under the agent session that the user has to close by hand. Closing from inside the review process races the agent's `session wait`: a companion that is already gone makes the wait exit 2, which the skill reads as "cancelled by the user".

## What Changes

- New command `agk review wait <run-dir> [--timeout SECONDS]`: waits for the review's outcome, prints it as one JSON object, and closes the companion (undoably) when the review completed. A failed (`blocked`) review keeps its companion open so the failure stays visible.
- `agk review wait` absorbs the outcome logic the skill now spells out by hand: `result.json` as the source of truth, a companion closed before any outcome (`cancelled`), a wait that timed out (`running`), and the inline (no companion) case.
- `agk review start` records the companion id in the run directory so `wait` needs only the run dir.
- The companion is still created with `--wait`, so a review whose outcome nobody collects stays visible as before.
- Skill `agterm-codex-review`: step 2 becomes `agk review wait <dir>`. Triage (`agk triage`) becomes optional and runs only when the user asks for it; by default the agent works through the findings in `review.json` directly.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `review-companion`: the run directory also records the companion id; the companion no longer stays open after a completed review once its outcome is collected; new requirement for `agk review wait` (outcome JSON, close on completed, keep on blocked, cancelled/running/inline cases).

## Impact

- Code: `internal/cli` (new `review wait` subcommand, `review start` writes the companion id, usage text), `internal/agterm` (`Ctl` gains wait and close, real `agtermctl session wait` / `session close --undoable` calls, fake updated).
- Skill: `skill/agterm-codex-review/SKILL.md` (wait step, optional triage, default act-on-findings flow).
- Docs: `AGENTS.md` usage block.
- No new dependencies. `agk triage` and the `findings-triage` capability are unchanged.
