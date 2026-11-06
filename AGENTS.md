# AGENTS.md

This file provides guidance to coding agents working in this repository.

## Overview

agterm-kit (`agk`) is a Go CLI that wires coding-agent workflows into [agterm](https://agterm.app)
companions through `agtermctl`. It runs two workers next to the main agent session, each in its own
visible terminal companion:

- the **applier** (`agk apply`): an interactive, autonomous claude or codex session that implements
  an OpenSpec change and later fixes findings it is sent, reporting each round in a file;
- the **reviewer** (`agk review`): a read-only codex or claude review of the diff with structured
  findings, kept alive between rounds so re-reviews resume its session.

Which harness and model each role uses comes from flags, `.agk.toml` and `~/.config/agk/config.toml`.
Installed with Homebrew; `agk setup` links the agent skills that teach the main session the loop.

## Commands

```bash
go build -o .bin/agk ./cmd/agk   # Build binary
go test ./...                    # Unit tests
go vet ./...                     # Vet
go mod tidy                      # Sync deps
```

## Install

```bash
brew install shurastik/tap/agk   # binary + skills in $(brew --prefix)/opt/agk/share/agk/skills
agk setup                        # link agk-* skills into ~/.claude/skills and ~/.codex/skills,
                                 # add .agk/ to the global git ignore, check dependencies
```

Rerun `agk setup` after every `brew upgrade agk`; it is idempotent (`--dry-run` previews).
Dev loop from a checkout: `go build -o .bin/agk ./cmd/agk && .bin/agk setup --skills-dir ./skills`.
Release: `scripts/release.sh X.Y.Z` tags, then renders `packaging/homebrew/agk.rb.tmpl` into the
tap checkout (`$AGK_TAP_DIR`, default `~/Projects/homebrew-tap`) and pushes it.

## Usage

```bash
agk apply start <change> [--applier H[:M]]   # applier companion; prints {"companion","dir","started"}
agk apply wait <dir>                         # {"status":"done|blocked|needs-human|cancelled|running","round":K,...}
agk apply send <dir> (<file>|--stdin)        # next round with findings/notes once the turn is over
agk apply close <dir>
agk review start [--base main] [--reviewer H[:M]]   # reviewer companion; prints {"companion","dir"}
agk review wait <dir>                        # {"status":"completed|blocked|cancelled|running","round":K,...}
agk review send <dir> [--note FILE]          # re-review round in the same reviewer session
agk review close <dir>
agk config                                   # resolved workers and the source of every value
agk setup [--skills-dir DIR] [--dry-run]
agk version
```

Worker configuration (`~/.config/agk/config.toml`, or `.agk.toml` in a repo root; flags win, then
the repo file, then the user file, then the defaults: applier `claude`, reviewer `codex`):

```toml
[applier]
harness = "claude"             # claude | codex
model = "claude-sonnet-5-5"    # optional; the harness default otherwise
effort = "high"                # optional
permission_mode = "auto"       # claude applier only; default auto
extra_args = ["--add-dir", "../shared"]

[reviewer]
harness = "codex"
model = "gpt-5.5"
```

The agent-facing workflows live in `skills/agk-apply/SKILL.md` and `skills/agk-review/SKILL.md`.

## Layout

| Path | Purpose |
|------|---------|
| `cmd/agk/` | CLI entry point and subcommands |
| `internal/agterm/` | `Ctl` interface over `agtermctl` (exec + `--json`: companions, status, wait, type, text, mailbox) and a fake for tests |
| `internal/config/` | Worker configuration: TOML layers, flags, resolution with per-key sources |
| `internal/harness/` | `Driver` per harness (claude, codex): interactive applier argv, headless applier turn, structured review turn; event-stream rendering; `RunStreaming` |
| `internal/rundir/` | `.agk/<kind>/<run>/` directories: numbering, rounds, `worker.json` |
| `internal/apply/` | Applier prompts and rules, round report validation, wait decision, openspec state |
| `internal/review/` | Review scope from git, prompts, review JSON parse/validate, status lines |
| `internal/cli/` | Subcommands over an injectable `Env` (Ctl, Driver, Collect, clock, ...) |
| `internal/fsx/` | `ReplaceFile`: temp file + rename, never writes through symlinks |
| `internal/setup/` | `agk setup`: skills dir resolution, link/ignore/dependency plan, apply |
| `internal/buildinfo/` | `Version`, stamped with `-ldflags -X` at release |
| `assets/` | Embedded files: review schema and prompts, applier prompts and rules, claude deny settings, reviewer tool list |
| `skills/` | Agent skills (`agk-*`) telling Claude Code and Codex how to use `agk`; shipped as `share/agk/skills/` |
| `packaging/` | Homebrew formula template rendered into the `shurastik/homebrew-tap` repo |
| `scripts/` | `release.sh`: tag a version and bump the tap formula |

Run artifacts land in `.agk/reviews/<timestamp>/` and `.agk/applies/<timestamp>/` of the repo, one
`rounds/<k>/` per round (gitignored here).

## Conventions

- Every `agtermctl`, harness and `openspec` call goes through an interface or an injected func
  (`agterm.Ctl`, `harness.Driver`, `Env.ApplyState`) so tests assert exact argv against fakes; no
  test touches a real agterm, claude or codex.
- `agtermctl` calls always pass `--target` (its default is the frontmost session, not the caller's)
  and `--socket $AGTERM_SOCKET` (agtermctl ignores the variable). The binary resolves like agterm's
  hooks: `$AGTERMCTL`, the app bundle, then PATH.
- A companion's `$AGTERM_SESSION_ID` is its own id. `--command` runs in the user's login shell
  (fish), so values with `\` or newlines are refused rather than quoted (paths, config values).
- Worker state lives on disk (`worker.json`, `round`, `rounds/<k>/result.json` or `report.json`);
  agterm status is only the wake-up signal. A run is pinned to the configuration it started with.
- The reviewer child runs without `AGTERM_*` variables so its own agterm status hooks cannot mark the
  companion done before agk has validated the output. The interactive applier keeps them: its hooks
  are how agk learns that a turn ended.
- Input reaches a live applier only as one typed line (`Read <prompt file> and follow it.` + Enter),
  and only when its turn is over and no permission prompt is open.
- Model output is untrusted: rendered through `termsafe` before it reaches a terminal, never put into
  status markdown beyond fixed strings and counts.
- Outside agterm (`AGTERM_ENABLED` unset) workers run inline in the foreground.

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
