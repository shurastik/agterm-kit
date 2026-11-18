# Proposal

## Why

The agk workflow (`/agk-propose` → `/agk-apply` → code review → verify → `/agk-archive`) stops
for the user at three to five points: the questions `openspec-propose` asks, a review finding the
agent disagrees with, a `needs-attention` verdict, an applier `blocked` report, the revdiff pass.
For a well-understood idea the user wants the whole run unattended, and afterwards a clear account
of what the model decided on their behalf, so they can judge the result before archiving.

## What Changes

- New agent skill `agk-autopilot` (`/agk-autopilot <idea>`, explicit invocation only). In one run it:
  writes the change with the project's `openspec-propose` workflow, runs the plan review loop
  (`agk review start --change`) until it approves, skips the revdiff pass, implements the change
  with the applier (`agk apply`), runs the code review loop (`agk review start --base`) with the
  findings going to the applier, runs `openspec-verify-change` until it reports 0 CRITICAL, and
  stops before `/agk-archive`.
- Every point where the existing skills would ask the user (a question of `openspec-propose`, a
  finding the agent disagrees with, `needs-attention`, applier `blocked`, verify CRITICAL or a
  WARNING it rejects, a change name collision) is decided by the model and recorded as an entry in
  `openspec/changes/<change>/decisions.md`, tagged `[scope]` or `[impl]`.
- A plan edit made during implementation (after applier `blocked`) goes through a plan review
  round before the applier gets it.
- The user is needed only when a worker needs a human (`needs-human`: a permission or trust prompt,
  a question in a companion), when a companion was closed (`cancelled`), or when a review loop
  exceeds its limit (5 rounds per plan reviewer, 5 rounds per code reviewer).
- The run ends with a report in chat: the `[scope]` decisions first, then `[impl]`, the review round
  counts, the verify result, the worktree path when there is one, and `/agk-archive <change>` as the
  only next step.
- Eval suite cases for `agk-autopilot`; the stand-in `agk` learns `apply start|wait|send` and
  diff-review outcomes so these cases can run without a real applier.
- README documents the skill.

## Capabilities

### New Capabilities

- `autopilot-workflow`: the `agk-autopilot` agent skill: the unattended run from idea to a verified
  change, its decision policy and decision log, its stop conditions and its final report, and the
  eval suite that checks it.

### Modified Capabilities

None. `agk setup` already links every `agk-*` skill directory it finds, and the existing skills
keep their behavior; `agk-autopilot` follows them with its own overrides.

## Impact

- New `skills/agk-autopilot/SKILL.md`; shipped with the other skills (`share/agk/skills/`), linked by
  `agk setup` with no code change.
- `evals/`: new cases and case plugins, the stand-in `agk` extended with the applier commands and
  diff-review rounds, an `openspec-verify-change` stub outcome per case, README table rows.
- `README.md`: the skill in the loop description and the quick start.
- No Go code change is expected.
