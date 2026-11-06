# Spec Delta

## MODIFIED Requirements

### Requirement: Run directory
Each review SHALL get a new run directory `.agk/reviews/<timestamp>/` under the repository root holding the prompt (`prompt.md`), the codex log (`codex.log`), the outcome (`result.json` with `status` `completed` or `blocked`, written before the final agterm status), on success the findings file (`review.json`), and, when the review runs in a companion, that companion's session id (`companion`).

#### Scenario: Run directory contents after a successful review
- **WHEN** a review finishes successfully
- **THEN** its run directory contains `prompt.md`, `codex.log`, `review.json` and `result.json` with status `completed`

#### Scenario: Outcome survives a lost status update
- **GIVEN** the final status update to agterm fails
- **WHEN** the review finishes
- **THEN** the update is retried, and `result.json` records the outcome regardless

#### Scenario: Two reviews do not collide
- **WHEN** two reviews are started one after another in the same repository
- **THEN** each has its own run directory and neither overwrites the other's files

#### Scenario: Companion id is recorded
- **GIVEN** the agent runs inside agterm
- **WHEN** `agk review start` creates companion C for a review
- **THEN** the run directory contains a `companion` file holding C before `agk review start` prints its JSON object

#### Scenario: No companion id outside agterm
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** a review runs inline
- **THEN** the run directory has no `companion` file

### Requirement: Companion session inside agterm
When `AGTERM_ENABLED` and `AGTERM_SESSION_ID` are set, `agk review start` SHALL create a terminal companion of the current session named `codex-review #<N>` (N is the review number), in the background without changing the selected session, running the review, and SHALL print one JSON object `{"companion": "<id>", "dir": "<run dir>"}` on stdout and exit 0 without waiting for the review.

#### Scenario: Companion appears under the agent session
- **GIVEN** the agent runs inside agterm session S
- **WHEN** it runs `agk review start --base main`
- **THEN** a session named `codex-review #<N>` appears as a companion under S, S stays selected, and stdout is a JSON object with the companion id and the absolute run directory

#### Scenario: Companion stays visible after the review
- **WHEN** the review inside the companion finishes, successfully or not
- **THEN** the companion session stays open showing the review output until `agk review wait` closes it after a completed review, the user dismisses it, or its parent closes

#### Scenario: Companion id cannot be recorded
- **GIVEN** the `companion` file cannot be written to the run directory
- **WHEN** `agk review start` has created the companion
- **THEN** the command closes that companion, prints an error to stderr and exits non-zero

## ADDED Requirements

### Requirement: Wait for the review outcome
`agk review wait <run-dir> [--timeout SECONDS]` SHALL block until the review in that run directory has an outcome or the timeout (default 1800 seconds) passes, then print one JSON object on stdout whose `status` is `completed`, `blocked`, `cancelled` or `running`, carrying the fields of `result.json` when it exists, and exit 0. `result.json` SHALL decide the outcome whenever it exists.

#### Scenario: Completed review with findings
- **GIVEN** the review in run dir D finishes with 3 findings and verdict `needs-attention`
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout is `{"status":"completed","findings":3,"verdict":"needs-attention"}` and the exit code is 0

#### Scenario: Failed review
- **GIVEN** the review in run dir D fails with reason `codex failed`
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout is a JSON object with `status` `blocked` and `reason` `codex failed`, and the exit code is 0

#### Scenario: Failure before the outcome could be recorded
- **GIVEN** the companion reports `blocked` and D has no `result.json`
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout is `{"status":"blocked"}` and the exit code is 0

#### Scenario: Invalid arguments
- **WHEN** the agent runs `agk review wait` with a path that is not a `.agk/reviews/<run>` directory, or with a `--timeout` that is not a positive integer
- **THEN** the command prints an error to stderr, stdout is empty, the exit code is 1 and no agterm command is executed

### Requirement: Completed review closes its companion
When the outcome is `completed` and the run directory records a companion, `agk review wait` SHALL close that companion so that it can be restored with agterm's undo. A failure to close SHALL be reported as a warning on stderr and SHALL NOT change stdout or the exit code.

#### Scenario: Companion closes after a completed review
- **GIVEN** companion C of run dir D reaches `completed`
- **WHEN** the agent runs `agk review wait D`
- **THEN** C is closed with undo available and the outcome is printed

#### Scenario: Close fails
- **GIVEN** closing companion C fails
- **WHEN** `agk review wait D` sees the outcome `completed`
- **THEN** stderr carries a warning naming C, stdout still carries the outcome and the exit code is 0

### Requirement: Failed review keeps its companion
When the outcome is `blocked`, `agk review wait` SHALL leave the companion open so the failure output stays visible.

#### Scenario: Blocked companion stays
- **GIVEN** companion C of run dir D reaches `blocked`
- **WHEN** the agent runs `agk review wait D`
- **THEN** C is still open after the command exits

### Requirement: Companion gone or wait timed out
When the companion leaves agterm or the timeout passes before an outcome is recorded, `agk review wait` SHALL report `cancelled` or `running` respectively and SHALL NOT close the companion; when `result.json` exists at that point, its outcome SHALL be reported instead, and a `completed` outcome SHALL close the companion only if it is still open.

#### Scenario: User closed the companion mid-review
- **GIVEN** the user closes companion C while the review in D is running and D has no `result.json`
- **WHEN** `agk review wait D` is waiting
- **THEN** it prints `{"status":"cancelled"}` and exits 0

#### Scenario: Timeout while the review still runs
- **GIVEN** the review in D runs longer than the timeout and D has no `result.json`
- **WHEN** the agent runs `agk review wait D --timeout 60`
- **THEN** after about 60 seconds it prints `{"status":"running"}`, exits 0 and C is still open

#### Scenario: Outcome recorded but status lost
- **GIVEN** D has `result.json` with status `completed` but the companion's final agterm status was never set
- **WHEN** the wait times out
- **THEN** the command prints the `completed` outcome and closes the companion

#### Scenario: User closed a finished companion
- **GIVEN** D has `result.json` with status `completed` and the user already closed companion C
- **WHEN** the agent runs `agk review wait D`
- **THEN** the command prints the `completed` outcome, exits 0 and stderr carries no warning

### Requirement: Outcome of an inline review
When the run directory records no companion, `agk review wait` SHALL read the outcome from `result.json` without calling agterm, and SHALL exit 1 with an error when `result.json` is missing.

#### Scenario: Review that ran outside agterm
- **GIVEN** run dir D of an inline review with `result.json` status `completed`
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout carries the `completed` outcome, the exit code is 0 and no agtermctl command is executed

#### Scenario: Inline review without an outcome
- **GIVEN** run dir D with neither a `companion` file nor `result.json`
- **WHEN** the agent runs `agk review wait D`
- **THEN** the command prints an error to stderr and exits 1
