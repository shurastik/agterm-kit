# Proposal

## Why

agk is installed by hand today: `go install ./cmd/agk` from a checkout plus a manual
`ln -sfn ~/Projects/agterm-kit/skill/agterm-codex-review ~/.claude/skills/...`. That only works on
this machine, only for Claude Code, and the skill drifts from the binary whenever one is rebuilt
without the other. agk is about to grow into the agent workflow for every project in `~/Projects`
(workers, flow, dashboard in later changes); all of it rests on one install path that puts a
matching binary and matching agent skills on a machine, with nothing written into the projects.

## What Changes

- Package agk for Homebrew: a public tap `shurastik/homebrew-tap` with a formula that builds `agk`
  from a tagged release and installs the agent skills under `share/agk/skills/`.
- New `agk setup`: idempotent, machine-level wiring. It links every skill from the installed
  skills directory into `~/.claude/skills/` and `~/.codex/skills/`, adds `.agk/` to the user's
  global git ignore file, removes the legacy `agterm-codex-review` link, and reports missing
  dependencies (`git`, `codex`, `claude`, `openspec`; `agtermctl` as optional).
- New `agk version`: prints the build version (stamped at release, `dev` otherwise).
- Move the skill `skill/agterm-codex-review/` to `skills/agk-review/` and rename it `agk-review`;
  every agk skill uses the `agk-` prefix from now on.
- A release script that tags a version and bumps the formula in the tap.
- Set the `go` directive in `go.mod` to the minimum the code needs so the formula builds with
  Homebrew's Go without a toolchain download.
- Docs (`AGENTS.md`, skill notes) switch from `go install` + manual symlink to
  `brew install` + `agk setup`; `--skills-dir` keeps the from-checkout dev loop.

Out of scope (later changes): applier/reviewer workers and their harness drivers and
`~/.config/agk/config.toml`, `agk flow`, the `agk gate` Stop hook and removal of the `bd` hooks,
the status dashboard, removal of `agk triage`, migrating the projects off beads.

## Capabilities

### New Capabilities
- `installation`: how agk and its agent skills get onto a machine and stay in sync: the release
  layout, `agk setup`, and `agk version`.

### Modified Capabilities

None. The review behavior in `review-companion` does not change; only the skill that drives it is
renamed and relocated, which no existing requirement covers.

## Impact

- New: `internal/setup/` (link planning, ignore-file edit, dependency checks), `agk setup` and
  `agk version` in `internal/cli/` and `cmd/agk/`, `scripts/release.sh`, `Formula/agk.rb` kept in
  the tap repo (a reference copy lives in this repo under `packaging/`).
- Moved: `skill/agterm-codex-review/SKILL.md` -> `skills/agk-review/SKILL.md`.
- Changed: `go.mod` `go` directive, `AGENTS.md` install and layout sections.
- External, needs the owner's go-ahead at apply time: making `shurastik/agterm-kit` public and
  creating the public `shurastik/homebrew-tap` repo.
- On this machine after the change: `~/go/bin/agk` and the manual skill symlink are replaced by the
  brew install plus `agk setup`.
