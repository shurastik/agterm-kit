# Tasks

## 1. Plan scope

- [x] 1.1 Add `review.PlanScope` and `review.CollectPlan(ctx, dir, change)`: git root, sorted regular files of `openspec/changes/<change>/`, main spec per `specs/<cap>/spec.md` delta, every file read through `readUntracked`; `ErrNothingToReview` for a change with no files, an error naming a missing change dir. Verify with table tests in `internal/review/scope_test.go` on temp repos: untracked change on a dirty tree (unrelated edit absent), committed change on a clean tree, modified capability (main spec present), new capability (no main spec), symlink not followed, empty and missing change dir.

## 2. Plan prompt

- [x] 2.1 Add `assets/plan-review-prompt.md` (role, plan checklist from the spec, finding bar, untrusted-input block, output contract; `<change_file>` and `<main_spec>` blocks) and embed it in `assets/assets.go`. Verify `go build ./...` and an assets test that the template parses.
- [x] 2.2 Add `review.PlanPrompt(PlanScope)` with the `MaxPromptDiffBytes` budget (files past it listed as "read it directly") and split `RereviewPrompt` into a wrapper over a rendered body used by both modes. Verify with `internal/review/prompt_test.go`: prompt names the change, every file is inside a data block, budget overflow marks files, invalid UTF-8 is escaped, existing diff re-review output is unchanged.

## 3. CLI `--change` mode

- [x] 3.1 Add `Env.CollectPlan` (default `review.CollectPlan`) and a fake in tests; add `change` (omitempty) to `reviewState`. Verify `go vet ./...` and that an existing run dir without `change` still loads in `review_send_test.go`.
- [x] 3.2 `agk review start --change NAME`: validate with `changeName` before reading files, reject an explicit `--base` together with `--change` (`flag.Visit`), collect the plan scope, render `PlanPrompt`, record `change` in `worker.json`, then the existing companion/inline path. Verify in `review_start_test.go`: invalid name, missing change and both flags exit 1 with no run dir and no `NewCompanion` call; empty change prints `nothing to review`; a valid change writes round 1 `prompt.md` from the plan prompt and `worker.json` with `change` and no `base`; inline mode prints `companion: null`.
- [x] 3.3 `agk review send` on a plan run: rebuild the scope with `CollectPlan` from the recorded change, re-review wrapper when a session is recorded, first-round plan prompt otherwise. Verify in `review_send_test.go`: edited artifact appears in round 2 with the note, missing change dir exits 1 and records no round, diff runs still use `Collect`.
- [x] 3.4 Update `agk review start` usage/help text and the `AGENTS.md` usage block with `--change`. Verify `go test ./internal/cli/...` (help output test, if any) and that the documented command line matches the flag set.

## 4. Setup dependency

- [x] 4.1 Report `revdiff` on `PATH` as an optional dependency in `internal/setup/deps.go`. Verify in `internal/setup/deps_test.go`: revdiff found, revdiff missing is optional and keeps exit 0.

## 5. Skills

- [x] 5.1 Write `skills/agk-propose/SKILL.md` (name `agk-propose`, explicit triggers only): check the project has the `openspec-propose` skill (stop with `openspec init`/`openspec update` advice otherwise); follow it to write the artifacts and continue instead of stopping; `agk review start --change` loop (wait in background, fix findings in the main session, `send --note`, disagreements raised to the user, companion kept open); revdiff file-review step with `--only` per artifact and `--description-file=<run dir>/revdiff-description.md`, `shasum` before and after, skip with a warning when revdiff is unavailable; one more review round when artifacts changed (findings return to the revdiff step); `agk review close`; final summary and stop without starting `agk apply`. Verify the existing release-layout/setup test still finds every `agk-*` skill with a matching `name`, and `.bin/agk setup --skills-dir ./skills --dry-run` lists the `agk-propose` link.
- [x] 5.2 Document the `--change` mode in `skills/agk-review/SKILL.md` (start command, `nothing to review` for an empty change, same wait/send/close loop) and point the "Overview"/"Usage" sections of `AGENTS.md` at `agk-propose`. Verify by reading the rendered commands against `agk review start -h`.

## 6. Integration

- [x] 6.1 Run `go test ./...`, `go vet ./...` and `go build -o .bin/agk ./cmd/agk`, then a manual plan review on this change from the dev build (`.bin/agk review start --change agk-propose`, inline outside agterm is fine) and confirm `review.json` validates and the prompt holds the change's files and the main `review-companion` and `installation` specs.

## 7. Run target in the review-companion requirements

- [x] 7.1 Cover the narrowed `review-companion` requirements with tests where missing: a diff review records base and no change and a plan review records change and no base (`review_start_test.go`); a legacy `worker.json` with base and no `change` is sent and waited on as a diff review; `agk review send` on a plan run with committed files and a clean tree starts round 2 with the change's files (`review_send_test.go`). Verify `go test ./internal/cli/...`.

## 8. Skill evals

- [x] 8.1 Spike the eval plumbing in a scratch copy (plugin resolution, PATH for stand-ins, what each grader sees); findings are folded into the "Skill evals" decision of `design.md`. Record them in `evals/README.md` as part of 8.4.
- [x] 8.2 Build the suite plugin and fixtures: `evals/.claude-plugin/plugin.json` (`experimental.evals: "cases"`), `evals/skills -> ../skills`; `evals/_fixtures/bin/agk` and `evals/_fixtures/bin/revdiff` stand-ins (argv to `agk-calls.log`, outcome script chosen by `EVAL_AGK_CASE` for `start`/`wait`/`send`/`close`, `apply` only logged; revdiff prints the case's annotations); stub skills `evals/_fixtures/stubs/openspec-propose` and `evals/_fixtures/stubs/revdiff`; a scaffold helper that fails unless `agk` and `revdiff` resolve to the stand-ins. Make `skills/agk-propose/SKILL.md` accept namespaced skill names (`<plugin>:openspec-propose`, `<plugin>:revdiff`) in its availability checks if it does not. Verify with `evals/_fixtures/test.sh` driving the stand-in `agk` through start, wait (findings), send, wait (approve), close and checking the log and the written `review.json`, and the PATH guard failing with the real `agk` first on `PATH`.
- [x] 8.3 Write one case per `propose-workflow` scenario in `evals/cases/<case>/` (plain propose request, project without OpenSpec skills, review follows the artifacts, findings in the first round, disputed finding, revdiff available, revdiff missing, user annotations changed the plan, nothing changed after the pass, plan done), each with its own plugin of symlinks in `evals/case-plugins/<case>/` (real `agk-propose` plus only the stubs it needs) named in `case.yaml` `plugins:`; deterministic graders first, `llm` graders over `agk-calls.log` for ordering and messages. Verify `scripts/eval-skills.sh` runs the suite with every case at or above the threshold on the current skill.
- [x] 8.4 Add `scripts/eval-skills.sh` (prepend `evals/_fixtures/bin` to `PATH`; `claude plugin eval evals --scaffold --trust-plugin --no-publish --ablation none`, the threshold from a baseline of 8.3, `--max-cost-usd`) and document it in `AGENTS.md` (Commands) and `evals/README.md` (layout, stand-ins, spike findings, cost). Verify by temporarily editing `skills/agk-propose/SKILL.md` to close the reviewer right after the first approve: the "user annotations changed the plan" case scores below the threshold; restore the file.

## Workflow follow-up

- Point step 2 of `~/.claude/CLAUDE.md` (outside this repo) at `/agk-propose`; the main session does this with the user, not the applier.
- Codex review of the diff, `/opsx:verify` with 0 CRITICAL, archive, `bd close`.
