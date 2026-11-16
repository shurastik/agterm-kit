# AGENTS.md

This file provides guidance to coding agents working in this repository.

## Overview

agterm-kit (`agk`) is a Go CLI that wires coding-agent workflows into [agterm](https://agterm.app)
companions through `agtermctl`. It runs two workers next to the main agent session, each in its own
visible terminal companion:

- the **applier** (`agk apply`): an interactive, autonomous claude or codex session that implements
  an OpenSpec change and later fixes findings it is sent, reporting each round in a file;
- the **reviewer** (`agk review`): a read-only codex or claude review of the diff with structured
  findings. In agterm it is an interactive harness session you can talk to: it writes its findings to
  a file that `agk review wait` validates, and a re-review is typed into the same live session.
  Outside agterm it runs headless and inline (`codex exec` / `claude -p` with the bundled schema).

The reviewer also reviews plans: `agk review start --change <name>` checks an OpenSpec change's
artifacts instead of a diff. The `agk-propose` skill (`/agk-propose`) runs the plan step around it:
the project's `openspec-propose` workflow writes the change, the reviewer reviews it until it
approves, the user annotates it in revdiff, and the skill stops before implementation. The
`agk-explore` skill (`/agk-explore`) is the step before it: it follows the project's
`openspec-explore` workflow and hands a settled idea to `/agk-propose` instead of `/opsx:propose`.
The `agk-archive` skill (`/agk-archive`) is the close step: it archives through the project's
`openspec-archive-change` only after the last code review approved the code as it is now (this
session's own `agk review` results plus `agk review fresh`, else a fallback `agk review start
--base`) and `openspec-verify-change` passed after it with 0 CRITICAL, then closes the change's
applier and reviewer companions. It never fixes anything or pushes, and never commits in the main
checkout. In a change worktree (branch `<change>` with `branch.<change>.agkBase`, made by `agk apply
start` in worktree mode) it also commits the code and runs `agk sync` before the gates, syncs again
before the archive, commits the archive and merges with `agk finish`.

Which harness and model each role uses comes from flags, `.agk.toml` and `~/.config/agk/config.toml`.
Installed with Homebrew; `agk setup` links the agent skills that teach the main session the loop.

## Commands

```bash
go build -o .bin/agk ./cmd/agk   # Build binary
go test ./...                    # Unit tests
go vet ./...                     # Vet
go mod tidy                      # Sync deps
scripts/eval-skills.sh           # Skill evals (claude plugin eval; spends API usage, not in go test)
```

## Install and usage

Installation, the quick start and the worker configuration (TOML layers, keys, `agk config`) are
documented for users in [README.md](README.md); do not duplicate them here. Dev loop from a checkout:
`go build -o .bin/agk ./cmd/agk && .bin/agk setup --skills-dir ./skills`.

The commands, for an agent driving `agk` (`agk --help` has the flags):

```bash
agk apply start <change> [--applier H[:M]] [--worktree | --no-worktree] [--no-pull]
                                             # applier companion; prints {"companion","dir","started","worktree","pulled"};
                                             # worktree mode ([applier] worktree = true): pull the base --ff-only
                                             # (--no-pull skips), commit the plan, run in .worktrees/<change>
agk apply wait <dir>                         # {"status":"done|blocked|needs-human|cancelled|running","round":K,...}
                                             # claude run with turn hooks (turn.json): a turn that ended with background
                                             # work in flight is a pause: running + "reason":"background"
agk apply send <dir> (<file>|--stdin)        # next round with findings/notes once the turn is over
agk apply close <dir>
agk review start [--base main [--for <change>] | --change <change>] [--reviewer H[:M]]
                                             # reviewer companion; prints {"companion","dir","started"}
                                             # (inline: {"companion":null,"dir"});
                                             # --change reviews an OpenSpec change's plan, not a diff
agk review wait <dir> [--timeout SECONDS]    # {"status":"completed|blocked|needs-human|cancelled|running","round":K,...}
                                             # needs-human + reason no-output|permission: the reviewer needs the user
agk review send <dir> [--note FILE] [--timeout SECONDS]
                                             # re-review round, typed into the same reviewer session once its turn
                                             # is over (default 600 s)
agk review fresh <dir>                       # {"fresh":true|false,"round":K}: read-only; does the current diff scope
                                             # still match the current round's fingerprint (rounds/<k>/scope.sha256)?
                                             # a plan run, a round without a fingerprint or a bad dir exits 1
agk review close <dir>
agk sync <change> [--no-pull]                # worktree mode: pull the base --ff-only (skipped without upstream or
                                             # --no-pull), rebase branch <change> onto it in .worktrees/<change>;
                                             # {"change","base","pulled","rebased","head"}; a conflict aborts, exit 1
agk finish <change>                          # worktree mode: needs a clean, archived, synced worktree; ff-merges the
                                             # base in the main checkout (no pull, no rebase), closes the worktree's
                                             # run companions, removes it, deletes the branch;
                                             # {"change","base","head","closed"}; a rerun completes what is left
agk config                                   # resolved workers and the source of every value
agk setup [--skills-dir DIR] [--dry-run]
agk version
```

The agent-facing workflows live in `skills/agk-apply/SKILL.md`, `skills/agk-review/SKILL.md`,
`skills/agk-explore/SKILL.md` (the brainstorm step: `/agk-explore` follows `openspec-explore` and
names `/agk-propose` at the handoff), `skills/agk-propose/SKILL.md` (the plan step:
`/agk-propose` writes a change, reviews it with `agk review start --change` and the user's revdiff
pass, and stops before `agk apply`) and `skills/agk-archive/SKILL.md` (the close step:
`/agk-archive` checks the review and verify gates, archives and closes the change's companions).

## Layout

| Path | Purpose |
|------|---------|
| `cmd/agk/` | CLI entry point and subcommands |
| `internal/agterm/` | `Ctl` interface over `agtermctl` (exec + `--json`: companions, status, info, wait, type, text) and a fake for tests |
| `internal/config/` | Worker configuration: TOML layers, flags, resolution with per-key sources |
| `internal/harness/` | `Driver` per harness (claude, codex): interactive applier and reviewer argv, headless applier turn, structured inline review turn; reviewer `extra_args` guard; event-stream rendering; `RunStreaming` |
| `internal/worktree/` | Real-git operations of worktree mode (`Git`): locate the main checkout and `.worktrees/<change>`, pull `--ff-only`, plan commit through a pathspec, `worktree add` with `branch.<change>.agkBase`, status, archived-on-branch, rebase with abort, ff-only merge, remove, `branch -d`; tested on temporary repositories with a bare `origin` |
| `internal/rundir/` | `.agk/<kind>/<run>/` directories: numbering, rounds, `worker.json` |
| `internal/apply/` | Applier prompts and rules, round report validation, wait decision, openspec state |
| `internal/review/` | Review scope from git (with a working-tree fingerprint per diff round), prompts (with the output-file variant) and the fix-up prompt, review JSON parse/validate, wait decision (`Decide`), info lines |
| `internal/cli/` | Subcommands over an injectable `Env` (Ctl, Driver, Collect, clock, ...); `worker.go` holds the plumbing both workers share (start handshake, sliced waits, typing a line once the turn is over) |
| `internal/fsx/` | `ReplaceFile`: temp file + rename, never writes through symlinks |
| `internal/setup/` | `agk setup`: skills dir resolution, link/ignore/dependency plan, apply |
| `internal/termsafe/` | Keeps untrusted model output from driving a terminal: control characters and bidi overrides become visible escapes |
| `internal/buildinfo/` | `Version`, stamped with `-ldflags -X` at release |
| `assets/` | Embedded files: review schema and prompts (incl. the fix-up prompt), applier prompts and rules, claude deny settings, reviewer tool list |
| `skills/` | Agent skills (`agk-*`) telling Claude Code and Codex how to use `agk`; shipped as `share/agk/skills/` |
| `packaging/` | Homebrew formula template rendered into the `shurastik/homebrew-tap` repo |
| `scripts/` | `release.sh`: tag a version and bump the tap formula; `eval-skills.sh`: run the skill evals |
| `evals/` | Eval suite of the three workflow skills (`agk-propose`, `agk-explore`, `agk-archive`) for `claude plugin eval`: cases, per-case plugins, stand-in `agk`/`revdiff`, stub skills (see `evals/README.md`) |

Run artifacts land in `.agk/reviews/<timestamp>/` and `.agk/applies/<timestamp>/` of the repo, one
`rounds/<k>/` per round (gitignored here; a diff review round also holds `scope.sha256`). An interactive review run also holds `schema.json`, the
reviewer's only writable directory `out/` (it writes `out/<k>.json`) and, for claude,
`reviewer-settings.json`.

## Conventions

- Every `agtermctl`, harness and `openspec` call goes through an interface or an injected func
  (`agterm.Ctl`, `harness.Driver`, `Env.ApplyState`) so tests assert exact argv against fakes; no
  test touches a real agterm, claude or codex.
- `agtermctl` calls always pass `--target` (its default is the frontmost session, not the caller's)
  and `--socket $AGTERM_SOCKET` (agtermctl ignores the variable). The binary resolves like agterm's
  hooks: `$AGTERMCTL`, the app bundle, then PATH.
- A companion's `$AGTERM_SESSION_ID` is its own id. `--command` runs in the user's login shell
  (fish), so values with `\` or newlines are refused rather than quoted (paths, config values).
- Worker state lives on disk (`worker.json`, `round`, `rounds/<k>/result.json`, `report.json` or the
  reviewer's `out/<k>.json`); agterm status is only the wake-up signal. A run is pinned to the
  configuration it started with.
- A claude applier's settings carry two agk hooks (hidden `agk apply hook turn|stop <run>`) that record
  `<run>/turn.json` (round, `running`/`stopped`, in-flight counts). The round comes from the kickoff
  line the applier received, never from `<run>/round` (`agk apply send` moves that from outside). The
  hooks print nothing and exit 0 always. `worker.json` `turn_hooks` opts a run into reading it.
- The interactive applier and reviewer keep their `AGTERM_*` variables: their harness hooks are how
  agk learns that a turn ended, and agk never sets a worker companion's agent status itself (it sets
  only info lines). A claude reviewer runs `--restricted`, which drops the user's hooks, so agk
  gives it its own through `--settings`: they run the hidden `agk review hook <state>`.
- The reviewer is read-only by its harness, not its prompt: codex in a `workspace-write` sandbox whose
  only writable root is `out/`, claude restricted with `Edit(//<out>/**)` as its one write rule
  (Write rules are not matched). `extra_args` that widen this are refused by `agk review start`, and
  agk's mandatory flags come after the user's tuning.
- Input reaches a live worker only as one typed line (`Read <prompt file> and follow it.` + Enter),
  and only when its turn is over and no permission prompt is open.
- Model output is untrusted: rendered through `termsafe` before it reaches a terminal, never put into
  status markdown beyond fixed strings and counts.
- Outside agterm (`AGTERM_ENABLED` unset) workers run inline in the foreground.
- User-facing changes update `README.md` in the same change: commands, flags, config keys and their
  resolution, install/setup, skills. Internal refactors do not touch it.

## Workflow

Follow `~/.claude/CLAUDE.md`: OpenSpec explore -> propose -> apply (TDD) -> codex review -> `/opsx:verify`
(0 CRITICAL) -> archive. Open work lives in `BACKLOG.md` (see Task tracking).

## Task tracking

Open work that is not yet an OpenSpec change lives in `BACKLOG.md` at the project root: one `##` section per item (kind and description), newest first, an epic's open children nested as `###`. It is the only place open work is tracked outside OpenSpec changes. Do not use markdown TODO lists or Todo-style agent tools.

- When you turn a backlog entry into an OpenSpec change, remove that entry from `BACKLOG.md`.
- When you find a bug or follow-up outside the current change's scope, append a new section to `BACKLOG.md` instead of doing the work silently.
- Durable knowledge that agents need across sessions goes into a `## Project notes` section of this file, one bullet per note.
