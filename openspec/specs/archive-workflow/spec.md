# archive-workflow Specification

## Purpose
The `agk-archive` agent skill: the close step of the agk workflow. It archives an OpenSpec change
only after the last code review approved the current code and verify passed after it, then closes
the change's agk companions. Eval cases in the skill eval suite check this behavior.

## Requirements

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
When the session cannot confirm the review gate, and no finding of the change's latest round is unresolved, the skill SHALL run a diff review with `agk review start --base <base> --for <change>` and wait for it. If the round approves and `agk review fresh` then reports `fresh: true` for it, the gate passes. A `fresh: false` result or an error SHALL stop the skill with nothing archived.

#### Scenario: Fresh session, review approves
- **GIVEN** a fresh session and a diff review that approves
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** `agk review start --base <base> --for add-widget` and `agk review wait` run before verify and before the archive

#### Scenario: Code changed during the fallback review
- **GIVEN** a fresh session whose fallback review approves, but `agk review fresh` then reports `fresh: false` because a file changed while the round ran
- **WHEN** the agent handles the outcome
- **THEN** it reports that the code changed during the review, and runs no verify and no archive

### Requirement: Fallback review without an approve
When the fallback review round has findings, the skill SHALL report them, leave the reviewer open and archive nothing. When `agk review start` answers `nothing to review` outside a change worktree, and `agk review fresh` on this session's own diff review run for the change printed a `merge_base` different from the base just tried, the skill SHALL run the fallback once more with `--base <merge_base>`. When `nothing to review` remains, in a change worktree, or without such a `merge_base`, the skill SHALL ask the user whether to archive without a code review or which base to review against, and SHALL archive nothing until the user answers.

#### Scenario: Fallback review has findings
- **GIVEN** a fresh session and a diff review whose round returns a finding
- **WHEN** the agent handles the outcome
- **THEN** it reports the finding, runs no verify and no archive, and does not close that reviewer

#### Scenario: Nothing to review
- **GIVEN** a fresh session where `agk review start --base main` exits 1 with `nothing to review`
- **WHEN** the agent handles the outcome
- **THEN** it asks the user whether to archive without a code review or which base to review against, and archives nothing until the user answers

#### Scenario: Nothing to review against the base, retried at the recorded merge base
- **GIVEN** a session on `main` whose diff review run for the change approved at merge base `M`, after which the code was edited and everything was committed on `main`, so `agk review fresh` printed `"fresh":false` with `"merge_base":"<M>"` and `agk review start --base main` exits 1 with `nothing to review`
- **WHEN** the agent handles the outcome
- **THEN** it runs `agk review start --base <M> --for <change>` and waits for that round, without asking the user for a base

#### Scenario: Retry at the merge base also has nothing to review
- **GIVEN** a fallback whose `agk review start --base <merge_base> --for <change>` also exits 1 with `nothing to review`
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

### Requirement: No fixes, commits only in a change worktree
The skill SHALL NOT push, edit project code or fix review findings or verify issues. It SHALL commit only in a change worktree, as the change worktree requirements say, and never in the main checkout. It SHALL end with a message naming the archived change, the review and verify results with any warnings, the companions it closed, and in a change worktree the commits it made and the outcome of `agk finish`.

#### Scenario: Archive done
- **GIVEN** a session in the main checkout, not in a change worktree
- **WHEN** the archive and the closes have run
- **THEN** the last message names the archived change and the closed companions, and no `git commit` or `git push` ran

#### Scenario: Archive done in a change worktree
- **GIVEN** a session in the change worktree of `add-widget`
- **WHEN** the archive, the closes, both commits and `agk finish` have run
- **THEN** the last message names the archived change, the closed companions, both commits and the base branch `agk finish` merged into, and no `git push` ran

### Requirement: Eval cases
The skill eval suite SHALL hold at least one case for each scenario of this capability. The cases SHALL run against the stand-in `agk` and stub OpenSpec skills, never a real reviewer, applier, agterm companion or OpenSpec archive of the repository.

#### Scenario: Archive cases run against stand-ins
- **WHEN** the developer runs the documented eval command
- **THEN** each `agk-archive` case runs and is scored, and none starts a real codex, claude reviewer or agterm companion

#### Scenario: A broken skill fails its case
- **GIVEN** `skills/agk-archive/SKILL.md` edited to archive without checking verify
- **WHEN** the suite runs
- **THEN** the "Verify reports CRITICAL" case scores below the threshold

### Requirement: Change worktree detection
The skill SHALL treat the session as working in a change worktree when the current directory is in a linked worktree (`git rev-parse --git-dir` differs from `git rev-parse --git-common-dir`), the current branch is the selected change's name and `git config branch.<change>.agkBase` is set. In a change worktree, a fallback diff review SHALL use that recorded base as `--base`. Otherwise the skill SHALL behave as in the main checkout.

#### Scenario: Fallback review in a change worktree
- **GIVEN** a fresh session on branch `add-widget` with `branch.add-widget.agkBase` set to `main`
- **WHEN** the user invokes `/agk-archive add-widget` and the review gate needs the fallback
- **THEN** the agent runs `agk review start --base main`

