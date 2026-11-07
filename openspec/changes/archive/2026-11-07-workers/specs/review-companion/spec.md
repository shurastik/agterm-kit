## ADDED Requirements

### Requirement: Read-only reviewer with structured output
Each review round SHALL run the configured reviewer harness non-interactively in the repository root, read-only, constrained to the bundled review output schema: codex through `codex exec` with a read-only sandbox, claude in print mode allowed only file reading, search and read-only git commands.

#### Scenario: Codex reviewer cannot modify the repository
- **GIVEN** the reviewer resolves to `codex`
- **WHEN** a round runs
- **THEN** codex runs with a read-only sandbox and the bundled schema, and the working tree is unchanged by the review

#### Scenario: Claude reviewer cannot modify the repository
- **GIVEN** the reviewer resolves to `claude` with model `claude-opus-5-5`
- **WHEN** a round runs
- **THEN** claude runs in print mode with model `claude-opus-5-5`, the bundled schema as its output schema, no edit, write or arbitrary shell tool allowed, and the working tree is unchanged by the review

### Requirement: Re-review round
`agk review send <run-dir> [--note FILE]` SHALL start round k+1 of a finished round k: rebuild the scope with the run's recorded base, write a re-review prompt (with the note, if given) to `rounds/<k+1>/`, record round k+1 and hand it to the reviewer, which SHALL resume its harness session from earlier rounds. It SHALL print `{"companion": <id or null>, "dir": "<run dir>", "round": k+1}`.

#### Scenario: Second round remembers the first
- **GIVEN** codex review run D whose round 1 completed with 2 findings and recorded codex session T
- **WHEN** the agent runs `agk review send D --note fixed.md`
- **THEN** round 2 resumes codex session T with a prompt that asks to re-check the earlier findings and includes the note, and its outcome lands in `rounds/2/`

#### Scenario: Nothing left to review
- **GIVEN** the diff against the recorded base is empty
- **WHEN** the agent runs `agk review send D`
- **THEN** it prints `nothing to review` to stderr, exits 1 and records no new round

#### Scenario: Round still running
- **GIVEN** round 1 of D has no `result.json` yet
- **WHEN** the agent runs `agk review send D`
- **THEN** it exits 1 saying the round is still running and records no new round

#### Scenario: No session to resume
- **GIVEN** round 1 of D failed before the reviewer reported a session id
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 starts a fresh reviewer session with the first-round prompt

#### Scenario: Re-review outside agterm
- **GIVEN** a review run D with no companion
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 runs in the foreground without calling agterm and stdout ends with the JSON object whose `companion` is `null`

### Requirement: Reviewer companion waits between rounds
After each round, successful or failed, the process in the reviewer companion SHALL stay running and wait for the next round request from `agk review send`, keeping the round's output visible.

#### Scenario: Request arrives
- **GIVEN** companion C of D finished round 1
- **WHEN** the agent runs `agk review send D`
- **THEN** C's status becomes `active` with info line `<harness> #<N> r2: reviewing` and round 2 runs in C

### Requirement: Close the reviewer
`agk review close <run-dir>` SHALL close the run's companion so it can be restored with agterm's undo, and SHALL exit 0 when the companion is already gone or the run has none.

#### Scenario: Close after approval
- **GIVEN** review run D whose last round approved and whose companion is open
- **WHEN** the agent runs `agk review close D`
- **THEN** the companion is closed with undo available and the exit code is 0

## MODIFIED Requirements

### Requirement: Run directory
Each review SHALL get a run directory `.agk/reviews/<timestamp>/` under the repository root holding the resolved reviewer configuration and base, the current round, the reviewer session id once known and, in agterm, the companion id (`companion`). Each round k SHALL get `rounds/<k>/` with `prompt.md`, `reviewer.log`, `result.json` (`completed` or `blocked`, written before the final agterm status) and, on success, `review.json`.

#### Scenario: Run directory contents after a successful review
- **WHEN** round 1 of a review finishes successfully
- **THEN** its run directory has `rounds/1/` containing `prompt.md`, `reviewer.log`, `review.json` and `result.json` with status `completed`

