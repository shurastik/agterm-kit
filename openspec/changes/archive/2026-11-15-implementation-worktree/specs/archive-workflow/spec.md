# Spec Delta

## RENAMED Requirements

- FROM: `### Requirement: No commits and no fixes`
- TO: `### Requirement: No fixes, commits only in a change worktree`

## MODIFIED Requirements

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

## ADDED Requirements

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
