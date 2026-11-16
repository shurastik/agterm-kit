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

<!-- Migrated from beads -->

## Review companion name: plan/code kind and round number (was agk-7t2)

**Type:** feature · **Priority:** P2

The reviewer companion is named <harness>-review #N where N is the repo-wide run number (#15 today), which reads like a pass number and does not say what is reviewed. Wanted: the name shows the kind (plan review via --change vs code review) and the number counted inside one agk review run, i.e. the current round, updated on every agk review send. Modifies the review-companion 'Review number' requirement and the companion name. Separate OpenSpec change after agk-propose.

## agk apply wait reports needs-human/no-report while the applier sleeps on its own Monitor (was agk-cii)

**Type:** bug · **Priority:** P2

Seen in interactive-reviewer apply round 2: the claude applier ended its turn waiting for a background Monitor (skill eval run), agent status went idle, no report yet. agk apply wait returned needs-human/no-report at once, twice, although the applier resumed by itself later and wrote a done report. Wait cannot tell 'turn over, waiting for the user' from 'turn over, sleeping until its own background task fires'. Worked around by waiting for rounds/<k>/report.json directly. Options: tell the applier (rules/prompt) to keep its turn alive (block on the background task) instead of ending it; or have wait treat idle with running background shells/monitors differently if agterm/claude exposes that; or a grace period before no-report.
