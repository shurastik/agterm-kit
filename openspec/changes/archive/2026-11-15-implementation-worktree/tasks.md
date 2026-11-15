# Tasks

## 1. The applier `worktree` setting

- [x] 1.1 Write failing tests in `internal/config/load_test.go` and `resolve_test.go`:
  - `[applier] worktree = true` loads, and resolves with the file as its source;
  - the default is `false` with source `default`;
  - `[reviewer] worktree = true` is refused naming `reviewer.worktree`;
  - `worktree = "yes"` is refused naming `applier.worktree`;
  - switching the harness by a later layer or by `--applier codex` keeps `worktree` and its source.

  Verify `go test ./internal/config` fails
- [x] 1.2 Add `Worktree *bool` to `Layer` and `Worktree bool` to `Worker` (`json:"worktree"`). Validate it as applier-only in `validate` and `apply`, keep it in `setHarness`, and add a `Flags.Worktree *bool` that sets it with source `flag`. Verify the 1.1 tests and `go test ./internal/config` pass
- [x] 1.3 Write failing tests in `internal/cli/config_test.go`: `agk config` lists the applier `worktree` with its source, and exits 1 on `[reviewer] worktree`. Make them pass in `internal/cli/config.go`, and verify `go test ./internal/cli -run Config` passes
- [x] 1.4 Document `[applier] worktree` (default `false`, what it turns on, the user-file tip for having it everywhere) in the README worker-configuration section. Verify `grep -n "worktree" README.md` shows the key in the config table or list

## 2. Git operations (`internal/worktree`)

- [x] 2.1 Create `internal/worktree` with a test helper that builds temporary repositories (main checkout plus a bare `origin`, with git identity, `commit.gpgsign=false` and `core.hooksPath` pointing at a test-controlled directory, as in `internal/review/scope_test.go`). Write failing tests for the locating operations:
  - main checkout and current worktree from inside the main checkout, a linked worktree and a subdirectory;
  - the current branch, and the error on a detached HEAD;
  - upstream present and absent;
  - lookup of `.worktrees/<change>`: a worktree of branch `<change>`, absent, a plain directory, and a worktree of another branch;
  - whether branch `<change>` exists.

  Verify `go test ./internal/worktree` fails
- [x] 2.2 Implement the locating operations over `git worktree list --porcelain`, `rev-parse` and `show-ref`. Verify the 2.1 tests pass
- [x] 2.3 Write failing tests for the start-side operations:
  - `PullFFOnly` fast-forwards a base behind `origin`, and fails on a diverged base and on a local edit that blocks it;
  - `CommitPaths` on an untracked `openspec/changes/c/` next to a modified `README.md` and a staged `main.go` commits only the change files with message `docs(openspec): plan c`, keeps `README.md` modified and `main.go` staged, does nothing on a clean directory, and returns the hook's output when a failing `pre-commit` hook is installed;
  - `Add` creates `.worktrees/c` on a new branch `c` at the base tip, sets `branch.c.agkBase`, and adds `/.worktrees/` to `info/exclude` only when `git check-ignore` does not already ignore it.

  Verify they fail
- [x] 2.4 Implement `PullFFOnly`, `CommitPaths` (pathspec `add` and `commit`, no `--no-verify`) and `Add`, including reading and writing `agkBase` and the exclude entry. Verify the 2.3 tests pass
- [x] 2.5 Write failing tests for the finish-side operations:
  - `Status` lists modified and untracked files, but not ignored `.agk/` files;
  - `ArchivedOnBranch` (read from the tree of branch `c`, so it works without the worktree) is false while `openspec/changes/c/` is committed, and true once it is replaced by `openspec/changes/archive/2026-01-01-c/`, but only after that is committed;
  - `IsAncestor`;
  - `Rebase` replays a branch onto a moved base, and on a conflict aborts, leaves the branch tip unchanged and returns the conflicting paths;
  - `MergeFFOnly` succeeds, and fails, leaving the base unchanged, when the main checkout has an uncommitted edit to a file the branch changes;
  - `Remove` succeeds with only ignored files and fails with an untracked file;
  - `DeleteBranch` removes the branch and its `agkBase`.

  Verify they fail
- [x] 2.6 Implement the finish-side operations. Verify `go test ./internal/worktree` and `go vet ./...` pass

