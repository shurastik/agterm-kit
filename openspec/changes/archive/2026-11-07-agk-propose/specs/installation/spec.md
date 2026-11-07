# Spec Delta

## MODIFIED Requirements

### Requirement: Dependency report
`agk setup` SHALL report whether `git`, `codex`, `claude` and `openspec` are found on `PATH`, and
whether `agtermctl` is found the way agterm's hooks resolve it (`$AGTERMCTL`, the app bundle,
`PATH`), and whether `revdiff` is found on `PATH`. A missing required tool SHALL make the command
exit 1 after the wiring is done; a missing `agtermctl` or `revdiff` is reported as optional and does
not.

#### Scenario: Missing openspec
- **GIVEN** `openspec` is not on `PATH`
- **WHEN** the user runs `agk setup`
- **THEN** the links and ignore line are still set up, the output names `openspec` as missing, and the exit status is 1

#### Scenario: No agterm
- **GIVEN** agterm is not installed and every required tool is present
- **WHEN** the user runs `agk setup`
- **THEN** the output reports `agtermctl` as optional and missing, and the exit status is 0

#### Scenario: No revdiff
- **GIVEN** `revdiff` is not on `PATH` and every required tool is present
- **WHEN** the user runs `agk setup`
- **THEN** the output reports `revdiff` as optional and missing, and the exit status is 0
