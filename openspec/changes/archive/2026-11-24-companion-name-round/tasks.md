# Tasks

## 1. agterm rename

- [x] 1.1 Add `Rename(ctx, target, name string) error` to `agterm.Ctl`, `*CLI` (`agtermctl session rename <name> --target <id>` through `runOK`, with the socket like every other call) and `agtermtest.Fake` (records the call like the other methods and can be told to fail it) in one step, since the fake asserts it implements `Ctl`; RED: a test of the exact `*CLI` argv; GREEN: verify `go build ./...` and `go test ./internal/agterm/...` pass

## 2. Round-based companion names

- [x] 2.1 Change `companionName` to take the round and return `[<change> · ]<harness>-<kind> r<round>`, and `reviewCompanionName` to take the round; start both workers with round 1 (`ApplyStart`, `startInteractive`) and drop their `rundir.Number` calls; update the start tests (`review_start_test.go`, `apply_test.go`, `worker_test.go`) to expect `codex-review r1`, `add-auth · codex-review r1`, `add-auth · claude-plan r1`, `add-auth · claude-apply r1`, including a repository with earlier runs that still gets `r1`; verify `go test ./internal/cli/...` passes
- [x] 2.2 Drop `Num` from `review.Label`: `String` gives `<harness> r<k>` (`<harness>` alone, or `review` for an unknown harness, when the round is unknown); remove `reviewRun.num` and its `rundir.Number` call; change the inline log lines to `agk: review round <k> started, run dir <dir>` and `agk: review round <k>: <n> finding(s), verdict <v> -> <path>`; update `internal/review` and `review_inline_test.go` tests for info lines (`codex r1: 3 findings (1 high)`, `codex r2: approve, no findings`, `claude r2: reviewing`) and log lines; verify `go test ./internal/review/... ./internal/cli/...` passes
- [x] 2.3 Remove `rundir.Number` and `TestNumber` once nothing calls it; verify `go vet ./...` and `go test ./internal/rundir/...` pass

## 3. Rename on send

- [x] 3.1 `agk review send`: once `typeWhenIdle` returned without error (started or not), rename the companion to the round-(k+1) name rebuilt from the run's recorded state; a rename error prints `agk: warning: rename reviewer <id>: <err>` and leaves stdout and the exit code unchanged; no rename on a refusal (busy, permission prompt, gone) or inline. Tests against the fake: diff review with `--for` renamed to `add-auth · codex-review r2`, plan review renamed to `add-x · claude-plan r3`, unconfirmed start still renamed, rename error is a warning with exit 0 and `"started": true`, busy reviewer not renamed, inline run calls no rename; verify `go test ./internal/cli/...` passes
- [x] 3.2 `agk apply send`: the same after `typeWhenIdle`, renaming to `<change> · <harness>-apply r<k+1>`, warning `agk: warning: rename applier <id>: <err>`. Tests: renamed to `add-auth · claude-apply r2`, unconfirmed start still renamed, rename error is a warning with stdout `{"round":2,"started":true}` and exit 0, busy applier not renamed, inline run calls no rename; verify `go test ./internal/cli/...` passes

## 4. Docs

- [x] 4.1 Update `agk --help` in `internal/cli/main.go` (`NAME · <harness>-review r<k>`, renamed on send) and its help test if one pins the text; verify `go test ./internal/cli/...` passes and `.bin/agk --help` shows the new name
- [x] 4.2 Update `README.md` (companion names section near the `<harness>-apply #N` lines: names carry the round and are renamed on every send), `skills/agk-review/SKILL.md` and `skills/agk-apply/SKILL.md` (every `#N` name); verify `grep -rn '#N\|#<N>' README.md skills/` finds no companion name left

## 5. Integration

- [x] 5.1 Run the quality gates and verify they pass: `go build -o .bin/agk ./cmd/agk`, `go vet ./...`, `go test ./...`
