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
Each review SHALL get a run directory `.agk/reviews/<timestamp>/` under the repository root. It holds:
- the resolved reviewer configuration;
- the review target (the base of a diff review, or the change of a plan review);
- whether the reviewer runs interactively;
- the current round;
- the reviewer session id once known;
- in agterm, the companion id (`companion`), the review output schema (`schema.json`) and the reviewer's output directory `out/`.

Each round k SHALL get `rounds/<k>/` with `prompt.md`, `result.json` (`completed` or `blocked`) and, on success, `review.json`. An inline round SHALL also get `reviewer.log`.

#### Scenario: Run directory contents after a successful review
- **WHEN** round 1 of a review finishes successfully
- **THEN** its run directory has `rounds/1/` containing `prompt.md`, `review.json` and `result.json` with status `completed`, plus `reviewer.log` when the review ran inline

#### Scenario: Outcome survives a lost status update
- **GIVEN** the final info line update to agterm fails
- **WHEN** the round's outcome is recorded
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
- **THEN** the run directory has no `companion` file and records that the reviewer does not run interactively

#### Scenario: Target of each kind is recorded
- **WHEN** a diff review with `--base main` and a plan review with `--change add-x` are started
- **THEN** the first run records base `main` and no change, the second records change `add-x` and no base

#### Scenario: Run directory from before plan reviews
- **GIVEN** a run directory whose recorded state has a base and no change field
- **WHEN** the agent runs `agk review send` or `agk review wait` on it
- **THEN** it is treated as a diff review against that base

### Requirement: Companion session inside agterm
When `AGTERM_ENABLED` and `AGTERM_SESSION_ID` are set, `agk review start` SHALL create a terminal companion of the current session named `<harness>-review #<N>`, where N is the review number. The companion is created in the background without changing the selected session. It SHALL run the reviewer harness interactively, and round 1 SHALL be started by a single line telling the reviewer to read `rounds/1/prompt.md`. The command SHALL print one JSON object `{"companion": "<id>", "dir": "<run dir>", "started": <bool>}` on stdout and exit 0 without waiting for the review.

#### Scenario: Companion appears under the agent session
- **GIVEN** the agent runs inside agterm session S and the reviewer resolves to `codex`
- **WHEN** it runs `agk review start --base main`
- **THEN** a session named `codex-review #<N>` appears as a companion under S, S stays selected, the companion runs an interactive codex session whose first prompt points at `rounds/1/prompt.md`, and stdout is a JSON object with the companion id, the absolute run directory and `started`

#### Scenario: Companion stays visible after the review
- **WHEN** a round in the companion finishes, successfully or not
- **THEN** the companion session stays open showing the reviewer's session until `agk review close`, the user dismisses it, or its parent closes

#### Scenario: Companion id cannot be recorded
- **GIVEN** the `companion` file cannot be written to the run directory
- **WHEN** `agk review start` has created the companion
- **THEN** the command closes that companion, prints an error to stderr and exits non-zero

### Requirement: Progress and outcome status
Inside agterm, the companion's agent status SHALL come only from the reviewer harness's turns: `active` while a turn runs, `completed` or `idle` when it ends, `blocked` while the reviewer waits for a permission answer. agk SHALL NOT set it. agk SHALL set the info line instead:
- `<harness> #<N> r<k>: reviewing` when it starts or sends round k;
- a line counting findings by severity when `agk review wait` records a completed round;
- a line stating the failure reason when it records a failed round.

#### Scenario: Findings reported
- **WHEN** `agk review wait` records round 1 of codex review #4 with 3 findings, one of them `high`
- **THEN** the companion's info line reads `codex #4 r1: 3 findings (1 high)` and its agent status is whatever the reviewer's turn reports

#### Scenario: Clean review
- **WHEN** `agk review wait` records round 2 of codex review #4 with verdict `approve` and no findings
- **THEN** the companion's info line reads `codex #4 r2: approve, no findings`

#### Scenario: Codex fails
- **WHEN** `agk review wait` records round 1 of codex review #4 as failed with reason `invalid review output`
- **THEN** the companion's info line names the failure, its agent status is not set to `blocked` by agk, no `review.json` is written for the round, and the reviewer session stays open for the next round

#### Scenario: Agent waits for the outcome
- **GIVEN** a review started with `agk review start` in agterm
- **WHEN** the agent runs `agk review wait <dir>`
- **THEN** the wait returns once round 1 has an outcome or the reviewer needs the user, with the matching status

