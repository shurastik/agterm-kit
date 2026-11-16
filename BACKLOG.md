# Backlog

Known bugs and follow-ups, newest first. Each entry becomes an OpenSpec change (`/agk-explore`)
when it is picked up.

## `agk review fresh` goes stale after committing on the base branch

**Seen:** 2026-11-08, archiving `agk-explore-archive`. The work was done directly on `main` and
reviewed with `agk review start --base main`. Right after `git commit` on `main`,
`agk review fresh` reported `{"fresh":false}`, although the commit held exactly the reviewed tree.

**Cause:** `fresh` recomputes `merge-base(<base>, HEAD)`. A commit on the base branch itself moves
that merge base to the new commit, so the scope becomes empty, which counts as not fresh. The
promise "committing does not change the fingerprint" holds only on a feature branch.

**Impact:** in a commit-on-main workflow, `/agk-archive` almost always fails its review gate after
the commit. Its fallback then hits `nothing to review` and has to ask the user.

**Fix idea:** record the round's merge base next to `scope.sha256`, and have `fresh` compute the
current scope against that recorded merge base instead of a fresh `merge-base`. Moving the base by
a rebase or merge should still count as stale. That needs a rule for "the recorded merge base is no
longer an ancestor of HEAD".

## Review companion name: plan/code kind and round number (was agk-7t2)

**Type:** feature · **Priority:** P2

The reviewer companion is named <harness>-review #N where N is the repo-wide run number (#15 today), which reads like a pass number and does not say what is reviewed. Wanted: the name shows the kind (plan review via --change vs code review) and the number counted inside one agk review run, i.e. the current round, updated on every agk review send. Modifies the review-companion 'Review number' requirement and the companion name. Separate OpenSpec change after agk-propose.
