# Backlog

Known bugs and follow-ups, newest first. Each entry becomes an OpenSpec change (`/agk-explore`)
when it is picked up.

## Explore/propose guards target `/opsx:*`, stock OpenSpec 2.0 names skills instead

**Type:** task · **Priority:** P2

OpenSpec 2.0 (2026-11-16) removed the `/opsx:*` slash commands. The stock `openspec-explore` now ends with "Use the `openspec-propose` skill" and `openspec-propose` with "use the `openspec-apply-change` skill or ask me to apply this change". The guards in `skills/agk-explore` (description and lines ~34–35), `skills/agk-propose` (~131), `openspec/specs/explore-workflow` and `openspec/specs/propose-workflow`, and the evals (`evals/cases/idea-settled`, `evals/cases/plan-done`, `evals/_fixtures/stubs/openspec-*`, `evals/README.md`) still forbid only `/opsx:propose` / `/opsx:apply`, so the new stock handoff text passes through. Update the specs, skills and eval graders/stubs to the 2.0 wording (behavioural spec change, goes through `/agk-explore`).

## Review companion name: plan/code kind and round number (was agk-7t2)

**Type:** feature · **Priority:** P2

The reviewer companion is named <harness>-review #N where N is the repo-wide run number (#15 today), which reads like a pass number and does not say what is reviewed. Wanted: the name shows the kind (plan review via --change vs code review) and the number counted inside one agk review run, i.e. the current round, updated on every agk review send. Modifies the review-companion 'Review number' requirement and the companion name. Separate OpenSpec change after agk-propose.
