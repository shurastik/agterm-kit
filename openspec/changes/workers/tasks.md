# Tasks

## 1. Run directories and agterm primitives

- [x] 1.1 Move run-dir creation and numbering into `internal/rundir` (`New(root, kind, now)` for `reviews` and `applies`, `Number(dir)`), plus read/write helpers for `worker.json`, `round` and `rounds/<k>/` per design D9; verify with tests ported from `review/rundir_test.go`, plus numbering per kind and round round-trips
- [x] 1.2 Add `Type`, `Text`, `MsgSend`, `MsgRead` to `agterm.Ctl`, `agterm.CLI` and the fake, and widen `Wait` to accept `idle`/`active`. `MsgRead` uses `--wait --ack --json` as the caller's own pane. Verify with argv tests (always `--target`/`--socket`, `--stdin` for typed text, the `--to SESSION:PANE` form) and response-parsing tests, including exit 124 and 2 for wait

## 2. Worker configuration (`internal/config`, `agk config`)

- [x] 2.1 Add `github.com/BurntSushi/toml`. Implement load and strict validation per design D8 and the "Configuration validation" requirement (unknown table/key, harness, types, `permission_mode` placement, unquotable `extra_args`). Verify with table tests, one per scenario, checking that errors name the file and key
- [x] 2.2 Implement resolution per the precedence, role-flag and harness-switch rules (flag > `.agk.toml` at the repo root > `$XDG_CONFIG_HOME`/`~/.config/agk/config.toml` > defaults, with the source of each key) and `Same()`. Verify with tests for every scenario of "Configuration sources and precedence", "Role flag" and "Worker roles and defaults"
- [x] 2.3 Add `agk config [--applier H[:M]] [--reviewer H[:M]]` with JSON output and the same-worker warning, plus the usage text. Verify with CLI tests for sources per key, exit 1 on an invalid file, and the warning on stderr

## 3. Harness drivers (`internal/harness`)

- [x] 3.1 Codex driver per design D5/D6: interactive applier argv, headless applier (`exec -s workspace-write --json`), review round 1 (`exec -s read-only --json --output-schema -o`, no `--ephemeral`), and resume (`exec resume <thread> -c sandbox_mode="read-only"`). The thread id comes from `thread.started`. Model, effort and `extra_args` are mapped. Verify with exact-argv tests against a fake `StreamExec` and a thread-id extraction test
- [x] 3.2 Claude driver per design D5/D6: interactive applier argv (`--permission-mode`, `--settings` deny file, `--append-system-prompt-file`, `--session-id`, model and effort), headless applier (`-p --session-id`/`--resume`), and review rounds (`-p --output-format stream-json --verbose --json-schema --permission-mode dontAsk --allowedTools <reviewer-tools>`, `--resume` later) with `structured_output` written to the output path. Verify with exact-argv tests and a test that a canned stream-json transcript produces `output.json`
- [x] 3.3 Event renderers for codex `--json` and claude `stream-json` per design D7: agent text, `$ cmd` lines, unknown events skipped, non-JSON passed through, raw stream kept in `events.jsonl`. Verify with golden tests over canned streams, including a hostile ESC/OSC 52 payload coming out escaped through termsafe

## 4. Reviewer rounds

- [x] 4.1 Move `review start`/`run` to the round layout and the configured driver: `worker.json` (config, base, session id), `rounds/1/`, `reviewer.log`, companion `<harness>-review #N`, info lines `<harness> #N r<k>:`, header with the round, and the same-worker warning. Verify by updating the `review_start`/`review_run` tests for every MODIFIED scenario of Run directory, Companion session, Progress and outcome status, Review numbering, Timestamped log and Findings validation, plus a claude-reviewer start test
- [x] 4.2 Turn `agk review run` into a loop per design D6: record `pane`, run the recorded round, set the status, then `MsgRead`, then the next round. A failed round keeps waiting. Verify with fake-Ctl tests: two rounds in one process, a failed round followed by a successful one, and the round-2 info line
- [x] 4.3 Add `agk review send <dir> [--note FILE]`: refuse while a round is running and on an empty scope; write the re-review prompt (new `assets/rereview-prompt.md`); resume the session or start fresh with no session id; send `round <k>` to `<companion>:<pane>`; run inline outside agterm. Verify with tests for each "Re-review round" scenario
- [x] 4.4 Change `agk review wait` to read the current round (`round` in its output) and never close the companion. Add `agk review close <dir>`, which tolerates a gone or absent companion. Verify by updating the wait tests for every MODIFIED scenario of Wait, Companion gone, and Outcome of an inline review; add close tests; remove the auto-close tests

