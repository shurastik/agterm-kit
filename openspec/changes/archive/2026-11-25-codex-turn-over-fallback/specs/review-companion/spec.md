# Spec Delta

## ADDED Requirements

### Requirement: Codex reviewer runs without the shared daemon
The interactive codex reviewer SHALL run with `--no-daemon`, so its hooks run in its own process with its own `AGTERM_*` variables and not in a shared codex app-server started from another session. The flag SHALL come after the configured model, effort and `extra_args`, and SHALL appear once, even when `extra_args` already has it. It needs codex 0.156.0 or newer.

#### Scenario: Codex reviewer argv
- **GIVEN** the agent runs inside agterm and the reviewer resolves to `codex` with no `extra_args`
- **WHEN** the agent runs `agk review start`
- **THEN** the companion's codex command line carries `--no-daemon` once, after the model and effort

#### Scenario: Flag already in extra_args
- **GIVEN** the reviewer resolves to `codex` with `extra_args = ["--no-daemon"]`
- **WHEN** the agent runs `agk review start`
- **THEN** the companion's codex command line carries `--no-daemon` exactly once, so codex does not exit on a repeated argument

#### Scenario: Claude reviewer unchanged
- **GIVEN** the agent runs inside agterm and the reviewer resolves to `claude`
- **WHEN** the agent runs `agk review start`
- **THEN** the claude command line carries no `--no-daemon`

### Requirement: Codex reviewer turn end from a settled screen
When a codex reviewer's status shows neither a turn end (`completed`, `idle`) nor a prompt (`blocked`), agk SHALL treat its turn as over if the round's outcome or output file is on disk, two screen reads 5 s apart are identical, and the screen shows no codex dialog to confirm, answer or allow a command. This holds for `agk review send` and for the fix-up decision of `agk review wait`; agk then types the line and warns on stderr.

#### Scenario: Status stuck at active after round 1
- **GIVEN** review run D with a codex reviewer whose round 1 has `result.json`, whose agent status stays `active`, and whose screen does not change
- **WHEN** the agent runs `agk review send D --note fixed.md`
- **THEN** the line for round 2 is typed into the companion within about two poll intervals, stderr carries a warning that the reviewer still reports `active` but its round is on disk and its screen has settled, stdout carries `"round": 2`, and the exit code is 0

#### Scenario: Screen still changing
- **GIVEN** review run D with a codex reviewer whose round 1 has `result.json`, whose status stays `active`, and whose screen changes between every two reads (its turn is still running)
- **WHEN** the agent runs `agk review send D --timeout 30`
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the reviewer is busy

#### Scenario: Settled screen shows a codex dialog
- **GIVEN** review run D with a codex reviewer whose round 1 has `result.json`, whose status stays `active`, and whose unchanged screen ends with a footer asking to press Enter to confirm
- **WHEN** the agent runs `agk review send D --timeout 30`
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the reviewer is busy

#### Scenario: Status still decides first
- **GIVEN** review run D with a codex reviewer whose round 1 has `result.json` and whose status turns `completed`
- **WHEN** the agent runs `agk review send D`
- **THEN** the line for round 2 is typed without waiting for the screen to settle, and no fallback warning is printed

#### Scenario: Screen cannot be read
- **GIVEN** review run D with a codex reviewer whose status stays `active` and whose screen read fails
- **WHEN** the agent runs `agk review send D --timeout 30`
- **THEN** the fallback does not apply, nothing is typed, and the command exits 1 saying the reviewer is busy

#### Scenario: Fix-up after an invalid output file
- **GIVEN** review run D with a codex reviewer that wrote an invalid `out/1.json`, whose status stays `active`, and whose screen does not change
- **WHEN** the agent runs `agk review wait D`
- **THEN** the wait types a fix-up line into the companion with the same stderr warning instead of waiting for a turn end until its timeout, and once the reviewer writes a valid `out/1.json` the wait prints `completed` for round 1

#### Scenario: No fix-up into a running turn
- **GIVEN** review run D with a codex reviewer that wrote an invalid `out/1.json`, whose status stays `active`, and whose screen changes between every two reads, or ends with a codex dialog
- **WHEN** the agent runs `agk review wait D --timeout 30`
- **THEN** no fix-up is typed and the wait prints `running` for round 1 when the timeout passes

#### Scenario: Claude reviewer keeps the status rule
- **GIVEN** review run D with a claude reviewer whose round 1 has `result.json`, whose status stays `active`, and whose screen does not change
- **WHEN** the agent runs `agk review send D --timeout 30`
- **THEN** nothing is typed, no new round is recorded and the command exits 1 saying the reviewer is busy
