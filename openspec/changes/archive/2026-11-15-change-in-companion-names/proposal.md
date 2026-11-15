# Proposal

## Why

The companions of different changes look the same in the agterm sidebar. Today they are named
`claude-apply #3`, `codex-review #8` or `codex-review #9`, so with two changes in flight, or a plan
review next to a code review, the user cannot tell which companion belongs to which change without
opening it. The change name is known for every applier and plan review, and the agent that starts a
code review knows it too, but none of them puts it in the companion's name.

## What Changes

- The applier companion is named `<change> · <harness>-apply #N` instead of `<harness>-apply #N`.
- A plan review (`agk review start --change <change>`) names its companion
  `<change> · <harness>-plan #N`, so it no longer looks the same as a code review.
- New optional flag `agk review start --for <change>` for a diff review. It names the change the diff
  implements and does not change what is reviewed. The run records it, and the companion is named
  `<change> · <harness>-review #N`. Without `--for` a diff review keeps the name
  `<harness>-review #N`.
- `--for` takes a kebab-case change name. It is refused together with `--change`, since a plan
  review already names its change.
- The `agk-review` skill passes `--for <change>` when the diff is the implementation of an OpenSpec
  change. The `agk-archive` skill passes it in its fallback review. Skill text that names companions
  shows the new names.
- The main agent session is not renamed: agk names only the companions it creates.
- README documents `--for` and the companion names.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `apply-worker`: the applier companion's name carries the change.
- `review-companion`: the companion name depends on the review kind and carries the change. `--for`
  is new: it is checked and recorded in the run.
- `archive-workflow`: the fallback diff review passes `--for <change>`.

## Impact

- `internal/cli/apply.go`, `internal/cli/review.go`: companion names; flag parsing, validation and
  recording of `--for` (`reviewState` gains a `for` field in `worker.json`). Run directories from
  older agk have no `for` field and behave as before.
- `skills/agk-review/SKILL.md`, `skills/agk-archive/SKILL.md`, `skills/agk-apply/SKILL.md`: commands
  and companion names.
- `evals/_fixtures/bin/agk`: the stand-in agk takes `--for` explicitly.
- `README.md`: the `--for` flag and the companion names.
- The archived change `implementation-worktree` also modified the apply-worker requirement
  "Applier companion inside agterm". This change's delta is based on the archived text.
