# Spec Delta

## Purpose

The `agk-propose` agent skill: the plan step of the agk workflow, which writes an OpenSpec change,
has a reviewer and then the user check it, and stops before implementation. Its behavior is
checked by an eval suite that drives a real agent against stand-ins for `agk` and `revdiff`.

## ADDED Requirements

### Requirement: Explicit invocation
The `agk-propose` skill SHALL run only when invoked by name (`/agk-propose`, "agk propose"). A plain request to propose an OpenSpec change SHALL NOT start a plan review.

#### Scenario: Plain propose request
- **WHEN** the user asks the agent to propose an OpenSpec change without naming `agk-propose`
- **THEN** the agent does not run `agk review start --change`

### Requirement: Preconditions
Before writing anything the skill SHALL check that the project has the `openspec-propose` skill and that `agk` is on `PATH`, and SHALL stop with setup advice when either is missing.

#### Scenario: Project without OpenSpec skills
- **GIVEN** a repository with no `openspec-propose` skill
- **WHEN** the user invokes `/agk-propose add a widget`
- **THEN** the agent tells the user to run `openspec init` or `openspec update`, creates no change and runs no `agk` command

### Requirement: Artifacts, then plan review
The skill SHALL write the change's artifacts with the project's `openspec-propose` workflow and then, instead of stopping, start a plan review with `agk review start --change <change>` from the repository root and wait for it in the background.

#### Scenario: Review follows the artifacts
- **WHEN** the user invokes `/agk-propose` with a clear request
- **THEN** the change's proposal, specs, design and tasks exist before `agk review start --change <change>` runs, and `agk review wait <dir>` runs after it

### Requirement: Findings are fixed and re-reviewed
On a round with findings the agent SHALL fix them in the planning artifacts itself and run `agk review send <dir> --note <file>` in the same run, repeating until a round passes. A finding the agent disagrees with SHALL be raised to the user, not dropped.

#### Scenario: Findings in the first round
- **GIVEN** round 1 of the plan review returns a finding on `tasks.md`
- **WHEN** the agent handles the outcome
- **THEN** `tasks.md` is edited, `agk review send` runs with a note naming the fix, and no `agk review close` runs before a passing round

#### Scenario: Disputed finding
- **GIVEN** a finding that contradicts what the user asked for
- **WHEN** the agent handles the outcome
- **THEN** the agent raises the finding with its reasoning and waits for the user's decision instead of editing or ignoring it

### Requirement: User pass in revdiff
After a passing round the skill SHALL open the change's files in revdiff in file-review mode, one `--only` per file, with a description file summarizing the review, address every annotation and relaunch until the user quits without annotations. When revdiff is not available it SHALL skip this step and warn the user.

#### Scenario: revdiff available
- **GIVEN** a passing plan review round and revdiff available
- **WHEN** the agent continues
- **THEN** revdiff is launched with `--only=` for every file of the change and `--description-file=<run dir>/revdiff-description.md`

#### Scenario: revdiff missing
- **GIVEN** a passing plan review round and no revdiff
- **WHEN** the agent continues
- **THEN** it tells the user the revdiff pass was skipped and goes on without launching revdiff

### Requirement: Nothing unreviewed is closed
The skill SHALL close the reviewer only when the last passing round saw the current artifacts. Any artifact change after that round, from the user's annotations or the agent's fixes, SHALL go through another `agk review send` round first.

#### Scenario: User annotations changed the plan
- **GIVEN** the reviewer passed the plan and the user's revdiff annotations led to an edit of `design.md`
- **WHEN** the user quits revdiff without further annotations
- **THEN** `agk review send` runs before `agk review close`

#### Scenario: Nothing changed after the pass
- **GIVEN** the reviewer passed the plan and the user quit revdiff without annotations
- **WHEN** the agent continues
- **THEN** it runs `agk review close <dir>` with no further review round

### Requirement: Stop before implementation
After closing the reviewer the skill SHALL report the change, the review rounds, the final verdict and any findings set aside, and SHALL stop without running `agk apply` or editing project code.

#### Scenario: Plan done
- **WHEN** the reviewer is closed
- **THEN** the agent's last message names the change and `agk apply start <change>` as the next step, and no `agk apply` command was run

### Requirement: Eval suite
The repository SHALL hold an eval suite for `agk-propose` runnable with `claude plugin eval`, with at least one case per scenario of this capability. Cases SHALL run against stand-ins for `agk` and `revdiff` that record their arguments, never a real reviewer, applier, agterm or revdiff TUI.

#### Scenario: Suite runs against stand-ins
- **WHEN** the developer runs the documented eval command
- **THEN** every case runs, none starts a real codex, claude reviewer, agterm companion or revdiff TUI, and the report scores each case

#### Scenario: A broken skill fails its case
- **GIVEN** `skills/agk-propose/SKILL.md` edited to close the reviewer right after the first approve
- **WHEN** the suite runs
- **THEN** the "User annotations changed the plan" case scores below the threshold
