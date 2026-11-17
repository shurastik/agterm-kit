# Spec Delta

## MODIFIED Requirements

### Requirement: Stop before implementation
After closing the reviewer the skill SHALL report the change, the review rounds, the final verdict and any findings set aside, name `/agk-apply <change>` as the only next step, and stop without running `agk apply`, invoking `/agk-apply` or editing project code. It SHALL NOT name the `openspec-apply-change` skill, "ask me to apply" or `agk apply start`, also when the project's `openspec-propose` workflow suggests them.

#### Scenario: Plan done
- **WHEN** the reviewer is closed
- **THEN** the agent's last message names the change and `/agk-apply <change>` as the next step, and no `agk apply` command was run

#### Scenario: Applier skill available
- **GIVEN** the `agk-apply` skill is available in the session
- **WHEN** the reviewer is closed
- **THEN** the agent does not invoke the `agk-apply` skill

#### Scenario: Propose workflow suggests another implementation step
- **GIVEN** the project's `openspec-propose` workflow ends by suggesting "use the `openspec-apply-change` skill or ask me to apply this change"
- **WHEN** the reviewer is closed
- **THEN** the agent's last message contains none of `openspec-apply-change`, "ask me to apply" and `agk apply start`
