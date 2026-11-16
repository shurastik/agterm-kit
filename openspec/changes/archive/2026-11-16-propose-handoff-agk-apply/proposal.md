# Proposal

## Why

`/agk-propose` ends by naming `/opsx:apply` or `agk apply start <change>` as the next step. Neither
is the step the agk workflow wants: `agk apply start` alone starts the applier but leaves the
wait/send/close loop to whoever reads the message, and `/opsx:apply` implements inline in the main
session, bypassing the applier (and newer OpenSpec releases no longer ship it). The `agk-apply`
skill runs the whole applier loop, so the handoff should name `/agk-apply <change>` and nothing
else.

## What Changes

- `skills/agk-propose/SKILL.md`, step 5: the last message names `/agk-apply <change>` as the only
  next step. It never names `/opsx:apply` or `agk apply start`, also when the project's
  `openspec-propose` skill suggests `/opsx:apply` or another way to implement (the same override
  `agk-explore` applies to `/opsx:propose`). The skill still never starts the implementation: it
  neither runs `agk apply` nor invokes `/agk-apply` itself.
- Eval case `plan-done`: the last message must contain `/agk-apply add-widget` and must contain
  neither `/opsx:apply` nor `agk apply start`. The `openspec-propose` stub already ends with
  `/opsx:apply`, so the case exercises the override. The case also loads a new `agk-apply` stub
  skill, discoverable like the real one, and fails if the agent invokes it.
- `README.md`, workflow step 2: names `/agk-apply <change>`, which runs the applier loop
  (`agk apply start`, `wait`, `send`), instead of the bare `agk apply start <change>`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `propose-workflow`: "Stop before implementation" names `/agk-apply <change>` as the next step and
  forbids naming `/opsx:apply` or `agk apply start`, and forbids invoking `/agk-apply`.

## Impact

- `skills/agk-propose/SKILL.md` (step 5).
- `evals/cases/plan-done/` (`case.yaml` description, graders), `evals/case-plugins/plan-done/skills/`
  (link to the stub), `evals/_fixtures/stubs/agk-apply/` (new stub), `evals/README.md` (layout table).
- `README.md` (workflow step 2).
- No Go code, CLI behavior or config changes. `agk-explore` already hands off only to `/agk-propose`
  and is unchanged.
