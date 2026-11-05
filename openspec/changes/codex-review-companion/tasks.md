# Tasks

## 1. Spike: agterm bridge and companion behavior

- [x] 1.1 Open a throwaway HTML page (nonce CSP meta, static script calling `agterm.request("session.overlay.submit", {args: {value: JSON.stringify([...])}})`, plus one un-nonced inline script) with `agtermctl session overlay open --html F --js --block --follow`; record the exact stdout JSON and exit code on submit and on ⌘W, whether `value` arrives verbatim, and whether the un-nonced script is blocked; write the findings into design.md D8 (and fix D2 if the outcome shape differs)
- [x] 1.2 Create a companion with `agtermctl session new --parent $AGTERM_SESSION_ID --command '<cmd printing $AGTERM_SESSION_ID and $0>' --wait --no-select --name spike --json`; record the `--json` response shape (where the id is), whether `--command` runs through a shell, and whether the companion's `$AGTERM_SESSION_ID` is its own id; update design.md D2/D3 accordingly and close the spike companion

## 2. agterm control wrapper

- [x] 2.1 Write failing tests for the real `agterm.Ctl` argv of `NewCompanion` (incl. `--parent --command --wait --no-select --name --cwd --json` and id parsing from the spike's response shape), `Status` and `Info`, using an injected exec func; implement until `go test ./internal/agterm/` passes
- [x] 2.2 Write failing tests for `OverlayHTML` argv (`--html --js --block --follow --target`, no `--cwd`) and exit mapping (0 → submitted with value, 2 → `ErrDismissed`, other → error with stderr, unparsable stdout → error); implement until tests pass
- [x] 2.3 Add a recording fake `Ctl` for higher-level tests and verify it with a small self-test

## 3. Review core

- [x] 3.1 Embed `assets/review-output.schema.json` (copied from the codex plugin) and implement `review.Parse` per design D6; tests cover valid output, invalid JSON, bad severity/verdict, missing required field, unknown field, line < 1, confidence outside [0,1], plus an enum-parity test against the embedded schema; `go test ./internal/review/` passes
- [x] 3.2 Implement git scope collection (merge base of `--base`, working-tree diff, untracked non-ignored files as pseudo-diffs, binary/oversize files by name only, empty-scope error `nothing to review`); tests use temporary git repos covering branch commits + uncommitted edit + untracked file, ignored file excluded, and clean tree
- [x] 3.3 Implement the embedded prompt template and diff cap (stat kept, note to read files past the cap); tests assert the diff and untracked content land in the prompt and that an oversize diff is truncated with the note
- [x] 3.4 Implement run-dir creation `.agk/reviews/<YYYYMMDD-HHMMSS>[-N]/` under the repo root with atomic collision suffix; test two runs at the same fake time get distinct dirs
- [x] 3.5 Implement `review.Runner` invoking `codex exec -s read-only -C <root> --ephemeral --output-schema <dir>/review-output.schema.json -o <dir>/codex-output.json -` with the prompt on stdin and output tee'd to `codex.log`; test exact argv, stdin content and log file via an injected exec func
- [x] 3.6 Implement info-line formatting per design D7; tests cover `codex: 3 findings (1 high)`, `codex: approve, no findings` and a failure reason

## 4. `agk review` commands

- [x] 4.1 Implement `agk review run <dir>` (status `active` + info, run codex, parse, write `review.json`, `completed` + summary; any failure → `blocked` + reason, raw output kept, non-zero exit); tests with fake `Ctl`/`Runner` cover findings, clean approve, codex failure and invalid output
- [x] 4.2 Implement `agk review start [--base]` inside agterm (scope check before any agterm call, run dir + `prompt.md`, `NewCompanion` with absolute `os.Executable()` path and shell-quoted args, JSON `{"companion","dir"}` on stdout); tests assert no `Ctl` call on empty scope and exact companion argv otherwise
- [x] 4.3 Implement the outside-agterm path of `review start` (inline run, no `Ctl` calls, `{"companion": null, "dir": ...}`, exit code mirrors the run); test with `AGTERM_ENABLED` unset
- [x] 4.4 Wire `cmd/agk/main.go` dispatch with usage text and exit codes 0/1/2/3; verify `go build ./cmd/agk` and `agk --help`, `agk review --help` output

## 5. `agk triage`

- [ ] 5.1 Implement `triage.Render` per design D8 (embedded `html/template`, numbered cards with all fields, radios defaulted by severity, notes, static nonce'd script with only the submit call, CSP meta with a per-render nonce); tests assert fields and defaults are present and that hostile title/body (`<script>…`, `" onmouseover="x`, `<img src=https://…>`) appear only escaped, with exactly one `<script>` element carrying the nonce
- [ ] 5.2 Implement `triage.Decide` per design D9; tests cover valid submit, non-JSON value, missing finding, unknown id, duplicate id, bad decision, and that `title`/`file` come from the findings file
- [ ] 5.3 Implement the `agk triage <review.json>` command inside agterm (invalid input → exit 1 with no UI; zero findings → `[]` exit 0; otherwise write `triage.html`, `OverlayHTML` on `$AGTERM_SESSION_ID`, decisions to stdout + `decisions.json` exit 0; dismissed → exit 2, no output; garbage → exit 1); tests with fake `Ctl`
- [ ] 5.4 Implement the outside-agterm markdown table fallback with exit 3 (pipes escaped, newlines collapsed); tests cover row count and column integrity with hostile cells

## 6. Skill and docs

- [ ] 6.1 Write `skill/agterm-codex-review/SKILL.md` per design D12 (trigger description, start → background wait → triage → act loop, exit-code handling incl. 2 and 3 fallbacks), symlink it into `~/.claude/skills/agterm-codex-review`, and verify the link resolves
- [ ] 6.2 Update `AGENTS.md` (commands, install via `go install ./cmd/agk`, gitignore `.agk/` in reviewed repos) and verify the documented build/install commands run as written

## 7. Integration checks

- [ ] 7.1 Run `gofmt -l .` (empty), `go vet ./...`, `go test ./...`, `go build ./...` and confirm all pass
- [ ] 7.2 E2E in agterm on a real repo (e.g. `~/Projects/gm-helper`, `--base HEAD~3`): companion `codex-review` appears under the session, info line changes, glyph ends `completed`, `agtermctl session wait` returns 0, `agtermctl tree --json` shows the companion's `parent`; `agk triage` opens the overlay (visible in `htmlOverlays`), submit prints decisions JSON and writes `decisions.json`, ⌘W exits 2
- [ ] 7.3 Fallback E2E: `env -u AGTERM_ENABLED agk triage <review.json>` prints the table and exits 3; `env -u AGTERM_ENABLED agk review start` runs inline and prints `"companion": null`
- [ ] 7.4 Run `openspec validate codex-review-companion --strict` and confirm it passes

## Workflow follow-up

- Codex review of the full diff until clean.
- `/opsx:verify` with 0 CRITICAL (CRITICAL → fix → codex review of the fix → verify again).
- Archive the change, `bd close` the epic, push code and beads.
