# Design

## Context

Current state (see proposal.md for motivation):
- `agk review start` creates `.agk/reviews/<ts>/` and spawns a companion that runs
  `agk review run <dir>`.
- That process runs `codex exec --ephemeral` once through `review.Runner`. It writes `result.json`
  and `review.json` and sets the agterm status.
- `agk review wait` closes the companion on `completed`.
- All external commands go through interfaces with fakes (`agterm.Ctl`, `review.Runner`,
  `StreamExec`).

Facts from a live spike on 2026-11-06 (agterm with agtermctl, claude 2.1.319, codex-cli 0.166):
- **Typing into a running claude works.** `agtermctl session type --stdin --target <companion>`
  into an interactive `claude` running in a background companion: one injection of `text\r`
  submits the prompt, and arrow keys (`\e[B`) and `\r` also work.
- **The end of a turn can be detected.** The global claude hooks set `active` and then
  `completed` (`--auto-reset`). `session wait --until active` followed by
  `session wait --until completed --until blocked` brackets one turn.
- **The trust prompt blocks.** In an untrusted folder, interactive claude stops at the
  workspace-trust prompt and sets **no** status, so a plain `wait` would hang.
- **Claude can resume with structured output.** `claude -p --session-id <uuid> --output-format
  json --json-schema '<schema>'` returns `structured_output`. `claude -p --resume <uuid> …` keeps
  the context.
- **Codex exposes its thread id.** `codex exec --json` emits
  `{"type":"thread.started","thread_id":…}` first. `codex exec resume <id>` takes
  `--output-schema`, `-o` and `--json`, but has no `-s`: the sandbox goes through
  `-c sandbox_mode=…`. An `--ephemeral` session cannot be resumed.
- **agtermctl offers mailboxes and screen text.** `msg send --to SESSION:PANE`, `msg read --wait
  --ack` and `session text --target`.

## Goals / Non-Goals

**Goals:**
- One driver per harness that turns a role and the resolved config into argv. Tests check the
  exact argv against fakes, as today.
- Every worker state readable from disk: report and result files. The agterm status is only a
  wake-up signal, never the source of truth.
- Keep today's review guarantees: sanitised output, timestamps, retried final status, validated
  output.

**Non-Goals:**
- No orchestration. `agk flow`, round limits and deciding which findings go where are the next
  change's job.
- No spec-review prompt.
- No drivers for opencode or pi; the interface leaves room for them.
- No attempt to auto-accept claude or codex folder-trust prompts. The handshake surfaces them.

## Decisions

### D1. Interactive applier plus a file report, not a headless applier
Inside agterm the applier is an interactive TUI session in a companion. Round 1 starts as the CLI's
positional prompt, `Read <abs>/rounds/1/prompt.md and follow it.`.
- The full prompt lives in the file, because `--command` goes through fish and agk refuses line
  breaks in it.
- "Finished" comes from the agent-status hooks. "How it finished" comes from
  `rounds/<k>/report.json`.

*Alternative:* `claude -p` headless in the companion. It is simpler, but the user cannot step in,
and every follow-up loses the live TUI. It is kept only as the fallback outside agterm (D9).

### D2. Follow-up rounds are typed as one line, only when the applier is idle
`apply send` does these steps in order:
1. Waits until the status is not `active`: `wait --until completed --until blocked --until idle`
   with `--timeout`. That set also matches an already-idle session. On `blocked` it refuses: a
   permission prompt is open, and typed text would answer it.
2. Writes `rounds/<k+1>/prompt.md` and records the round.
3. Types `Read <abs>/rounds/<k+1>/prompt.md and follow it.\r`.
4. Requires `active` within 30 s (handshake).

A single line with no embedded newlines cannot be split into several submits.
*Alternative:* `codex queue` exists, but only for codex; claude has no equivalent, so typing is
the common path.

### D3. Start handshake
After `session new`, `apply start` waits up to 60 s for `active|completed|blocked`. On timeout it
reads `session text` (last 20 lines, sanitised with termsafe) into a stderr warning and prints
`"started": false`.
- The trust prompt, an expired login or a bad flag all surface the same way.
- The companion stays open so the user can fix the cause in place.

### D4. Wait logic for the applier
1. If the current round's report exists, return it (`done`/`blocked`, or `needs-human`
   `invalid-report`).
2. Otherwise wait `--until completed --until blocked --until idle`, then re-read the report,
   because hooks can fire before the file is flushed.
3. With no report:
   - `blocked` → `needs-human` `permission`.
   - `completed`/`idle` → `needs-human` `no-report`.
   - `ErrGone` → `cancelled`.
   - Timeout → `running`.

Waiting on `idle` is safe only once the round was seen `active`. The start and send handshakes
record that as `rounds/<k>/started`. `completed --auto-reset` therefore cannot be missed: the worst
case is that the reset already happened, and then `idle` matches.

