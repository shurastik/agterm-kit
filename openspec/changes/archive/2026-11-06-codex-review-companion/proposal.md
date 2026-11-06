## Why

The codex review step of the workflow runs as a background shell job inside the Claude session: the user cannot watch it, and its findings are triaged as free text in chat. agterm 0.40 added terminal companions and HTML overlays that can return a structured result, which makes a visible review plus a click-through triage possible.

## What Changes

- New Go CLI `agk` in this repository.
- `agk review start --base <ref>` spawns a terminal companion named `codex-review` under the current agterm session and prints the companion id and the run directory.
- `agk review run <dir>` (the companion's command) builds a prompt from the git diff, runs `codex exec` read-only with a JSON output schema, writes `review.json`, and reports progress and the outcome through the companion's sidebar status glyph and info line.
- `agk triage <review.json>` opens a blocking HTML overlay on the parent session listing the findings, lets the user mark each one fix / skip / discuss with an optional note, and prints the decisions as JSON on stdout.
- Outside agterm, `review start` runs the review inline and `triage` prints a markdown table and exits with a distinct code so the agent falls back to asking in chat.
- A Claude skill (`skill/agterm-codex-review/SKILL.md`) tells the agent when and how to call `agk` and how to act on the decisions.

## Capabilities

### New Capabilities

- `review-companion`: running a codex review of a git diff as a visible agterm companion session, producing a validated structured findings file and signalling progress and completion through agterm status.
- `findings-triage`: presenting review findings to the user for per-finding decisions (fix / skip / discuss + note) and returning those decisions as machine-readable JSON, with a non-agterm fallback.

### Modified Capabilities

## Impact

- New repository code: `cmd/agk`, `internal/agterm`, `internal/review`, `internal/triage`, `assets/review-output.schema.json`, `skill/agterm-codex-review/`.
- Runtime dependencies: `agtermctl` (agterm 0.40+), `codex` CLI (0.166+), `git`. No Go dependencies beyond the standard library are planned.
- Reviewed repositories get a `.agk/reviews/<timestamp>/` directory with run artifacts; users should gitignore `.agk/`.
- The user's global workflow (codex review step) gains an agterm-native path through the skill; the existing codex plugin is not modified.
- Security surface: the triage page runs with JavaScript enabled and the overlay bridge can issue any control-socket command, so codex-produced text must never be able to inject markup or script.
