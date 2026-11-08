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

## `agk review send` to an interactive codex reviewer does not submit the kickoff line

**Seen:** 2026-11-08, rounds 2 and 3 of an interactive codex reviewer started by the checkout
`agk` (after `interactive-reviewer`). `agk review send` printed `warning: reviewer ... did not
report round N as started: wait timed out`. The companion showed the line
`Read .../rounds/N/prompt.md and follow it.` in codex's input box, typed but not submitted. Round 1
(at `start`) was fine.

**Workaround used:** `agtermctl session type "$(printf '\r')" --target <companion>` submitted it,
and the round ran normally.

**Cause (suspected):** the Enter is typed in the same burst as the text and codex's TUI drops it,
or it arrives before the input box is ready after the previous turn.

**Fix idea:** send the Enter as a separate `type` call after a short delay, or after the text shows
up in the screen buffer. If the round does not report started within a few seconds, retry the Enter
once before the warning. Check the applier path too (`kickoff` in `internal/cli/apply.go`), since it
uses the same mechanism.
