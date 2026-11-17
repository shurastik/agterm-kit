# Proposal

## Why

`agk review fresh` recomputes `merge-base(<base>, HEAD)` every time it runs. When the work is
committed on the base branch itself (a commit-on-main workflow), that commit moves the merge base
onto itself. The scope becomes empty and an approve reads as stale, even though the commit holds
exactly the reviewed tree. `/agk-archive` then fails its review gate almost every time after the
commit, and its fallback (`agk review start --base main`) hits `nothing to review` and has to ask
the user for a base.

## What Changes

- A diff review round records the merge base its scope was built from in `rounds/<k>/merge-base`,
  next to `scope.sha256` and before the round's reviewer starts.
- `agk review fresh` collects the current scope against the round's recorded merge base instead of
  recomputing it from the run's base, so a commit on the base branch that keeps the reviewed tree
  keeps the round fresh. An edit, a rebase onto a moved base, or a merge still makes it stale, and
  so does history where the recorded merge base is no longer an ancestor of `HEAD`.
- A round written before this change (with `scope.sha256` but without `merge-base`) is checked as
  before, against the merge base recomputed from the run's base.
- `agk review fresh` prints the merge base it compared against: `{"fresh":…,"round":K,"merge_base":"<sha>"}`.
  It adds a field; existing fields keep their meaning.
- `/agk-archive`: when its fallback review outside a change worktree answers `nothing to review`
  and the session's own diff review run for the change gave a `merge_base`, the skill retries the
  fallback once with `--base <merge_base>` instead of asking the user. Without one it asks, as now.
  A change worktree keeps reviewing against its recorded base.
- README and the command list in `AGENTS.md` describe the recorded merge base and the new field;
  the `agk-archive` skill drops the claim that the merge base never moves on a plain commit.
- The backlog entry "`agk review fresh` goes stale after committing on the base branch" is removed.

## Capabilities

### New Capabilities

### Modified Capabilities
- `review-companion`: a diff round records its merge base; the freshness check compares against
  that recorded merge base and prints it.
- `archive-workflow`: the review fallback retries `nothing to review` with the merge base from the
  session's own review run, outside a change worktree.

## Impact

- `internal/cli/review.go`: `initRound`, `reviewScope`/`reviewPrompt` (carry the merge base),
  `ReviewFresh` (collect against the recorded merge base, print `merge_base`).
- `internal/review`: a new ancestry check of a commit against `HEAD` (injected into `Env` like
  `Collect`); `Collect(root, <merge-base sha>)` already yields that sha as the merge base when it is
  an ancestor of `HEAD`.
- `skills/agk-archive/SKILL.md`, the eval stand-in `evals/_fixtures/bin/agk` and a new eval case
  for the retry.
- `README.md`, `AGENTS.md`, `BACKLOG.md`.
- No new dependency. Run directories from older agk keep working.
