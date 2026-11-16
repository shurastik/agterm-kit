# Tasks

## 1. Eval first (RED)

- [x] 1.1 In `evals/cases/plan-done/graders/names-next-step.md` change the pattern to
  `/agk-apply add-widget`; verify with `grep -n 'agk-apply add-widget' evals/cases/plan-done/graders/names-next-step.md`
- [x] 1.2 Add `evals/cases/plan-done/graders/not-opsx-apply.md` and
  `evals/cases/plan-done/graders/not-agk-apply-start.md`: `type: regex`, `target: last_message`,
  `match: not_contains`, patterns `/opsx:apply` and `agk apply start` (shaped like
  `evals/cases/idea-settled/graders/not-opsx-propose.md`); verify both files exist with those
  front-matter keys
- [x] 1.3 Update the `description` in `evals/cases/plan-done/case.yaml` to name `/agk-apply <change>`
  as the next step and the absence of `/opsx:apply` and `agk apply start`; verify
  `grep -n '/agk-apply <change>' evals/cases/plan-done/case.yaml` matches and
  `evals/_fixtures/test.sh` passes
- [x] 1.4 Add the stub `evals/_fixtures/stubs/agk-apply/SKILL.md`: `name: agk-apply`, the
  description of `skills/agk-apply/SKILL.md` verbatim plus "(Eval stub of the agk skill.)", and a
  body that runs `agk apply start <change>` (the stand-in only logs it) and stops; link it as
  `evals/case-plugins/plan-done/skills/agk-apply` like the other stubs; list it among the stub
  skills in the `evals/README.md` layout table. Verify `readlink` on the link resolves to the stub
  and `evals/_fixtures/test.sh` passes
- [x] 1.5 Add `evals/cases/plan-done/graders/no-agk-apply-skill.md`: `type: tool_used`,
  `tool: Skill`, `input_match: agk-apply`, `min: 0`, `max: 0` (shaped like
  `evals/cases/disputed-finding/graders/no-config-write.md`); verify the file exists with those keys
- [x] 1.6 Run `scripts/eval-skills.sh --case plan-done --runs 1` against the unchanged skill and
  confirm the case fails on `names-next-step` (and `not-opsx-apply` / `not-agk-apply-start` when the
  old handoff leaks)

## 2. Skill handoff (GREEN)

- [x] 2.1 In `skills/agk-propose/SKILL.md` step 5, make the last message name `/agk-apply <change>`
  as the only next step; state that it replaces whatever the `openspec-propose` workflow suggested
  (`/opsx:apply`, "ask me to apply", `agk apply start`) and that neither `/opsx:apply` nor
  `agk apply start` is ever named; change "Never start `agk apply`" to cover invoking `/agk-apply`
  too. Verify `grep -n 'opsx:apply\|agk apply start' skills/agk-propose/SKILL.md` shows them only in
  the "never name" sentence
- [x] 2.2 Run `scripts/eval-skills.sh --case plan-done` (3 runs) and confirm the case passes the
  threshold with every grader green
- [x] 2.3 Sensitivity check of `no-agk-apply-skill`: temporarily change step 5 of
  `skills/agk-propose/SKILL.md` to invoke the `agk-apply` skill and then still end with the
  correct `/agk-apply <change>` message, run `scripts/eval-skills.sh --case plan-done --runs 1`,
  confirm `no-agk-apply-skill` fails, then restore the step 5 text from 2.1 and confirm
  `git diff skills/agk-propose/SKILL.md` shows only the 2.1 change

## 3. Docs

- [x] 3.1 In `README.md` workflow step 2, replace "`agk apply start <change>` launches the applier"
  with `/agk-apply <change>` running the applier loop (`agk apply start`, `agk apply wait`,
  `agk apply send`); verify the step reads correctly and the "With the CLI" section still documents
  the bare commands
- [x] 3.2 Run `go build ./... && go test ./... && go vet ./...` and `openspec validate
  propose-handoff-agk-apply --strict`; all pass

## Workflow follow-up

- Codex review of the diff until clean, then `/opsx:verify` with 0 CRITICAL.
- Archive with `/agk-archive`.
