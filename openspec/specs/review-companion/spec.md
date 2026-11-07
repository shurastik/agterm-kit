# review-companion Specification

## Purpose
Runs a codex code review of a git diff as a visible agterm companion session under the agent's session, producing a validated structured findings file and signalling progress and completion through agterm status so the agent can wait for it.

## Requirements

### Requirement: Review scope from git
`agk review start` without `--change` (a diff review) SHALL review the changes between the merge base of `--base` (default `main`) and the working tree: committed changes, staged and unstaged tracked changes, and untracked files that are not ignored.

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
Each review SHALL get a run directory `.agk/reviews/<timestamp>/` under the repository root holding the resolved reviewer configuration, the review target (the base of a diff review, or the change of a plan review), the current round, the reviewer session id once known and, in agterm, the companion id (`companion`). Each round k SHALL get `rounds/<k>/` with `prompt.md`, `reviewer.log`, `result.json` (`completed` or `blocked`, written before the final agterm status) and, on success, `review.json`.

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

#### Scenario: Target of each kind is recorded
- **WHEN** a diff review with `--base main` and a plan review with `--change add-x` are started
- **THEN** the first run records base `main` and no change, the second records change `add-x` and no base

#### Scenario: Run directory from before plan reviews
- **GIVEN** a run directory whose recorded state has a base and no change field
- **WHEN** the agent runs `agk review send` or `agk review wait` on it
- **THEN** it is treated as a diff review against that base

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

### Requirement: Inline fallback outside agterm
When `AGTERM_ENABLED` is not set, `agk review start` SHALL run the review in the current process without creating a companion or calling agterm, and SHALL print `{"companion": null, "dir": "<run dir>"}` after it finishes, exiting 0 on success and non-zero on failure.

#### Scenario: Review outside agterm
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the user runs `agk review start`
- **THEN** no agtermctl command is executed, the review runs to completion in the foreground, and stdout ends with the JSON object whose `companion` is `null`

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

### Requirement: Failed review keeps its companion
When the outcome is `blocked`, `agk review wait` SHALL leave the companion open so the failure output stays visible.

#### Scenario: Blocked companion stays
- **GIVEN** companion C of run dir D reaches `blocked`
- **WHEN** the agent runs `agk review wait D`
- **THEN** C is still open after the command exits

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
`agk review send <run-dir> [--note FILE]` SHALL start round k+1 of a finished round k: rebuild the run's scope from its recorded target (the diff against the recorded base, or the plan scope of the recorded change), write a re-review prompt (with the note, if given) to `rounds/<k+1>/`, record round k+1 and hand it to the reviewer, which SHALL resume its harness session from earlier rounds. It SHALL print `{"companion": <id or null>, "dir": "<run dir>", "round": k+1}`.

#### Scenario: Second round remembers the first
- **GIVEN** codex review run D whose round 1 completed with 2 findings and recorded codex session T
- **WHEN** the agent runs `agk review send D --note fixed.md`
- **THEN** round 2 resumes codex session T with a prompt that asks to re-check the earlier findings and includes the note, and its outcome lands in `rounds/2/`

#### Scenario: Nothing left to review
- **GIVEN** a diff review run D and the diff against the recorded base is empty
- **WHEN** the agent runs `agk review send D`
- **THEN** it prints `nothing to review` to stderr, exits 1 and records no new round

#### Scenario: Plan re-review on a clean tree
- **GIVEN** a plan review run D of change `add-x`, whose files are committed, and a clean working tree
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 starts with the change's files in its prompt; the empty git diff does not matter

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

### Requirement: Plan review scope
`agk review start --change <name>` SHALL review the planning artifacts of OpenSpec change `<name>`: every file under `openspec/changes/<name>/` of the repository root, and the main spec `openspec/specs/<capability>/spec.md` of each capability that has a delta under the change's `specs/`. Files SHALL be read from disk whatever their git state; nothing else in the repository is part of the scope.

