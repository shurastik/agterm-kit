# Proposal

## Why

`/agk-autopilot` runs the whole workflow unattended in the main session and its companions, but the
agterm sidebar shows nothing that sets such a run apart from a session the user drives by hand. The
user cannot tell at a glance which session is under the autopilot right now, or which companions an
autopilot started.

## What Changes

- New `agk autopilot on|off`: sets or clears an autopilot info line (`🤖 autopilot`) on the
  caller's agterm session. The line expires by itself (`--hide-after`), so a crashed autopilot does
  not leave it behind for good.
- New `--autopilot` flag on `agk apply start` and `agk review start`: the run records it in
  `worker.json` together with the session to mark (the caller's). Such a run:
  - names its companion with a `🤖 ` prefix (`🤖 add-auth · claude-apply r1`) and tints the
    companion's sidebar icon; `agk apply send` / `agk review send` keep the prefix when they rename
    the companion for the next round; the companion keeps the mark until it is closed;
  - refreshes the autopilot line of the recorded session while `agk apply wait` /
    `agk review wait` poll and when `send` types a round, so the line outlives a worker that runs
    for hours.
- Outside agterm all of it is a no-op; a failed mark is a warning, never a failed command.
- The `agk-autopilot` skill runs `agk autopilot on` before its first step, passes `--autopilot` to
  every `agk apply start` and `agk review start`, and runs `agk autopilot off` on every exit
  (finished or stopped) before its final report.
- README documents the command, the flag and the marks.

## Capabilities

### New Capabilities
- `autopilot-marker`: the sidebar marks of an autopilot run: `agk autopilot on|off` on the main
  session, the self-expiring line and its refresh, and the `--autopilot` mark of the companions.

### Modified Capabilities
- `apply-worker`: `agk apply start` takes `--autopilot`; the companion names of start and of the
  per-round rename follow the run's autopilot mark.
- `review-companion`: `agk review start` takes `--autopilot`; the companion names of start and of
  the per-round rename follow the run's autopilot mark.
- `autopilot-workflow`: the skill turns the mark on at its start, passes `--autopilot` to the
  workers and turns the mark off on every exit.

## Impact

- `internal/agterm`: `Info` gains an expiry, new `ClearInfo` and `Tint`; the fake follows.
- `internal/cli`: new `autopilot` command; `--autopilot` on both starts; `worker.json` of both
  workers gains the autopilot fields; `companionName` takes the prefix; the refresh hooks into the
  shared wait loop (`waitPolling`) and the sends.
- `skills/agk-autopilot/SKILL.md`, the autopilot eval cases and the stand-in `agk` of `evals/`.
- `README.md`.
