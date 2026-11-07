# Spec Delta

## MODIFIED Requirements

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

## ADDED Requirements

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
