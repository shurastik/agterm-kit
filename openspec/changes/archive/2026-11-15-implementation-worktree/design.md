# Design

## Context

`agk apply start` resolves the repository root with `git rev-parse --show-toplevel` (`Env.RepoRoot`),
checks the change with `openspec instructions apply` (`Env.ApplyState`), resolves the worker config
from that root and creates the run directory, the companion's working directory and the inline run
all under it. Git is called directly only by `internal/review` (`gitOut`), and its tests run real git
on temporary repositories. agtermctl, the harnesses and openspec go through injected fakes. A run
directory's root is recovered from its path (`rundir.Root`), so `wait`, `send` and `close` work on
any root without knowing about worktrees.

`agk-archive` (archived with `agk-explore-archive`) gates on the review (`agk review fresh`) and on
verify, archives, closes companions and never commits. The applier is denied `git commit` by its
settings and sandbox. Both stay that way: commits in this change come from agk itself (the plan) and
from the main session following `agk-archive` (code and archive).

Two git behaviors this design relies on were checked on a scratch repository. `git worktree remove`
without `--force` succeeds when the only extra files are ignored (`.agk/`) and refuses untracked
ones. `git branch -d` removes the branch's `branch.<name>.*` config section.

## Goals / Non-Goals

**Goals:**
- Deterministic git steps in Go: every refusal is checked before anything changes, and every failure
  leaves the base branch, the worktree and the branch as they were, or with the merge kept and the
  leftovers named.
- No change for users who do not enable the mode.
- The same `apply wait/send/close` and `review` commands work unchanged inside a worktree.

**Non-Goals:**
- Pushing anything, or opening pull requests.
- Worktrees for the reviewer, or a worktree per review round.
- Resolving rebase conflicts, or recovering a branch whose worktree was removed by hand.
- Moving the run artifacts of a removed worktree into the main checkout.

## Decisions

1. **Git steps in Go, orchestration in the skill.** `agk apply start` and the new `agk sync` and
   `agk finish` run the git steps. `agk-archive` only decides when to commit, sync and finish.
   *Alternatives:* skill-only git (the model runs `git merge` and `git worktree remove` from prose,
   checked only by evals), or `bd worktree create/remove` (it couples agk to beads, which agk does not
   otherwise know about).

2. **A `internal/worktree` package over real git, behind an `Env` interface.** The package runs git
   with `exec` like `internal/review` and is tested on temporary repositories, including a bare
   repository as `origin` for the pull cases. It exposes small operations:
   - locate the main checkout and the current worktree (`git worktree list --porcelain`, whose first
     entry is the main checkout);
   - current branch and upstream;
   - `PullFFOnly`, `CommitPaths`, `Add`;
   - `Status`, `ArchivedOnBranch`, `IsAncestor`, `Rebase` (with abort and conflict list),
     `MergeFFOnly`, `Remove`, `DeleteBranch`;
   - read and write `agkBase`, ensure the exclude entry.
   `Env.Worktree` holds the interface, so `internal/cli` tests assert the call sequence and inject
   failures with a fake, as they do for agtermctl.
   *Alternative:* fakes at the exec level for git too. That gives weaker tests for rebase and pull
   semantics than real repositories, and the review code already sets the precedent.

3. **Order inside `apply start`.** Every refusal comes before the first change:
   1. parse the flags and refuse both worktree flags together;
   2. resolve the repo root and the config from it;
   3. locate the worktree context: if the cwd is inside a linked worktree, use it as the run root
      and skip the git steps;
   4. in the main checkout with the mode on, look up `.worktrees/<change>`:
      - a worktree of branch `<change>`: reuse it and read the apply state there;
      - otherwise refuse a detached HEAD, an existing branch `<change>` or a foreign directory, then
        read the apply state in the main checkout;
   5. for a new worktree: pull; when the pull ran, read the apply state again; commit the plan, add
      the exclude entry, set `agkBase`, `git worktree add -b` (the base is recorded first, so a
      hook failing after the add created the branch keeps it; a failed add without the branch removes
      the key);
   6. continue the existing flow with the run root swapped.
   Because the first apply-state check comes before the pull, a change that is not ready creates
   nothing. The second check catches a change finished or removed upstream. The pull it follows is
   kept, since it only fast-forwards the base. A failure after the plan commit (for example in
   `worktree add`) keeps that commit, since it is wanted anyway, and exits 1.

