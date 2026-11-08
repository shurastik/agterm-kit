# Spec Delta

## Purpose

The `agk-archive` agent skill: the close step of the agk workflow. It archives an OpenSpec change
only after the last code review approved the current code and verify passed after it, then closes
the change's agk companions. Eval cases in the skill eval suite check this behavior.

## ADDED Requirements

### Requirement: Explicit invocation
The `agk-archive` skill SHALL run only when invoked by name (`/agk-archive`, "agk archive"). A plain request to archive a change SHALL NOT start it.

#### Scenario: Plain archive request
- **WHEN** the user asks the agent to archive an OpenSpec change without naming `agk-archive`
- **THEN** the agent does not invoke the `agk-archive` skill

### Requirement: Preconditions
Before any review, verify or archive step, the skill SHALL check that the project has the `openspec-archive-change` and `openspec-verify-change` skills and that `agk` is on `PATH`. When any of these is missing, it SHALL stop with setup advice and archive nothing.

#### Scenario: Project without OpenSpec skills
- **GIVEN** a repository with no `openspec-archive-change` skill
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** the agent tells the user to run `openspec init` or `openspec update`, runs no `agk` command and archives nothing

### Requirement: Change selection
The skill SHALL archive one change. It is the change named in the invocation. Without a name, it is the change this session worked on, then the only active change. Otherwise the skill SHALL ask the user. It SHALL use the same change in every later step.

#### Scenario: One active change, no name
- **GIVEN** a repository whose only active change is `add-widget`
- **WHEN** the user invokes `/agk-archive` without a name in a fresh session
- **THEN** the agent announces `add-widget` and every later step works on `add-widget`

### Requirement: Review gate from the session
The review gate SHALL pass when this session's tool output shows that the latest diff review round run for the change approved, with no findings other than ones the user set aside, and `agk review fresh` on that run reports `fresh: true` for that round. A user's claim or a compaction summary SHALL NOT count. A `fresh: false` result or a `fresh` error SHALL count as unconfirmed.

#### Scenario: Approved earlier in the session
- **GIVEN** a session where `agk review wait` reported an approve with no findings for the change, and `agk review fresh` reports `fresh: true`
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** the agent runs `agk review fresh` and no new `agk review start`

#### Scenario: File added after the approve
- **GIVEN** a session where a diff review round for the change approved and then a new untracked file was created, so `agk review fresh` reports `fresh: false`
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** the agent runs a new `agk review start --base` before verify and before the archive

#### Scenario: Review of another change
- **GIVEN** a session whose only diff review was run as the code review of `other-change`
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** that review does not count for `add-widget`, and the agent runs its own `agk review start --base`

### Requirement: Latest findings stop the archive
When the latest diff review round this session ran for the change has findings the user did not set aside, the skill SHALL report them and archive nothing, without starting a new review. An approve in an earlier round of that run SHALL NOT override them.

#### Scenario: Later round found issues
- **GIVEN** a session where round 1 of a diff review for the change approved and round 2 of the same run, with no file changed in between, returned a finding
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** the agent reports the round 2 finding and archives nothing

### Requirement: Review fallback
When the session cannot confirm the review gate, and no finding of the change's latest round is unresolved, the skill SHALL run a diff review with `agk review start --base <base>` and wait for it. If the round approves and `agk review fresh` then reports `fresh: true` for it, the gate passes. A `fresh: false` result or an error SHALL stop the skill with nothing archived.

#### Scenario: Fresh session, review approves
- **GIVEN** a fresh session and a diff review that approves
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** `agk review start --base` and `agk review wait` run before verify and before the archive

#### Scenario: Code changed during the fallback review
- **GIVEN** a fresh session whose fallback review approves, but `agk review fresh` then reports `fresh: false` because a file changed while the round ran
- **WHEN** the agent handles the outcome
- **THEN** it reports that the code changed during the review, and runs no verify and no archive

### Requirement: Fallback review without an approve
When the fallback review round has findings, the skill SHALL report them, leave the reviewer open and archive nothing. When `agk review start` answers `nothing to review`, the skill SHALL ask the user whether to archive without a code review or which base to review against, and SHALL archive nothing until the user answers.

#### Scenario: Fallback review has findings
- **GIVEN** a fresh session and a diff review whose round returns a finding
- **WHEN** the agent handles the outcome
- **THEN** it reports the finding, runs no verify and no archive, and does not close that reviewer

#### Scenario: Nothing to review
- **GIVEN** a fresh session where `agk review start --base main` exits 1 with `nothing to review`
- **WHEN** the agent handles the outcome
- **THEN** it asks the user whether to archive without a code review or which base to review against, and archives nothing until the user answers

