## MODIFIED Requirements

### Requirement: Companion session inside agterm
When `AGTERM_ENABLED` and `AGTERM_SESSION_ID` are set, `agk review start` SHALL create a terminal companion of the current session, named after round 1. The companion SHALL be named:
- `<change> · <harness>-plan r1` for a plan review of `<change>`;
- `<change> · <harness>-review r1` for a diff review started with `--for <change>`;
- `<harness>-review r1` for a diff review without `--for`.

The companion is created in the background without changing the selected session. It SHALL run the reviewer harness interactively, and round 1 SHALL be started by a single line telling the reviewer to read `rounds/1/prompt.md`. The command SHALL print one JSON object `{"companion": "<id>", "dir": "<run dir>", "started": <bool>}` on stdout and exit 0 without waiting for the review.

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

### Requirement: Progress and outcome status
Inside agterm, the companion's agent status SHALL come only from the reviewer harness's turns: `active` while a turn runs, `completed` or `idle` when it ends, `blocked` while the reviewer waits for a permission answer. agk SHALL NOT set it. agk SHALL set the info line instead:
- `<harness> r<k>: reviewing` when it starts or sends round k;
- a line counting findings by severity when `agk review wait` records a completed round;
- a line stating the failure reason when it records a failed round.

#### Scenario: Findings reported
- **WHEN** `agk review wait` records round 1 of a codex review with 3 findings, one of them `high`
- **THEN** the companion's info line reads `codex r1: 3 findings (1 high)` and its agent status is whatever the reviewer's turn reports

#### Scenario: Clean review
- **WHEN** `agk review wait` records round 2 of a codex review with verdict `approve` and no findings
- **THEN** the companion's info line reads `codex r2: approve, no findings`

#### Scenario: Codex fails
- **WHEN** `agk review wait` records round 1 of a codex review as failed with reason `invalid review output`
- **THEN** the companion's info line names the failure, its agent status is not set to `blocked` by agk, no `review.json` is written for the round, and the reviewer session stays open for the next round

#### Scenario: Agent waits for the outcome
- **GIVEN** a review started with `agk review start` in agterm
- **WHEN** the agent runs `agk review wait <dir>`
- **THEN** the wait returns once round 1 has an outcome or the reviewer needs the user, with the matching status

#### Scenario: Round in progress
- **WHEN** `agk review send` starts round 2 of a claude review
- **THEN** the companion's info line reads `claude r2: reviewing`

### Requirement: Timestamped review log
The inline review SHALL print a header line with the round, start date and time and run directory. It SHALL show the reviewer's progress as readable lines. Every output line, in the terminal and in the round's `reviewer.log`, SHALL carry the local date and time prefix `[YYYY-MM-DD HH:MM:SS]`. Inside agterm the companion SHALL show the harness's own interface instead.

#### Scenario: Log lines carry date and time
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the reviewer reports `thinking` and then `done` during round 1 of a review started at 2026-11-05 17:53:55
- **THEN** the first line reads `[2026-11-05 17:53:55] agk: review round 1 started, run dir <dir>` and the progress is shown as `[<date> <time>] thinking` and `[<date> <time>] done`, both on screen and in `rounds/1/reviewer.log`

#### Scenario: Harness interface inside agterm
- **GIVEN** the agent runs inside agterm
- **WHEN** round 1 runs in the companion
- **THEN** the companion shows the reviewer harness's interactive interface, not an agk log, and `rounds/1/` has no `reviewer.log`

### Requirement: Reviewer companion waits between rounds
After each round, successful or failed, the reviewer session SHALL stay open in its companion, keeping the round visible, until `agk review send` types the next round into it or the companion is closed.

#### Scenario: Request arrives
- **GIVEN** companion C of D finished round 1
- **WHEN** the agent runs `agk review send D`
- **THEN** one line is typed into C, C's status becomes `active`, its info line reads `<harness> r2: reviewing`, and round 2 runs in the same reviewer session

## ADDED Requirements

### Requirement: Reviewer companion renamed for each round
Once `agk review send` has typed round k+1 into the reviewer companion, it SHALL rename the companion to the name `agk review start` gives it with `r<k+1>` in place of `r1`. A failed rename SHALL be a warning on stderr and SHALL NOT change the command's stdout or exit code. A round that was not typed (busy, permission prompt, gone) SHALL NOT rename it.

#### Scenario: Second round renames the companion
- **GIVEN** review run D of `add-auth` with companion `add-auth · codex-review r1` whose round 1 completed
- **WHEN** the agent runs `agk review send D`
- **THEN** after the line for round 2 is typed, the companion is named `add-auth · codex-review r2`

#### Scenario: Plan review renamed
- **GIVEN** plan review run D of `add-x` with companion `add-x · claude-plan r2` whose round 2 completed
- **WHEN** the agent runs `agk review send D`
- **THEN** the companion is named `add-x · claude-plan r3`

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

## REMOVED Requirements

### Requirement: Review numbering
**Reason**: The repository-wide run number read like a pass number and told the user nothing; the companion name and info lines carry the round instead.
**Migration**: None needed. Run directories keep their timestamp names; `agk review` commands take the run directory, never a number.
