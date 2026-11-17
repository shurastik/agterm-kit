# Tasks

## 1. Evals speak OpenSpec 2.0 (RED)

- [x] 1.1 Rewrite the last paragraph of `evals/_fixtures/stubs/openspec-explore/SKILL.md` to the
  2.0 handoff from design.md ("Use the `openspec-propose` skill …; `openspec-apply-change`
  implements it once tasks exist") and step 2 of `evals/_fixtures/stubs/openspec-propose/SKILL.md`
  to "The artifacts are ready for review. When you are ready, use the `openspec-apply-change` skill
  or ask me to apply this change.", then stop. Verify `grep -rn opsx evals/_fixtures` prints nothing
- [x] 1.2 In `evals/cases/idea-settled/graders/` replace `not-opsx-propose.md` with
  `not-openspec-propose.md` and `not-openspec-apply-change.md` (`type: regex`,
  `target: last_message`, `match: not_contains`, one token each); in
  `evals/cases/plan-done/graders/` replace `not-opsx-apply.md` with `not-openspec-apply-change.md`
  and `not-ask-me-to-apply.md` (token `ask me to apply`). Update both `case.yaml` descriptions to
  name the new tokens instead of `/opsx:*`. Verify
  `grep -rn opsx evals/cases/idea-settled evals/cases/plan-done` prints nothing and
  `bash evals/_fixtures/test.sh` prints `ok`
- [x] 1.3 Update the `openspec-explore` stub paragraph of `evals/README.md` (it ends by suggesting
  the `openspec-propose` skill and naming `openspec-apply-change`, which `agk-explore` must replace
  with `/agk-propose`) and add the matching sentence for the `openspec-propose` stub (it suggests the
  `openspec-apply-change` skill or "ask me to apply", which `agk-propose` must replace with
  `/agk-apply <change>`). Verify by reading the section and that
  `grep -rn opsx evals --exclude-dir=results` prints nothing
- [x] 1.4 With the user's approval (spends API usage), run
  `scripts/eval-skills.sh --case idea-settled --runs 1` and `--case plan-done --runs 1` against the
  unchanged skills and record here, from the per-grader results of the report (not the case score),
  which forbidden-token graders (`not-openspec-*`, `not-ask-me-to-apply`) failed: any of them
  failing confirms the leak, even when the case score clears the threshold. If none fails, note it;
  the guard change still goes in, the graders then only guard against regression
  - Result (1 run each, unchanged skills): no forbidden-token grader failed. `idea-settled` 1.00
    ($0.15): `not-openspec-propose` and `not-openspec-apply-change` passed. `plan-done` 1.00
    ($0.23): `not-openspec-apply-change`, `not-ask-me-to-apply` and `not-agk-apply-start` passed.
    No leak reproduced in one run; the graders guard against regression.

## 2. Guards in the skills (GREEN)

- [x] 2.1 In `skills/agk-explore/SKILL.md` rewrite the description's "instead of /opsx:propose" as
  handing off instead of the stock `openspec-propose` handoff, and section "2. Handoff" as the rule
  from design.md: `/agk-propose` is the only next step named, whatever the project's
  `openspec-explore` suggests; no other way to start the proposal or the implementation is named
  (slash command, skill name, "ask me to" phrase), for example the `openspec-propose` or
  `openspec-apply-change` skill, in the last message or the closing summary. Verify
  `grep -n opsx skills/agk-explore/SKILL.md` prints nothing
- [x] 2.2 In `skills/agk-propose/SKILL.md` section "5. Close and stop" replace the `/opsx:apply`
  list with the same rule: `/agk-apply <change>` is the only next step; never the
  `openspec-apply-change` skill, "ask me to apply" or `agk apply start`, however the
  `openspec-propose` workflow ended. Verify `grep -rn opsx skills` prints nothing
- [x] 2.3 In `README.md` change the `/agk-explore` line ("instead of `/openspec-propose`") to name
  the `openspec-propose` skill's handoff. Verify by reading the line
- [x] 2.4 Remove the backlog entry "Explore/propose guards target `/opsx:*`, stock OpenSpec 2.0
  names skills instead" from `BACKLOG.md`. Verify `grep -n opsx BACKLOG.md` prints nothing
- [x] 2.5 With the user's approval (spends API usage), run
  `AGK_EVAL_THRESHOLD=1 scripts/eval-skills.sh --case idea-settled` and the same for
  `--case plan-done` (3 runs each). Acceptance is per grader, not the averaged score: every
  forbidden-token grader (`not-openspec-propose`, `not-openspec-apply-change`,
  `not-ask-me-to-apply`, `not-agk-apply-start`) passes in every run of both cases, read from the
  report's per-grader results; a single failure blocks the task. A run that misses the threshold
  only on a non-token grader (an `llm` grader) is rerun once and noted. Then run
  `--case plain-explore-request` and `--case plain-propose-request` once each and verify they still
  pass. Record per-grader outcomes, scores and cost here
  - Result (changed skills, `AGK_EVAL_THRESHOLD=1`): `idea-settled` 3 runs, 1.00 each, $0.44;
    `not-openspec-propose` and `not-openspec-apply-change` passed in every run (so did the other
    three graders). `plan-done` 3 runs, 1.00 each, $0.71; `not-openspec-apply-change`,
    `not-ask-me-to-apply` and `not-agk-apply-start` passed in every run (so did the other four).
    `plain-explore-request` 1.00 ($0.12) and `plain-propose-request` 1.00 ($0.11), all graders
    passed. No reruns. Total of this change's paid evals, 1.4 included: $1.76.

## Workflow follow-up

- Codex review of the diff until clean, then `/openspec-verify-change` with 0 CRITICAL.
- Archive with `/agk-archive`; the archive syncs the `explore-workflow` and `propose-workflow`
  specs, after which `grep -rn opsx openspec/specs` prints nothing.