#### Scenario: Outcome survives a lost status update
- **GIVEN** the final status update to agterm fails
- **WHEN** the round finishes
- **THEN** the update is retried, and the round's `result.json` records the outcome regardless

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
When `AGTERM_ENABLED` and `AGTERM_SESSION_ID` are set, `agk review start` SHALL create a terminal companion of the current session named `<harness>-review #<N>` (N is the review number), in the background without changing the selected session, running round 1, and SHALL print one JSON object `{"companion": "<id>", "dir": "<run dir>"}` on stdout and exit 0 without waiting for the review.

#### Scenario: Companion appears under the agent session
- **GIVEN** the agent runs inside agterm session S and the reviewer resolves to `codex`
- **WHEN** it runs `agk review start --base main`
- **THEN** a session named `codex-review #<N>` appears as a companion under S, S stays selected, and stdout is a JSON object with the companion id and the absolute run directory

#### Scenario: Companion stays visible after the review
- **WHEN** a round inside the companion finishes, successfully or not
- **THEN** the companion session stays open showing the review output until `agk review close`, the user dismisses it, or its parent closes

#### Scenario: Companion id cannot be recorded
- **GIVEN** the `companion` file cannot be written to the run directory
- **WHEN** `agk review start` has created the companion
- **THEN** the command closes that companion, prints an error to stderr and exits non-zero

### Requirement: Progress and outcome status
The review process SHALL report through the companion's agterm status: `active` with info line `<harness> #<N> r<k>: reviewing` while round k runs; `completed` with an info line counting findings by severity on success; `blocked` with an info line stating the failure reason on any failure.

#### Scenario: Findings reported
- **WHEN** round 1 of codex review #4 returns 3 findings, one of them `high`
- **THEN** the companion status becomes `completed` and its info line reads `codex #4 r1: 3 findings (1 high)`

#### Scenario: Clean review
- **WHEN** round 2 of codex review #4 returns verdict `approve` with no findings
- **THEN** the companion status becomes `completed` and its info line reads `codex #4 r2: approve, no findings`

#### Scenario: Codex fails
- **WHEN** the reviewer process exits non-zero or cannot be started
- **THEN** the companion status becomes `blocked`, its info line names the failure, no `review.json` is written for the round, and the companion keeps waiting for the next round

#### Scenario: Agent waits for the outcome
- **GIVEN** a review started with `agk review start`
- **WHEN** the agent runs `agtermctl session wait --target <companion> --until completed --until blocked`
- **THEN** the wait returns once round 1 has finished, with the matched status

### Requirement: Review numbering
Each review SHALL have a number N: the 1-based position of its run directory among the run directories of the repository, ordered by creation time. The number SHALL stay the same for the whole run, across all its rounds.

#### Scenario: Numbers count up per repository
- **GIVEN** a repository with seven earlier run directories under `.agk/reviews/`
- **WHEN** a codex review is started
- **THEN** it is review #8: the companion is named `codex-review #8` and its info lines start with `codex #8 r1:`

#### Scenario: Later runs do not renumber earlier ones
- **WHEN** another review is started while review #8 is still running
- **THEN** the running review keeps number 8 in its status updates and the new one is #9

### Requirement: Timestamped review log
The review process SHALL print a header line with the review number, round, start date and time and run directory, and SHALL show the reviewer's progress as readable lines, prefixing every output line, in the companion terminal and in the round's `reviewer.log`, with the local date and time `[YYYY-MM-DD HH:MM:SS]`.

#### Scenario: Log lines carry date and time
- **WHEN** the reviewer reports `thinking` and then `done` during round 1 of review #3 started at 2026-11-05 17:53:55
- **THEN** the first line reads `[2026-11-05 17:53:55] agk: review #3 round 1 started, run dir <dir>` and the progress is shown as `[<date> <time>] thinking` and `[<date> <time>] done`, both on screen and in `rounds/1/reviewer.log`

### Requirement: Untrusted output cannot drive the terminal
Reviewer output shown in the companion terminal and written to `reviewer.log` SHALL carry terminal control characters and bidi overrides only as visible `\uXXXX` (or `\xNN` for invalid bytes) escapes, including sequences split across output chunks.

#### Scenario: Escape sequence from a reviewed file
- **GIVEN** an untracked file containing an OSC 52 clipboard sequence and a screen-clear sequence
- **WHEN** the reviewer echoes it while reviewing
- **THEN** the companion terminal and `reviewer.log` show the sequence as visible escapes and contain no raw ESC or BEL byte

