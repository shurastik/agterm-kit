# Proposal

## Why

The applier implements a change in the main checkout, next to the user's own edits and next to other
sessions working on other changes. Nothing isolates one change's code from another's, and closing a
change leaves committing and merging to the user by hand. Implementation should run on a branch of
its own in a git worktree, and `/agk-archive`, which already marks the end of the work, should merge
that branch into the shared branch and clean up after itself. The plan itself should land on the
shared branch before implementation starts, so that parallel sessions see which changes are in
flight.

## What Changes

- **Worktree mode for `agk apply start`.** When enabled, starting a change from the main checkout:
  1. updates the current branch with `git pull --ff-only`. It skips the pull when the branch has no
     upstream or `--no-pull` is given, and stops when the pull fails;
  2. commits the change's artifacts (`openspec/changes/<change>/`) on that branch, and only those
     paths, with a fixed message. Nothing happens when they are already committed;
  3. creates the worktree `.worktrees/<change>` on a new branch `<change>` and records the base
     branch in `git config branch.<change>.agkBase`;
  4. runs the applier, its run directory and its companion in the worktree. stdout gains
     `"worktree"` (the path, or `null`) and `"pulled"`.
  After a pull, the change's apply state is read again before the plan commit. A second
  `apply start` for the same change reuses the existing worktree.
- New config key `[applier] worktree` (boolean, default `false`) with the flags `--worktree` /
  `--no-worktree` on `agk apply start`; `agk config` shows it with its source.
- **New command `agk sync <change>`.** On a clean worktree it pulls the base (`--ff-only`, same
  rules) and rebases the change's branch onto it, printing whether the branch moved. A conflict
  aborts the rebase and leaves the branch as it was.
- **New command `agk finish <change>`.** It merges a finished worktree change and removes it:
  - it requires a clean worktree, the change archived on its branch, and a branch already synced
    onto the current base;
  - it fast-forwards the base in the main checkout, with no pull and no rebase, so the merged tree
    is exactly the one the gates checked;
  - it closes the companions of the runs under the worktree's `.agk/`, removes the worktree and
    deletes the branch with `git branch -d`.
  A refusal or a failed fast-forward changes nothing. A rerun after the merge finishes whatever
  cleanup is left, also when the worktree is already gone.
- **`/agk-archive` finishes a worktree change.** It applies when the session works in the change's
  worktree: the current branch is `<change>` and `branch.<change>.agkBase` is set. The steps:
  1. commit the code, then `agk sync`, so the review and verify gates check the rebased code. A
     rebasing sync counts as a code change for verify;
  2. after the gates, run `agk sync` again. If the branch moved, both gates run again;
  3. archive, then commit the archive;
  4. run `agk finish <change>` from the main checkout.
  In the main checkout the skill still never commits. It never pushes in either case.
- Skills and docs: `agk-apply` tells the main session to work in the reported worktree, and
  `agk-archive` gains the sync and finish steps. New eval cases cover the worktree path of `agk-archive`.
  `README.md` and `AGENTS.md` describe the mode, the key, `agk sync` and `agk finish`.

## Capabilities

### New Capabilities

- `change-worktree`: the git side of worktree mode. It covers preparing the base branch (pull, plan
  commit) and creating or reusing `.worktrees/<change>` for `agk apply start`. It also covers
  `agk sync` rebasing the branch, and `agk finish` fast-forwarding the base and removing the
  worktree.

### Modified Capabilities

- `apply-worker`: the run directory, the companion's working directory and the inline run move into
  the change's worktree in worktree mode, and `apply start` stdout gains `worktree` and `pulled`.
- `worker-config`: the applier gains the `worktree` key and the `--worktree` / `--no-worktree`
  flags. Validation and `agk config` cover them.
- `archive-workflow`: in a change worktree the skill commits the code and the archive, runs
  `agk sync` before the gates and again before the archive, and runs `agk finish`.
  The "No commits and no fixes" requirement becomes "commits only in a change worktree,
  never fixes". New eval cases cover these scenarios.

## Impact

- Code: a new `internal/worktree` package with git operations on a real repository, tested against
  temporary repositories, including a bare remote for pull. `internal/config` gets the `worktree` key.
  `internal/cli`: `apply start` worktree mode, the new `sync` and `finish` commands and the usage text. `Env`
  gains an injectable worktree interface, so CLI tests assert calls against a fake.
- Skills: `skills/agk-apply/SKILL.md`, `skills/agk-archive/SKILL.md` and its description.
- Evals: new `agk-archive` cases, `sync` and `finish` support in the stand-in `evals/_fixtures/bin/agk`, and
  a scaffold that puts the workspace on the change's branch with `agkBase` set. `evals/README.md`
  lists the cases.
- Docs: `README.md` (worktree mode, the config key, `agk sync`, `agk finish`, workflow) and `AGENTS.md`
  (commands, layout, conventions).
- Behaviour: off by default, so existing users see no change. The user who wants it everywhere sets
  `[applier] worktree = true` in `~/.config/agk/config.toml`. With the mode on, agk commits to the
  user's base branch (the plan) and merges into it (`agk finish`). It never pushes.
- Depends on `agk-explore-archive` (archived): `archive-workflow` and `agk review fresh` exist in the
  main specs. The worktree flow also avoids the `fresh` staleness after committing on the base
  branch (BACKLOG), because the code is committed on the change's branch.
