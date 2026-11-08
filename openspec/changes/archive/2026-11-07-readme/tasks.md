# Tasks

## 1. Facts

- [x] 1.1 Build a fresh binary (`go build -o .bin/agk ./cmd/agk`) and collect `.bin/agk --help`
  plus `--help` of every subcommand (`apply start|wait|send|close`, `review start|wait|send|close`,
  `config`, `setup`, `version`) into a scratch file outside the repository; verify each command
  exits 0 or prints usage, and note any difference from AGENTS.md Usage
- [x] 1.2 Read `internal/config` (layers, precedence, `setHarness` reset, accepted keys and values,
  `--applier`/`--reviewer` parsing) and `internal/setup` (skills dir resolution, links, git
  excludes, dependency checks); verify by writing down, in the same scratch file, the exact keys,
  defaults and dependency list the README will state, each with the file it came from
- [x] 1.3 Check minimum versions claimed in `openspec/config.yaml` (agterm 0.40+, codex-cli 0.166+)
  against the code and the specs; verify each one is either confirmed with a source or marked
  "list without a version" for README Requirements

## 2. README.md

- [x] 2.1 Write README.md sections 1-2 (title, pitch, how it works with the ASCII diagram, short
  agterm and OpenSpec primers with links) per design.md; verify the diagram uses only ASCII
  characters (`LC_ALL=C grep -n '[^ -~]' README.md` reports nothing inside the diagram block) and
  the pitch's first line equals the new description text
- [x] 2.2 Write sections 3-4 (requirements, installation incl. `agk setup` behavior, `--dry-run`,
  rerun after `brew upgrade agk`, build from a checkout); the requirements state that `agk setup`
  requires `git`, `codex`, `claude` and `openspec` and exits 1 when one is missing, separately from
  which harness CLIs the configured workers run; verify every statement against the scratch facts
  from 1.2-1.3 and that `.bin/agk setup --dry-run --skills-dir ./skills` behaves as described,
  including its exit code with a required tool removed from `PATH` (e.g. `PATH` without `codex`)
- [x] 2.3 Write section 5 (quick start: OpenSpec setup of the target repository with `openspec
  init`/`openspec update` so it has `openspec-propose`, then agent-driven, then the bare CLI loops
  with their JSON output); verify every command and flag shown appears in the `--help` output from
  1.1 (and `openspec init --help`/`openspec update --help` for the OpenSpec step), every printed
  JSON shape matches the `cmd/agk`/`internal/cli` output, and the sequence covers each precondition
  of the `propose-workflow` spec (project `openspec-propose` skill, `agk` on `PATH`)
- [x] 2.4 Write section 6 (configuration: files, precedence, keys, `agk config`, the harness-reset
  and `harness[:model]` pitfalls) with a full TOML example; verify by writing that example to a
  temporary `.agk.toml` in a scratch git repository and checking `agk config` there resolves the
  values and sources the README claims, then deleting the scratch repository
- [x] 2.5 Write sections 7-8 (development: build, test, vet, evals with the API-cost warning,
  release; license MIT); verify `go build ./... && go vet ./... && go test ./...` pass as written

## 3. AGENTS.md

- [x] 3.1 Replace the Install and Usage sections with a pointer to README.md, keeping only the
  `agk` command summary an agent needs; verify nothing removed is missing from README.md (diff the
  removed lines against README.md by hand) and that `skills/*/SKILL.md` do not reference the
  removed sections
- [x] 3.2 Add the `internal/termsafe/` row to Layout and the README convention (user-facing changes
  to commands, flags, config keys and their resolution, install/setup or skills update README.md in
  the same change); verify both by reading the rendered AGENTS.md

## 4. Project metadata

- [x] 4.1 Rewrite `openspec/config.yaml` `context` to the current system per design.md and add the
  README check to `operations.apply.guidance`; verify `openspec context --json` succeeds and
  `openspec instructions tasks --change readme --json` shows the new context text
- [x] 4.2 Set `desc` in `packaging/homebrew/agk.rb.tmpl` to the new description; verify it is under
  80 characters, has no leading article and does not start with "agk"
- [x] 4.3 Fix the package doc comment in `cmd/agk/main.go` (no overlays); verify `go vet ./...` and
  `go test ./...` pass
- [x] 4.4 Set the GitHub repository description with
  `gh repo edit shurastik/agterm-kit --description "<new description>"`; verify with
  `gh repo view shurastik/agterm-kit --json description`

## 5. Integration

- [x] 5.1 Check that README.md, AGENTS.md, the formula `desc` and the GitHub description agree
  (same description text; no command, flag or key in AGENTS.md contradicts README.md); verify by
  grepping each `agk` subcommand and config key across both files

## Workflow follow-up

- Codex review of the diff until clean, then `/opsx:verify` with 0 CRITICAL.
- Archive with `openspec archive readme --skip-specs`, then `bd close` the epic.
- The new formula `desc` reaches the tap with the next `scripts/release.sh`.