### Requirement: Findings validation
The review SHALL accept reviewer output only if it is a JSON object matching the review output schema; otherwise it SHALL treat the round as failed and SHALL NOT write `review.json`.

#### Scenario: Malformed codex output
- **WHEN** reviewer output is not valid JSON, or a finding has a severity outside `critical|high|medium|low` or a missing required field
- **THEN** the round is reported as failed with reason `invalid review output` and the raw output is kept in the round directory for inspection

### Requirement: Wait for the review outcome
`agk review wait <run-dir> [--timeout SECONDS]` SHALL block until the current round has an outcome or the timeout (default 1800 seconds) passes, then print one JSON object on stdout with `round` and a `status` of `completed`, `blocked`, `cancelled` or `running`, carrying the fields of the round's `result.json` when it exists, and exit 0. That `result.json` SHALL decide the outcome whenever it exists.

#### Scenario: Completed review with findings
- **GIVEN** round 1 of run dir D finishes with 3 findings and verdict `needs-attention`
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout is `{"status":"completed","round":1,"findings":3,"verdict":"needs-attention"}`, the exit code is 0 and the companion stays open

#### Scenario: Failed review
- **GIVEN** round 1 of run dir D fails with reason `codex failed`
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout is a JSON object with `status` `blocked`, `round` 1 and `reason` `codex failed`, and the exit code is 0

#### Scenario: Failure before the outcome could be recorded
- **GIVEN** the companion reports `blocked` and the current round of D has no `result.json`
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout is `{"status":"blocked","round":1}` and the exit code is 0

#### Scenario: Invalid arguments
- **WHEN** the agent runs `agk review wait` with a path that is not a `.agk/reviews/<run>` directory, or with a `--timeout` that is not a positive integer
- **THEN** the command prints an error to stderr, stdout is empty, the exit code is 1 and no agterm command is executed

### Requirement: Companion gone or wait timed out
When the companion leaves agterm or the timeout passes before the current round has an outcome, `agk review wait` SHALL report `cancelled` or `running` respectively; when the round's `result.json` exists at that point, its outcome SHALL be reported instead. `agk review wait` SHALL NOT close the companion.

#### Scenario: User closed the companion mid-review
- **GIVEN** the user closes companion C while round 1 of D is running and has no `result.json`
- **WHEN** `agk review wait D` is waiting
- **THEN** it prints `{"status":"cancelled","round":1}` and exits 0

#### Scenario: Timeout while the review still runs
- **GIVEN** round 1 of D runs longer than the timeout and has no `result.json`
- **WHEN** the agent runs `agk review wait D --timeout 60`
- **THEN** after about 60 seconds it prints `{"status":"running","round":1}`, exits 0 and C is still open

#### Scenario: Outcome recorded but status lost
- **GIVEN** round 1 of D has `result.json` with status `completed` but the companion's final agterm status was never set
- **WHEN** the wait times out
- **THEN** the command prints the `completed` outcome

#### Scenario: User closed a finished companion
- **GIVEN** round 1 of D has `result.json` with status `completed` and the user already closed companion C
- **WHEN** the agent runs `agk review wait D`
- **THEN** the command prints the `completed` outcome, exits 0 and stderr carries no warning

### Requirement: Outcome of an inline review
When the run directory records no companion, `agk review wait` SHALL read the outcome from the current round's `result.json` without calling agterm, and SHALL exit 1 with an error when that file is missing.

#### Scenario: Review that ran outside agterm
- **GIVEN** run dir D of an inline review whose round 1 `result.json` has status `completed`
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout carries the `completed` outcome, the exit code is 0 and no agtermctl command is executed

#### Scenario: Inline review without an outcome
- **GIVEN** run dir D with no `companion` file and no `result.json` for its current round
- **WHEN** the agent runs `agk review wait D`
- **THEN** the command prints an error to stderr and exits 1

## REMOVED Requirements

### Requirement: Read-only codex run with structured output
**Reason**: The reviewer harness is now configurable; the codex-only requirement is replaced by "Read-only reviewer with structured output", which keeps the codex behavior and adds claude.
**Migration**: None for codex users; the default reviewer is still codex with a read-only sandbox and the bundled schema.

### Requirement: Completed review closes its companion
**Reason**: The reviewer now stays alive between rounds so a re-review can resume its session; closing is the caller's decision.
**Migration**: Run `agk review close <run-dir>` once the review loop is over.
