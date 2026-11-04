# Spec Delta

## Purpose

Runs a codex code review of a git diff as a visible agterm companion session under the agent's session, producing a validated structured findings file and signalling progress and completion through agterm status so the agent can wait for it.

## ADDED Requirements

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
Each review SHALL get a new run directory `.agk/reviews/<timestamp>/` under the repository root holding the prompt (`prompt.md`), the codex log (`codex.log`) and, on success, the findings file (`review.json`).

#### Scenario: Run directory contents after a successful review
- **WHEN** a review finishes successfully
- **THEN** its run directory contains `prompt.md`, `codex.log` and `review.json`

#### Scenario: Two reviews do not collide
- **WHEN** two reviews are started one after another in the same repository
- **THEN** each has its own run directory and neither overwrites the other's files

### Requirement: Companion session inside agterm
When `AGTERM_ENABLED` and `AGTERM_SESSION_ID` are set, `agk review start` SHALL create a terminal companion of the current session named `codex-review`, in the background without changing the selected session, running the review, and SHALL print one JSON object `{"companion": "<id>", "dir": "<run dir>"}` on stdout and exit 0 without waiting for the review.

#### Scenario: Companion appears under the agent session
- **GIVEN** the agent runs inside agterm session S
- **WHEN** it runs `agk review start --base main`
- **THEN** a session named `codex-review` appears as a companion under S, S stays selected, and stdout is a JSON object with the companion id and the absolute run directory

#### Scenario: Companion stays visible after the review
- **WHEN** the review inside the companion finishes, successfully or not
- **THEN** the companion session stays open showing the review output until the user dismisses it or its parent closes

### Requirement: Progress and outcome status
The review process SHALL report through the companion's agterm status: `active` with info line `codex: reviewing` while running; `completed` with an info line counting findings by severity on success; `blocked` with an info line stating the failure reason on any failure.

#### Scenario: Findings reported
- **WHEN** codex returns 3 findings, one of them `high`
- **THEN** the companion status becomes `completed` and its info line reads `codex: 3 findings (1 high)`

#### Scenario: Clean review
- **WHEN** codex returns verdict `approve` with no findings
- **THEN** the companion status becomes `completed` and its info line reads `codex: approve, no findings`

#### Scenario: Codex fails
- **WHEN** the codex process exits non-zero or cannot be started
- **THEN** the companion status becomes `blocked`, its info line names the failure, no `review.json` is written, and the review process exits non-zero

#### Scenario: Agent waits for the outcome
- **GIVEN** a review started with `agk review start`
- **WHEN** the agent runs `agtermctl session wait --target <companion> --until completed --until blocked`
- **THEN** the wait returns once the review has finished, with the matched status

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
