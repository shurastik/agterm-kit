# Tasks

## 1. Turn-state file and hook command

- [x] 1.1 In `internal/apply`, add the turn-state type (`round`, `running` / `stopped` with `background` and `wakeups` counts), its parse from `turn.json` and its build from a `Stop` hook input (`background_tasks`, `session_crons`, missing fields as empty); verify with table tests covering the spike's paused and final inputs, a missing field and invalid JSON
- [x] 1.2 Add the hidden `agk apply hook <turn|stop> <run-dir>` (`internal/cli/apply_hook.go`, dispatch in `main.go`): `turn` writes `<run>/turn.json` running with round k when the input `prompt` is exactly `kickoff(<run>/rounds/<k>/prompt.md)`, else with the existing file's round, else nothing; `stop` keeps the existing file's round and writes nothing without one; neither reads `<run>/round`; stdin capped at 1 MiB, `fsx.ReplaceFile`, always exit 0 with empty stdout and stderr; verify with tests for a kickoff of round 1 and of round 2, a `<task-notification>` prompt with `<run>/round` moved to 2 (keeps round 1), a user-typed prompt, `turn` with no file and no kickoff, `stop` with tasks, `stop` with none, `stop` after `<run>/round` moved to k+1 (records round k), `stop` with no file, a non-apply dir and garbage stdin (nothing written, exit 0, no output)

## 2. Applier settings with hooks

- [x] 2.1 Build the claude applier settings from the deny-rule asset plus `UserPromptSubmit` → `apply hook turn` and `Stop` → `apply hook stop` entries with the quoted agk path and run dir; verify with a test asserting the exact JSON (deny rules intact, both hook commands quoted)
- [x] 2.2 Write those settings in `applierFiles` for claude runs and record `turn_hooks: true` in `worker.json`; refuse a run dir with a backslash or line break before any companion is created; verify with `agk apply start` tests (claude: settings and `turn_hooks`; codex: neither; bad path: exit 1, no companion created in the fake `Ctl`)

## 3. Wait decides with turn state

- [x] 3.1 Extend `apply.Decide` with the turn state (only of the current round) and `Outcome` with `background`; verify with table tests for every spec scenario of "Wait for the applier" (pause, pause on timeout → `running`/`background`, stopped with 0 → `no-report`, running while status `completed`, earlier round's state counts as missing, no turn state → status only)
- [x] 3.2 Implement the hooked wait loop in `ApplyWait` (D6): start phase also ends on a turn state of the current round and marks the round started; wait for `blocked` only while running or paused; end slices when the report appears or `turn.json` changes; 30-second fallback when the turn state is missing after the turn shows over; a status check before `no-report`; keep the old path when `turn_hooks` is unset; verify with fake-`Ctl` tests for paused-then-done, paused past the timeout, resumed turn ending with a question, the hook-lags-status race, never-recorded turn state, a late round-1 `stop` after round 2 was published, round 1 resumed while a busy send of round 2 published and rolled back the round (stays paused, `running`/`background` on timeout), round 2 started unseen (paused → `running`/`background`; stopped with 0 → `no-report` at once), companion closed after stopped with 0 (→ `cancelled`) and a run without hooks
- [x] 3.3 Update `skills/agk-apply/SKILL.md` (`running` with `reason` `background`: wait again; on a second such result show the user and point at the companion) and README (the new reason and the `background` count next to `apply wait`); verify the wording against the spec and run `go test ./...`

## 4. Integration

- [x] 4.1 Run `go build -o .bin/agk ./cmd/agk`, `go vet ./...` and `go test ./...` and verify all pass
- [x] 4.2 E2E on a live companion: start a claude applier on a throwaway change whose task runs a background `sleep 60` shell and ends its turn; verify `<run>/turn.json` goes round 1 `stopped` (1 background task) → `running` → `stopped` (0), and that `agk apply wait` returns only after the report, not `no-report`; then `agk apply send` a short note and verify `turn.json` records round 2 (the typed kickoff reaches the hook unaltered)

## Workflow follow-up

- Codex review of the diff until clean, then `/opsx:verify` with 0 CRITICAL.
- Archive with `/agk-archive`.
