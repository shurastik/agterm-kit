# Design

## Context

Greenfield repository; see proposal.md for motivation and specs/ for required behavior. Constraints that shape the approach:

- agterm 0.40 is driven only through the `agtermctl` CLI (control socket). Relevant verbs: `session new --parent --command --wait --no-select --name --json`, `session status`, `session info --markdown --hide-after`, `session wait --until`, `session overlay open --html --js --block --follow`.
- An `--html` overlay page runs JavaScript only with `--js`. With `--js` the page gets a bridge, `agterm.request(cmd, {args})`, that can issue **any** control-socket command. `--block` makes `overlay open` print `{"pageID","outcome","value"}` and exit 0 on submit, 2 on dismiss, 1 on error. A plain HTML form without JS maps fields to separate args and cannot carry one JSON value, so `--js` is required.
- The codex plugin's `review` command returns free text; only its `adversarial-review` emits structured JSON, and the plugin lives at a versioned cache path. `codex exec` (0.166) supports `-s read-only`, `-C <dir>`, `--output-schema <file>`, `-o <file>` and reading the prompt from stdin (`-`).
- Codex output is untrusted input: it is derived from repository content that may itself be hostile.

## Goals / Non-Goals

**Goals:**
- Unit-testable without agterm or codex: every external process goes through a small interface with a fake that records argv.
- No dependency on the codex plugin's install path; the output schema and prompt ship inside the binary.
- Standard library only.

**Non-Goals:**
- OpenSpec dashboard page companion (next change).
- Re-running the review automatically until approve; the agent drives the loop via the skill.
- Supporting multiple concurrent reviews per session with distinct names; each review gets its own companion but they share the name `codex-review`.
- Windows/Linux support; agterm is macOS-only.

## Decisions