4. **The plan commit through a pathspec.** `git add -- openspec/changes/<change>` then
   `git commit -m "docs(openspec): plan <change>" -- openspec/changes/<change>`. A pathspec commit
   implies `--only`: other staged entries stay staged and are not committed. When
   `git status --porcelain -- <dir>` is empty, nothing is committed. Hooks run normally. A failing
   hook stops the start with its output.
   *Alternative:* `git stash` around a plain commit. It touches the user's unrelated work.

5. **The base recorded in `git config branch.<change>.agkBase`.** Git config is shared by all
   worktrees, survives the removal of `.agk/`, and disappears with `git branch -d`. It also serves as
   the marker `agk-archive` uses to recognise a change worktree.
   *Alternatives:* `worker.json` (lives in the worktree's `.agk/`, one per run, so `finish` would
   have to search runs), or a file in the git common directory (needs its own cleanup).

6. **`.worktrees/` hidden through `<git-common-dir>/info/exclude`.** It is local, needs no commit
   and leaves the user's `.gitignore` alone. When `git check-ignore -q .worktrees/` already
   succeeds, nothing is written.

7. **Pull rules shared by start and sync.** The upstream comes from
   `git rev-parse --abbrev-ref <base>@{upstream}`. When there is none, no pull runs and the result is
   `pulled: false`. Otherwise `git pull --ff-only` runs in the main checkout, which has the base
   checked out in both commands. A diverged base, an unreachable remote or local edits that block
   the fast-forward stop the command with git's message. `--no-pull` is the escape hatch for offline
   work.

8. **Rebase in `sync`, before the gates; only a fast-forward in `finish`.** Rebasing at merge time
   would put a tree into the base that no review or verify saw. A rebase without a textual conflict
   can still break the build, for example a call to a function whose signature changed upstream.
   So the rebase is its own command, and the skill runs the gates after it. `finish` merges only a
   branch that already contains the base, so the merged tree is the one the gates checked.
   - **`sync`:**
     1. locate the worktree, `agkBase` and the main checkout on the base;
     2. check that the worktree is clean (`git status --porcelain`, which leaves out ignored files);
     3. pull;
     4. `git -C <worktree> rebase <base>`. On a conflict, record
        `git diff --name-only --diff-filter=U`, then `rebase --abort`;
     5. report `rebased` by comparing the branch tip before and after.
     A failed rebase leaves the branch where it was. A pull that ran stays, since it only
     fast-forwards the base.
   - **`finish`, the rerun check first:** when `<change>` is already an ancestor of the base, the
     merge was done by an earlier attempt. The sync check below is skipped, since the base may have
     moved on since, and so is the merge.
   - **`finish`, the checks:**
     1. the same location checks;
     2. a clean worktree;
     3. archived in the branch's tree (`git ls-tree <change>` on `openspec/changes/<change>` and
        `openspec/changes/archive/*-<change>`);
     4. unless the rerun check matched, the base is an ancestor of `<change>`. Otherwise refuse and
        point at `agk sync`.
   - **`finish`, the merge:** `git -C <main> merge --ff-only <change>`. A failure changes nothing.
   - **`finish`, the cleanup:** close every run's companion under the worktree's `.agk/applies` and
     `.agk/reviews` with the existing close code, `git worktree remove` without `--force`, then
     `git branch -d`.
   - **Rerun:** when `<change>` is already an ancestor of the base, `finish` skips the merge. When
     the worktree is already gone, it skips the closes and the removal too, and deletes the branch.
   Rebase plus fast-forward keeps the history linear and makes `branch -d` safe, because the branch
   is then contained in the base.
   *Alternatives:* rebase inside `finish` (merges unchecked trees, the review finding that led
   here); `merge --no-ff` (a merge commit per change, and conflicts surface in the main checkout);
   squash (the branch is not contained, so deleting it needs `-D`).

