# Spec Delta

## ADDED Requirements

### Requirement: Claude applier turn state
A claude applier SHALL have hooks in its settings that record its turn state in the run's `turn.json` with the round the turn belongs to: on a turn start, `running` with round k when the prompt is round k's kickoff line, else with the round already recorded (none: nothing); on a turn end, `stopped` with that round and the number of background tasks in flight and of scheduled wakeups. The hooks SHALL NOT set the agent status, print anything or fail the turn.

#### Scenario: Turn ends with a background shell in flight
- **GIVEN** a claude applier in round 1 runs a test suite as a background shell and ends its turn
- **WHEN** its `Stop` hook runs
- **THEN** `turn.json` records round 1, `stopped`, 1 background task and 0 wakeups

#### Scenario: Applier resumes by itself
- **GIVEN** `turn.json` records round 1, `stopped`, 1 background task
- **WHEN** the background shell finishes and claude starts a new turn with its task notification
- **THEN** `turn.json` records round 1, `running`

#### Scenario: Turn ends with nothing in flight
- **WHEN** the applier ends a turn of round k with no background task and no scheduled wakeup
- **THEN** `turn.json` records round k, `stopped`, 0 background tasks and 0 wakeups

#### Scenario: Turn of the previous round ends after the next round was published
- **GIVEN** the applier's round-1 turn is still running, `turn.json` records round 1, `running`, and `agk apply send` has already made round 2 the run's current round
- **WHEN** that turn's `Stop` hook runs
- **THEN** `turn.json` records round 1, `stopped`, and nothing is recorded for round 2

#### Scenario: Round 1 resumes while round 2 is being sent
- **GIVEN** `turn.json` records round 1, `stopped` with 1 background task, and the applier starts a turn with its task notification
- **WHEN** `agk apply send` makes round 2 the run's current round before that turn's start hook runs, the send then gives up because the turn is busy and puts the current round back to 1, and the turn ends with another background task in flight
- **THEN** `turn.json` records round 1 at every step, and `agk apply wait D` for round 1 keeps treating the applier as paused

#### Scenario: Round's kickoff line starts the turn
- **GIVEN** `turn.json` records round 1, `stopped` with nothing in flight
- **WHEN** the applier starts a turn whose prompt is the line `Read <run dir>/rounds/2/prompt.md and follow it.`
- **THEN** `turn.json` records round 2, `running`

#### Scenario: Line the user typed into the applier
- **GIVEN** `turn.json` records round 2, `stopped` with nothing in flight
- **WHEN** the user types a question into the companion and a turn starts
- **THEN** `turn.json` records round 2, `running`

#### Scenario: Turn end without a recorded start
- **GIVEN** the run has no `turn.json`
- **WHEN** a `Stop` hook runs
- **THEN** it records nothing

#### Scenario: Hook cannot record
- **GIVEN** the run directory is gone or the hook input is not valid JSON
- **WHEN** a turn-state hook runs
- **THEN** it writes nothing, prints nothing and exits 0, and the applier's turn goes on

#### Scenario: Codex applier
- **GIVEN** the applier resolves to `codex`
- **WHEN** `agk apply start` starts it
- **THEN** no turn-state hooks are installed and no turn state is recorded

#### Scenario: Run directory path that cannot go into a hook command
- **GIVEN** the applier resolves to `claude` and the run directory path contains a backslash or a line break
- **WHEN** `agk apply start` prepares the run
- **THEN** it exits 1 with an error naming the path and starts no companion

## MODIFIED Requirements

