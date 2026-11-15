# Spec Delta

## Purpose

Gives each OpenSpec change its own git worktree and branch for implementation: `agk apply start`
prepares the base branch and creates or reuses the worktree, `agk sync` rebases the branch onto the
updated base, and `agk finish` fast-forwards the base to the finished branch and removes the
worktree.

## ADDED Requirements

### Requirement: Worktree mode of apply start
`agk apply start <change>` SHALL run in worktree mode when the resolved applier `worktree` setting is true and the command runs in the main checkout of the repository. In worktree mode it SHALL prepare the base branch, then create or reuse the change's worktree, before it creates a run directory. Without worktree mode it SHALL behave as before and create no worktree, commit nothing and pull nothing.

#### Scenario: Mode off
- **GIVEN** the applier `worktree` setting resolves to false
- **WHEN** the agent runs `agk apply start add-auth` in the main checkout
- **THEN** no worktree, branch or commit is created, no `git pull` runs, and stdout has `"worktree": null` and `"pulled": false`

### Requirement: Base branch
In worktree mode, the base branch SHALL be the branch checked out in the main checkout when `agk apply start` runs. A detached HEAD SHALL be refused with exit 1 before anything is changed.

#### Scenario: Detached HEAD
- **GIVEN** worktree mode and a main checkout on a detached HEAD
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the command exits 1 saying a base branch is needed, and no pull, commit, worktree or run directory happens

### Requirement: Pull before creating the worktree
Before it commits the plan or creates the worktree, `agk apply start` SHALL run `git pull --ff-only` on the base branch in the main checkout. It SHALL skip the pull when the base branch has no upstream or `--no-pull` is given, and SHALL report in stdout `"pulled"` whether a pull ran. A failed pull SHALL exit 1 with git's message, and the command SHALL commit nothing and create no worktree.

#### Scenario: Base behind its upstream
- **GIVEN** worktree mode, base `main` with an upstream that has one new commit, and a main checkout whose local changes do not touch that commit's files
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** `main` is fast-forwarded to the upstream before the worktree is created, the worktree starts from that commit, and stdout has `"pulled": true`

#### Scenario: Base diverged from its upstream
- **GIVEN** worktree mode and a base branch with a local commit that its upstream lacks while the upstream has a commit the base lacks
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the command exits 1 with git's fast-forward error, and no plan commit, worktree or run directory is created

#### Scenario: No upstream
- **GIVEN** worktree mode and a base branch without an upstream
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** no pull runs, the worktree is created, and stdout has `"pulled": false`

#### Scenario: Pull disabled
- **GIVEN** worktree mode and a base branch with an upstream
- **WHEN** the agent runs `agk apply start add-auth --no-pull`
- **THEN** no `git pull` runs and stdout has `"pulled": false`

### Requirement: Apply state checked again after the pull
When a pull ran, `agk apply start` SHALL read the change's apply state again in the main checkout before the plan commit, and refuse as for a change that is not ready (missing, blocked or all tasks done) with exit 1. The pull SHALL stay; no plan commit, worktree or run directory SHALL be created.

#### Scenario: Change finished upstream
- **GIVEN** worktree mode, change `add-auth` ready locally, and an upstream commit that checks off its last task
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** `main` is fast-forwarded, the command exits 1 with `nothing to apply`, and no plan commit, worktree or run directory is created

### Requirement: Plan committed on the base branch
After the pull and before creating the worktree, `agk apply start` SHALL commit the uncommitted content of `openspec/changes/<change>/` on the base branch with the message `docs(openspec): plan <change>`. The commit SHALL contain only paths under that directory and SHALL leave every other change in the main checkout, staged or not, as it was. When that directory has nothing to commit, no commit SHALL be made. A failed commit SHALL exit 1 and create no worktree.

#### Scenario: Untracked plan next to other edits
- **GIVEN** worktree mode, an untracked `openspec/changes/add-auth/`, a modified `README.md` and a staged `main.go`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the base branch gains one commit `docs(openspec): plan add-auth` holding only the files of `openspec/changes/add-auth/`, while `README.md` stays modified and `main.go` stays staged