#### Scenario: Round in progress
- **WHEN** `agk review send` starts round 2 of claude review #5
- **THEN** the companion's info line reads `claude #5 r2: reviewing`

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
The inline review SHALL print a header line with the review number, round, start date and time and run directory. It SHALL show the reviewer's progress as readable lines. Every output line, in the terminal and in the round's `reviewer.log`, SHALL carry the local date and time prefix `[YYYY-MM-DD HH:MM:SS]`. Inside agterm the companion SHALL show the harness's own interface instead.

#### Scenario: Log lines carry date and time
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the reviewer reports `thinking` and then `done` during round 1 of review #3 started at 2026-11-05 17:53:55
- **THEN** the first line reads `[2026-11-05 17:53:55] agk: review #3 round 1 started, run dir <dir>` and the progress is shown as `[<date> <time>] thinking` and `[<date> <time>] done`, both on screen and in `rounds/1/reviewer.log`

#### Scenario: Harness interface inside agterm
- **GIVEN** the agent runs inside agterm
- **WHEN** round 1 runs in the companion
- **THEN** the companion shows the reviewer harness's interactive interface, not an agk log, and `rounds/1/` has no `reviewer.log`

### Requirement: Untrusted output cannot drive the terminal
Reviewer output that agk prints, to the inline review's terminal or to `reviewer.log`, SHALL carry terminal control characters and bidi overrides only as visible `\uXXXX` escapes (or `\xNN` for invalid bytes), including sequences split across output chunks. Inside agterm, what agk puts into status lines and its own stdout SHALL be limited to fixed strings, counts and paths.

#### Scenario: Escape sequence from a reviewed file
- **GIVEN** a review running outside agterm and an untracked file containing an OSC 52 clipboard sequence and a screen-clear sequence
- **WHEN** the reviewer echoes it while reviewing
- **THEN** the terminal and `reviewer.log` show the sequence as visible escapes and contain no raw ESC or BEL byte

#### Scenario: Finding text stays out of status lines
- **GIVEN** a finding whose title contains an escape sequence
- **WHEN** `agk review wait` records the round
- **THEN** the companion's info line and the wait's stdout carry only counts, the verdict and fixed strings, never the finding's text

### Requirement: Findings validation
The review SHALL accept reviewer output only if it is a JSON object matching the review output schema, and SHALL NOT write `review.json` otherwise. An inline round with invalid output fails at once. Inside agterm, `agk review wait` SHALL handle invalid output only once the reviewer's turn is over. It then types a fix-up instruction into the session, at most twice per round, and on the third invalid output SHALL fail the round.

#### Scenario: Malformed codex output
- **GIVEN** a review running outside agterm
- **WHEN** reviewer output is not valid JSON, or a finding has a severity outside `critical|high|medium|low` or a missing required field
- **THEN** the round is reported as failed with reason `invalid review output` and the raw output is kept in the round directory for inspection

#### Scenario: Invalid output file gets a fix-up
- **GIVEN** the reviewer of run D wrote `out/1.json` with an unknown severity and its turn is over
- **WHEN** `agk review wait D` sees it
- **THEN** it writes the validation error to a fix-up prompt in `rounds/1/` and types one line pointing at that prompt into the session, records no outcome, and keeps waiting

#### Scenario: Output still invalid after two fix-ups
- **GIVEN** the reviewer's output for round 1 was invalid at the end of three turns
- **WHEN** `agk review wait` sees the third invalid output
- **THEN** round 1 is recorded as `blocked` with reason `invalid review output`, the raw output is copied into `rounds/1/` and no further fix-up is typed

#### Scenario: Half-written output mid-turn
- **GIVEN** `out/1.json` is not yet valid JSON while the reviewer's turn still runs
- **WHEN** `agk review wait` checks it
- **THEN** it types nothing and keeps waiting for the turn to end

#### Scenario: Fix-up start not confirmed
- **GIVEN** `agk review wait D` typed fix-up 1 for round 1, the reviewer never reported `active` afterwards, and `out/1.json` and the `completed` status are unchanged from before the fix-up
- **WHEN** the agent runs `agk review wait D` again
- **THEN** it types no second fix-up and records no outcome; it waits for the agent status `active` and reports `running` when the timeout passes first

#### Scenario: Half-rewritten output after an unconfirmed fix-up
- **GIVEN** fix-up 1 of round 1 was typed without `active` being seen, and `out/1.json` changed but is not valid JSON while the status still reads the old `completed`
- **WHEN** `agk review wait D` checks it
- **THEN** it types no second fix-up and does not fail the round; once `out/1.json` becomes valid it records round 1 as `completed`