#### Scenario: Untracked change on a dirty main
- **GIVEN** change `add-x` exists only as untracked files on `main` and an unrelated tracked file has an uncommitted edit
- **WHEN** the user runs `agk review start --change add-x`
- **THEN** the review prompt contains the content of every file of `openspec/changes/add-x/` and does not contain the unrelated edit

#### Scenario: Committed change on a clean tree
- **GIVEN** change `add-x` is committed and the working tree is clean
- **WHEN** the user runs `agk review start --change add-x`
- **THEN** the review starts and its prompt contains the change's files

#### Scenario: Main spec of a modified capability
- **GIVEN** change `add-x` has a delta `specs/review-companion/spec.md` and `openspec/specs/review-companion/spec.md` exists
- **WHEN** a plan review of `add-x` is started
- **THEN** the prompt contains the current main `review-companion` spec, marked apart from the change's files

#### Scenario: New capability without a main spec
- **GIVEN** change `add-x` has a delta `specs/new-cap/spec.md` and there is no `openspec/specs/new-cap/`
- **WHEN** a plan review of `add-x` is started
- **THEN** the review starts and the prompt carries the delta with no main spec for `new-cap`

#### Scenario: Symlink inside the change
- **GIVEN** a file in the change directory is a symlink
- **WHEN** a plan review is started
- **THEN** the prompt lists that path as not followed and does not include its target's content

### Requirement: Plan review target checks
`agk review start` SHALL exit non-zero without creating a run directory or a companion when `--change` names an invalid change name (not kebab-case), a change directory that does not exist, or is combined with `--base`. A change with no files SHALL get `nothing to review`.

#### Scenario: Unknown change
- **GIVEN** there is no `openspec/changes/add-y/`
- **WHEN** the user runs `agk review start --change add-y`
- **THEN** the command exits 1 naming the missing change, and no run directory or companion is created

#### Scenario: Change together with a base
- **WHEN** the user runs `agk review start --change add-x --base main`
- **THEN** the command exits 1 saying the two flags exclude each other

#### Scenario: Path-like change name
- **WHEN** the user runs `agk review start --change ../secrets`
- **THEN** the command exits 1 rejecting the name and reads no file

#### Scenario: Empty change directory
- **GIVEN** `openspec/changes/add-x/` exists and holds no files
- **WHEN** the user runs `agk review start --change add-x`
- **THEN** it prints `nothing to review` to stderr and exits non-zero

### Requirement: Plan review prompt
A plan review round SHALL use a plan-review prompt that names the change and asks for material findings in the planning artifacts: contradictions between proposal, specs, design and tasks; requirements without testable scenarios; deltas that conflict with the main specs; requirements no task implements; design claims that do not match the code. It SHALL mark all inlined file content as untrusted data.

#### Scenario: Prompt targets the plan
- **WHEN** round 1 of a plan review of `add-x` is written
- **THEN** its `prompt.md` names `add-x`, asks to review planning artifacts rather than a code diff, and wraps every inlined file in a block declared as data, not instructions

#### Scenario: Findings point at artifact lines
- **WHEN** a plan review round completes with findings
- **THEN** `review.json` matches the bundled review output schema, with file paths relative to the repository root

### Requirement: Plan re-review round
A plan review run SHALL record its change in the run directory instead of a base, and `agk review send` on such a run SHALL rebuild the plan scope of that change from disk for round k+1 and resume the reviewer's session like any re-review.

#### Scenario: Re-review sees edited artifacts
- **GIVEN** plan review run D of `add-x` whose round 1 completed with findings, after which `tasks.md` was edited
- **WHEN** the agent runs `agk review send D --note fixed.md`
- **THEN** round 2's prompt is a re-review prompt with the note and the edited `tasks.md`, and it resumes the round 1 reviewer session

#### Scenario: Change gone before a re-review
- **GIVEN** plan review run D of `add-x` and `openspec/changes/add-x/` no longer exists
- **WHEN** the agent runs `agk review send D`
- **THEN** it exits 1 naming the missing change and records no new round