#### Scenario: Plan already committed
- **GIVEN** worktree mode and `openspec/changes/add-auth/` committed with no local changes
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** no new commit is made on the base branch

### Requirement: Create the change worktree
`agk apply start` SHALL create the worktree `<main checkout>/.worktrees/<change>` on a new branch `<change>` starting at the base branch, record the base branch name in `git config branch.<change>.agkBase` before running `git worktree add`, and make `.worktrees/` ignored in the main checkout through the repository's `info/exclude` when it is not ignored already.

#### Scenario: New worktree
- **GIVEN** worktree mode, base `main`, and neither a branch `add-auth` nor `.worktrees/add-auth`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** `.worktrees/add-auth` is a worktree on the new branch `add-auth` at the tip of `main`, `git config branch.add-auth.agkBase` is `main`, and `git status` in the main checkout does not list `.worktrees/`

### Requirement: Base kept across a failed worktree add
When `git worktree add` fails after creating branch `<change>` (for example in a post-checkout hook), `branch.<change>.agkBase` SHALL keep the base resolved before the failure. When it fails and branch `<change>` does not exist, `agk apply start` SHALL remove the key again.

#### Scenario: Post-checkout hook fails
- **GIVEN** a main checkout on `main` with a post-checkout hook that exits 1
- **WHEN** `agk apply start add-widget --worktree` fails in `git worktree add`, the user switches the main checkout to `release` and runs `agk apply start add-widget --worktree` again
- **THEN** the second start reuses `.worktrees/add-widget` and `branch.add-widget.agkBase` is `main`

### Requirement: Conflicting branch or directory refused
In worktree mode, `agk apply start` SHALL refuse with exit 1, before the pull, when a branch `<change>` exists without the worktree `.worktrees/<change>`, or when `.worktrees/<change>` exists and is not a worktree of branch `<change>`.

#### Scenario: Branch exists without a worktree
- **GIVEN** worktree mode and a branch `add-auth` with no worktree
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the command exits 1 naming the branch, and no pull, commit, worktree or run directory happens

#### Scenario: Foreign directory in the way
- **GIVEN** worktree mode and a plain directory `.worktrees/add-auth` that is not a worktree
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the command exits 1 naming the directory and changes nothing

### Requirement: Reuse the change worktree
When `.worktrees/<change>` already is a worktree of branch `<change>`, `agk apply start` in worktree mode SHALL reuse it: no pull, no plan commit and no new branch. The change's apply state SHALL be read in that worktree, and the run SHALL start there.

#### Scenario: Second start after a crash
- **GIVEN** worktree mode and an existing worktree `.worktrees/add-auth` on branch `add-auth` with tasks still open
- **WHEN** the agent runs `agk apply start add-auth` again from the main checkout
- **THEN** no `git pull` and no commit run, a new run directory is created under `.worktrees/add-auth/.agk/applies/`, and stdout has `"worktree"` set to that worktree and `"pulled": false`

### Requirement: Start inside a linked worktree
When `agk apply start` runs inside a linked worktree rather than the main checkout, it SHALL create no worktree, pull nothing and commit nothing, and SHALL run the applier in that worktree. stdout `"worktree"` SHALL be that worktree's path when it is the change's worktree `.worktrees/<change>`, and `null` otherwise.

#### Scenario: Started from the change's worktree
- **GIVEN** worktree mode and the agent's working directory inside `.worktrees/add-auth`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** no worktree, pull or commit happens, the run directory is created under `.worktrees/add-auth/.agk/applies/`, and stdout has `"worktree"` set to `.worktrees/add-auth`

### Requirement: Sync and finish locate the change worktree
`agk sync <change> [--no-pull]` and `agk finish <change>` SHALL work from the main checkout or any worktree of the repository. Each SHALL refuse with exit 1, changing nothing, unless `.worktrees/<change>` is a worktree of branch `<change>`, `branch.<change>.agkBase` is set, and the main checkout has that base branch checked out. The one exception is a rerun of `agk finish` after an earlier merge.

