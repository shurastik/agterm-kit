# Spec Delta

## MODIFIED Requirements

### Requirement: Companion session inside agterm
When `AGTERM_ENABLED` and `AGTERM_SESSION_ID` are set, `agk review start` SHALL create a terminal companion of the current session, where N is the review number. The companion SHALL be named:
- `<change> · <harness>-plan #<N>` for a plan review of `<change>`;
- `<change> · <harness>-review #<N>` for a diff review started with `--for <change>`;
- `<harness>-review #<N>` for a diff review without `--for`.

The companion is created in the background without changing the selected session. It SHALL run the reviewer harness interactively, and round 1 SHALL be started by a single line telling the reviewer to read `rounds/1/prompt.md`. The command SHALL print one JSON object `{"companion": "<id>", "dir": "<run dir>", "started": <bool>}` on stdout and exit 0 without waiting for the review.

#### Scenario: Companion appears under the agent session
- **GIVEN** the agent runs inside agterm session S and the reviewer resolves to `codex`
- **WHEN** it runs `agk review start --base main`
- **THEN** a session named `codex-review #<N>` appears as a companion under S, S stays selected, the companion runs an interactive codex session whose first prompt points at `rounds/1/prompt.md`, and stdout is a JSON object with the companion id, the absolute run directory and `started`

#### Scenario: Diff review of a change
- **GIVEN** the agent runs inside agterm session S and the reviewer resolves to `codex`
- **WHEN** it runs `agk review start --base main --for add-auth`
- **THEN** the companion under S is named `add-auth · codex-review #<N>`

#### Scenario: Plan review of a change
- **GIVEN** the agent runs inside agterm session S and the reviewer resolves to `claude`
- **WHEN** it runs `agk review start --change add-auth`
- **THEN** the companion under S is named `add-auth · claude-plan #<N>`

#### Scenario: Agent session keeps its name
- **GIVEN** the agent runs in agterm session S
- **WHEN** it runs `agk review start --change add-auth`
- **THEN** agk renames no session: S keeps its name

#### Scenario: Companion stays visible after the review
- **WHEN** a round in the companion finishes, successfully or not
- **THEN** the companion session stays open showing the reviewer's session until `agk review close`, the user dismisses it, or its parent closes

#### Scenario: Companion id cannot be recorded
- **GIVEN** the `companion` file cannot be written to the run directory
- **WHEN** `agk review start` has created the companion
- **THEN** the command closes that companion, prints an error to stderr and exits non-zero

## ADDED Requirements

### Requirement: Change of a diff review
`agk review start` SHALL accept `--for <change>` for a diff review: the OpenSpec change the diff implements. It SHALL record the change in the run's state and SHALL NOT change the review scope or the prompt. The name SHALL be kebab-case. The directory `openspec/changes/<change>/` does not have to exist. A run without the field, also one from an older agk, is a diff review with no change.

#### Scenario: Change is recorded
- **WHEN** the agent runs `agk review start --base main --for add-auth`
- **THEN** the run records base `main` and the change `add-auth` the diff is for, and records no plan review change

#### Scenario: Scope does not change
- **GIVEN** the same working tree and base
- **WHEN** one review starts with `--for add-auth` and another without it
- **THEN** both rounds have the same scope fingerprint, and their prompts differ only in the paths of their own run directories (the review schema and the output file), each pointing at its own run

#### Scenario: Default base
- **WHEN** the agent runs `agk review start --for add-auth`
- **THEN** it is a diff review against `main` for change `add-auth`

#### Scenario: Recorded outside agterm
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the agent runs `agk review start --base main --for add-auth`
- **THEN** the review runs inline and its run records the change `add-auth`

#### Scenario: Invalid change name
- **WHEN** the agent runs `agk review start --for ../secrets`
- **THEN** the command exits 1 rejecting the name, and no run directory or companion is created

#### Scenario: Together with a plan review
- **WHEN** the agent runs `agk review start --change add-auth --for add-auth`
- **THEN** the command exits 1 saying `--for` and `--change` exclude each other, and no run directory or companion is created
