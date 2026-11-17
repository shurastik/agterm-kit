# Design

## Context

The guards live only in agent-facing text: `skills/agk-explore/SKILL.md` (description and section
"2. Handoff") and `skills/agk-propose/SKILL.md` (section "5. Close and stop"). The evals that
check them load a stub of the stock skill through each case plugin
(`evals/case-plugins/<case>/skills/openspec-*` links to `evals/_fixtures/stubs/openspec-*`) and
grade the last message with `regex` graders (`match: not_contains`). No Go code is involved.

## Goals / Non-Goals

**Goals:**
- A guard that does not depend on the stock workflow's current wording, with the 2.0 names as
  examples.
- Evals whose stubs say what the stock 2.0 skills say, so a leak shows up as a failing case.

**Non-Goals:**
- Any OpenSpec 1.x (`/opsx:*`) handling: no example, no grader, no eval case.
- Copying the stock skills into the stubs; a stub keeps only the lines that matter to the case.
- Changing `agk-archive` or `agk-apply`: they invoke the stock skills by name and do not pass on
  their handoff text.

## Decisions

- **Rule, then examples.** The skill text states what the one next step is and that no other way
  to start the proposal or the implementation is named (slash command, skill name, "ask me to"
  phrase); the 2.0 names follow as examples. A list alone is what broke when the names changed.
  Alternative: list the 2.0 names only. Rejected: the next rename leaks again.
- **Graders reject the 2.0 tokens literally.** `idea-settled` rejects `openspec-propose` and
  `openspec-apply-change`; `plan-done` rejects `openspec-apply-change` and `ask me to apply`
  (next to its existing `agk apply start` grader). One `regex` grader file per token, like the
  existing ones. `openspec-propose` is not a substring of `/agk-propose`, so the positive grader
  and the negative one do not collide. Alternative: an `llm` grader judging "names no other next
  step". Rejected: slower, costs more and is less deterministic for a fixed string.
- **Acceptance per grader, not per averaged score.** A case score is the mean of its graders, and
  the suite threshold is 0.8: `plan-done` with seven graders scores 6/7 ≈ 0.86 when the last
  message names `/agk-apply add-widget` and also `openspec-apply-change`, so a leak would pass.
  The forbidden-token graders are therefore read one by one from the report: GREEN needs each of
  them to pass in every run, and the target cases run with `AGK_EVAL_THRESHOLD=1`. Alternative:
  raise the suite threshold. Rejected: it would also tighten every unrelated case, where `llm`
  graders are allowed an occasional miss.
- **No new eval case for the capture path.** The `openspec-explore` stub ends like the stock skill,
  suggesting the `openspec-propose` skill and naming `openspec-apply-change` as the step after it,
  so `idea-settled` covers both names. Alternative: a separate "change captured" case. Rejected:
  one more paid eval run for the same guard.
- **Stub wording.** The `openspec-explore` stub ends with "Ready to start? Use the
  `openspec-propose` skill and this becomes a change; `openspec-apply-change` implements it once
  tasks exist." The `openspec-propose` stub ends with "The artifacts are ready for review. When you
  are ready, use the `openspec-apply-change` skill or ask me to apply this change." Both are taken
  from the stock 2.0 skills.

## Risks / Trade-offs

- [The agent mentions `openspec-propose` legitimately, e.g. "`/agk-propose` runs the
  `openspec-propose` workflow"] → the explore guard forbids naming it in the last message at all;
  the grader is strict on purpose.
- [A project still on OpenSpec 1.x gets `/opsx:propose` from its stock skill] → the general rule
  still covers it; it is just no longer named or tested. `openspec update` moves such a project to
  2.0.
- [Stock wording drifts again] → the stubs are a snapshot; the rule does not depend on them.
