# Tasks

## 1. `--no-daemon` for codex workers

- [x] 1.1 In `internal/harness/codex_test.go`, add failing argv tests: `InteractiveReviewer` and `Interactive` carry `--no-daemon` once, after model, effort and `extra_args`; with `extra_args = ["--no-daemon"]` the token appears exactly once. Verify they fail with `go test ./internal/harness/`
- [x] 1.2 Append `--no-daemon` in both builders in `internal/harness/codex.go`, unless `extra_args` already holds that exact token. Update the doc comments with the reason: hooks would otherwise run in a shared daemon with another session's `AGTERM_*`. Verify `go test ./internal/harness/` passes
- [x] 1.3 Update the existing companion argv expectations in `internal/cli` tests that assert a full codex command line, and verify `go test ./internal/cli/` passes
- [x] 1.4 README `## Requirements`: name codex 0.156.0 as the minimum (first release with `--no-daemon`, needed by the reviewer and applier companions) and replace "The README names no minimum versions: none is verified" accordingly. Verify by reading the section

## 2. Settle check in `typeWhenIdle`

- [x] 2.1 Make the agterm fake (`internal/agterm/agtermtest`) able to script successive screens, with a `TextSeq` popped per call like `WaitSeq`, or `OnText` mutating `TextOut`. Cover it in `fake_test.go` and verify `go test ./internal/agterm/...` passes
- [x] 2.2 In `internal/cli/worker_test.go`, add failing tests for `typeWhenIdle` with a settle argument. Use a short `pollSlice` and `WaitSeq` timeouts for each case: status stuck (timeouts) with evidence true and identical screens types the line and prints the fallback warning; screens that differ every read time out as busy with nothing typed; a dialog marker in the last 12 lines (each of the four markers, case-insensitive) times out busy; evidence false never reads the screen and times out busy; a `Text` error times out busy; a `completed` status types at once with no `Text` call and no warning; `blocked` still refuses with `errPrompt`; a nil settle argument keeps today's single `Wait`. Verify they fail
- [x] 2.3 Implement it in `internal/cli/worker.go`. Add a `settler` helper (evidence predicate plus previous screen; one `Ctl.Text` read of `settleLines` per call; settled when the read matches the previous one and has no dialog marker; a reset method) and an optional `*settler` argument to `typeWhenIdle`. With it set, wait through `waitPolling` over the same budget and the same `completed|blocked|idle` set, and call the settler between slices (design decisions 2 and 4). Then print the fixed warning, type the line and run `confirmStart` as before. Keep the existing callers compiling with nil. Verify `go test ./internal/cli/` passes

## 3. Callers

- [x] 3.1 `ReviewSend` (`internal/cli/review.go`): pass a settle argument only when `run.state.Worker.Harness` is codex, with evidence "round k has `result.json`". Add tests to `review_send_test.go` for the review-companion scenarios "Status stuck at active after round 1", "Screen still changing", "Settled screen shows a codex dialog", "Status still decides first", "Screen cannot be read" and "Claude reviewer keeps the status rule"; verify they pass with `go test ./internal/cli/`
- [x] 3.2 `review.Decide` (`internal/review/decide.go`): add a `settled bool` input that counts as a turn end next to `completed`/`idle` (design decision 3a). First add failing cases to its tests: settled with an invalid output gives `Fixup`; settled with no output gives `NeedsHuman` no-output, as a turn end does; settled does not override gone, timeout, `blocked`, an unstarted fix-up or the `MaxFixups` limit; a valid output still completes. Verify `go test ./internal/review/` passes
- [x] 3.3 `waitInteractive` / `typeFixup` (`internal/cli/wait.go`): for codex reviewers, keep a `settler` with evidence "`out/<k>.json` exists". Call it before `Decide` only while the output exists, does not parse and the status is not `completed`/`idle`/`blocked`, and reset it otherwise and after a fix-up is typed. Pass its result to `Decide` and the same settler to `typeWhenIdle` in `typeFixup`; claude reviewers pass `settled = false` and nil. Add `ReviewWait` tests for "Fix-up after an invalid output file" (stuck `active`, unchanged screen: the fix-up is typed with the warning, and a valid `out/1.json` written afterwards completes round 1) and "No fix-up into a running turn" (changing screens, and a dialog marker: nothing typed, `running` at the timeout). Verify `go test ./internal/cli/` passes
- [x] 3.4 `ApplySend` (`internal/cli/apply.go`): pass a settle argument for codex appliers, with evidence "the current round's `report.json` exists". Add tests for the apply-worker scenarios "Status stuck at active after round 1", "No report yet", "Screen still changing" and "Claude applier keeps the status rule"; verify they pass with `go test ./internal/cli/`
- [x] 3.5 README: next to the `send` timeouts paragraph, describe the codex fallback: the status first, then the round on disk plus a screen unchanged for 5 s and no codex dialog, with a warning on stderr. Verify by reading the paragraph against the specs

## 4. Integration and cleanup

- [x] 4.1 Remove the "Codex reviewer stuck `active` after the hooks-trust prompt" entry from `BACKLOG.md` and verify the file has no other references to it
- [x] 4.2 Run `go build ./... && go vet ./... && go test ./...` and verify all pass
- [x] 4.3 Manual check on the spike stand (`<scratchpad>/spike/repo`, or any scratch git repo) with a codex 0.171+ companion started with the reviewer's argv plus `--no-daemon`. Confirm that two `agtermctl session text --lines 40` reads 5 s apart are identical once the turn is idle, and differ while a turn runs (the `Working (Ns …)` timer). Confirm the hook log shows the companion's own `AGTERM_SESSION_ID` even with the shared daemon running. Record the result in the apply report

## Workflow follow-up

- Codex review of the diff until clean, then `/openspec-verify-change` with 0 CRITICAL, then `/agk-archive`.