9. **`agk-archive` in a change worktree: commit and sync first, sync again last.** The worktree is
   detected by a linked worktree (`git rev-parse --git-dir` differs from `--git-common-dir`),
   `git rev-parse --abbrev-ref HEAD` equal to the change and `agkBase` set. The linked-worktree check
   keeps a change branch checked out in the main checkout from being treated as one, where
   `agk finish` would remove the main checkout's own directory. Then:
   1. before the gates, `git add -A` and one commit with a conventional message the agent writes,
      then `agk sync`. The gates therefore see the committed, rebased code, including anything a
      pre-commit hook changed. A plain commit does not stale an earlier approve, because the
      fingerprint hashes content and the merge base does not move. A rebasing sync moves the merge
      base, so `agk review fresh` turns false, and the skill counts it as a code change for verify;
   2. after the gates and the freshness check, `agk sync` again. `rebased: false` means the gates
      saw exactly the tree that will be merged. `rebased: true` loops back to the review gate;
   3. after the archive and the closes, a pathspec commit `chore(openspec): archive <change>` of
      `openspec/` only. Code changed after the gates (an editor, a hook) stays out of it; the skill
      then checks that the commit holds only `openspec/` paths and the worktree is clean, and stops
      before `finish` otherwise, since `finish` would refuse the dirty worktree or merge unchecked
      code from a hook;
   4. `cd` to the main checkout (the first `worktree` line of `git worktree list --porcelain`), then
      `agk finish <change>`.
   The fallback review uses `--base <agkBase>`. The `cd` matters because the worktree directory is
   removed under the shell otherwise. A finish that refuses because the base moved during the
   archive (seconds) is reported, not handled: the user decides whether to sync and check by hand.

10. **stdout stays additive.** `apply start` always prints `worktree` and `pulled`. Existing callers
    ignore unknown keys. `sync` and `finish` each print one JSON object on success. Failures go to
    stderr with exit 1, matching the other commands.

11. **Evals run in a real linked worktree.** A scaffold creates the repository on `main` next to
    the workspace and makes the workspace a linked worktree of it on branch `add-widget`
    (`git worktree add`), with `branch.add-widget.agkBase=main`, which is exactly what the skill
    detects. Branch state alone is not enough: the skill would then find the main checkout in its
    own directory, and a careful agent rightly refuses to run `agk finish` there. The stand-in
    `agk` gains:
    - `sync`: it logs its arguments and prints `rebased` from the case outcome (`sync-rebased-<n>`
      marks the n-th call as rebasing), or exits 1 with a conflict message on `sync-conflict`;
    - `finish`: it logs its arguments and succeeds, or exits 1 with a base-moved message on
      `finish-fail`.
    The child's real `git commit` runs in the scaffold repository, and graders read `git log` and
    the stand-in log. The cases:
    - `worktree-archive`: detection, fallback base, code committed and synced first, finish
      succeeds, archive done in a worktree;
    - `worktree-sync-conflict`;
    - `worktree-gate-fails`;
    - `worktree-verified-then-rebased`: a rebasing first sync re-runs verify;
    - `worktree-base-moved`: a rebasing second sync re-runs both gates;
    - `worktree-finish-fails`;
    - `worktree-archive-failed`;
    - `branch-without-base`;
    - `branch-in-main-checkout`: `add-widget` with `agkBase` checked out in the main checkout;
    - `worktree-code-after-gates`: a code file changes after the second sync.

## Risks / Trade-offs

- [agk commits on the user's base branch without asking] → The mode is opt-in. The commit holds
  only `openspec/changes/<change>/` with a fixed message and is never pushed.
- [A pre-commit hook fails or is slow on the plan commit] → The start stops with the hook's output.
  Nothing else is created.
- [Rebase rewrites the change branch] → The branch is local and never pushed. A conflict aborts the
  rebase and restores the branch.
- [Every base movement during the gates costs another review and verify round] → It is the price of
  merging only checked trees. The first sync, before the gates, absorbs what moved during
  implementation, which is the long window.
- [Worktree removal deletes the run logs under its `.agk/`] → Accepted. The code and the archive
  are in git history, and the logs are gitignored scratch.
- [Two sessions finish changes on the same base at once] → The second `finish` sees a base its
  branch lacks and refuses, changing nothing. Its user syncs and checks again.
- [The shell of the main session sits in a removed directory] → The skill changes to the main
  checkout before `agk finish`. `finish` itself never needs the current directory to survive.
- [Plan reviewer runs live in the main checkout's `.agk/`, not in the worktree] → `/agk-propose`
  closes them when it finishes. `finish` closes only the worktree's runs.
- [A user removes a worktree by hand] → Unless the branch is already merged, `finish` refuses, naming the missing worktree. The branch
  and `agkBase` are the user's to delete. A later `apply start` refuses the existing branch.

## Migration Plan

Ship in the next release. `worktree` defaults to `false`, so nothing changes until a user sets
`[applier] worktree = true` (in `~/.config/agk/config.toml` to have it everywhere) or passes
`--worktree`. `agk setup` relinks the updated skills. To roll back, unset the key. Worktrees already
created can still be finished with `agk finish`, or removed by hand.
