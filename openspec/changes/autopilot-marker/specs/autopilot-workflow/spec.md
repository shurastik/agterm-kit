# Spec Delta

## ADDED Requirements

### Requirement: Autopilot mark
After its preconditions passed and before it writes anything, the skill SHALL run `agk autopilot on`. It SHALL run `agk autopilot on` again right after every verify run. It SHALL pass `--autopilot` to every `agk apply start` and `agk review start` it runs. On every exit, finished or stopped, it SHALL run `agk autopilot off` before its final report. A `needs-human` it waits on again is not an exit and SHALL NOT turn the mark off. A failed `on` or `off` SHALL NOT stop the run.

#### Scenario: Mark on from the start
- **WHEN** the user invokes `/agk-autopilot add a widget` in a project that passes the preconditions
- **THEN** `agk autopilot on` runs before the change is written, and every `agk apply start` and `agk review start` of the run carries `--autopilot`

#### Scenario: Mark renewed after verify
- **GIVEN** the code review approved
- **WHEN** the skill has run `openspec-verify-change`
- **THEN** `agk autopilot on` runs before the next `agk` command or the end of the run

#### Scenario: Mark off on a finished run
- **GIVEN** the last code review approved and verify after it reported 0 CRITICAL
- **WHEN** the skill ends
- **THEN** `agk autopilot off` runs after the last `agk` command of the run and before the final report

#### Scenario: Mark off on a stop
- **GIVEN** `agk review wait` returns `cancelled`
- **WHEN** the skill stops
- **THEN** `agk autopilot off` runs before the report

#### Scenario: Needs-human keeps the mark
- **GIVEN** `agk apply wait` returns `needs-human` with reason `permission`
- **WHEN** the skill tells the user and runs the wait again
- **THEN** no `agk autopilot off` runs in between

#### Scenario: No mark without the preconditions
- **GIVEN** a repository with no `openspec-propose` skill
- **WHEN** the user invokes `/agk-autopilot add a widget`
- **THEN** no `agk autopilot on` runs
