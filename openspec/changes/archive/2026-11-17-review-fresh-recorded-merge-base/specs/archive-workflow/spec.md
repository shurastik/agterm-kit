## MODIFIED Requirements

### Requirement: Fallback review without an approve
When the fallback review round has findings, the skill SHALL report them, leave the reviewer open and archive nothing. When `agk review start` answers `nothing to review` outside a change worktree, and `agk review fresh` on this session's own diff review run for the change printed a `merge_base` different from the base just tried, the skill SHALL run the fallback once more with `--base <merge_base>`. When `nothing to review` remains, in a change worktree, or without such a `merge_base`, the skill SHALL ask the user whether to archive without a code review or which base to review against, and SHALL archive nothing until the user answers.

#### Scenario: Fallback review has findings
- **GIVEN** a fresh session and a diff review whose round returns a finding
- **WHEN** the agent handles the outcome
- **THEN** it reports the finding, runs no verify and no archive, and does not close that reviewer

#### Scenario: Nothing to review
- **GIVEN** a fresh session where `agk review start --base main` exits 1 with `nothing to review`
- **WHEN** the agent handles the outcome
- **THEN** it asks the user whether to archive without a code review or which base to review against, and archives nothing until the user answers

#### Scenario: Nothing to review against the base, retried at the recorded merge base
- **GIVEN** a session on `main` whose diff review run for the change approved at merge base `M`, after which the code was edited and everything was committed on `main`, so `agk review fresh` printed `"fresh":false` with `"merge_base":"<M>"` and `agk review start --base main` exits 1 with `nothing to review`
- **WHEN** the agent handles the outcome
- **THEN** it runs `agk review start --base <M> --for <change>` and waits for that round, without asking the user for a base

#### Scenario: Retry at the merge base also has nothing to review
- **GIVEN** a fallback whose `agk review start --base <merge_base> --for <change>` also exits 1 with `nothing to review`
- **WHEN** the agent handles the outcome
- **THEN** it asks the user whether to archive without a code review or which base to review against, and archives nothing until the user answers
