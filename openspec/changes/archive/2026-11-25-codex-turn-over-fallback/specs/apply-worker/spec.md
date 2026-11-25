# Spec Delta

## ADDED Requirements

### Requirement: Codex applier runs without the shared daemon
The interactive codex applier SHALL run with `--no-daemon`, so its hooks run in its own process with its own `AGTERM_*` variables and not in a shared codex app-server started from another session. The flag SHALL come after the configured model, effort and `extra_args`, and SHALL appear once, even when `extra_args` already has it. It needs codex 0.156.0 or newer.

#### Scenario: Codex applier argv
- **GIVEN** the agent runs inside agterm and the applier resolves to `codex` with no `extra_args`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the companion's codex command line carries `--no-daemon` once, after the model and effort

#### Scenario: Flag already in extra_args
- **GIVEN** the applier resolves to `codex` with `extra_args = ["--no-daemon"]`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the companion's codex command line carries `--no-daemon` exactly once

#### Scenario: Claude applier unchanged
- **GIVEN** the agent runs inside agterm and the applier resolves to `claude`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the claude command line carries no `--no-daemon`

### Requirement: Codex applier turn end from a settled screen
Before typing a follow-up round into a codex applier whose status does not show the turn over (`completed`, `idle`) or a prompt (`blocked`), `agk apply send` SHALL treat the turn as over when the current round's report file is on disk, two reads of the companion's screen taken 5 s apart are identical, and the screen shows no codex dialog asking to confirm, submit an answer or allow a command. It SHALL then type the line and warn on stderr.

#### Scenario: Status stuck at active after round 1
- **GIVEN** apply run D with a codex applier whose round 1 has `report.json`, whose agent status stays `active`, and whose screen does not change
- **WHEN** the agent runs `agk apply send D findings.md`
- **THEN** the line for round 2 is typed into the companion within about two poll intervals, stderr carries a warning that the applier still reports `active` but its round is on disk and its screen has settled, stdout carries `"round":2`, and the exit code is 0

#### Scenario: No report yet
- **GIVEN** apply run D with a codex applier whose round 1 has no `report.json`, whose status stays `active`, and whose screen does not change
- **WHEN** the agent runs `agk apply send D notes.md --timeout 30`
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the applier is busy

#### Scenario: Screen still changing
- **GIVEN** apply run D with a codex applier whose round 1 has `report.json`, whose status stays `active`, and whose screen changes between every two reads
- **WHEN** the agent runs `agk apply send D notes.md --timeout 30`
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the applier is busy

#### Scenario: Claude applier keeps the status rule
- **GIVEN** apply run D with a claude applier whose round 1 has `report.json`, whose status stays `active`, and whose screen does not change
- **WHEN** the agent runs `agk apply send D notes.md --timeout 30`
- **THEN** nothing is typed, no new round is recorded and the command exits 1 saying the applier is busy
