# Spec Delta

## MODIFIED Requirements

### Requirement: Companion session inside agterm
When `AGTERM_ENABLED` and `AGTERM_SESSION_ID` are set, `agk review start` SHALL create a terminal companion of the current session, named after round 1. The companion SHALL be named:
- `<change> · <harness>-plan r1` for a plan review of `<change>`;
- `<change> · <harness>-review r1` for a diff review started with `--for <change>`;
- `<harness>-review r1` for a diff review without `--for`.

A run started with `--autopilot` SHALL prefix that name with `🤖 ` (see the `autopilot-marker` capability). The companion is created in the background without changing the selected session. It SHALL run the reviewer harness interactively, and round 1 SHALL be started by a single line telling the reviewer to read `rounds/1/prompt.md`. The command SHALL print one JSON object `{"companion": "<id>", "dir": "<run dir>", "started": <bool>}` on stdout and exit 0 without waiting for the review.

#### Scenario: Companion appears under the agent session
- **GIVEN** the agent runs inside agterm session S and the reviewer resolves to `codex`
- **WHEN** it runs `agk review start --base main`
- **THEN** a session named `codex-review r1` appears as a companion under S, S stays selected, the companion runs an interactive codex session whose first prompt points at `rounds/1/prompt.md`, and stdout is a JSON object with the companion id, the absolute run directory and `started`

#### Scenario: Diff review of a change
- **GIVEN** the agent runs inside agterm session S and the reviewer resolves to `codex`
- **WHEN** it runs `agk review start --base main --for add-auth`
- **THEN** the companion under S is named `add-auth · codex-review r1`

#### Scenario: Plan review of a change
- **GIVEN** the agent runs inside agterm session S and the reviewer resolves to `claude`
- **WHEN** it runs `agk review start --change add-auth`
- **THEN** the companion under S is named `add-auth · claude-plan r1`

#### Scenario: Autopilot diff review
- **GIVEN** the agent runs inside agterm session S and the reviewer resolves to `codex`
- **WHEN** it runs `agk review start --base main --for add-auth --autopilot`
- **THEN** the companion under S is named `🤖 add-auth · codex-review r1`

#### Scenario: No run number in the name
- **GIVEN** a repository with seven earlier run directories under `.agk/reviews/`
- **WHEN** a codex review is started with `agk review start --base main`
- **THEN** the companion is named `codex-review r1`, with no `#8` or other run number

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

### Requirement: Reviewer companion renamed for each round
Once `agk review send` has typed round k+1 into the reviewer companion, it SHALL rename the companion to the name `agk review start` gives it with `r<k+1>` in place of `r1`, the `🤖 ` prefix of an autopilot run included. A failed rename SHALL be a warning on stderr and SHALL NOT change the command's stdout or exit code. A round that was not typed (busy, permission prompt, gone) SHALL NOT rename it.

#### Scenario: Second round renames the companion
- **GIVEN** review run D of `add-auth` with companion `add-auth · codex-review r1` whose round 1 completed
- **WHEN** the agent runs `agk review send D`
- **THEN** after the line for round 2 is typed, the companion is named `add-auth · codex-review r2`

#### Scenario: Plan review renamed
- **GIVEN** plan review run D of `add-x` with companion `add-x · claude-plan r2` whose round 2 completed
- **WHEN** the agent runs `agk review send D`
- **THEN** the companion is named `add-x · claude-plan r3`

#### Scenario: Autopilot plan review renamed
- **GIVEN** autopilot plan review run D of `add-x` with companion `🤖 add-x · claude-plan r2` whose round 2 completed
- **WHEN** the agent runs `agk review send D`
- **THEN** the companion is named `🤖 add-x · claude-plan r3`

#### Scenario: Unconfirmed round still renames
- **GIVEN** the reviewer of D does not report round 2 as started after the line is typed
- **WHEN** `agk review send D` prints `"started": false`
- **THEN** the companion is named after round 2, because round 2 stays recorded

#### Scenario: Rename fails
- **GIVEN** agterm refuses the rename of D's companion
- **WHEN** the agent runs `agk review send D` and round 2 is typed and starts
- **THEN** stderr carries a warning naming the rename failure, stdout carries `"round": 2` and `"started": true`, and the exit code is 0

#### Scenario: Reviewer busy keeps the name
- **GIVEN** the reviewer of D is mid-turn
- **WHEN** `agk review send D --timeout 30` gives up after 30 seconds
- **THEN** no rename is attempted and the companion keeps its name

#### Scenario: Inline review has nothing to rename
- **GIVEN** a review run D with no companion
- **WHEN** the agent runs `agk review send D`
- **THEN** agk calls no agterm rename
