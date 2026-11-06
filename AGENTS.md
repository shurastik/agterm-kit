# AGENTS.md

This file provides guidance to coding agents working in this repository.

## Overview

agterm-kit (`agk`) is a Go CLI that wires coding-agent workflows into [agterm](https://agterm.app)
companions and HTML overlays through `agtermctl`. First feature: run a codex review as a visible
terminal companion under the Claude session, then triage its findings in an HTML overlay and hand
the decisions back to the agent as JSON.

## Commands

```bash
go build -o .bin/agk ./cmd/agk   # Build binary
go install ./cmd/agk             # Install to $GOBIN
go test ./...                    # Unit tests
go vet ./...                     # Vet
go mod tidy                      # Sync deps
```

## Usage

```bash
agk review start [--base main]   # spawn the "codex-review #N" companion; prints {"companion","dir"}
A="${AGTERMCTL:-/Applications/agterm.app/Contents/MacOS/agtermctl}"; [ -x "$A" ] || A=agtermctl
"$A" session wait --socket "$AGTERM_SOCKET" --target <companion> --until completed --until blocked --timeout 1800
agk triage <dir>/review.json     # overlay; prints decisions JSON (exit 2 dismissed, 3 no agterm)
```

The agent-facing workflow lives in `skill/agterm-codex-review/SKILL.md`, installed with
`ln -sfn ~/Projects/agterm-kit/skill/agterm-codex-review ~/.claude/skills/agterm-codex-review`.
Reviewed repos should gitignore `.agk/`.

## Layout

| Path | Purpose |
|------|---------|
| `cmd/agk/` | CLI entry point and subcommands |
| `internal/agterm/` | `Ctl` interface over `agtermctl` (exec + `--json`) and a fake for tests |
| `internal/review/` | Prompt from git diff, codex runner, review JSON parse/validate, run dir |
| `internal/cli/` | Subcommands over an injectable `Env` (Ctl, Runner, Collect, clock) |
| `internal/fsx/` | `ReplaceFile`: temp file + rename, never writes through symlinks |
| `internal/triage/` | HTML triage page (`html/template` + embedded CSS/JS) and outcome parsing |
| `assets/` | Embedded files (review output schema, review prompt template) |
| `skill/` | Claude skill telling the agent how to use `agk`; symlinked into `~/.claude/skills/` |

Run artifacts land in `.agk/reviews/<timestamp>/` of the reviewed repo (gitignored here).

## Conventions

- Every `agtermctl` and `codex` call goes through an interface (`agterm.Ctl`, `review.Runner`) so tests
  assert exact argv against fakes; no test touches a real agterm or codex.
- `agtermctl` calls always pass `--target` (its default is the frontmost session, not the caller's)
  and `--socket $AGTERM_SOCKET` (agtermctl ignores the variable). The binary resolves like agterm's
  hooks: `$AGTERMCTL`, the app bundle, then PATH.
- A companion's `$AGTERM_SESSION_ID` is its own id. `--command` runs in the user's login shell
  (fish), so paths with `\` or newlines are refused rather than quoted.
- The codex child runs without `AGTERM_*` variables so its own agterm status hooks cannot mark the
  companion done before agk has validated the output.
- Codex output is untrusted. Render it only through `html/template`, never put it into `data-agterm`
  attributes, and keep the page CSP strict. The overlay bridge can run any socket command.
- Outside agterm (`AGTERM_ENABLED` unset) commands degrade to plain terminal output with a distinct exit code.

## Workflow

Follow `~/.claude/CLAUDE.md`: OpenSpec explore -> propose -> apply (TDD) -> codex review -> `/opsx:verify`
(0 CRITICAL) -> archive -> `bd close`. Tasks live in beads (`bd`), prefix `agk`.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:1105d646 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->