## 5. Applier (`internal/apply`, `agk apply`)

- [ ] 5.1 Add the applier assets per design D10 (`apply-prompt.md`, `fix-prompt.md`, `applier-rules.md`, claude deny settings), rendered with `text/template` and absolute paths. Verify with tests that the round-1 prompt names the change, the `openspec instructions apply` command and the report path, and that the deny file covers commit, push, tag, reset --hard, rebase, checkout -- and gh pr
- [ ] 5.2 Add report parsing and validation (`done`/`blocked`, `summary`, `problem`/`artifact` for blocked) and the pure wait decision over (report, status, error) per design D4. Verify with table tests covering done, blocked, invalid-report, permission, no-report, cancelled and running
- [ ] 5.3 Add `agk apply start <change> [--applier H[:M]]`: kebab-case check, readiness through an injected `openspec instructions apply --json` runner (missing, blocked, `nothing to apply`), run dir, companion `<harness>-apply #N` with the driver's interactive argv and kickoff line, and the 60 s handshake with the `started:false` warning showing screen lines. Verify with CLI tests for each "Change must be ready", "Applier companion" and "Start handshake" scenario, plus codex applier argv
- [ ] 5.4 Add `agk apply wait <dir> [--timeout]`, which never closes the companion, and `agk apply close <dir>`. Verify with fake-Ctl tests for each "Wait for the applier" scenario and for close of an open or gone companion
- [ ] 5.5 Add `agk apply send <dir> (<file>|--stdin) [--timeout]` per design D2: wait for the turn to end, refuse on `blocked`, busy or gone, write `rounds/<k+1>/prompt.md`, record the round, type one line plus `\r`, then the active handshake. Verify with tests for each "Follow-up round" scenario, asserting exactly one `Type` call with no embedded newline
- [ ] 5.6 Add the inline applier outside agterm (headless round 1, resume on send, wait reads the report, sanitised output on stderr). Verify with tests asserting that no agtermctl call is made and that the recorded session id is reused for round 2

## 6. Triage removal

- [ ] 6.1 Delete `internal/triage`, `internal/cli/triage.go` and their tests, `Ctl.OverlayHTML` and its tests, and the `agk triage` usage text; mark exit codes 2 and 3 as reserved in the usage text. Verify that `go build ./... && go test ./... && go vet ./...` pass and `grep -rn triage --include='*.go' .` finds nothing

## 7. Skills and docs

- [ ] 7.1 Rewrite `skills/agk-review/SKILL.md`: start, wait, act, `send` for re-review, `close` when the loop is over, no triage, and how to choose the harness via config or flags. Verify that the front matter parses and every command in it matches `agk --help`
- [ ] 7.2 Add `skills/agk-apply/SKILL.md`: start (and what to do on `started:false`), wait outcomes and the reaction to each, send findings or notes, close; never type into a working applier. Verify `agk setup --skills-dir ./skills --dry-run` plans links for both skills
- [ ] 7.3 Update `AGENTS.md`: overview (workers, no triage), usage, layout (`internal/config`, `internal/harness`, `internal/rundir`, `internal/apply`; drop `internal/triage`), conventions (the applier keeps `AGTERM_*`, typed input is one line), and a config example. Verify that the documented commands match `agk --help`

## 8. Integration checks

- [ ] 8.1 Run a live smoke test in agterm in a scratch git repo with a tiny OpenSpec change. Check:
  - the claude applier: start (handshake `true` in a trusted folder, `false` with screen lines in an untrusted one), wait `done`, send a note, wait for round 2, close;
  - the codex reviewer: start, then send round 2, which resumes the same thread, then close;
  - the codex applier and the claude reviewer: one round each.

  Record the results in the change's design Open Questions (the codex `\r` and trust behaviour), and fix any driver constants with tests
- [ ] 8.2 Run the quality gates: `go vet ./... && go test ./... && go build -o .bin/agk ./cmd/agk`, plus `openspec validate workers --strict`

## Workflow follow-up

- Codex review of the diff until clean, then `/opsx:verify` (0 CRITICAL).
- Release 0.2.0 with `scripts/release.sh 0.2.0`, `brew upgrade agk`, `agk setup`.
- Archive the change and close its bead.
