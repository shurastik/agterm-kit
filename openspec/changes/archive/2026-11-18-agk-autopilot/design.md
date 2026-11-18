# Design

## Context

The workflow is spread over skills that each stop for the user at defined points:
`agk-propose` (questions of `openspec-propose`, disputed plan findings, `needs-attention`, the revdiff
pass, the stop before implementation), `agk-apply` (`blocked`, `needs-human`), `agk-review`
(disputed code findings, `needs-attention`) and the verify step (CRITICAL, WARNING triage).
`/agk-archive` accepts a run only when the last code review approved the current files and verify
passed after it (`agk review fresh`, this session's own results). The applier's rules forbid it to
guess: on any ambiguity it reports `blocked`. The review scope is every changed file except
`.agk/`, so files under `openspec/changes/<change>/` are part of the code review diff.

`openspec validate --strict` accepts an extra `decisions.md` in a change directory (checked on a
copy of an archived change).

## Goals / Non-Goals

**Goals:**
- One skill that drives the existing loops unattended and leaves a run `/agk-archive` accepts as is.
- One place, kept with the change, where every model decision taken for the user is readable.

**Non-Goals:**
- No `agk` subcommand: the decisions are made by the main session's model, not by the CLI.
- No archive, commit, push or merge; no change to the existing skills' behavior.
- No unattended answering of permission or trust prompts.

## Decisions

### The skill follows the sibling skills with a list of overrides

`skills/agk-autopilot/SKILL.md` says, step by step, which sibling skill to follow (`agk-propose`
steps 0–2, `agk-apply`, `agk-review`, `openspec-verify-change`) and lists the overrides that replace
every "ask the user" and "stop" in them: decide and log; never launch revdiff; do not stop after the
plan; round limits. The same pattern already works for `agk-explore` over `openspec-explore`.

Alternative: copy the loops into the new skill. Rejected: two copies of the plan review loop and the
manifest checks would drift. Risk of the chosen way: a sibling rewording can change autopilot
behavior; the eval cases catch the visible part.

### Decision log in the change directory

`openspec/changes/<change>/decisions.md`, one `## D<n> [scope|impl] <title>` entry with `Open:`,
`Decision:` and `Why:` lines. `[scope]` covers what users or callers of the change would see
(behavior, interface, requirement added or dropped); everything else is `[impl]`. The file is created
right after `openspec-propose` writes the change, before the first plan review round, so the plan
reviewer reads the propose-time decisions together with the plan; it is archived with the change.

Alternative: `.agk/autopilot/<run>/`. Rejected: gitignored and invisible to the reviewers, gone after
cleanup.

### Write the decision before the round that follows it

A decision is appended before the review round, note or verify run that acts on it, never after the
last one. A decision that can only arise after verify (a WARNING set aside) therefore triggers one
more code review round and one more verify.

Ordering alone does not prove freshness: an editor or another process can touch a file while a
round or verify runs. So the skill checks it the way `/agk-archive` does: `agk review fresh <dir>`
right after a code review approve (before verify) and again right after a verify with 0 CRITICAL
(before closing anything). `fresh: false` sends the run back to a code review round and verify,
within the round limit; an exit 1 of `agk review fresh` stops the run with its error in the report.
The archive step later finds a fresh approve and a verify after it in this session.

### Plan reviewer lifetime and the worktree

- Main checkout (no worktree): the plan reviewer started by the plan loop stays open after its
  approve. An applier `blocked` that needs a planning edit is fixed in place and re-reviewed with
  `agk review send` on that reviewer, which remembers the earlier rounds.
- Worktree mode (`agk apply start` printed a `worktree`): from then on the change's files the
  applier reads are the worktree's copy, while the plan reviewer reads the main checkout's. The skill
  closes the main-checkout plan reviewer right after the start, and on the first planning edit starts
  `agk review start --change <change>` from the worktree, reusing it for later edits. Its first round
  reviews the whole plan again, which costs one round but loses nothing.

Every plan round, in either place, takes the checksum manifest of `agk-propose` step 2 in the
directory its reviewer reads, so an approve counts only for the files it saw.

### One code reviewer for the whole run

The first `done` report starts `agk review start --base <base> --for <change>`; every later round,
including the ones after verify fixes, is `agk review send` on it. `<base>` is
`branch.<change>.agkBase` in worktree mode, else the branch checked out when the run started.

### Round limits per reviewer run

The `round` number `agk review wait` returns is the counter: round 6 of a plan or code reviewer is
never sent. The applier has no limit of its own; its rounds are bounded by the reviewer limits
(every applier round after the first answers a review or a blocked edit that went through a plan
round).

### The worktree plan commit is allowed

In worktree mode `agk apply start` pulls the base, commits `openspec/changes/<change>/` (the plan
and `decisions.md` so far) on the base branch and creates the worktree. That is existing agk
behavior the user opted into with `[applier] worktree = true`; the skill does not force
`--no-worktree`. It is the only commit of a run: the skill itself never runs `git commit`, and
the report names the plan commit so the user knows the base branch moved.

Alternative: always `--no-worktree`. Rejected: it would silently override the user's configured
mode and leave `/agk-archive` without the worktree flow (sync, finish) the user chose.

### Who closes what

A finished run closes both reviewers (their rounds stay on disk for `agk review fresh`) and leaves
the applier open, so the user can still send it notes before `/agk-archive` closes it. A stopped run
closes nothing: the user continues from the open companions.

### Evals run against an extended stand-in

The stand-in `agk` today refuses `apply` (only `apply close` is logged). It gains `apply start`,
`apply wait` and `apply send` with per-case outcome files (`apply-round-<k>.json`), and separate
outcome names for plan and code review rounds of the same case, so one case can play a whole run.
`agk review wait` today always prints `completed`; it gains status outcome files
(`plan-wait-<k>.json`, `code-wait-<k>.json`) printed as the wait's answer instead of playing a
review, so `cancelled` and `needs-human` can be played. The `openspec-verify-change` stub reads a
per-case `verify.txt` and the existing `edit-during-verify` marker; it gains a per-call sequence.

## Risks / Trade-offs

- [The model picks the wrong scope] → `[scope]` entries come first in the report, the plan reviewer
  sees them, and no code is archived, committed or merged before the user reads them. In worktree
  mode the plan itself is committed on the base branch by `agk apply start`; dropping a rejected
  plan then takes a revert of that one commit, which the report names.
- [The followed skills still say "ask the user"] → the overrides are listed by name in the skill,
  and the disputed-finding and blocked eval cases fail if the agent asks.
- [A long unattended run burns tokens on a loop] → 5 rounds per reviewer, then a stop with a report.
- [The applier blocks again and again on its own ambiguity rule] → each blocked edit goes through a
  plan round, so the plan reviewer's limit ends the loop.
- [Code review sees `decisions.md` and `tasks.md` edits as part of the diff] → intended: the review
  covers the log, and `agk review fresh` stays honest about it.
