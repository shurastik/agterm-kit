# Tasks

## 1. Companion name helper

- [x] 1.1 Write a failing table test for `companionName(change, harness, kind, num)` in `internal/cli` (for example `internal/cli/worker_test.go`). Cases: `("", "codex", "review", 3)` → `codex-review #3`; `("add-auth", "claude", "apply", 1)` → `add-auth · claude-apply #1`; `("add-auth", "codex", "plan", 8)` → `add-auth · codex-plan #8`. Verify `go test ./internal/cli -run CompanionName` fails
- [x] 1.2 Implement `companionName` in `internal/cli/worker.go`. The separator is ` · ` (U+00B7 between spaces). Verify the 1.1 test passes

## 2. Applier companion name

- [x] 2.1 Update the expected `NewCompanion` names in `internal/cli/apply_test.go` (`claude-apply #1`, `codex-apply #1`, `claude-apply #3`) to `<change> · <harness>-apply #N` with each test's change, and add an assertion that no `agtermctl` call renames a session (the fake records no rename-like method). Verify `go test ./internal/cli -run Apply` fails
- [x] 2.2 Build the applier companion name in `ApplyStart` (`internal/cli/apply.go`) with `companionName(change, w.Harness, "apply", num)`. Verify the 2.1 tests and `go test ./internal/cli` pass
- [x] 2.3 Update `skills/agk-apply/SKILL.md`: the companion is `<change> · <harness>-apply #N`. Verify with `grep -n 'apply #' skills/agk-apply/SKILL.md` that no old form is left

## 3. Plan review companion name

- [x] 3.1 Add a failing test in `internal/cli/review_start_test.go`: `agk review start --change add-x` with a claude reviewer calls `NewCompanion` with `add-x · claude-plan #1`. Verify `go test ./internal/cli -run ReviewStart` fails
- [x] 3.2 In `ReviewStart` (`internal/cli/review.go`), name a plan review's companion `companionName(state.Change, w.Harness, "plan", num)` and leave a diff review without `--for` as `<harness>-review #N`. Verify the 3.1 test passes and the existing diff-review name assertions (`codex-review #1`, `claude-review #1`) are unchanged

## 4. `--for` on a diff review

- [x] 4.1 Write failing tests in `internal/cli/review_start_test.go`:
  - `--base main --for add-auth` → `NewCompanion` name `add-auth · codex-review #1`, and `worker.json` holds `"for":"add-auth"`, base `main` and no `change`;
  - `--for add-auth` alone → base `main`, `for` `add-auth`;
  - `--for ../secrets` and `--for ""` → exit 1 naming the bad value, no run directory, no `Ctl` call;
  - `--change add-auth --for add-auth` → exit 1 saying the two exclude each other, no run directory, no `Ctl` call;
  - the same tree reviewed with and without `--for` gives byte-identical `rounds/1/scope.sha256`. The two `rounds/1/prompt.md` are identical once each run's own `schema.json` and `out/1.json` paths are replaced by a placeholder, and each original prompt names its own run's absolute paths. For two inline runs, which carry no output paths, the prompts are compared byte for byte;
  - inline (`AGTERM_ENABLED` unset) `--for add-auth` → `worker.json` records `for`.

  Verify `go test ./internal/cli -run ReviewStart` fails
- [x] 4.2 Add `For string \`json:"for,omitempty"\`` to `reviewState`, parse `--for` in `ReviewStart` and validate it with the `changeName` regexp. Refuse it with `--change` using the same `fs.Visit` rule as `--change`/`--base`, before any run directory exists. Name the companion `companionName(state.For, w.Harness, "review", num)`. Update the `--help`/usage text of `review start`. Verify the 4.1 tests and `go test ./internal/cli` pass
- [x] 4.3 Add a test that `agk review send`, `wait`, `fresh` and `close` on a run dir whose `worker.json` has `for` treat it as a diff review, and on an older run without the field behave as before. Verify `go test ./internal/cli` passes
- [x] 4.4 Update `skills/agk-review/SKILL.md`: the start command is `agk review start --base <base> [--for <change>]`, with `--for` passed when the diff implements a known OpenSpec change. Companion names in the text are `<change> · <harness>-review #N` and `<harness>-review #N`. Verify by reading the skill's Loop step 1 and the plan-review section

## 5. Archive fallback and evals

- [x] 5.1 Make the stand-in `evals/_fixtures/bin/agk` parse `--for` (and `--for=`) on `review start`, reject a non-kebab-case value and record it in the fake `worker.json`. Run `evals/_fixtures/test.sh` and verify it passes
- [x] 5.2 Change the `fresh-session-archive` grader (`evals/cases/fresh-session-archive/graders/ordered-steps.md`) to require `--for add-widget` on the `agk review start` line. Check it in either flag order. Verify the regex with a quick local match against a sample `agk-calls.log` line in both orders
- [x] 5.3 Update `skills/agk-archive/SKILL.md`: every diff review it starts passes `--for <change>`, both the fallback start and the second gate run after a rebasing `agk sync`. Verify with `grep -n 'review start' skills/agk-archive/SKILL.md` that each diff-review command carries `--for <change>`
- [x] 5.4 Run the archive eval cases (`scripts/eval-skills.sh`, limited to `agk-archive` cases if the script allows it) and verify `fresh-session-archive` passes and the other archive cases score as before. This spends API usage, so say so before running it. Only three cases were run, with the user's approval, one run each: `fresh-session-archive` (1.00, $0.31), `worktree-archive` (1.00, $0.30), `worktree-base-moved` (1.00, $0.37)

## 6. Docs

- [x] 6.1 Update `README.md`: `agk review start --for <change>` in the command reference, and the companion names (`<change> · <harness>-apply #N`, `<change> · <harness>-plan #N`, `<change> · <harness>-review #N` with `--for`, `<harness>-review #N` without). Verify with `grep -n 'apply #\|review #\|plan #\|--for' README.md`
- [x] 6.2 Update the command block in `AGENTS.md` (`agk review start [--base main [--for <change>] | --change <change>]`). Verify `go build ./... && go vet ./... && go test ./...` pass

## Workflow follow-up

- Codex review of the diff, then `/opsx:verify` with 0 CRITICAL, then `/agk-archive`.
- Live smoke test in agterm with the checkout binary: `.bin/agk review start --change change-in-companion-names` and `.bin/agk review start --for change-in-companion-names` show the new names in the sidebar.