#### Scenario: Permission prompt during an unconfirmed fix-up
- **GIVEN** fix-up 1 of round 1 was typed without `active` being seen, `out/1.json` is unchanged and the status is `blocked`
- **WHEN** `agk review wait D` checks it
- **THEN** it prints `status` `needs-human` with `reason` `permission` at once, types nothing and writes no `result.json`

#### Scenario: Invalid output outlasts the timeout
- **GIVEN** `out/1.json` stays invalid JSON and the reviewer's turn keeps running
- **WHEN** the agent runs `agk review wait D --timeout 60`
- **THEN** after about 60 seconds it prints `{"status":"running","round":1}` and exits 0

### Requirement: Inline fallback outside agterm
When `AGTERM_ENABLED` is not set, `agk review start` SHALL run the review in the current process without creating a companion or calling agterm, and SHALL print `{"companion": null, "dir": "<run dir>"}` after it finishes, exiting 0 on success and non-zero on failure.

#### Scenario: Review outside agterm
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the user runs `agk review start`
- **THEN** no agtermctl command is executed, the review runs to completion in the foreground, and stdout ends with the JSON object whose `companion` is `null`

### Requirement: Wait for the review outcome
`agk review wait <run-dir> [--timeout SECONDS]` SHALL block until the current round has an outcome, the reviewer needs the user, or the timeout passes (default 1800 seconds). It SHALL then print one JSON object on stdout with `round` and a `status` of `completed`, `blocked`, `needs-human`, `cancelled` or `running`, and exit 0. When the round's `result.json` exists, the object SHALL carry its fields and that file SHALL decide the outcome. Inside agterm, a valid reviewer output file SHALL complete the round even while the reviewer's turn still runs.

#### Scenario: Completed review with findings
- **GIVEN** the reviewer of run dir D wrote a valid `out/1.json` with 3 findings and verdict `needs-attention`
- **WHEN** the agent runs `agk review wait D`
- **THEN** `rounds/1/review.json` and `rounds/1/result.json` are written, stdout is `{"status":"completed","round":1,"findings":3,"verdict":"needs-attention"}`, the exit code is 0 and the companion stays open

#### Scenario: Failed review
- **GIVEN** round 1 of run dir D was recorded as failed with reason `invalid review output`
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout is a JSON object with `status` `blocked`, `round` 1 and `reason` `invalid review output`, and the exit code is 0

#### Scenario: Failure before the outcome could be recorded
- **GIVEN** the companion of D reports `blocked` and the current round has no `result.json` and no output file
- **WHEN** the agent runs `agk review wait D`
- **THEN** stdout is `{"status":"needs-human","round":1,"reason":"permission"}`, the exit code is 0 and no `result.json` is written

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
The reviewer SHALL be unable to modify the repository, enforced by its harness, not only by its prompt:
- **inline, outside agterm:** `codex exec` with a read-only sandbox, or claude in print mode, both constrained to the bundled review output schema;
- **interactive, in agterm:** codex in a sandbox whose only writable root is the run's `out/` directory, or claude in restricted mode allowed only reading, search, read-only git and writing under `out/`.

#### Scenario: Codex reviewer cannot modify the repository
- **GIVEN** the agent runs inside agterm and the reviewer resolves to `codex`
- **WHEN** the reviewer tries to write a file in the repository or under `.git`
- **THEN** the sandbox denies it; codex runs with `--sandbox workspace-write`, `--ask-for-approval never`, the run's `out/` directory as its working directory, its configured extra writable roots reset to none and the tmp roots excluded

#### Scenario: User config widens the codex sandbox
- **GIVEN** the user's codex config lists the repository under `sandbox_workspace_write.writable_roots`
- **WHEN** an interactive codex reviewer tries to write a repository file
- **THEN** the sandbox denies it, because agk's own writable-roots reset comes after every configured value

#### Scenario: Claude reviewer cannot modify the repository
- **GIVEN** the agent runs inside agterm and the reviewer resolves to `claude` with model `claude-opus-5-5`
- **WHEN** a round runs
- **THEN** claude runs interactively in restricted mode with model `claude-opus-5-5` and in `dontAsk` mode; it can only read, search, run the allowlisted read-only git commands and write under the run's `out/` directory, and the working tree is unchanged by the review

#### Scenario: Inline reviewer keeps the schema
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** a round runs with either harness
- **THEN** codex runs `codex exec` with a read-only sandbox and the bundled schema, or claude runs in print mode with the bundled schema as its output schema, and the working tree is unchanged by the review