## 3. Worktree mode of `agk apply start`

- [x] 3.1 Define the worktree interface the CLI needs, add `Env.Worktree` wired to `internal/worktree` in `NewEnv`, and add a recording fake for `internal/cli` tests with injectable results per operation. Verify `go build ./...` passes and the existing `go test ./internal/cli` stays green
- [x] 3.2 Write failing tests in `internal/cli/apply_test.go` for the flags and the context (fake worktree, fake `Ctl`):
  - `--worktree --no-worktree` exits 1 before any worktree call;
  - with the mode off, no worktree call runs and stdout has `"worktree":null,"pulled":false`;
  - inside a linked worktree with the mode on, no pull, commit or add runs, the run dir is under that worktree, and `worktree` is set only when that worktree is `.worktrees/<change>`.

  Verify they fail
- [x] 3.3 Write failing tests for a new worktree in the main checkout:
  - the call order is apply state in the main root, then pull, apply state again (only when the pull ran), `CommitPaths`, `Add`;
  - a change that is not ready on the second check exits 1 with no `CommitPaths`, `Add` or run dir;
  - the run dir and the companion `cwd` are in the worktree, and the inline headless round runs with the worktree as root;
  - stdout carries the absolute worktree path and `pulled` from the fake;
  - `--no-pull` and a missing upstream run no pull;
  - a detached HEAD, an existing branch without a worktree and a foreign directory each exit 1 with no pull, commit, add or run dir;
  - a not-ready change exits 1 before the pull;
  - a failing pull exits 1 before `CommitPaths`;
  - a failing commit exits 1 before `Add`.

  Verify they fail
- [x] 3.4 Write a failing test for the reuse path: an existing `.worktrees/<change>` of branch `<change>` gets its apply state read in the worktree, no pull, commit or add runs, and a new run dir is created under the worktree's `.agk/applies/`. Verify it fails
- [x] 3.5 Implement the worktree mode in `ApplyStart` (design decision 3): the flags into `config.Flags`, the context lookup, the refusals before the first change, the git steps, then the existing flow with the run root swapped, and `worktree` plus `pulled` in `applyStart`. Verify the 3.2–3.4 tests and `go test ./internal/cli` pass
- [x] 3.6 Update the `apply start` usage text in `internal/cli/apply.go` and `main.go` (`--worktree`, `--no-worktree`, `--no-pull`, the new stdout keys). Document worktree mode for `agk apply start` in `README.md`: the steps, the plan commit, the pull rules, reuse and the new stdout keys. Update the `agk apply start` line in `AGENTS.md` too. Verify `go test ./internal/cli -run Usage` (or the main usage test) passes and `grep -n "no-pull" README.md AGENTS.md` shows both

## 4. `agk sync` and `agk finish`

- [x] 4.1 Write failing tests in a new `internal/cli/sync_test.go` (fake worktree, a temp worktree path):
  - each location check exits 1 with no pull or rebase: a bad change name, no worktree `.worktrees/<change>`, a missing `agkBase`, the main checkout on another branch (both names in the message);
  - uncommitted files exit 1 naming them, with no pull;
  - the call order is pull, then rebase, and stdout is `{"change","base","pulled","rebased","head"}` with `rebased` from the tip comparison;
  - `--no-pull` and a missing upstream skip the pull;
  - a failed pull exits 1 with no rebase;
  - a rebase conflict exits 1 naming the files.

  Verify they fail
- [x] 4.2 Implement `agk sync <change> [--no-pull]` in `internal/cli/sync.go` (design decision 8), with the location checks shared with `finish`. Add it to the dispatch and usage text in `internal/cli/main.go`. Verify the 4.1 tests and `go test ./internal/cli` pass
- [x] 4.3 Write failing tests in a new `internal/cli/finish_test.go` (fake worktree, fake `Ctl`, run dirs created under a temp worktree path) for the checks and the merge:
  - the shared location checks exit 1 with no merge, close, remove or delete;
  - uncommitted files exit 1 naming them;
  - the change not archived on its branch exits 1;
  - a base that is not an ancestor of the branch exits 1 naming `agk sync`;
  - no pull and no rebase is ever called;
  - a failed ff-merge exits 1 with no cleanup.

  Verify they fail
