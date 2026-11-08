# explore-workflow Specification

## Purpose
The `agk-explore` agent skill: the brainstorm step of the agk workflow. It runs the project's
OpenSpec explore workflow and hands a settled idea to `/agk-propose`, so the plan goes through the
plan review. Eval cases in the skill eval suite check this behavior.

## Requirements

### Requirement: Explicit invocation
The `agk-explore` skill SHALL run only when invoked by name (`/agk-explore`, "agk explore"). A plain request to explore or think through an idea SHALL NOT start it.

#### Scenario: Plain explore request
- **WHEN** the user asks the agent to think through an idea without naming `agk-explore`
- **THEN** the agent does not invoke the `agk-explore` skill

### Requirement: Precondition
Before exploring, the skill SHALL check that the project has the `openspec-explore` skill. When the skill is missing, it SHALL stop with setup advice.

#### Scenario: Project without OpenSpec skills
- **GIVEN** a repository with no `openspec-explore` skill
- **WHEN** the user invokes `/agk-explore should widgets be filterable`
- **THEN** the agent tells the user to run `openspec init` or `openspec update`, and creates no change

### Requirement: Explore through the project's workflow
The skill SHALL follow the project's `openspec-explore` skill for the whole exploration, with all of its guardrails: it writes no code, and it captures change artifacts only with the user's confirmation.

#### Scenario: Exploration writes no code
- **WHEN** the user invokes `/agk-explore` with an idea for a new command
- **THEN** the agent writes no files outside `openspec/changes/`, and runs no `agk` command

### Requirement: Handoff to agk-propose
When the exploration is ready to become a change, or a change is captured, the skill SHALL name `/agk-propose` as the next step instead of `/opsx:propose`. It SHALL NOT start `/agk-propose` itself.

#### Scenario: Idea settled
- **GIVEN** an `/agk-explore` conversation where the user says the idea is settled and asks what comes next
- **WHEN** the agent answers
- **THEN** its last message names `/agk-propose` and does not name `/opsx:propose`, and the agent did not run `agk review start`

### Requirement: Eval cases
The skill eval suite SHALL hold at least one case for each scenario of this capability. The cases SHALL run against the stand-in `agk` and a stub `openspec-explore` skill, never a real reviewer, applier or agterm companion.

#### Scenario: Explore cases run against stand-ins
- **WHEN** the developer runs the documented eval command
- **THEN** each `agk-explore` case runs and is scored, and none starts a real codex, claude reviewer or agterm companion