### Requirement: Re-review round
`agk review send <run-dir> [--note FILE] [--timeout SECONDS]` SHALL start round k+1 after round k has an outcome. It SHALL:
1. rebuild the run's scope from its recorded target (the diff against the recorded base, or the plan scope of the recorded change);
2. write a re-review prompt to `rounds/<k+1>/`, including the note when given;
3. record round k+1;
4. hand the round to the reviewer.

Inside agterm, the round reaches the live reviewer session as one typed line, once the session's turn is over (waiting up to `--timeout`, default 600 seconds). Inline, the reviewer resumes its harness session. The command SHALL print `{"companion": <id or null>, "dir": "<run dir>", "round": k+1}`.

#### Scenario: Second round remembers the first
- **GIVEN** review run D in agterm whose round 1 completed with 2 findings
- **WHEN** the agent runs `agk review send D --note fixed.md`
- **THEN** one line pointing at `rounds/2/prompt.md` is typed into the same reviewer session, that prompt asks it to re-check the earlier findings, includes the note and names `out/2.json` as the output file, and the outcome lands in `rounds/2/`

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

#### Scenario: Reviewer still busy
- **GIVEN** round 1 of D has an outcome but the reviewer's turn has not ended
- **WHEN** the agent runs `agk review send D --timeout 30` and the turn does not end within 30 seconds
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the reviewer is busy

#### Scenario: Reviewer waits for a permission answer
- **GIVEN** the reviewer of D shows a permission prompt (agent status `blocked`)
- **WHEN** the agent runs `agk review send D`
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the reviewer waits for the user

#### Scenario: Round recorded mid-turn
- **GIVEN** `agk review wait D` recorded round 1 from a valid output file while the reviewer's turn was still running
- **WHEN** the agent runs `agk review send D` before that turn ends
- **THEN** the command waits for the turn to end before typing round 2, and with a too-short `--timeout` exits 1 saying the reviewer is busy

#### Scenario: Send after a failed round
- **GIVEN** round 1 of D was recorded as failed with reason `invalid review output` and the reviewer's turn is over
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 is typed into the session; the failure is not mistaken for a permission prompt

#### Scenario: No session to resume
- **GIVEN** an inline review run D whose round 1 failed before the reviewer reported a session id
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 starts a fresh reviewer session with the first-round prompt

#### Scenario: Re-review outside agterm
- **GIVEN** a review run D with no companion
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 runs in the foreground without calling agterm and stdout ends with the JSON object whose `companion` is `null`

### Requirement: Reviewer companion waits between rounds
After each round, successful or failed, the reviewer session SHALL stay open in its companion, keeping the round visible, until `agk review send` types the next round into it or the companion is closed.

#### Scenario: Request arrives
- **GIVEN** companion C of D finished round 1
- **WHEN** the agent runs `agk review send D`
- **THEN** one line is typed into C, C's status becomes `active`, its info line reads `<harness> #<N> r2: reviewing`, and round 2 runs in the same reviewer session

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

### Requirement: Reviewer start handshake
After creating the reviewer companion, `agk review start` SHALL wait up to 60 seconds for the companion's agent status to become `active`, `completed` or `blocked`. If none arrives, it SHALL report `"started": false`, print a warning to stderr with the last lines of the companion's screen, and leave the companion open for the user.

#### Scenario: Folder trust prompt
- **GIVEN** the reviewer shows its workspace-trust prompt in the companion and sets no status
- **WHEN** 60 seconds pass after the companion was created
- **THEN** stdout carries `"started": false`, stderr carries the warning with the screen lines showing the trust prompt, the exit code is 0 and the companion stays open

#### Scenario: Reviewer starts
- **GIVEN** the reviewer reports `active` 5 seconds after the companion was created
- **WHEN** `agk review start` waits
- **THEN** it prints `"started": true` and exits 0

### Requirement: Reviewer output file
Inside agterm, the prompt of each round k SHALL name the absolute path of the run's `schema.json` and of `out/<k>.json`. It SHALL tell the reviewer to write its review there as one JSON object matching that schema, and SHALL give the repository root for reading files. Every git command it prescribes SHALL work from the reviewer's working directory under its permissions: `git -C <root>` for codex, plain `git` for claude. File paths in findings stay relative to the repository root.

#### Scenario: Prompt names the output file
- **WHEN** `agk review start` writes `rounds/1/prompt.md` for a review in agterm
- **THEN** it names the absolute paths of `schema.json` and `out/1.json` and the repository root, and asks for paths relative to that root