#### Scenario: Branch without a recorded base
- **GIVEN** a session on branch `add-widget` with no `branch.add-widget.agkBase`
- **WHEN** the archive succeeds
- **THEN** the agent makes no commit and runs no `agk finish`

#### Scenario: Change branch checked out in the main checkout
- **GIVEN** a session in the main checkout with branch `add-widget` checked out and `branch.add-widget.agkBase` set to `main`
- **WHEN** the archive succeeds
- **THEN** the agent makes no commit and runs no `agk sync` or `agk finish`

### Requirement: Commit the code and sync before the gates
In a change worktree, before the review gate, the skill SHALL commit every uncommitted change of the worktree in one commit with a conventional-commit message it writes from the change's work (no commit when there is nothing to commit), then run `agk sync <change>`. When `agk sync` exits 1, the skill SHALL report its message and stop: no gate, archive, archive commit or `agk finish`.

#### Scenario: Code committed and synced first
- **GIVEN** a session in the change worktree of `add-widget` with an uncommitted code file
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** a `git commit` holding the code file runs, then `agk sync add-widget`, both before any `agk review` command and before verify

#### Scenario: Sync conflict
- **GIVEN** a session in the change worktree of `add-widget` where `agk sync add-widget` exits 1 with a rebase conflict
- **WHEN** the agent handles the failure
- **THEN** it reports the conflict, runs no review, verify, archive or `agk finish`, and runs no git command to resolve it

#### Scenario: Gate fails in a change worktree
- **GIVEN** a session in the change worktree of `add-widget`, a sync that succeeded and a verify run that reports a CRITICAL issue
- **WHEN** the agent handles the result
- **THEN** no archive, no archive commit and no `agk finish` run

### Requirement: A rebasing sync is a code change
When `agk sync` reports `"rebased": true`, the verify gate SHALL count the sync as a code change: only a verification report produced after it passes the gate. The review gate SHALL rely on `agk review fresh`, whose fingerprint includes the merge base.

#### Scenario: Verified before a rebasing sync
- **GIVEN** a session in the change worktree of `add-widget` that verified the change with 0 CRITICAL, after which `agk sync add-widget` reports `"rebased": true`
- **WHEN** the agent reaches the verify gate
- **THEN** it runs `openspec-verify-change` again before the archive

### Requirement: Sync again before the archive
In a change worktree, after the freshness check after verify is clean and before handing over to `openspec-archive-change`, the skill SHALL run `agk sync <change>` again. On `"rebased": false` it SHALL continue to the archive. On `"rebased": true` it SHALL return to the review gate and run both gates again before the archive. A failed sync SHALL stop the skill as before the gates.

#### Scenario: Base moved during the gates
- **GIVEN** a session in the change worktree of `add-widget` with passing gates, where the second `agk sync add-widget` reports `"rebased": true`
- **WHEN** the agent handles the result
- **THEN** it runs the review gate again (a new `agk review start --base main` when `agk review fresh` is not fresh) and verify again, and does not archive before a later sync reports `"rebased": false`

### Requirement: Commit the archive and finish
In a change worktree, after a successful archive and the closes, the skill SHALL commit the archive with a `chore(openspec): archive <change>` message, staging and committing only paths under `openspec/` (a pathspec commit), then change to the main checkout's root and run `agk finish <change>`. When the archive failed it SHALL commit nothing more and SHALL NOT run `agk finish`.

#### Scenario: Finish succeeds
- **GIVEN** a session in the change worktree of `add-widget` and a successful archive
- **WHEN** the agent continues after the closes
- **THEN** a `git commit` of the archive runs, then `agk finish add-widget` runs from the main checkout's root, and the last message names the base branch and its new tip from the finish output

#### Scenario: Archive failed in a change worktree
- **GIVEN** a session in the change worktree of `add-widget` and an archive attempt that does not move the change
- **WHEN** the agent handles the failure
- **THEN** it makes no archive commit and runs no `agk finish`

### Requirement: Archive commit holds only the archive
The archive commit SHALL never carry code changed after the gates. When the commit holds a path outside `openspec/` (for example one a pre-commit hook added) or the worktree is not clean after it, the skill SHALL report it and SHALL NOT run `agk finish`.

#### Scenario: Code changed after the gates
- **GIVEN** a session in the change worktree of `add-widget` with passing gates, where a code file changes after the second sync
- **WHEN** the agent commits the archive
- **THEN** the archive commit holds only paths under `openspec/`, the changed code file stays uncommitted, and the agent reports it and runs no `agk finish`

### Requirement: Finish failure stops the skill
When `agk finish` exits 1, the skill SHALL report its message and what it left and stop, without retrying, forcing or resolving anything itself. The commits and the archive SHALL stay.

#### Scenario: Finish fails
- **GIVEN** a session in the change worktree of `add-widget` where `agk finish add-widget` exits 1 because the base moved after the last sync
- **WHEN** the agent handles the failure
- **THEN** it reports the message and that the worktree and branch are left, runs no `agk sync`, rebase or merge itself, and does not run `agk finish` again