- [x] 4.4 Write failing tests for the cleanup and the rerun:
  - the call order after the ff-merge is closes, remove, delete, and stdout is `{"change","base","head","closed"}`;
  - open companions of runs under the worktree's `.agk/applies/` and `.agk/reviews/` are closed through the fake `Ctl`, and a gone companion counts as closed;
  - a failed close exits 1 naming the run, with no remove or delete;
  - a failed remove exits 1 naming the worktree, with no delete;
  - a failed branch delete exits 1 naming the branch;
  - a rerun with the branch contained in the base skips the merge;
  - a rerun with the worktree already gone and the branch contained skips the merge, the closes and the removal, and deletes the branch;
  - a rerun after the base advanced past the merged branch (branch contained, base not an ancestor of the branch) does not refuse with `agk sync`, and completes the cleanup;
  - no worktree and no branch exits 1.

  Verify they fail
- [x] 4.5 Implement `agk finish <change>` in `internal/cli/finish.go` (design decision 8), reusing `closeCompanion` for the runs, and add it to the dispatch and usage text. Verify the 4.3–4.4 tests and `go test ./internal/cli` pass
- [x] 4.6 Add an end-to-end test in `internal/cli` that uses the real `internal/worktree` on a temporary repository with a bare `origin` and a fake `Ctl`:
  - `apply start` in worktree mode creates the worktree, with the fake driver, inline and with no applier work;
  - a code commit is made on the branch;
  - a plan commit of another change lands on `main`, and `agk sync` reports `rebased: true`;
  - `agk finish` before archiving refuses;
  - an archive commit is made, and a second `agk sync` reports `rebased: false`;
  - `agk finish` leaves `main` linear with the change's commits on top, `.worktrees/<change>` gone, and the branch and `agkBase` deleted.

  Verify it passes
- [x] 4.7 Document `agk sync` and `agk finish` in `README.md` (what each checks, why the rebase happens before the gates, the fast-forward-only merge, the cleanup, failures and rerun) and in the `AGENTS.md` command block, with `internal/worktree` in its Layout table. Verify `grep -n "agk sync\|agk finish" README.md AGENTS.md` shows both commands in both files

## 5. Skills and their evals

- [x] 5.1 Update `skills/agk-apply/SKILL.md`: the start step reads `worktree` from stdout and, when it is set, works in that directory (`cd <worktree>`) for the review and verify steps. Mention `--worktree`/`--no-worktree`/`--no-pull` and that the work is finished by `/agk-archive`. Verify `grep -n "worktree" skills/agk-apply/SKILL.md` shows the step
- [x] 5.2 Add the stand-in support for the archive-skill evals:
  - `sync` in `evals/_fixtures/bin/agk`: it logs its arguments, prints `rebased: true` on the n-th call when the case outcome has a `sync-rebased-<n>` marker (`false` otherwise), and exits 1 with a conflict message on `sync-conflict`;
  - `finish`: it logs its arguments, prints a success JSON, or exits 1 with a base-moved message on `finish-fail`;
  - a scaffold option in `evals/_fixtures/scaffold-archive.sh` that leaves the workspace repository on branch `add-widget` with `branch.add-widget.agkBase=main` and `main` holding the plan commit;
  - self-test cases in `evals/_fixtures/test.sh` for all of these.

  Verify `evals/_fixtures/test.sh` passes
- [x] 5.3 Add the eval cases (case dir, case plugin, outcomes, graders that read the stand-in log and `git log`):
  - `worktree-archive`: Change worktree detection, Fallback review in a change worktree, Code committed and synced first, Finish succeeds, Archive done in a change worktree;
  - `worktree-sync-conflict`: Sync conflict;
  - `worktree-gate-fails`: Gate fails in a change worktree;
  - `worktree-verified-then-rebased`: Verified before a rebasing sync;
  - `worktree-base-moved`: Base moved during the gates;
  - `worktree-finish-fails`: Finish fails;
  - `worktree-archive-failed`: Archive failed in a change worktree;
  - `branch-without-base`: Branch without a recorded base.

  Add them to the scenarios table in `evals/README.md`. Verify `evals/_fixtures/test.sh` passes and each new case directory has `case.yaml`, `prompt.md`, `scaffold.sh` and graders
