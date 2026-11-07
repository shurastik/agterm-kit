# Design

## Context

See proposal.md - Why. Today `agk review start` builds a `review.Scope` from git (merge base of
`--base`, diff, untracked files) through the injected `Env.Collect`, renders `assets/review-prompt.md`,
and records `{worker, base, session}` in `worker.json`; `agk review send` rebuilds the scope from
the recorded base and wraps the prompt in `assets/rereview-prompt.md`. Untracked files are read by
`readUntracked` (64 KiB cap, no symlinks, no binaries, valid UTF-8 only). The OpenSpec skills
(`openspec-propose` and friends) are generated per project by `openspec init` / `openspec update`
into the repository's `.claude/skills`, not into the global skills directories. The revdiff
plugin is installed for Claude Code only and opens as an agterm overlay through its own launcher.

## Goals / Non-Goals

**Goals:**
- One explicit entry point (`/agk-propose`) that leaves the user with a plan reviewed by the
  reviewer and by the user, and stops before implementation.
- Plan review reuses the review run machinery unchanged: run dir, companion, rounds, wait, send,
  close, output schema, reviewer config.

**Non-Goals:**
- Hooking or editing the generated `openspec-propose` skill, or forking the `spec-driven` schema.
- A gate in `agk apply start` that requires an approved plan review (possible later: the run dir
  now records the change).
- OpenSpec stores (`--store`) and `openspec/` roots other than the git root.
- Running revdiff from a Codex main session.

## Decisions

**Wrapper skill, not a hook or a schema fork.** `agk-propose` tells the main session to follow the
project's `openspec-propose` workflow and then continue with the review loop, overriding that
workflow's "stop after the artifacts" step (a review edits only planning artifacts, so the planning
boundary holds). Alternatives: `operations.propose.guidance` is not read by the propose skill; a
forked schema with a `review` artifact gates apply but must track upstream template changes; a
Claude Code Stop hook covers Claude only and has to guess when propose ended. The cost of the
wrapper is that a plain `/opsx:propose` bypasses it; the global workflow is pointed at
`/agk-propose` instead.

**Separate plan scope collector.** A new `review.CollectPlan(ctx, dir, change)` returns a
`review.PlanScope{Root, Change, Files, MainSpecs}`, injected as `Env.CollectPlan` like `Collect`.
It resolves the root with `git rev-parse --show-toplevel`, walks `openspec/changes/<change>/`
(sorted, regular files only, `.openspec.yaml` included) and, for each `specs/<cap>/spec.md` delta,
reads `openspec/specs/<cap>/spec.md` when present. Every file goes through `readUntracked`, so the
size cap, symlink, binary and encoding rules match the diff review. Alternative: reuse the git
scope with a pathspec - it misses committed-and-unchanged artifacts and pulls in nothing of the
main specs.

**Own prompt, shared re-review wrapper.** `assets/plan-review-prompt.md` keeps the role, finding
bar, untrusted-input and output-contract blocks of the code review prompt and swaps the task and
method for the plan checklist from the spec. Files are inlined as `<change_file path=...>` and
`<main_spec path=...>` within the same `MaxPromptDiffBytes` budget; past it a file is listed as
"read it directly". The re-review wrapper is generic already, so `RereviewPrompt` is split into a
wrapper taking a rendered body, used by both modes.

**Run state.** `reviewState` gains `change` and `base` becomes optional (both omitempty); a plan
run has `change` set and no `base`, and a run dir from before this change (base, no change) is a
diff review. `ReviewSend` picks `CollectPlan` or `Collect` by that field; everything after scope
collection is shared. The companion name and header stay as they are (`<harness>-review #N`).

**Flag checks.** `--change` is validated with the existing kebab-case `changeName` pattern before
any file is read. `--base` given explicitly (seen through `flag.Visit`) together with `--change`
is an error; the default `main` alone is not.

