## Why

Inside agterm, the applier companion shows a live claude or codex session. The reviewer companion
shows only a timestamped log: `agk review run` drives `codex exec --json` (or `claude -p
--output-format stream-json`) and prints a few rendered events. The user can watch the reviewer but
cannot read its full reasoning in the harness's own UI. They also cannot step in mid-review to point
it somewhere, ask a question or correct a misreading. The reviewer is the one worker the user cannot
talk to.

## What Changes

- **The reviewer runs as an interactive session inside agterm, like the applier.**
  - The companion `<harness>-review #N` runs the harness's own TUI (interactive `codex` or
    `claude`).
  - Each round starts with one typed line that tells the reviewer to read `rounds/<k>/prompt.md`.
  - A re-review is typed into the same live session once its turn is over, so the reviewer keeps
    its context without `exec resume` / `--resume`.
  - The agterm status hooks stay on for the reviewer: the end of a turn comes from its agent
    status, as for the applier.
- **The reviewer writes its findings to a file.**
  - Interactive harnesses cannot enforce an output schema. So the prompt names the schema file and
    an output file `<run>/out/<k>.json`, and the reviewer writes its JSON object there.
  - `agk review wait` validates that file with the existing review parser. It then writes
    `rounds/<k>/review.json` and `result.json` and sets the companion's summary info line.
  - When the file is invalid and the turn is over, `agk review wait` types a fix-up instruction
    into the session. It does this a bounded number of times, then fails the round with `invalid
    review output`.
- **Read-only stays enforced by the harness, not by the prompt.**
  - **codex:** runs with `--sandbox workspace-write --ask-for-approval never`, with `<run>/out` as
    its working directory, configured extra writable roots reset and the tmp roots excluded. The
    only writable place is `out/`; the repository, `.git` and the rest of the run directory are
    denied by the sandbox.
  - **claude:** runs with `--restricted --strict-mcp-config` and tools `Read,Grep,Glob,Bash,Write`
    in `dontAsk` mode. Its allow rules are the read-only git commands plus `Write` under
    `<run>/out/`.
  - `--restricted` ignores user settings and with them agterm's claude status hooks. So agk adds
    its own hooks through `--settings`: they run a new hidden `agk review hook <state>` that sets
    the companion's agent status.
  - Reviewer `extra_args` that would widen these boundaries (sandbox, approvals, permission mode,
    tools, extra dirs, settings) are refused in both modes. agk's mandatory flags come after the
    user's tuning.
- **Agent status belongs to the harness.** In agterm, agk no longer sets the reviewer companion's
  agent status. It sets only info lines (`r<k>: reviewing`, the findings summary, the failure
  reason). The status then always means "turn running / over / waiting for permission".
- **New wait status `needs-human`.**
  - `agk review wait` reports `needs-human` with reason `no-output` when the reviewer's turn ended
    without an output file. It reports reason `permission` when the reviewer asks for permission.
  - `agk review start` reports `"started"`, from the same start handshake the applier uses.
- **Outside agterm nothing changes.** The inline review keeps `codex exec` / `claude -p` with the
  bundled schema, the timestamped log and sanitised output.
- **BREAKING (in agterm only):** the companion no longer runs `agk review run`. That hidden
  subcommand, the mailbox loop and the `pane` file are removed. A run directory started by an
  older agk can still be waited on through its `result.json`, but `agk review send` refuses it and
  asks for a new review.
- **Plan reviews (`--change`) use the same mechanics.** Only their prompt differs.
- **Skills:** `agk-review` and `agk-propose` handle `needs-human` and the `started` field.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `review-companion`: inside agterm, the reviewer becomes an interactive harness session that
  writes its findings to a file validated by `agk review wait`. This changes the run directory
  layout, the companion command, status reporting, the log (now inline-only), findings validation
  (fix-up retries), the wait statuses (`needs-human`), the read-only guarantees per harness, how a
  re-review reaches the reviewer, and how the reviewer stays alive between rounds. It adds a start
  handshake and rules for older run directories.

## Impact

- **Code:**
  - `internal/harness`: a new interactive reviewer argv per driver and claude reviewer settings
    with the hooks.
  - `internal/cli`:
    - `review.go`: start, send and close; removes `ReviewRun`, `nextRound` and the `pane` file.
    - `wait.go`: a decision over the output file and agent status, plus fix-up typing.
    - `apply.go`: the shared handshake, wait and type helpers move where both workers can use
      them.
    - `main.go`: drops `review run` and adds `review hook`.
  - `internal/review`: prompt templates gain an output-file variant, and a decision function maps
    the output and status to the wait outcome.
  - `assets/`: review prompts and a fix-up prompt.
- **Skills:** `skills/agk-review/SKILL.md` and `skills/agk-propose/SKILL.md`.
- **Docs:** `AGENTS.md` conventions. "The reviewer child runs without `AGTERM_*`" no longer holds
  inside agterm.
- **Dependencies:** codex-cli with `sandbox_workspace_write.exclude_*` config keys (verified on
  0.167.0) and Claude Code with `--restricted`, `--tools` and `--settings` hooks (verified by a
  spike task before the claude path is built).