### Requirement: Verify gate
The verify gate SHALL pass when this session holds a verification report the agent produced by following `openspec-verify-change` for the change, after the passing review round and the last code change, with 0 CRITICAL. A user's claim or a compaction summary SHALL NOT count. Otherwise the skill SHALL run `openspec-verify-change` itself. A CRITICAL issue SHALL stop the skill before the archive.

#### Scenario: Verified earlier in the session
- **GIVEN** a session where the code review approved, then verify reported 0 CRITICAL, and nothing changed afterwards
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** the agent runs no new verify and no new `agk review start` before the archive

#### Scenario: Verify reports CRITICAL
- **GIVEN** a verify run on the change that reports a CRITICAL issue
- **WHEN** the agent handles the result
- **THEN** it reports the issue and archives nothing

#### Scenario: Verify reports warnings only
- **GIVEN** a verify run on the change with 0 CRITICAL and one WARNING
- **WHEN** the agent handles the result
- **THEN** it continues to the archive and lists the warning in its final message

### Requirement: Freshness check after verify
After the verify gate passes and immediately before handing over to `openspec-archive-change`, the skill SHALL run `agk review fresh` again on the approving review round. A `fresh: false` result or an error SHALL stop the skill with nothing archived and the reviewer left open. The check does not apply when the user chose to archive without a code review.

#### Scenario: Code changed during verify
- **GIVEN** a passing review gate and a verify run during which a file of the change's scope changes
- **WHEN** the agent runs `agk review fresh` before the archive and it reports `fresh: false`
- **THEN** it reports that the code changed after the review, runs no archive and closes no companion

### Requirement: Archive through the project's workflow
Once both gates pass and the freshness check after verify is clean, the skill SHALL archive the change with the project's `openspec-archive-change` skill and follow that skill's own checks and questions.

#### Scenario: Gates pass
- **GIVEN** a passing review gate and a passing verify gate
- **WHEN** the agent continues
- **THEN** it follows `openspec-archive-change` for the selected change, and the change is moved under `openspec/changes/archive/`

### Requirement: Close the change's companions
After a successful archive, the skill SHALL run `agk apply close` for every run under `.agk/applies/` whose `worker.json` records the change. It SHALL run `agk review close` for every run under `.agk/reviews/` that records the change, and for every diff review run this session ran for the change. A diff review run of uncertain change SHALL stay open, as SHALL runs of other changes. A failed close SHALL be reported and SHALL NOT undo the archive.

#### Scenario: Applier and plan reviewer still open
- **GIVEN** `.agk/applies/1/` and `.agk/reviews/2/` record the change `add-widget`, and `.agk/applies/3/` records `other-change`
- **WHEN** the archive of `add-widget` succeeds
- **THEN** `agk apply close` runs on `.agk/applies/1` and `agk review close` runs on `.agk/reviews/2`, and nothing closes `.agk/applies/3`

#### Scenario: Diff reviewer of another change in the session
- **GIVEN** a session that ran one diff review as the code review of `other-change` and the archive's own fallback review for `add-widget`
- **WHEN** the archive of `add-widget` succeeds
- **THEN** `agk review close` runs on the fallback review's run dir and not on the `other-change` review's run dir

#### Scenario: Archive failed
- **GIVEN** an archive attempt that does not move the change
- **WHEN** the agent handles the failure
- **THEN** it closes no companion and reports the failure

### Requirement: No commits and no fixes
The skill SHALL NOT commit, push, edit project code or fix review findings or verify issues. It SHALL end with a message naming the archived change, the review and verify results with any warnings, and the companions it closed.

#### Scenario: Archive done
- **WHEN** the archive and the closes have run
- **THEN** the last message names the archived change and the closed companions, and no `git commit` or `git push` ran

### Requirement: Eval cases
The skill eval suite SHALL hold at least one case for each scenario of this capability. The cases SHALL run against the stand-in `agk` and stub OpenSpec skills, never a real reviewer, applier, agterm companion or OpenSpec archive of the repository.

#### Scenario: Archive cases run against stand-ins
- **WHEN** the developer runs the documented eval command
- **THEN** each `agk-archive` case runs and is scored, and none starts a real codex, claude reviewer or agterm companion

#### Scenario: A broken skill fails its case
- **GIVEN** `skills/agk-archive/SKILL.md` edited to archive without checking verify
- **WHEN** the suite runs
- **THEN** the "Verify reports CRITICAL" case scores below the threshold