### D1. CLI shape: `flag` subcommands, single binary
`agk review start [--base main]`, `agk review run <dir>` (internal, the companion's command), `agk triage <review.json>`. Hand-rolled dispatch over `flag.FlagSet`; no cobra, three commands do not justify a dependency. Each command is a function taking an `Env` struct (`Ctl`, `Runner`, `Git`, stdout/stderr, getenv, now) so `cmd/agk` tests drive commands end to end against fakes.

### D2. `agterm.Ctl` interface over `agtermctl`
```go
type Ctl interface {
    NewCompanion(ctx, parent, name, command, cwd string) (id string, err error)
    Status(ctx, target, state string) error
    Info(ctx, target, markdown string) error
    OverlayHTML(ctx, target, file string) (Outcome, error) // --js --block --follow
}
```
The real implementation builds argv and runs `agtermctl` through an injectable `exec` function; `NewCompanion` passes `--json` and parses the id from the response, which the spike showed as `{"ok":true,"result":{"id":"…","parent":"…"}}`. `OverlayHTML` maps exit 0 → `Outcome{Submitted, Value}`, exit 2 → `ErrDismissed`, anything else → error with stderr. Spike: submit prints `{"pageID":"…","outcome":"submitted","value":"<string>"}` (exit 0) and close prints `{"outcome":"dismissed","pageID":"…"}` (exit 2); `value` is the exact string the page passed. Both the exit code and `outcome` must agree, otherwise it is an error. Every call adds `--socket $AGTERM_SOCKET` when set (agtermctl does not read the variable itself, review finding), and the binary resolves like agterm's own hooks: `$AGTERMCTL`, then `/Applications/agterm.app/Contents/MacOS/agtermctl`, then PATH. Tests assert exact argv. Alternative considered: talking to the control socket directly. Rejected: undocumented wire format, and `agtermctl` already handles socket discovery.

### D3. Companion runs `agk review run <dir>` with an absolute binary path
`review start` creates the run dir and writes `prompt.md` before spawning, so empty-scope and git errors surface synchronously in the agent's shell. It then calls `NewCompanion(parent=$AGTERM_SESSION_ID, name="codex-review", command="<os.Executable()> review run <dir>", cwd=<repo root>)` with `--wait --no-select`. The absolute path avoids depending on the companion shell's PATH. Arguments are shell-quoted in case `--command` goes through a shell (confirmed in the spike).

Spike results (agterm 0.40.0): the companion's `$AGTERM_SESSION_ID` is its **own** id, so `run` targets itself through it; no `companion.id` handshake is needed. `--command` is executed by the user's login shell (fish here, not `sh`). POSIX single-quoting with `'\''` for embedded quotes works in fish and sh alike, but fish treats `\` inside single quotes as an escape, so `start` refuses executable or run-dir paths containing a backslash or newline instead of trying to quote them. Every `agtermctl` call passes `--target` explicitly: its default is the frontmost active session, not the caller's. The command is prefixed with `env 'PATH=<caller PATH>' 'CODEX_HOME=<caller value>' 'AGTERMCTL=<caller value>'` (only these, when set; never secrets, since the command line is visible in ps) so `codex` and `agtermctl` resolve inside the companion even when agterm starts it without a login shell (review finding).

### D4. Review scope and prompt
`git merge-base <base> HEAD` → `git diff` with `review.DiffFlags` (`--no-ext-diff --no-textconv --ignore-submodules=none`: raw content, since textconv, external diff tools or submodule ignore config could otherwise hide changes, review findings); the truncation note in the prompt gives codex the exact same command, quoted for the shell (working tree vs merge base, covering commits, staged and unstaged) plus `git ls-files --others --exclude-standard -z` for untracked files, whose contents are appended as pseudo-diffs (binary files, files over 64 KiB and files that are not valid UTF-8 listed by name only; invalid UTF-8 bytes are shown as `\xNN` before the size cap is applied (escaping grows them 4x), because codex refuses a prompt that is not valid UTF-8, review finding). Empty diff and no untracked files → `nothing to review`. The diff in the prompt is capped (~400 KiB); past the cap the prompt keeps the `--stat` and tells codex to read the remaining files itself, which it can do in the read-only sandbox. The prompt is an embedded `text/template` (`assets/review-prompt.md`) adapted from the plugin's review stance: material findings only, grounded in file and line, output per schema.

### D5. Codex invocation through `review.Runner`
```
codex exec -s read-only -C <root> --ephemeral \
  --output-schema <dir>/review-output.schema.json -o <dir>/codex-output.json -
```
Prompt on stdin; stdout and stderr tee'd to the companion terminal and `<dir>/codex.log`. The embedded schema is written into the run dir because codex needs a file path. `--ephemeral` keeps review runs out of the user's codex session history. The child environment drops every `AGTERM_*` variable: the user's global codex hooks call agterm's status script, which would otherwise mark the companion `completed` at codex's Stop, before agk has validated the output and written `review.json` (review finding). No timeout by default: reviews can take minutes, and the companion is visible and closable. Codex runs in its own process group; cancellation (SIGINT, SIGTERM, or SIGHUP when the companion closes) first sends the group SIGINT, which codex handles by interrupting the turn and cleaning up its tool processes (they run in their own sessions, outside our group), then SIGTERM after 5 s, then SIGKILL 3 s later; the final stage runs synchronously before returning because agk exits right after, and it runs after every exit (cancel, crash or normal), so a crashed npm wrapper cannot leave the native child running (review finding) and `WaitDelay` bounds the wait for pipes, because the npm codex wrapper spawns a native child that would otherwise keep stdout open (review finding).

### D6. Validation without a JSON-schema library
`review.Parse` decodes with `DisallowUnknownFields` into typed structs and checks enums (verdict, severity), required non-empty strings, `line_start/line_end >= 1`, `confidence` in [0,1]. This mirrors the bundled schema; a test loads the embedded schema and asserts the Go-side enums match it so they cannot drift silently. Valid output is re-encoded to `review.json`; invalid output stays as `codex-output.json` and the run is `blocked` with `invalid review output`.

### D7. Status and info lines
Every line is labelled with the review number N (`codex #N:`); the companion is named `codex-review #N`. N is the 1-based position of the run dir among `.agk/reviews/*` sorted by timestamp then collision suffix, recomputed by `run` from its own dir, so a later run never renumbers an earlier one and no counter file is needed. `active` + `codex: reviewing` (with `--hide-after 0`), then `completed` + `codex: N findings (k critical, m high)` (only non-zero critical/high counts named) or `codex: approve, no findings`, or `blocked` + `codex: <reason>`. Severity counts use info markdown color (`critical`/`high` red/orange). The info line is plain enough that the spec scenarios match its visible text. Each status update runs on a context detached from the review's (`context.WithoutCancel` + 5 s timeout), so a run cancelled by Ctrl-C or SIGTERM still reports `blocked` instead of leaving `session wait` hanging on `active` (review finding). The final `completed`/`blocked` update is retried (3 attempts, 500 ms apart), and before it the run writes `<dir>/result.json` (`status`, `reason`, `findings`, `verdict`); the skill waits with `--timeout` and falls back to that file, so one lost status update cannot stall the workflow (review finding).

### D8. Triage page: server-rendered form, inert data, nonce CSP
`triage.Render` produces one self-contained HTML file, written per call to a fresh `<dir>/triage-<random>.html` (`os.CreateTemp`: exclusive create, never through a symlink) and removed when triage ends, so concurrent triage calls of one report never share a page path (review finding) from an embedded `html/template` with inline CSS and JS:
- Each finding renders as a card with radio inputs `d-<n>` (fix/skip/discuss, default by severity) and a textarea `note-<n>`. All finding text goes through `html/template` contextual escaping; nothing from findings is placed in attributes other than escaped `value`/`id` built from the integer index, in `data-agterm`, or inside `<script>`.
- The script is static: on submit it reads the radios and notes, builds `[{id, decision, note}]`, and calls `agterm.request("session.overlay.submit", {args: {value: JSON.stringify(list)}})`. It contains no other bridge call.
- CSP meta: `default-src 'none'; script-src 'nonce-<random>'; style-src 'unsafe-inline'; img-src 'none'; form-action 'none'`. A per-render random nonce instead of `'unsafe-inline'` scripts means that even an escaping bug could not run injected script. Event handlers are attached with `addEventListener`, never inline attributes.
- The overlay is opened without `--cwd`, so the page has no file access.
- Spike confirmed: with this CSP the bridge object `agterm` is still injected (`typeof agterm === "object"`), an un-nonced inline script is blocked, and `agterm.request("session.overlay.submit", {args: {value}})` delivers `value` byte for byte (quotes, `<>&`, non-ASCII).

Alternative considered: passing findings as JSON into the script and rendering client-side. Rejected: puts untrusted text into the script context and widens the attack surface for no gain.

### D9. Outcome to decisions
`triage.Decide(findings, outcome)` parses `value`, requires exactly one entry per finding id 1..N, decision in {fix, skip, discuss}, note as string (trimmed, length-capped). It returns decisions enriched with `title` and `file` from the findings file (not from the page), so the page cannot alter what the agent believes it is fixing. Output goes to stdout and `decisions.json`. `decisions.json`, `review.json` and `prompt.md` are written through `fsx.ReplaceFile` (temp file + rename), so a symlink planted at one of those paths is replaced rather than written through (review finding). If the triage is cancelled while the page is open, `OverlayHTML` closes the session's overlay on a detached context, but only after `tree --json --window $AGTERM_WINDOW_ID` (tree shows one window, the frontmost by default) confirms the overlay shown is still this call's page file (paths compared after resolving symlinks); a cancellation before `open` touches nothing, since the page belongs to the agterm app and would otherwise outlive agk and block the next overlay (review finding).

### D10. Fallbacks and exit codes
`AGTERM_ENABLED` unset: `review start` calls the same review routine in-process with a no-op status reporter; `triage` prints a markdown table (pipes escaped, newlines collapsed) and exits 3. Exit codes: 0 ok, 1 error, 2 dismissed, 3 no agterm UI.

### D11. Run directory naming
`.agk/reviews/<YYYYMMDD-HHMMSS>/` under `git rev-parse --show-toplevel`; if it exists, append `-2`, `-3`, …, created with `os.Mkdir` (not `MkdirAll` for the leaf) so a collision is detected atomically.

### D12. Skill
`skill/agterm-codex-review/SKILL.md`, installed by symlink into `~/.claude/skills/`. It tells the agent: run `agk review start --base <base>`; wait with a background `agtermctl session wait --target <companion> --until completed --until blocked`; on `blocked` read `codex.log` and report; on `completed` run `agk triage <dir>/review.json`; then fix `fix` items, raise `discuss` items in chat with their notes, ignore `skip`; re-run until approve. On triage exit 3 use `AskUserQuestion`; on exit 2 ask in chat. `agk` itself is installed with `go install ./cmd/agk`.

### D14. Terminal-safe output
`internal/termsafe` owns the rule for what may reach a terminal raw (no C0 except `\n`/`\t`, no DEL, no C1, no bidi embeddings/overrides/isolates, no U+2028/2029). `termsafe.Writer` applies it to the codex stream before stamping, holding an incomplete UTF-8 sequence across writes; the triage fallback table uses the same rule. Codex echoes the prompt, which inlines untracked repository files, so this is a trust boundary (review finding). A source test rejects literal bidi controls in `.go` files.

### D13. Timestamped log
`review.StampWriter` prefixes each line with `[YYYY-MM-DD HH:MM:SS] ` (local time, taken when the line starts, correct across partial writes). `Codex.Run` wraps the combined terminal + `codex.log` stream with it; `run` prints a stamped header `agk: review #N started, run dir <dir>` and a stamped summary line. exec funnels codex stdout and stderr through one pipe when they share a writer, so the stamp writer sees no concurrent writes.

## Risks / Trade-offs

- [Bridge behaves differently than documented: `agterm.request` unavailable under the CSP, a different submit command name, or `value` not passed through verbatim] → Spike is task 1; D3/D8 are revised before any code depends on them.
- [`$AGTERM_SESSION_ID` inside a companion points at the parent] → Spike checks; fallback described in D3.
- [Hand-written validator drifts from the bundled schema] → Test asserts enum parity against the embedded schema file.
- [Huge diffs blow the prompt] → Capped diff plus stat; codex reads files itself.
- [Codex hangs] → No automatic timeout; the user sees it in the companion and can close it, which makes `session wait` exit 2; the skill treats exit 2 as cancelled.
- [Parent session closes mid-review] → Companion closes with it; nothing to clean up beyond the run dir.
- [Overlay cleanup on cancel is check-then-act: between `tree` confirming our page and `overlay close`, the user could close it and another process open a new overlay, which close would then hit] → Accepted residual risk: agterm 0.40 has no close-by-pageID and `agtermctl` dies on SIGINT/SIGTERM without closing its page (verified), so no atomic cleanup exists; the window is a few milliseconds and needs two independent actions inside it. Dropping cleanup would bring back the stuck-page bug. Tracked as a follow-up to request close-by-pageID upstream.
- [Concurrent reviews] → Each review gets its own number and companion (`codex-review #N`); ids differ and `start` prints the id.
