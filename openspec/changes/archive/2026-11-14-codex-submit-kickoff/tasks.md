# Tasks

## 1. Codex argv disables paste-burst detection

- [x] 1.1 Write failing tests in `internal/harness/codex_test.go`:
  - `TestCodexInteractive` expects `-c tui.disable_paste_burst=true` after the tuning (`-m`, effort, `extra_args`) and before the kickoff;
  - `TestCodexInteractiveReviewer` expects it after the tuning, next to the mandatory sandbox flags and before the kickoff, for each of its cases;
  - with `extra_args = ["-c", "tui.disable_paste_burst=false"]`, agk's `=true` comes later in argv, for both drivers.

  Verify `go test ./internal/harness -run Codex` fails
- [x] 1.2 Add the flag in `Interactive` and `InteractiveReviewer` of `internal/harness/codex.go`, with a comment naming the cause (codex treats the typed burst as a paste and turns its Enter into a newline). Verify the 1.1 tests and `go test ./internal/harness` pass, and that the claude driver tests are unchanged
- [x] 1.3 Check that the reviewer `extra_args` guard (`internal/harness/guard.go`) does not refuse `-c tui.disable_paste_burst=...`, adding a guard test case if none covers it. Verify `go test ./internal/harness -run Guard` passes

## 2. Conditional Enter retry in `typeWhenIdle`

- [x] 2.1 Write failing tests in `internal/cli/worker_test.go` with scripted `WaitSeq` and `TextOut`:
  - first `active` wait is `10s`; on `active` there is no `Text` and no second `Type` (update `TestTypeWhenIdleTypesAndConfirms`);
  - first wait times out, the status check returns `completed`, the screen shows the line wrapped across two rows with indentation → exactly one `Type(companion, "\r")`, then `Wait(active, 20s)` → started;
  - same, but the status check returns `blocked` → no `Text`, no second `Type`, not started;
  - the status check times out → no second `Type`, not started;
  - the status is `idle` but the line is not on screen → no second `Type`, `Wait(active, 20s)`, not started when it times out;
  - the retry's `Wait(active)` times out → not started, and the stderr warning says the line may still be in the input box and to press Enter in the companion;
  - a deadline leaving 5 s → first wait `5s`, no status check, no retry; a deadline leaving 15 s → first wait `10s`, then retry wait `5s`;
  - the injected clock advances during the calls (from the fake's `OnWait` / `OnType` hooks): with a deadline leaving 11 s, a first wait that takes 10 s and a status check that takes 2 s → no `Text`-driven Enter and no last wait; with 15 s left and the status check taking 2 s → the last `Wait(active)` gets `3s`, not `5s`;
  - every call after the typed line gets a context whose deadline is the handshake end (assert that the fake sees a context with a deadline on each of these calls, recording `ctx.Deadline()` in the fake if it does not yet), while the initial wait for the turn end and the line's own `Type` keep the caller's context;
  - a handshake call that fails with `context.DeadlineExceeded` (scripted through `WaitSeq` and `TextErr`) → not started, no further `Type`, no error returned, so the caller keeps the round.

  Verify `go test ./internal/cli -run TypeWhenIdle` fails
- [x] 2.2 Implement the two-step handshake in `internal/cli/worker.go` (an absolute handshake end `min(Now()+30s, deadline)` with the remaining time recomputed from `Env.Now` before every step, all handshake calls under one `context.WithTimeout(ctx, end - Now())` with any of their errors meaning "not started", a `sendFirstHandshake = 10 * time.Second` constant, the current-status check through `Ctl.Wait` with all four states and a 1 s timeout, the screen check over the last `handshakeLines` lines with whitespace removed, one `Type("\r")`) and the new warning text. Verify the 2.1 tests pass and `go test ./internal/cli` passes, updating the expected call sequences in existing send/fix-up tests where the handshake changed
- [x] 2.3 Add a fix-up case to `internal/cli/review_wait_test.go`: the fix-up line is not picked up, the screen shows it, one Enter is typed, `active` arrives, and the fix-up is recorded as started (`fixupStartedFile`). Verify `go test ./internal/cli -run Wait` passes

## 3. `started` in the send JSON

- [x] 3.1 Write failing tests: in `internal/cli/review_send_test.go`, a companion send prints `"started":true` when `active` arrives and `"started":false` (exit 0, round 2 kept, no `started` marker) when it does not, and the inline send prints no `started` field; in `internal/cli/apply_test.go`, the same for `agk apply send` (`{"round":2,"started":true}`, `{"round":2,"started":false}`, inline `{"round":2}`). Verify `go test ./internal/cli -run 'Send'` fails
- [x] 3.2 Add `Started *bool` with `json:"started,omitempty"` to `sendResult` (`internal/cli/review.go`) and `sendRound` (`internal/cli/apply.go`), set only on the companion path. Verify the 3.1 tests and `go test ./...` pass
- [x] 3.3 Update `README.md`: the `apply send` / `review send` example outputs show `started`, and one sentence says what `"started": false` means (line typed but not picked up, probably still in the input box; press Enter in the companion, then wait as usual). Verify `grep -n '"started"' README.md` shows the send lines

## 4. Skills and backlog

- [x] 4.1 In `skills/agk-review/SKILL.md` step 4, `skills/agk-apply/SKILL.md` step 3 and `skills/agk-propose/SKILL.md` step 2 item 4, document the send stdout with `started` and a branch for `"started": false`: tell the user the line is waiting in the companion's input box and to press Enter there, then go on with the wait step; an inline send (no `started` field) goes straight to the wait. Verify by reading the three steps, and run `scripts/eval-skills.sh` only if the user agrees to spend the API usage
- [x] 4.2 Update `evals/_fixtures` if its stand-in `agk` or `test.sh` checks the exact send JSON, so the fixture matches the new output. Verify `evals/_fixtures/test.sh` passes (or that `grep -rn '"round"' evals/` shows no exact-match on the send output that needs a change)
- [x] 4.3 Remove the BACKLOG.md entry "`agk review send` to an interactive codex reviewer does not submit the kickoff line". Verify `grep -n "kickoff line" BACKLOG.md` finds nothing

## 5. Integration

- [x] 5.1 Run the quality gates: `go build -o .bin/agk ./cmd/agk`, `go vet ./...`, `go test ./...`. Verify all pass
- [x] 5.2 Live smoke test with the checkout binary in agterm: `.bin/agk review start --base main` with a codex reviewer on a small diff, then at least three `.bin/agk review send` rounds after idle pauses. Verify every send prints `"started":true` with no warning, and in the companion that an extra Enter on an empty composer (typed by hand with `agtermctl session type $'\r'`) starts no turn

## Workflow follow-up

- Codex review of the diff until clean, then `/opsx:verify` with 0 CRITICAL.
- Comment on issue #1 with the fix and close it once released.
- `/agk-archive`.