#### Scenario: Main checkout on another branch
- **GIVEN** `branch.add-auth.agkBase` is `main` and the main checkout has `release` checked out
- **WHEN** the agent runs `agk sync add-auth` or `agk finish add-auth`
- **THEN** the command exits 1 naming both branches and changes nothing

#### Scenario: No change worktree
- **GIVEN** no worktree `.worktrees/add-auth` and no branch `add-auth`
- **WHEN** the agent runs `agk finish add-auth`
- **THEN** the command exits 1 naming the missing worktree and changes nothing

### Requirement: Sync requires committed work
`agk sync` SHALL refuse with exit 1, changing nothing, when the worktree has uncommitted or untracked files outside ignored paths.

#### Scenario: Uncommitted work before sync
- **GIVEN** `.worktrees/add-auth` with one modified tracked file
- **WHEN** the agent runs `agk sync add-auth`
- **THEN** the command exits 1 naming the file, and no pull or rebase runs

#### Scenario: Ignored run artifacts
- **GIVEN** `.worktrees/add-auth` whose only extra files are under the ignored `.agk/`
- **WHEN** the agent runs `agk sync add-auth`
- **THEN** the clean check passes

### Requirement: Sync rebases onto the updated base
`agk sync` SHALL update the base branch with `git pull --ff-only` under the same rules as `agk apply start` (skipped with no upstream or `--no-pull`), then rebase branch `<change>` onto the base branch in the worktree. It SHALL print `{"change","base","pulled","rebased","head"}`, where `rebased` says whether the branch tip changed and `head` is the new tip, and exit 0.

#### Scenario: Base moved on while implementing
- **GIVEN** a committed change on branch `add-auth`, and base `main` that gained a plan commit of another change after the worktree was created
- **WHEN** the agent runs `agk sync add-auth`
- **THEN** the commits of `add-auth` are replayed on top of `main`, and stdout has `"rebased": true` and the new tip

#### Scenario: Base unchanged
- **GIVEN** a committed change on branch `add-auth` that already starts at the tip of `main`, and no upstream commits
- **WHEN** the agent runs `agk sync add-auth`
- **THEN** the branch tip is unchanged and stdout has `"rebased": false`

### Requirement: Sync stops on a failed pull or conflict
A failed pull SHALL make `agk sync` exit 1 with git's message before any rebase. A rebase conflict SHALL abort the rebase, leaving branch `<change>` where it was, and exit 1 naming the conflicting files; a pull that already ran SHALL stay. The worktree and the branch SHALL stay in both cases. A sync interrupted during the rebase (SIGINT, SIGTERM, SIGHUP) SHALL still abort the rebase before it exits.

#### Scenario: Rebase conflict
- **GIVEN** a committed change whose branch and base both changed the same lines of a file
- **WHEN** the agent runs `agk sync add-auth`
- **THEN** the rebase is aborted, branch `add-auth` points where it pointed before, the worktree still exists, and the command exits 1 naming the file

### Requirement: Finish requires a committed, archived, synced change
`agk finish` SHALL refuse with exit 1, changing nothing, when the worktree has uncommitted or untracked files outside ignored paths, when the tree of branch `<change>` still holds `openspec/changes/<change>/` or holds no `openspec/changes/archive/*-<change>/` directory, or, unless branch `<change>` is already contained in the base branch, when the base branch has commits that branch `<change>` lacks. The last refusal SHALL tell the agent to run `agk sync`.

#### Scenario: Change not archived
- **GIVEN** a worktree `.worktrees/add-auth` whose branch still holds `openspec/changes/add-auth/`
- **WHEN** the agent runs `agk finish add-auth`
- **THEN** the command exits 1 saying the change is not archived, and the base branch, the branch and the worktree are unchanged

#### Scenario: Uncommitted work
- **GIVEN** an archived change in `.worktrees/add-auth` with one modified tracked file
- **WHEN** the agent runs `agk finish add-auth`
- **THEN** the command exits 1 naming the uncommitted file and changes nothing

