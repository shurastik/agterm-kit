## Why

The code loop of the planned `agk flow` needs two long-lived agents next to the main session: an
applier that implements a change and a reviewer that checks the diff. Today agk can only run a
one-shot codex review. Its companion closes after each run, and codex starts from scratch every
time. Nothing runs the applier. The projects carry hand-written `opsx-applier` copies instead,
and they have already drifted. The engine is also fixed: you cannot swap in claude as the
reviewer or codex as the applier.

This change adds the workers: an applier and a reviewer, each running in a visible agterm
companion. The main session can start them, wait for them, send them more work and close them.
Each worker's harness (claude or codex) and model come from configuration. `agk flow` (the next
change) will be built from these primitives.

## What Changes

- **New `agk apply start|wait|send|close`.** The applier is an interactive `claude` (or `codex`)
  session in a `<harness>-apply #N` companion that works on its own:
  - Its instructions come from prompts built into the binary.
  - It records each round's outcome in a report file (`done` or `blocked`).
  - It is not allowed to commit or push.
  - Each later round reaches the live session as a one-line pointer typed into its prompt, so the
    session keeps its context.
  - `agk apply wait` reports `done`, `blocked`, `needs-human` (a permission prompt, or a turn that
    ended without a report), `cancelled` or `running`.
  - Outside agterm, the applier runs headless in the foreground.
- **The reviewer stays alive between review rounds.**
  - **BREAKING:** `agk review wait` no longer closes the companion. The new `agk review close`
    does.
  - The new `agk review send` starts the next round on the same harness session (`codex exec
    resume` / `claude -p --resume`), so the reviewer remembers its earlier findings.
  - **BREAKING:** the run directory changes from flat files to `rounds/<k>/`. The companion becomes
    `<harness>-review #N`. Info lines become `<harness> #N r<k>: …`. The log becomes
    `reviewer.log`.
- **The reviewer harness is configurable.** It runs `codex exec` read-only, as today, or `claude
  -p` with read-only tools. Both use the bundled output schema, and output from both is still
  validated, sanitised and logged with timestamps.
- **New worker configuration.**
  - Per role (`applier`, `reviewer`): `harness`, `model`, `effort`, `extra_args`, plus
    `permission_mode` for a claude applier.
  - Sources, strongest first: the `--applier` / `--reviewer` flags (`HARNESS[:MODEL]`), then
    `.agk.toml` in the repository root, then `~/.config/agk/config.toml`, then built-in defaults
    (applier `claude`, reviewer `codex`).
  - A run records the configuration it resolved and keeps using it.
  - The new `agk config` prints the resolved roles.
- **BREAKING:** `agk triage` and its HTML overlay are removed. Review findings now go straight to
  the applier, and the user's checkpoint becomes a revdiff of the final diff (in `flow`).
- The `agk-review` skill is updated for rounds and close, and drops triage. A new `agk-apply`
  skill teaches the main session to drive the applier.

## Capabilities

### New Capabilities
- `apply-worker`: the applier in an agterm companion or inline. Covers the start handshake,
  autonomy guardrails, per-round report, wait outcomes, follow-up rounds, close and the run
  directory.
- `worker-config`: which harness, model and options each worker role uses. Covers the config
  sources and their precedence, validation, flag overrides, pinning a run to its resolved
  configuration, and `agk config`.

### Modified Capabilities
- `review-companion`:
  - Reviewer harness chosen from configuration; the codex-only read-only requirement is replaced.
  - Round-based run directory, plus re-review rounds that resume the reviewer's session.
  - The companion persists until `agk review close` instead of closing on `completed`.
  - Companion name, info lines and log name follow the harness and the round.
- `findings-triage`: retired. Every requirement is removed together with `agk triage`.

## Impact

- **Code:**
  - New packages `internal/config`, `internal/harness` (claude and codex drivers, readable
    rendering of their JSON event streams) and `internal/apply`.
  - `internal/review` moves to rounds and the harness drivers.
  - `internal/cli` gains `apply`, `config`, `review send` and `review close`, and loses `triage`.
  - `internal/triage` is deleted.
  - `agterm.Ctl` gains `Type`, `Text`, `MsgSend` and `MsgRead`.
- **Assets:** new applier prompts and rules, a re-review prompt and the reviewer read-only tool
  list.
- **Dependencies:** a TOML parser (`github.com/BurntSushi/toml`), the first non-stdlib module.
  At runtime: `claude` with agterm's agent-status hooks for the applier in agterm, and `openspec`
  to check the change.
- **Skills:** `skills/agk-review` is rewritten and `skills/agk-apply` is new; both ship via the
  existing `agk setup`.
- **Docs:** the `AGENTS.md` overview, usage, layout and conventions.
- **Out of scope** (later changes):
  - `agk flow` and its state machine, the spec-review loop and prompt, revdiff gating, `/agk`,
    the Stop hook, the dashboard, and migrating projects and beads.