- [x] 5.4 Update `skills/agk-archive/SKILL.md` and its description (spec `archive-workflow` as modified):
  - the worktree detection;
  - the code commit and `agk sync` before the gates, and stop on a sync failure;
  - the fallback base from `agkBase`;
  - a rebasing sync counts as a code change for verify;
  - the second `agk sync` before the archive, which loops back to the gates when it rebased;
  - the archive commit after the closes;
  - `cd` to the main checkout and `agk finish`;
  - stop on a finish failure;
  - the report lines;
  - "never commits" narrowed to the main checkout, and never pushes.

  Run the new cases with `scripts/eval-skills.sh --case <case> --runs 3`, and also the existing `fresh-session-archive`, `approved-in-session` and `verify-critical`. Verify each scores at least 0.8
- [x] 5.5 Update the workflow text in `README.md` and `AGENTS.md`: the overview of `agk-archive`, which no longer says it never commits, and the flow from apply in a worktree through `/agk-archive` (sync, gates, archive, finish). Verify `grep -n "never commits" README.md AGENTS.md` no longer states it without the main-checkout qualifier

## 6. Integration

- [x] 6.1 Run `go build -o .bin/agk ./cmd/agk`, `go test ./...`, `go vet ./...` and `evals/_fixtures/test.sh`. Verify they all pass
- [x] 6.2 Manual check with the built binary in a scratch clone (with an `origin`) and `[applier] worktree = true`:
  - `.bin/agk apply start <change>` outside agterm commits the plan and creates the worktree;
  - commit a stub code change on the branch, push a commit to `origin`, and run `.bin/agk sync <change>`, which reports `rebased: true`;
  - commit an archived change, then run `.bin/agk finish <change>`, which merges and removes the worktree;
  - `git log --oneline --graph` shows linear history.

  Record the commands and the output summary in the change's final report

## Workflow follow-up

- Code review of the diff, `/opsx:verify` with 0 CRITICAL, then `/agk-archive`.
- After release, set `[applier] worktree = true` in `~/.config/agk/config.toml` to use the mode by default.

## 7. Code review round 1 fixes

- [x] 7.1 `agk-archive` detects a change worktree only in a linked worktree (`git rev-parse --git-dir` differs from `--git-common-dir`), plus the branch and `agkBase` checks (spec `archive-workflow`, Change worktree detection). Add the eval case `branch-in-main-checkout`: `add-widget` with `agkBase` checked out in the main checkout; no commit, no `agk sync`, no `agk finish`
- [x] 7.2 The archive commit is a pathspec commit of `openspec/` only; after it the skill checks that the commit holds only `openspec/` paths and that `git status --porcelain` is empty, and otherwise reports and runs no `agk finish`. Add the eval case `worktree-code-after-gates`
- [x] 7.3 Change the worktree eval scaffolds to a real linked worktree (`git worktree add` from a main repository next to the workspace) instead of branch state only (design decision 11). Restore the `reports-finish` llm grader in `worktree-archive`; it failed because the scaffold put the main checkout in the workspace, not because of the grader. Re-score every worktree case and `branch-without-base` (3 runs, >= 0.8)
- [x] 7.4 `apply start` reusing a worktree records `branch.<change>.agkBase` when it is missing (a `git worktree add` that failed in a post-checkout hook after creating the worktree); test it with a failing post-checkout hook on a temp repository
- [x] 7.5 A sync interrupted during the rebase still runs the conflict listing and `rebase --abort` with a context that is not the cancelled one (`context.WithoutCancel` plus its own timeout); test that a cancelled context during the rebase leaves the branch at its old tip and no rebase in progress
- [x] 7.6 Re-run the gates of 6.1
- [x] 7.7 Record `branch.<change>.agkBase` before `git worktree add -b`, from the base resolved before any change, and remove it when the add fails and branch `<change>` does not exist (spec `change-worktree`, Create the change worktree). Drop the reuse-time `recordMissingBase`, which wrote the main checkout's current branch rather than the original base and was skipped for a start inside the linked worktree. Tests on real git: a failing post-checkout hook keeps `agkBase` = the original base, also after the main checkout switched branches; an add that fails before creating the branch leaves no `agkBase`. Re-run the gates of 6.1
