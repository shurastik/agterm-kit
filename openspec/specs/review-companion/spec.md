# review-companion Specification

## Purpose
Runs a codex code review of a git diff as a visible agterm companion session under the agent's session, producing a validated structured findings file and signalling progress and completion through agterm status so the agent can wait for it.

## Requirements

### Requirement: Review scope from git
`agk review start` SHALL review the changes between the merge base of `--base` (default `main`) and the working tree: committed changes, staged and unstaged tracked changes, and untracked files that are not ignored.

#### Scenario: Branch commits plus uncommitted edits
- **GIVEN** a branch with commits on top of `main`, one modified tracked file and one new untracked file
- **WHEN** the user runs `agk review start --base main`
- **THEN** the review prompt contains the committed diff, the uncommitted modification and the untracked file's content

#### Scenario: Ignored files are excluded
- **GIVEN** an untracked file matched by `.gitignore`
- **WHEN** a review is started
- **THEN** that file does not appear in the review prompt

### Requirement: Nothing to review
`agk review start` SHALL exit non-zero with the message `nothing to review` and SHALL NOT create a companion or run codex when the review scope is empty.

#### Scenario: Clean tree on base
- **GIVEN** a repository with no changes relative to the merge base and no untracked files
- **WHEN** the user runs `agk review start`
- **THEN** the command exits with a non-zero status, prints `nothing to review` to stderr, and no companion session appears

### Requirement: Run directory
Each review SHALL get a new run directory `.agk/reviews/<timestamp>/` under the repository root holding the prompt (`prompt.md`), the codex log (`codex.log`), the outcome (`result.json` with `status` `completed` or `blocked`, written before the final agterm status) and, on success, the findings file (`review.json`).

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

### Requirement: Companion session inside agterm
When `AGTERM_ENABLED` and `AGTERM_SESSION_ID` are set, `agk review start` SHALL create a terminal companion of the current session named `codex-review #<N>` (N is the review number), in the background without changing the selected session, running the review, and SHALL print one JSON object `{"companion": "<id>", "dir": "<run dir>"}` on stdout and exit 0 without waiting for the review.

#### Scenario: Companion appears under the agent session
- **GIVEN** the agent runs inside agterm session S
- **WHEN** it runs `agk review start --base main`
- **THEN** a session named `codex-review #<N>` appears as a companion under S, S stays selected, and stdout is a JSON object with the companion id and the absolute run directory

#### Scenario: Companion stays visible after the review
- **WHEN** the review inside the companion finishes, successfully or not
- **THEN** the companion session stays open showing the review output until the user dismisses it or its parent closes

### Requirement: Progress and outcome status
The review process SHALL report through the companion's agterm status: `active` with info line `codex #<N>: reviewing` while running; `completed` with an info line counting findings by severity on success; `blocked` with an info line stating the failure reason on any failure.

#### Scenario: Findings reported
- **WHEN** codex returns 3 findings, one of them `high`
- **THEN** the companion status becomes `completed` and its info line reads `codex #<N>: 3 findings (1 high)`

#### Scenario: Clean review
- **WHEN** codex returns verdict `approve` with no findings
- **THEN** the companion status becomes `completed` and its info line reads `codex #<N>: approve, no findings`

#### Scenario: Codex fails
- **WHEN** the codex process exits non-zero or cannot be started
- **THEN** the companion status becomes `blocked`, its info line names the failure, no `review.json` is written, and the review process exits non-zero

#### Scenario: Agent waits for the outcome
- **GIVEN** a review started with `agk review start`
- **WHEN** the agent runs `agtermctl session wait --target <companion> --until completed --until blocked`
- **THEN** the wait returns once the review has finished, with the matched status

### Requirement: Review numbering
Each review SHALL have a number N: the 1-based position of its run directory among the run directories of the repository, ordered by creation time. The number SHALL stay the same for the whole run.

#### Scenario: Numbers count up per repository
- **GIVEN** a repository with seven earlier run directories under `.agk/reviews/`
- **WHEN** a review is started
- **THEN** it is review #8: the companion is named `codex-review #8` and its info lines start with `codex #8:`

#### Scenario: Later runs do not renumber earlier ones
- **WHEN** another review is started while review #8 is still running
- **THEN** the running review keeps number 8 in its status updates and the new one is #9

### Requirement: Timestamped review log
The review process SHALL print a header line with the review number, start date and time and run directory, and SHALL prefix every output line, in the companion terminal and in `codex.log`, with the local date and time `[YYYY-MM-DD HH:MM:SS]`.

#### Scenario: Log lines carry date and time
- **WHEN** codex prints `thinking` and then `done` during review #3 started at 2026-11-05 17:53:55
- **THEN** the first line reads `[2026-11-05 17:53:55] agk: review #3 started, run dir <dir>` and each codex line is shown as `[<date> <time>] thinking` and `[<date> <time>] done`, both on screen and in `codex.log`

### Requirement: Untrusted output cannot drive the terminal
Reviewer output shown in the companion terminal and written to `codex.log` SHALL carry terminal control characters and bidi overrides only as visible `\uXXXX` (or `\xNN` for invalid bytes) escapes, including sequences split across output chunks.

#### Scenario: Escape sequence from a reviewed file
- **GIVEN** an untracked file containing an OSC 52 clipboard sequence and a screen-clear sequence
- **WHEN** codex echoes it while reviewing
- **THEN** the companion terminal and `codex.log` show the sequence as visible escapes and contain no raw ESC or BEL byte

### Requirement: Read-only codex run with structured output
The review SHALL run codex non-interactively with a read-only sandbox in the repository root, constrained to the bundled review output schema (verdict, summary, findings with severity/title/body/file/line range/confidence/recommendation, next steps).

#### Scenario: Codex cannot modify the repository
- **WHEN** a review runs
- **THEN** codex is invoked with a read-only sandbox and the repository working tree is unchanged by the review

### Requirement: Findings validation
The review SHALL accept codex output only if it is a JSON object matching the review output schema; otherwise it SHALL treat the run as failed and SHALL NOT write `review.json`.

#### Scenario: Malformed codex output
- **WHEN** codex output is not valid JSON, or a finding has a severity outside `critical|high|medium|low` or a missing required field
- **THEN** the run is reported as failed with reason `invalid review output` and the raw output is kept in the run directory for inspection

### Requirement: Inline fallback outside agterm
When `AGTERM_ENABLED` is not set, `agk review start` SHALL run the review in the current process without creating a companion or calling agterm, and SHALL print `{"companion": null, "dir": "<run dir>"}` after it finishes, exiting 0 on success and non-zero on failure.

#### Scenario: Review outside agterm
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the user runs `agk review start`
- **THEN** no agtermctl command is executed, the review runs to completion in the foreground, and stdout ends with the JSON object whose `companion` is `null`
