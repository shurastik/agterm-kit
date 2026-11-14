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

## `agk apply wait` reports `needs-human` / `no-report` while the applier is still working

**Seen:** 2026-11-08, round 1 of the `agk-explore-archive` applier (claude). The applier started
long eval runs as background shells, ended its turn while they ran, and resumed when they finished.
`agk apply wait` returned `{"status":"needs-human","reason":"no-report"}` at once, and kept doing so
on every later call while the companion screen showed the applier working.

**Cause (suspected):** the turn-end signal is the companion's agterm agent status. Claude's status
stays at done/idle after a turn that ends with background tasks pending, and the resumed turn does
not set it back to active. So `wait` sees "turn over, no report".

**Workaround used:** wait for `rounds/<k>/report.json` to appear, then run `agk apply wait`.

**Fix idea:** treat a round as finished only when the report exists or the status has been
done/idle for some time with no new output. Also check whether the claude hooks set `active` on a
resumed turn (background-task notification), and set it from agk if they do not.
