# Design

## Context

See proposal.md - Why. User-facing usage today lives in AGENTS.md (Install, Usage, the worker
config TOML), the skills' `SKILL.md` files and the specs; the formula `desc`, the GitHub description,
`openspec/config.yaml` `context` and the `cmd/agk/main.go` doc comment still describe the retired
findings overlay. Behavior is defined by the specs under `openspec/specs/` (`apply-worker`,
`review-companion`, `propose-workflow`, `worker-config`, `installation`) and by the code.

## Goals / Non-Goals

**Goals:**
- One user-facing document, README.md, that a stranger can follow from `brew install` to a first
  review round, without reading AGENTS.md or the specs.
- Every command, flag, config key, path and default in README.md matches the current code.
- A standing rule, seen by both the main session (AGENTS.md) and the applier (OpenSpec apply
  guidance), that keeps README.md current.

**Non-Goals:**
- A full CLI reference or a docs site; README.md shows the commands, `agk <cmd> --help` and the
  skills carry the detail.
- Documenting internals (run directory format, prompts, harness argv) beyond what a user sees.
- Screenshots or recordings; no binary assets in this change.
- Changing any behavior, flag or default.

## Decisions

**README.md is for people, AGENTS.md is for agents.** README.md owns: what agk is, how it works,
requirements, installation, quick start, configuration, development, license. AGENTS.md keeps the
overview, commands (build/test/vet/evals), layout, conventions and workflow, and replaces its
Install and Usage sections with a short pointer to README.md. Its command summary stays only as far
as an agent driving `agk` needs it (the `agk-*` skills already teach the loops). Alternative: keep
both complete and in sync by hand; rejected, because the two copies diverge at the next flag change.

**README.md outline** (English, like the rest of the repository):

1. Title and pitch: what agk does in two or three sentences; macOS and agterm; outside agterm the
   workers run inline in the foreground.
2. How it works: an ASCII diagram of the main session, the applier companion and the reviewer
   companion, with the rounds on disk under `.agk/`; the loop propose -> plan review -> apply ->
   diff review -> findings -> applier fixes -> re-review; one paragraph each on agterm (terminal,
   companions) and OpenSpec (changes, artifacts) with links, for readers who know neither.
3. Requirements: macOS, agterm, `git`, `openspec`, `claude`, `codex`, `revdiff` (optional, used by
   `/agk-propose`). README states what `agk setup` checks as it is today: `git`, `codex`, `claude`
   and `openspec` are required (`internal/setup/deps.go` `Required`), and `agk setup` exits 1 when
   any of them is missing even though the skill links are already created; `agtermctl` and
   `revdiff` are optional. It then says separately that a run only executes the harnesses its
   workers are configured with, so the second harness CLI has to be installed for `agk setup` to
   pass, not for the workers. Minimum versions are stated only where they are verified during
   implementation; otherwise the tool is listed without one.
4. Installation: `brew install shurastik/tap/agk`, `agk setup` (what it links and changes, `--dry-run`,
   rerun after `brew upgrade agk`), building from a checkout.
5. Quick start: first prepare the target repository for OpenSpec (`openspec init` in a repository
   without `openspec/`, `openspec update` in one that has it, choosing the agent tools so the
   project gets its `openspec-propose` skill; `/agk-propose` stops without creating a change when
   that skill is missing, see the `propose-workflow` spec), and say that `agk setup` does not do
   this; then through the agent in an agterm session (`/agk-propose <idea>`, then asking for
   the applier and the review, which the `agk-*` skills drive); then the bare CLI loop (`agk review
   start` / `wait` / `send` / `close`, `agk apply start` / `wait` / `send` / `close`) with what each
   prints.
6. Configuration: the files and their precedence (flags, `.agk.toml` in the repo root,
   `~/.config/agk/config.toml`, defaults applier `claude` / reviewer `codex`), the keys (`harness`,
   `model`, `effort`, `permission_mode`, `extra_args`), `agk config` showing every value's source,
   and the pitfalls: a layer that switches `harness` drops `model`, `effort` and `extra_args` set
   by lower layers, so set `harness` together with them; `--applier` and `--reviewer` take
   `harness[:model]`, and effort is set only in a config file.
7. Development: build, test, vet, skill evals (they spend API usage), release.
8. License: MIT.

**Facts are taken from the code, not from AGENTS.md.** While writing, every command and flag is
checked against `agk <cmd> --help` from a fresh build, config keys and precedence against
`internal/config`, setup behavior against `internal/setup` and the `installation` spec. AGENTS.md
is a starting point only; where it disagrees with the code, the code wins and AGENTS.md is fixed in
the same change.

**The README rule lives in two places.** AGENTS.md Conventions gets the rule for any session working
in the repository. `openspec/config.yaml` `operations.apply.guidance` gets an entry ("before the
review, check README.md against the change's user-facing behavior and update it"), because the
applier receives that guidance in its OpenSpec apply instructions whatever it reads of AGENTS.md.
The trigger is concrete: commands, flags, config keys and their resolution, install/setup, skills.
Internal refactors do not touch README.md.

**One description everywhere.** The formula `desc`, the GitHub description and the README pitch's
first line say the same thing. Proposed text, within Homebrew's audit rules (under 80 characters,
no leading article, does not start with the formula name):
`Applier and reviewer companions for coding agents in the agterm terminal`.

**`openspec/config.yaml` `context` describes today's system:** Go CLI over `agtermctl`; two workers
(applier, reviewer) in terminal companions; harness drivers for claude and codex; TOML config via
`github.com/BurntSushi/toml` (the one non-stdlib dependency); model output rendered through
`termsafe`; inline fallback outside agterm. The overlay, `html/template`, CSP and page-companion
lines go.

## Risks / Trade-offs

- [README drifts from the code again] -> the two-place rule above, with a concrete trigger list.
- [A version minimum in Requirements is wrong] -> state a minimum only when verified during
  implementation; otherwise list the tool without a version.
- [The new `desc` is not visible in `brew info` until the next release] -> accepted; the template
  is the source and `scripts/release.sh` renders it.
- [`gh repo edit` changes public metadata outside git] -> it is one task at the end of the apply,
  with the exact command; the old text is in proposal.md if it has to be restored.

## Migration Plan

None: documentation and metadata. Rollback is reverting the commit and rerunning `gh repo edit` with
the old description.