### Requirement: Wait for the applier
`agk apply wait <run-dir> [--timeout SECONDS]` (default 3600) SHALL print one JSON object with `status` and `round` and exit 0. The status SHALL be `done` or `blocked` from a valid report of the current round (with the report's fields), `needs-human` when the turn ended without a valid report or the agent asks for permission, `cancelled` when the companion is gone, or `running` on timeout. It SHALL NOT close the companion.

For a run whose applier records its turn state, only a turn state of the current round counts; one of an earlier round counts as missing. A recorded turn state of the current round SHALL count as the round having started. A turn that ended with background tasks in flight or a scheduled wakeup SHALL count as a pause, not as the end of the round: `agk apply wait` SHALL keep waiting until the report appears, the applier resumes and ends a turn, a permission prompt opens, the companion is gone or the timeout passes. A timeout during a pause SHALL print `running` with `reason` `background` and `background` set to the number of tasks and wakeups in flight. While the recorded turn state is `running`, a `completed` or `idle` status SHALL NOT end the wait. A turn state that is still missing after the round started and the status shows the turn over SHALL be waited for up to 30 seconds and then treated as a turn that ended with nothing in flight. Before printing `no-report`, the wait SHALL check the companion once more: gone gives `cancelled`, a permission prompt gives `permission`. Runs without recorded turn state SHALL keep deciding from the agent status alone.

#### Scenario: Round finished
- **GIVEN** the applier of run dir D wrote a valid `done` report for round 1
- **WHEN** the agent runs `agk apply wait D`
- **THEN** stdout is a JSON object with `status` `done`, `round` 1 and the report's `summary`, and the companion stays open

#### Scenario: Permission prompt
- **GIVEN** the applier's agent status becomes `blocked` before it wrote a report
- **WHEN** `agk apply wait D` is waiting
- **THEN** it prints `status` `needs-human` with `reason` `permission`

#### Scenario: Turn ended with a question
- **GIVEN** the applier's turn ends (status `completed` or `idle`), the current round has no report and its turn state records `stopped` with nothing in flight
- **WHEN** `agk apply wait D` sees it
- **THEN** it prints `status` `needs-human` with `reason` `no-report`

#### Scenario: Paused on a background task, then done
- **GIVEN** the applier's status is `completed`, round 1 has no report and its turn state records `stopped` with 1 background task
- **WHEN** `agk apply wait D` runs and the applier later resumes and writes a valid `done` report
- **THEN** the wait does not return before the report exists, and then prints `status` `done`

#### Scenario: Paused past the timeout
- **GIVEN** round 1 has no report and its turn state records `stopped` with 2 background tasks and 0 wakeups
- **WHEN** `agk apply wait D --timeout 60` runs and nothing changes for 60 seconds
- **THEN** it prints `{"status":"running","round":1,"reason":"background","background":2}`

#### Scenario: Resumed turn ends with a question
- **GIVEN** the applier was paused on a background task in round 1
- **WHEN** it resumes, ends that turn with nothing in flight and writes no report
- **THEN** `agk apply wait D` prints `status` `needs-human` with `reason` `no-report`

#### Scenario: Status shows the turn over before the hook ran
- **GIVEN** the applier's status turns `completed` while its turn state still records `running`
- **WHEN** the `Stop` hook then records `stopped` with nothing in flight and there is no report
- **THEN** `agk apply wait D` prints `needs-human` with `reason` `no-report` only after the turn state records `stopped`

#### Scenario: Turn state never recorded
- **GIVEN** a claude run with turn-state hooks whose round 2 has been seen active, has no report and no turn state of round 2
- **WHEN** the status shows the turn over and no turn state of round 2 appears within 30 seconds
- **THEN** `agk apply wait D` prints `needs-human` with `reason` `no-report`

#### Scenario: Previous round's turn state is not the current round's
- **GIVEN** the run is in round 2, `turn.json` records round 1, `stopped` with nothing in flight, and the status still shows round 1's turn over
- **WHEN** `agk apply wait D` runs
- **THEN** it does not print `no-report` on that turn state, and keeps waiting for round 2 to start

#### Scenario: Round started unseen, applier paused
- **GIVEN** `agk apply send` returned `"started": false` for round 2, the applier ran a short turn that was never seen `active`, and `turn.json` records round 2, `stopped` with 1 background task
- **WHEN** `agk apply wait D --timeout 60` runs and nothing changes
- **THEN** it prints `{"status":"running","round":2,"reason":"background","background":1}`

#### Scenario: Round started unseen, turn ended with a question
- **GIVEN** `agk apply send` returned `"started": false` for round 2, and `turn.json` records round 2, `stopped` with nothing in flight and no report exists
- **WHEN** `agk apply wait D` runs
- **THEN** it prints `status` `needs-human` with `reason` `no-report` without waiting for the timeout

#### Scenario: Companion closed after a turn ended without a report
- **GIVEN** `turn.json` records the current round, `stopped` with nothing in flight, there is no report, and the user closed the companion
- **WHEN** `agk apply wait D` runs
- **THEN** it prints `status` `cancelled`

#### Scenario: Run without turn-state hooks
- **GIVEN** a codex applier, or a claude run started by an agk without turn-state hooks
- **WHEN** its turn ends without a report
- **THEN** `agk apply wait D` prints `needs-human` with `reason` `no-report` at once

#### Scenario: Garbage report
- **GIVEN** `rounds/1/report.json` is not a JSON object with a valid `status`
- **WHEN** `agk apply wait D` reads it
- **THEN** it prints `status` `needs-human` with `reason` `invalid-report`

#### Scenario: User closed the applier
- **GIVEN** the companion of D is closed before a report exists
- **WHEN** `agk apply wait D` is waiting
- **THEN** it prints `status` `cancelled`