#### Scenario: Truncated diff read from the codex working directory
- **GIVEN** a codex diff review in agterm whose diff exceeds the prompt budget
- **WHEN** the reviewer runs the prompt's read-the-rest command from `out/`
- **THEN** the command carries `-C <root>` and reproduces the rest of the repository's diff, not a diff of `out/`

#### Scenario: Claude git commands match its allow rules
- **GIVEN** a claude review in agterm
- **WHEN** `rounds/1/prompt.md` is written
- **THEN** every git command it prescribes is plain `git <subcommand>` matching the reviewer's read-only git allow rules, with no `-C`

#### Scenario: Inline prompt keeps the schema contract
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** `agk review start` writes `rounds/1/prompt.md`
- **THEN** the prompt asks for the JSON object as the reply, matching the harness-enforced schema, and names no output file

### Requirement: Reviewer needs the user
Inside agterm, `agk review wait` SHALL report `needs-human` without recording an outcome:
- with reason `no-output` when the reviewer's turn ended (status `completed` or `idle`) and the current round has no output file;
- with reason `permission` when the reviewer's agent status is `blocked`.

A later `agk review wait` SHALL pick the round up again once the user and the reviewer have produced the output file.

#### Scenario: Turn ended with a question
- **GIVEN** the reviewer of D ends its turn with a question and writes no `out/1.json`
- **WHEN** `agk review wait D` sees the turn end
- **THEN** it prints `{"status":"needs-human","round":1,"reason":"no-output"}` and writes no `result.json`

#### Scenario: Permission prompt
- **GIVEN** the reviewer's agent status becomes `blocked` before it wrote its output
- **WHEN** `agk review wait D` is waiting
- **THEN** it prints `status` `needs-human` with `reason` `permission`

#### Scenario: User helped, reviewer finished
- **GIVEN** an earlier `agk review wait D` reported `needs-human` and the user then talked the reviewer into writing a valid `out/1.json`
- **WHEN** the agent runs `agk review wait D` again
- **THEN** it records and prints the `completed` outcome of round 1

### Requirement: Claude reviewer status hooks
Restricted mode ignores the user's settings and so their agent-status hooks. So a claude reviewer in agterm SHALL get hooks from agk's own settings file: they set the companion's agent status to `active` when a turn starts, `completed` when it ends and `blocked` on a permission request.

#### Scenario: Turn end reported without user hooks
- **GIVEN** a claude reviewer in agterm whose user settings would be ignored
- **WHEN** its first turn ends
- **THEN** the companion's agent status becomes `completed`

### Requirement: Reviewer arguments cannot widen its permissions
`agk review start` SHALL refuse reviewer `extra_args` that change the reviewer's sandbox, approvals, permission mode, allowed tools, extra directories or settings, for both harnesses and in both modes. It SHALL exit 1 naming the argument, before creating a run directory.

#### Scenario: Codex sandbox override in config
- **GIVEN** `.agk.toml` sets the reviewer's `extra_args = ["--sandbox", "danger-full-access"]`
- **WHEN** the agent runs `agk review start`
- **THEN** it exits 1 naming `--sandbox`, creates no run directory and starts no companion

#### Scenario: Claude permission override
- **GIVEN** the reviewer resolves to `claude` with `extra_args = ["--dangerously-skip-permissions"]`
- **WHEN** the agent runs `agk review start`
- **THEN** it exits 1 naming `--dangerously-skip-permissions`

#### Scenario: Harmless tuning passes
- **GIVEN** the reviewer resolves to `codex` with `extra_args = ["-c", "model_verbosity=\"low\""]`
- **WHEN** the agent runs `agk review start`
- **THEN** the review starts with that argument before agk's mandatory sandbox flags

### Requirement: Run directories from an older agk
A run directory that does not record whether its reviewer runs interactively but has a companion was started by an older agk. `agk review wait` SHALL report the outcome of its current round from `result.json` and otherwise exit 1. In both cases, and for `agk review send`, it SHALL say the run was started by an older agk and a new review is needed. `agk review close` SHALL still close its companion.

#### Scenario: Send to an older run
- **GIVEN** run dir D with a companion and no record of an interactive reviewer
- **WHEN** the agent runs `agk review send D`
- **THEN** it exits 1 saying D was started by an older agk and a new review is needed, and records no new round

#### Scenario: Wait on a finished older run
- **GIVEN** run dir D with a companion, no record of an interactive reviewer and `rounds/1/result.json` with status `completed`
- **WHEN** the agent runs `agk review wait D`
- **THEN** it prints the `completed` outcome and exits 0
