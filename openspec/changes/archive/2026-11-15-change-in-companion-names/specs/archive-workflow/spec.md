# Spec Delta

## MODIFIED Requirements

### Requirement: Review fallback
When the session cannot confirm the review gate, and no finding of the change's latest round is unresolved, the skill SHALL run a diff review with `agk review start --base <base> --for <change>` and wait for it. If the round approves and `agk review fresh` then reports `fresh: true` for it, the gate passes. A `fresh: false` result or an error SHALL stop the skill with nothing archived.

#### Scenario: Fresh session, review approves
- **GIVEN** a fresh session and a diff review that approves
- **WHEN** the user invokes `/agk-archive add-widget`
- **THEN** `agk review start --base <base> --for add-widget` and `agk review wait` run before verify and before the archive

#### Scenario: Code changed during the fallback review
- **GIVEN** a fresh session whose fallback review approves, but `agk review fresh` then reports `fresh: false` because a file changed while the round ran
- **WHEN** the agent handles the outcome
- **THEN** it reports that the code changed during the review, and runs no verify and no archive
