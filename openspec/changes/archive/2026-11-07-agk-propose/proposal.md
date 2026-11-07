# Proposal

## Why

The workflow reviews code automatically (`agk review` after apply), but the plan written by
`/opsx:propose` goes to implementation unreviewed: nobody checks that proposal, specs, design and
tasks agree, that scenarios are testable or that tasks cover the requirements. The generated
`openspec-propose` skill cannot be hooked (it ignores `operations.*.guidance` and is overwritten by
`openspec update`), and `agk review` only reviews a git diff with a code-review prompt, so a
planning-stage review has no place to run today.

## What Changes

- New agent skill `agk-propose`, shipped in `skills/` and linked by `agk setup`. Invoked explicitly
  (`/agk-propose`), it runs the loop in the main session:
  1. the `openspec-propose` workflow writes the change artifacts (stops if the project has no
     such skill);
  2. `agk review start --change <name>` reviews the plan; the main session fixes findings and
     re-reviews in the same reviewer session until approve; the reviewer companion stays open;
  3. revdiff opens the change's artifacts in the main session for the user's annotations; the
     main session addresses them, relaunching revdiff until the user quits without annotations;
     skipped with a warning when revdiff is not available (e.g. a Codex main session);
  4. when the user's pass changed any artifact, one more review round runs (findings go back to
     step 3); then the reviewer is closed and the skill stops. It never starts `agk apply`.
- New `agk review start --change <name>` mode: the scope is the change's artifact files plus the
  current main specs of the capabilities its deltas touch, read from disk regardless of git state;
  the prompt is a plan-review prompt. `--change` and `--base` are mutually exclusive. The run
  records the change, so `agk review send` rebuilds the same plan scope.
- An eval suite for the `agk-propose` skill, run with `claude plugin eval` against stand-ins for
  `agk` and `revdiff`, so the skill's loop is tested like code (one case per spec scenario).
- `agk setup` reports `revdiff` as an optional dependency.
- `agk-review` skill and `AGENTS.md` document the `--change` mode; the user's global workflow
  (`~/.claude/CLAUDE.md`, outside this repo) is pointed at `/agk-propose` for the plan step.

## Capabilities

### New Capabilities

- `propose-workflow`: the `agk-propose` skill's plan loop (artifacts, plan review, user pass in
  revdiff, nothing unreviewed closed, stop before apply) and the eval suite that checks it.

### Modified Capabilities

- `review-companion`: adds the plan review mode (`--change`): its scope, prompt and target checks;
  the diff-only requirements (git scope, run directory, re-review round) are narrowed to the run's
  recorded target.
- `installation`: the dependency report adds `revdiff` as optional.

## Impact

- Code: `internal/review` (plan scope collection, plan prompt), `internal/cli/review.go` (`--change`
  flag, run state, send), `internal/setup/deps.go`, `assets/` (plan review prompt).
- Skills: new `skills/agk-propose/SKILL.md`; `skills/agk-review/SKILL.md` gains the `--change` mode.
- Evals: new `evals/` suite with stand-in `agk` and `revdiff` scripts; runs real Claude children,
  so it costs API usage and is not part of `go test`.
- Docs: `AGENTS.md` usage. The user's `~/.claude/CLAUDE.md` changes outside the repo.
- No change to the review output schema, companion lifecycle, apply worker or config format.
- Not covered: OpenSpec stores (`--store`); the plan scope assumes `openspec/` at the git root.