When a round has no `started` marker, `wait` first waits for `active` (round 1 also accepts
`completed` or `blocked`), marks the round, and only then waits for the end. This covers a start
handshake that timed out, for example on a trust prompt. The live smoke test caught the gap: without
the marker, a fresh session's `idle` read as `no-report`.

### D5. Guardrails per harness
**Claude applier:**
- `--permission-mode <cfg, default auto>`.
- `--settings <dir>/claude-settings.json` with `permissions.deny` for `git commit`, `git push`,
  `git tag`, `git reset --hard`, `git rebase`, `git checkout --` and `gh pr`. Deny rules merge
  with the user's settings.
- `--append-system-prompt-file <dir>/applier-rules.md` (rules from the binary).
- `--session-id <uuid>` (recorded, so the user can `claude --resume` later).
- `--model` / `--effort` when set, then `extra_args`.

**Codex applier:**
- `--sandbox workspace-write --ask-for-approval never -C <root>`, `-m`, and
  `-c model_reasoning_effort=…` when set.
- The rules are prepended to the prompt file, because codex has no flag to append a system prompt.
- The workspace-write sandbox keeps `.git` read-only, which blocks commits.

The applier companion keeps the `AGTERM_*` environment (its own hooks set its status). That is
unlike the reviewer child, which still strips it.

