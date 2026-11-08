# Proposal

## Why

The workflow now starts its plan and implementation steps with `agk-*` commands (`/agk-propose`,
`agk-apply`, `agk-review`), but its first and last steps are still bare OpenSpec commands, and both
have gaps in the agk loop. `/opsx:explore` hands off to `/opsx:propose`, which skips the plan
review. `/opsx:archive` archives without checking that the last code review approved the current
code and that verify ran after it. Those two checks live only in prose: the user's global
instructions and `operations.archive.guidance`. Archiving also leaves the change's applier and
reviewer companions open. Two thin `agk-*` skills make the whole loop one family of commands and
close these gaps.

## What Changes

- New agent skill `agk-explore`, shipped in `skills/` and linked by `agk setup`. Run only when the
  user invokes it (`/agk-explore`). It follows the project's `openspec-explore` skill unchanged,
  except at the handoff: when the exploration is ready to become a change, it names `/agk-propose`
  instead of `/opsx:propose`. It stops with setup advice when the project has no
  `openspec-explore` skill.
- New agent skill `agk-archive`, shipped and linked the same way. Run only when the user invokes it
  (`/agk-archive [<change>]`). In the main session it:
  1. checks that the last code review of the change approved the code as it is now. Which review
     round belongs to the change, and its verdict, come from this session's own `agk review`
     results. Whether the code is unchanged since then comes from `agk review fresh`. When the
     session cannot confirm the gate, the skill runs the review itself
     (`agk review start --base <base>`). It stops on findings, and on `nothing to review` it asks
     the user;
  2. checks that verify ran after the last code change and review with 0 CRITICAL. Otherwise it
     runs the project's `openspec-verify-change` skill and stops on any CRITICAL;
  3. archives through the project's `openspec-archive-change` skill;
  4. closes the change's applier and reviewer companions (`agk apply close`, `agk review close`):
     the runs on disk that record the change, plus the diff review runs of this session;
  5. reports the result. It never commits, pushes or fixes code.
- Each diff review round records a fingerprint of its scope: the merge base, plus every in-scope
  path with the hash of its working-tree content. Committing does not change it. The new
  `agk review fresh <run-dir>` prints `{"fresh":true|false,"round":K}`: whether the current scope
  still matches the current round's fingerprint.
- Eval cases for both skills in the existing `evals/` suite (`claude plugin eval`), with stand-in
  `agk` support for diff reviews and `apply close`, and stub `openspec-explore`,
  `openspec-verify-change` and `openspec-archive-change` skills.
- The shipped-skills test lists `agk-explore` and `agk-archive`. `AGENTS.md` and the user's global
  workflow (`~/.claude/CLAUDE.md`, outside this repo) name `/agk-explore` for the brainstorm step
  and `/agk-archive` for the close step.

## Capabilities

### New Capabilities

- `explore-workflow`: the `agk-explore` skill. It is invoked explicitly, checks its precondition,
  delegates to `openspec-explore`, and hands off to `/agk-propose`. It also owns the eval cases
  that check this behavior.
- `archive-workflow`: the `agk-archive` skill. It is invoked explicitly and checks its
  preconditions. Its review gate comes from session context plus `agk review fresh`, and its verify
  gate from session context. Each gate has a fallback run when it cannot be confirmed. It archives through `openspec-archive-change`, closes
  the change's companions, and makes no commits. It also owns the eval cases that check this
  behavior.

### Modified Capabilities

- `review-companion`: adds two behaviors, the per-round scope fingerprint of a diff review and
  `agk review fresh`. Existing requirements are unchanged.

`installation` is not modified: `agk setup` already links every `agk-*` skill directory, and its
release-layout and skill-linking requirements already cover new skills.

## Impact

- Skills: new `skills/agk-explore/SKILL.md` and `skills/agk-archive/SKILL.md`. `skills/agk-review/SKILL.md`
  documents `agk review fresh`.
- Code: `internal/review` (the scope fingerprint), `internal/cli/review.go` (writing
  `rounds/<k>/scope.sha256` for diff rounds, the `fresh` subcommand) and `cmd/agk` usage, with unit
  tests.
- Tests: `internal/setup/repo_skills_test.go` expects the two new skills.
- Evals: new cases, case plugins, stub skills and outcomes under `evals/`. The stand-in
  `evals/_fixtures/bin/agk` gains `review start --base` and `apply close`. `evals/README.md` lists
  the new cases and their baseline. The new cases run real Claude children and spend API usage, as
  the existing ones do.
- Docs: `AGENTS.md` (skills, workflow). The `readme` change (in progress, no `README.md` yet) lists
  the workflow skills: whichever of the two changes lands second adds the other's skills to it.
- Outside the repo: `~/.claude/CLAUDE.md` workflow steps 1 and 6.
- The run state gains a file per diff round. It is additive: rounds written by older agk have no
  fingerprint, and `fresh` reports them as an error, which the skill treats as "cannot confirm".
  A diff review run still does not record its change, so the session links runs to changes.
