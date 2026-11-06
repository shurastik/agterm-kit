# Tasks

## 1. agterm: wait and close

- [x] 1.1 Add `Wait` and `Close` to `agterm.Ctl` with `ErrGone`/`ErrTimeout` (design D3); implement them on `CLI` and verify with `go test ./internal/agterm/` that tests assert exact argv (`session wait --target T --until completed --until blocked --timeout 1800 --json`, `session close --target T --undoable --json`, `--socket` appended) and the mappings: wait exit 0 → parsed status, exit 2 → `ErrGone`, exit 1 `no such session` → `ErrGone`, exit 124 → `ErrTimeout`, other exit → error with stderr; close `{"ok":false,"error":"no such session: …"}` → `ErrGone`, other failure → error
- [x] 1.2 Extend `agtermtest.Fake` to record `Wait`/`Close` and script their results; verify with the fake's own test and `go vet ./...`

## 2. review start records the companion

- [x] 2.1 Write `<run dir>/companion` after `NewCompanion` and before printing JSON; on write failure close the companion and exit 1 (design D2); verify with `review_start_test.go` cases: file holds the fake's id, inline run leaves no file, a failing write (run dir made read-only) closes the companion and exits 1 with empty stdout

## 3. review wait

- [x] 3.1 Add `agk review wait <run-dir> [--timeout SECONDS]` with argument validation (not a `.agk/reviews/<run>` dir, non-positive or non-integer timeout, malformed `companion` file → exit 1, no `Ctl` calls) and wire it into `reviewCmd` and the usage text; verify with tests in a new `review_wait_test.go` and `agk review --help` output
- [x] 3.2 Implement outcome resolution per design D4 and verify with tests covering every spec scenario: completed with findings (outcome JSON, `Close` called), blocked with and without `result.json` (no `Close`), cancelled (`ErrGone`, no `result.json`), running (`ErrTimeout`, no `result.json`, no `Close`), timeout with `completed` `result.json` (`Close` called), `result.json` present before waiting (no `Wait` call), user already closed the companion (`Close` → `ErrGone`, no stderr warning), close failure (warning, exit 0), inline run with and without `result.json` (no `Ctl` calls / exit 1), unparsable or unknown-status `result.json` (exit 1)

## 4. Skill and docs

- [x] 4.1 Rewrite `skill/agterm-codex-review/SKILL.md` per design D6 (wait step with one branch per `status`, inline runs also via `wait`, triage optional and only on the user's request, default act-on-findings flow); verify by reading the rendered file and checking that every `status` value and every remaining `agk` exit code is covered
- [x] 4.2 Update the `AGENTS.md` usage block (replace the raw `agtermctl session wait` lines with `agk review wait <dir>`); verify the documented commands match `agk --help`

## 5. Integration

- [x] 5.1 Quality gates: `go build ./... && go vet ./... && go test ./...` pass, then `go install ./cmd/agk`
- [x] 5.2 Live check inside agterm on a scratch repo with a small diff: `agk review start` → `agk review wait <dir>` prints `completed` and the `codex-review #N` companion disappears from the sidebar (⌘Z brings it back); a run forced to fail (e.g. `PATH` without codex in the forwarded env) prints `blocked` and its companion stays open

## Workflow follow-up

- Codex review of the diff until clean (via the updated `agterm-codex-review` skill), then `/opsx:verify` with 0 CRITICAL.
- Archive the change, `bd close` the epic, push code and beads.