#### Scenario: Base moved after the sync
- **GIVEN** an archived, committed change on branch `add-auth`, and base `main` that gained a commit after the last `agk sync add-auth`
- **WHEN** the agent runs `agk finish add-auth`
- **THEN** the command exits 1 saying `main` has commits the branch lacks and naming `agk sync`, and changes nothing

### Requirement: Finish fast-forwards the base
Once its checks pass, `agk finish` SHALL fast-forward the base branch to branch `<change>` in the main checkout with `git merge --ff-only`. It SHALL NOT pull or rebase. A failed fast-forward SHALL exit 1 with git's message and leave the base branch, the worktree and the branch unchanged.

#### Scenario: Synced change merged
- **GIVEN** an archived, committed change on branch `add-auth` that was synced onto the current tip of `main`
- **WHEN** the agent runs `agk finish add-auth`
- **THEN** `main` points at the tip of `add-auth`, with no merge commit and no pull

#### Scenario: Main checkout blocks the fast-forward
- **GIVEN** an archived, synced change that modifies `README.md`, and a main checkout with an uncommitted edit to `README.md`
- **WHEN** the agent runs `agk finish add-auth`
- **THEN** `main` is unchanged, the worktree and the branch still exist, and the command exits 1 with git's message

### Requirement: Finish cleans up
After the fast-forward, `agk finish` SHALL close the companion of every run under the worktree's `.agk/applies/` and `.agk/reviews/` (a companion already gone counts as closed), remove the worktree without forcing, and delete branch `<change>` with `git branch -d`. It SHALL then print `{"change","base","head","closed"}` (the base branch, its new tip, the run directories it closed) and exit 0.

#### Scenario: Clean finish
- **GIVEN** an archived, synced change in `.worktrees/add-auth` with an applier run whose companion is still open
- **WHEN** the agent runs `agk finish add-auth` and the fast-forward succeeds
- **THEN** the applier companion is closed, `.worktrees/add-auth` no longer exists, branch `add-auth` and `branch.add-auth.agkBase` are gone, and stdout names `main`, its new tip and the closed run directory

### Requirement: Cleanup failure keeps the merge
When a cleanup step fails, `agk finish` SHALL exit 1 naming what is left (open companions, the worktree, the branch), keep the merge, and skip the remaining cleanup steps.

#### Scenario: Worktree removal fails
- **GIVEN** a fast-forward that succeeded and a worktree that git refuses to remove
- **WHEN** `agk finish add-auth` reaches the removal
- **THEN** `main` keeps the merged commits, branch `add-auth` is not deleted, and the command exits 1 naming the worktree that is left

### Requirement: Finish after an earlier merge
When branch `<change>` exists, `branch.<change>.agkBase` is set and the branch is already contained in the base branch, `agk finish` SHALL skip the fast-forward and run only the cleanup steps still due: a missing worktree counts as already removed. A rerun after any failed cleanup step SHALL therefore complete it.

#### Scenario: Rerun after a failed removal
- **GIVEN** an earlier `agk finish add-auth` that fast-forwarded `main` and then failed to remove the worktree, which was since fixed
- **WHEN** the agent runs `agk finish add-auth` again
- **THEN** no merge runs, the worktree is removed, the branch is deleted, and the command exits 0

#### Scenario: Rerun after a failed branch delete
- **GIVEN** an earlier `agk finish add-auth` that merged and removed `.worktrees/add-auth` and then failed to delete branch `add-auth`
- **WHEN** the agent runs `agk finish add-auth` again
- **THEN** it skips the merge, the closes and the removal, deletes branch `add-auth` and its `agkBase`, and exits 0

#### Scenario: Base advanced between the attempts
- **GIVEN** an earlier `agk finish add-auth` that merged and then failed a cleanup step, after which another session committed to `main`
- **WHEN** the agent runs `agk finish add-auth` again
- **THEN** it does not ask for `agk sync`, skips the merge, completes the remaining cleanup and exits 0
