# Proposal

## Why

OpenSpec 2.0 removed the `/opsx:*` slash commands. The stock `openspec-explore` now hands off with
"Use the `openspec-propose` skill" (and names `openspec-apply-change` after a capture), and the
stock `openspec-propose` ends with "use the `openspec-apply-change` skill or ask me to apply this
change". The handoff guards of `agk-explore` and `agk-propose` list only the 1.x names
(`/opsx:propose`, `/opsx:apply`), so the 2.0 handoff text passes straight through to the user, and
the eval stubs still speak 1.x, so the evals stay green while the guard leaks.

## What Changes

- `agk-explore`: the handoff guard becomes a general rule. The only next step it names is
  `/agk-propose`; it never names any other way to start the proposal or the implementation, as a
  slash command, a skill name or an "ask me to" phrase. The examples are the 2.0 names
  (`openspec-propose`, `openspec-apply-change`). This also closes the capture path, where the stock
  workflow names `openspec-apply-change` and the guard said nothing about it.
- `agk-propose`: same rule for its closing message. The only next step is `/agk-apply <change>`;
  it never names `openspec-apply-change`, "ask me to apply" or `agk apply start`.
- **BREAKING** (for projects still on OpenSpec 1.x): every `/opsx:*` mention leaves the skills, the
  `explore-workflow` and `propose-workflow` specs, the eval graders, the stubs and
  `evals/README.md`. 1.x wording is no longer named or covered by an eval case; the general rule
  still applies to it.
- Eval stubs `openspec-explore` and `openspec-propose` end with the 2.0 handoff wording, close to
  the stock skills. The graders `not-opsx-propose` and `not-opsx-apply` are replaced by graders that
  reject the 2.0 names in the last message (`idea-settled`: `openspec-propose`,
  `openspec-apply-change`; `plan-done`: `openspec-apply-change`, "ask me to apply"). Case
  descriptions and `evals/README.md` follow.
- README: the `/agk-explore` line says it hands off instead of the `openspec-propose` skill (it
  names a `/openspec-propose` slash command that does not exist).
- The backlog entry "Explore/propose guards target `/opsx:*`, stock OpenSpec 2.0 names skills
  instead" is removed.

## Capabilities

### New Capabilities

### Modified Capabilities
- `explore-workflow`: the handoff requirement names `/agk-propose` as the only next step and
  forbids the stock 2.0 handoff names (`openspec-propose`, `openspec-apply-change`) instead of
  `/opsx:propose`.
- `propose-workflow`: the stop-before-implementation requirement forbids `openspec-apply-change`
  and "ask me to apply" instead of `/opsx:apply`; its scenario uses the 2.0 stock wording.

## Impact

- `skills/agk-explore/SKILL.md`, `skills/agk-propose/SKILL.md` (agent-facing text only, no Go code).
- `openspec/specs/explore-workflow/spec.md`, `openspec/specs/propose-workflow/spec.md`.
- `evals/_fixtures/stubs/openspec-explore/SKILL.md`, `evals/_fixtures/stubs/openspec-propose/SKILL.md`,
  `evals/cases/idea-settled/`, `evals/cases/plan-done/`, `evals/README.md`.
- `README.md` (one line), `BACKLOG.md`.
- Shipped skills change with the next release; `agk setup` links them as before.