### D6. Reviewer rounds and resume
The companion runs `agk review run <dir>`, now a loop:
1. Run the round recorded in `round`.
2. Write the round's `result.json` and set the status.
3. Record `$AGTERM_PANE_ID` in `<dir>/pane` on the first pass.
4. Block in `agtermctl msg read --wait --ack` (through `Ctl.MsgRead`, as the companion's own pane).
5. A message `round <k>` triggers the next pass.

`review send` checks that the current round has a `result.json`. It then rebuilds the scope with
the recorded base, writes the re-review prompt, records round k+1, and sends `msg send --to
<companion>:<pane> "round <k+1>"`. Outside agterm, `send` runs the round in the foreground.

**Resume:**
- **Codex:** round 1 uses `codex exec -s read-only -C <root> --json --output-schema S -o out -`,
  without `--ephemeral`. The thread id comes from the first `thread.started` event and is stored
  in `worker.json`. Round k>1 runs `codex exec resume <thread> -c sandbox_mode="read-only" --json
  --output-schema S -o out -` with cwd `<root>`.
- **Claude:** round 1 runs `claude -p --session-id <uuid> --output-format stream-json --verbose
  --json-schema <schema> --permission-mode dontAsk --allowedTools <read-only list>`. Round k>1 is
  the same with `--resume <uuid>`. The `result` event's `structured_output` is written to
  `output.json`.
- With no recorded session id, the round starts fresh with the first-round prompt.

*Alternative:* a new reviewer per round. That loses memory, and the explore session explicitly
wanted re-reviews that remember earlier findings.

### D7. One event renderer per harness
`--json` (codex) and `stream-json` (claude) make stdout machine-readable. A renderer turns events
into short human lines before the existing pipeline (termsafe writer, then stamp writer, then the
terminal and `reviewer.log`):
- agent messages: the text;
- commands: `$ <cmd>`;
- reasoning: a one-line summary.

Unknown events are skipped, and lines that are not JSON pass through. The raw stream is kept in
`rounds/<k>/events.jsonl` for debugging.

### D8. Config: BurntSushi/toml, strict decode
- Decode into a struct with `toml.MetaData.Undecoded()` and reject unknown keys.
- Each file layer is merged per key into a `Resolved` record that keeps the source of each key.
- A flag that changes the harness clears the file values that belong to another harness
  (worker-config spec).
- The repository file is found at `git rev-parse --show-toplevel`, the same root the run dir uses.

*Alternative:* hand-roll a TOML subset. That saves one dependency, but it is a parser to maintain
with weaker errors. `go.sum` pins the module; Homebrew builds from source with module download.

### D9. Package layout
| Package | Role |
|---|---|
| `internal/config` | Load, validate, resolve, `Same(applier, reviewer)` |
| `internal/harness` | `Driver` interface, `claude.go`, `codex.go`, event renderers, read-only tool list |
| `internal/rundir` | `New(root, kind, now)` and `Number(dir)`, generalised from `review/rundir.go`; `worker.json` and `round` read/write |
| `internal/apply` | Prompts, report parse/validate, wait decision as a pure function over (report, status, err) |
| `internal/review` | Keeps scope, prompt, parse, status lines; `Runner` takes a round spec and returns the session id |
| `internal/cli` | Adds `apply.go` and `config.go`; `review.go` gains `send`/`close` and the loop; `triage.go` is deleted |

`Driver` (sketch):
```go
type Driver interface {
    Interactive(w config.Worker, root string, files ApplierFiles, kickoff string) []string
    Headless(ctx, w, root, prompt, session string, out io.Writer) (session string, err error) // inline applier
    Review(ctx, w, root, prompt, schemaPath, outPath, session string, out io.Writer) (session string, err error)
}
```

`agterm.Ctl` gains `Type(target, text)`, `Text(target, lines)`, `MsgSend(to, body)` and
`MsgRead(ctx) (id, body)`, and loses `OverlayHTML`, which only triage used.

### D10. Assets
New embedded files:
- `apply-prompt.md`: round 1. A generic port of the projects' `opsx-applier`; project gates come
  from `AGENTS.md`.
- `fix-prompt.md`: round k>1, wrapping the sent message.
- `applier-rules.md`: no user, no guessing, no commit/push/deploy/archive/bd, and the
  report-file contract with its JSON shape.
- `rereview-prompt.md`: re-check the earlier findings against the new diff, look for
  regressions, and use the same schema.
- `reviewer-tools.txt`: the claude read-only allowlist.

Prompts are rendered with `text/template`. Paths are absolute.

### D11. Triage removal
Delete `internal/triage`, `internal/cli/triage.go`, the `agk triage` usage text,
`Ctl.OverlayHTML` and its tests. Exit codes 2 and 3 stay defined but unused, documented as
reserved. The skill drops its triage section.

## Risks / Trade-offs

- [The user types in the applier companion while `send` types] → `send` types only when the
  turn is over. A collision with live typing in the same instant is still possible. That is
  accepted and documented in the skill: do not type into a working applier while the main session
  drives it.
- [The agent-status hooks are missing or silent (seen with codex)] → `apply wait` polls the report
  every 5 s, so rounds still end. The handshake fails
  (`started: false`), so the cause is visible at once. `wait` would only reach `running` on
  timeout. Out of scope: `agk setup` checking the hooks.
- [Claude `auto` permission mode is unavailable or changes] → the mode is configurable
  (`permission_mode`). A bad mode shows up in the handshake.
- [Codex workspace-write blocks network (module downloads)] → this is codex's default policy. The
  user can add `extra_args = ["-c", "sandbox_workspace_write.network_access=true"]`.
- [Codex session files pile up without `--ephemeral`] → this is codex's own housekeeping (`codex
  archive`/`delete`). The reviewer stores its thread id, so the user can clean up.
- [Renderers break when the event formats drift] → unknown events are skipped and raw events are
  kept. The parse of the final output does not depend on the renderer: codex writes `-o`, and
  claude's `result` event is parsed separately.
- [`msg read --wait` addressed to the wrong pane] → the loop records its own `$AGTERM_PANE_ID`.
  `send` addresses exactly `<companion>:<pane>` and fails loudly when `pane` is missing.
- [Same model reviews itself] → a warning only, by design.

## Migration Plan

- Old run dirs (flat `result.json`) are not migrated. `review wait` on an old dir errors with "not
  a round-based run directory". Runs are disposable.
- Release as 0.2.0 via `scripts/release.sh`, then rerun `agk setup` to link the new
  `agk-apply` skill.
- Rollback is `brew` pinning to 0.1.0. No persistent state depends on the new format.

## Smoke test results (2026-11-06, agterm, claude 2.1.319, codex-cli 0.166)

- **Claude applier.**
  - In an untrusted folder, start reported `started:false` with the trust prompt on screen. After the
    prompt was answered, round 1 ended `done` and round 2 (a typed line) ended `done`.
  - Status hooks fire for both `active` and `completed`.
  - The live run caught a gap: a fresh session's `idle` was read as `no-report` before the round had
    begun. The fix is the `started` marker (D4).
- **Codex applier.**
  - Codex has its own folder-trust prompt, and the handshake shows it the same way.
  - `\r` submits typed input, and codex writes the report file.
  - On this machine, codex never changed the companion's agent status (it stayed `idle` even in a
    trusted folder), although `[[hooks.UserPromptSubmit]]` calls agterm's codex hook and the hooks
    feature is on.
  - Consequences:
    - `apply wait` polls the report between 5-second status waits, so codex rounds still end when
      the report appears.
    - `apply send` sees `idle`, so it never refuses a busy codex. Codex queues typed input.
    - The `send` handshake prints a warning.
    - `needs-human` detection (`permission`, `no-report`) needs working hooks.
- **Codex reviewer.** Round 1 approved and recorded its thread. Round 2 resumed the same thread id.
  `close` closed the companion.
  - The reviewer had loaded the `agk-review` skill on its own, so the review prompt now forbids
    running `agk`.
- **Claude reviewer.** The first run failed because claude rejects `--json-schema` when the schema
  declares `$schema` draft 2020-12. The driver now strips `$schema`. After the fix, round 1 approved
  and `close` closed the companion.

## Open Questions

- Why codex's agterm status hooks do not report in companions here (agterm hook script or codex hook
  environment). agk works without them through report polling; fixing them only restores
  `needs-human` detection for codex.
