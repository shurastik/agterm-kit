# Spec Delta

## MODIFIED Requirements

### Requirement: Handoff to agk-propose
When the exploration is ready to become a change, or a change is captured, the skill SHALL name `/agk-propose` as the only next step, whatever the project's `openspec-explore` workflow suggests. It SHALL NOT name any other way to start the proposal or the implementation, such as the `openspec-propose` or `openspec-apply-change` skill. It SHALL NOT start `/agk-propose` itself.

#### Scenario: Idea settled
- **GIVEN** an `/agk-explore` conversation where the user says the idea is settled and asks what comes next, and the project's `openspec-explore` workflow suggests the `openspec-propose` skill and names `openspec-apply-change`
- **WHEN** the agent answers
- **THEN** its last message names `/agk-propose` and contains neither `openspec-propose` nor `openspec-apply-change`, and the agent did not run `agk review start`