**revdiff step.** The skill uses the revdiff skill in file-review mode, one `--only=<file>` per
artifact, with `--description-file=<run dir>/revdiff-description.md` holding the reviewer's last
summary, the round count and findings the user set aside (the run dir is gitignored and already
the review's scratch space). When the revdiff skill is not available the step is skipped with a
warning.

**What "reviewed" means: checksum manifests.** Right before every `agk review start` and
`agk review send` the skill writes a `shasum` manifest of the artifacts (`<run dir>/pending.sha`;
for start, a temp file moved in once the run dir is known). When a round passes, the current files
are compared with that manifest and it becomes `<run dir>/approved.sha` only on a match, so edits
made while a round ran are never pinned as approved. Before closing, the artifacts are compared
with `approved.sha`, not with their state before the revdiff pass: any difference (the user's
annotations, or fixes made after the last approve) goes through another `agk review send` first.
Alternative considered: one manifest taken before revdiff and compared after it - it misses fixes
made after the last approve and edits made during a running round (both found by code review).

**Skill evals with `claude plugin eval`.** Skills are prose for an agent, so their tests are evals:
`claude plugin eval` (Claude Code 2.1.319) runs a real Claude child per case and scores it with
graders (`regex`, `tool_order`, `tool_used`, `file_exists`, `llm`). The spike (task 8.1) settled
how it behaves; the layout follows from it:
- **The suite plugin is `evals/`.** The repository root does not resolve as a plugin without a
  manifest, and an eval dir must sit inside the plugin root. So `evals/.claude-plugin/plugin.json`
  makes `evals/` the plugin under test, `evals/skills` is a symlink to `../skills`, and the manifest's
  `experimental.evals` points at `cases`, so cases live in `evals/cases/<case>/` (`prompt.md` with
  `max_turns`/`allowed_tools`, `case.yaml`, `graders/*.md`). The repository root stays a plain repo:
  agk skills ship through `agk setup`, not as a Claude Code plugin. Alternatives: a root manifest
  (makes the whole repo an installable plugin), `evals/plugin/evals/<case>/` (same effect, longer paths).
- **Each case names its own plugin.** Skills in the case workspace's `.claude/skills` are not loaded,
  and `case.yaml` `plugins:` replaces the plugin under test rather than adding to it. So every case
  has its own plugin, `evals/case-plugins/<case>/` (manifest plus symlinks), named in `case.yaml` as
  `plugins: ["../../case-plugins/<case>"]`; it cannot sit inside `evals/cases/<case>/` because every
  run scans the whole eval dir and refuses directory symlinks outside the current case's own plugins.
  The plugin holds `skills/agk-propose` linked to the real skill,
  and only the stubs that case needs from `evals/_fixtures/stubs/` - `openspec-propose` (writes a
  fixed small change, so cases test `agk-propose`, not OpenSpec or the model's planning) and
  `revdiff` (calls the stand-in `revdiff`). "Project without OpenSpec skills" leaves out the first,
  "revdiff missing" the second. Skills load namespaced (`<plugin>:agk-propose`); the skill's
  availability checks accept the namespaced names. Alternatives: stubs in the suite plugin (cannot
  vary per case), workspace files read instead of skills (the "revdiff available" check could never pass).
- **Stand-ins come first on `PATH` through the runner.** `execution.env` takes only `EVAL_*` keys and a
  workspace `.claude/settings.json` `env.PATH` is ignored, but the agent inherits the operator's
  `PATH`. `scripts/eval-skills.sh` prepends `evals/_fixtures/bin` (stand-in `agk` and `revdiff`) and
  runs `claude plugin eval evals --scaffold --trust-plugin --no-publish --ablation none` with the
  agreed threshold and `--max-cost-usd`; `EVAL_AGK_CASE` in `case.yaml` selects the case's outcome
  script. Every `scaffold_script` first checks that `agk` and `revdiff` resolve to the stand-ins and
  fails the case otherwise, so a run outside the script never reaches the real tools. The agent's
  Bash sandbox cannot read the case and plugin directories, so the scaffold then stages what the
  stubs and stand-ins read (the change writer, the case's outcome script) into `.eval/` in the
  case workspace.
- **Stand-in `agk`** appends its argv to `agk-calls.log` in the workspace and plays the case's review
  outcomes: `start`/`send` print the run JSON, `wait` writes `rounds/<k>/review.json` and prints the
  status, `close` exits 0, `apply` is only logged. **Stand-in `revdiff`** logs its arguments and
  prints the case's annotations.
- **Graders.** Deterministic first: `regex` on `trace` (the Bash commands run) or on
  `{source: file, path: agk-calls.log}`, `tool_order`, `tool_used` with `input_match`, `file_exists`
  (files created during the run). `llm` graders, given `agk-calls.log`, judge ordering and the
  agent's messages (disputed finding raised, revdiff skip warned, final summary).
The suite is not part of `go test` or the release: it spends API usage and needs Claude Code.
Alternative to `claude plugin eval`: the skill-creator eval harness - not scripted or scored the
same way, and needs the skill-creator plugin.

## Risks / Trade-offs

- [LLM runs are nondeterministic, so a case can flake] → each case runs 3 times (the default) and
  the threshold is set from a baseline run on the finished skill; a skill change that breaks a
  behavior drops a case well below it (checked by the broken-skill scenario).
- [Evals cost API usage] → run on demand via `scripts/eval-skills.sh` with `--max-cost-usd`, not
  in `go test`.

- [Reviewer and user ping-pong: the reviewer flags what the user asked for] → the skill follows the
  `agk-review` rule: a finding the main session disagrees with is raised in chat and the user
  decides; the user's revdiff pass is the last word.
- [Large changes exceed the prompt budget] → files past the budget are listed for the reviewer to
  read from disk; the read-only reviewer can do that.
- [Main session forgets to use `/agk-propose`] → `~/.claude/CLAUDE.md` names it as the plan step;
  a later apply-side gate can close the gap.
- [The project has no `openspec-propose` skill (not initialised, or a Codex main session without
  OpenSpec skills)] → the skill stops and says to run `openspec init` or `openspec update`.

## Migration Plan

No data migration. After release, `agk setup` links `agk-propose` like the other skills (rerun it
after `brew upgrade agk`). Existing review run dirs have no `change` field and keep working as
diff reviews.
