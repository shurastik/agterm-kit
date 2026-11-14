## MODIFIED Requirements

### Requirement: Re-review round
`agk review send <run-dir> [--note FILE] [--timeout SECONDS]` SHALL start round k+1 after round k has an outcome. It SHALL:
1. rebuild the run's scope from its recorded target (the diff against the recorded base, or the plan scope of the recorded change);
2. write a re-review prompt to `rounds/<k+1>/`, including the note when given;
3. record round k+1;
4. hand the round to the reviewer.

Inside agterm, the round reaches the live reviewer session as one typed line, once the session's turn is over (waiting up to `--timeout`, default 600 seconds), and the command SHALL print `{"companion": "<id>", "dir": "<run dir>", "round": k+1, "started": <bool>}`, where `started` tells whether the reviewer reported the round as started. Inline, the reviewer resumes its harness session and the command SHALL print `{"companion": null, "dir": "<run dir>", "round": k+1}`.

#### Scenario: Second round remembers the first
- **GIVEN** review run D in agterm whose round 1 completed with 2 findings
- **WHEN** the agent runs `agk review send D --note fixed.md`
- **THEN** one line pointing at `rounds/2/prompt.md` is typed into the same reviewer session, that prompt asks it to re-check the earlier findings, includes the note and names `out/2.json` as the output file, stdout carries `"round": 2` and `"started": true`, and the outcome lands in `rounds/2/`

#### Scenario: Round start not confirmed
- **GIVEN** review run D in agterm whose round 1 completed, and the reviewer does not report `active` after the line for round 2 is typed, even after the Enter retry
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 stays recorded, stderr carries a warning that the reviewer did not report round 2 as started and tells the user to look at the companion (press Enter if the line is still in its input box, answer a permission prompt deliberately), stdout carries `"round": 2` and `"started": false`, and the exit code is 0, so `agk review wait D` can still pick the round up once it starts

#### Scenario: Nothing left to review
- **GIVEN** a diff review run D and the diff against the recorded base is empty
- **WHEN** the agent runs `agk review send D`
- **THEN** it prints `nothing to review` to stderr, exits 1 and records no new round

#### Scenario: Plan re-review on a clean tree
- **GIVEN** a plan review run D of change `add-x`, whose files are committed, and a clean working tree
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 starts with the change's files in its prompt; the empty git diff does not matter

#### Scenario: Round still running
- **GIVEN** round 1 of D has no `result.json` yet
- **WHEN** the agent runs `agk review send D`
- **THEN** it exits 1 saying the round is still running and records no new round

#### Scenario: Reviewer still busy
- **GIVEN** round 1 of D has an outcome but the reviewer's turn has not ended
- **WHEN** the agent runs `agk review send D --timeout 30` and the turn does not end within 30 seconds
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the reviewer is busy

#### Scenario: Reviewer waits for a permission answer
- **GIVEN** the reviewer of D shows a permission prompt (agent status `blocked`)
- **WHEN** the agent runs `agk review send D`
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the reviewer waits for the user

#### Scenario: Round recorded mid-turn
- **GIVEN** `agk review wait D` recorded round 1 from a valid output file while the reviewer's turn was still running
- **WHEN** the agent runs `agk review send D` before that turn ends
- **THEN** the command waits for the turn to end before typing round 2, and with a too-short `--timeout` exits 1 saying the reviewer is busy

#### Scenario: Send after a failed round
- **GIVEN** round 1 of D was recorded as failed with reason `invalid review output` and the reviewer's turn is over
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 is typed into the session; the failure is not mistaken for a permission prompt

#### Scenario: No session to resume
- **GIVEN** an inline review run D whose round 1 failed before the reviewer reported a session id
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 starts a fresh reviewer session with the first-round prompt

#### Scenario: Re-review outside agterm
- **GIVEN** a review run D with no companion
- **WHEN** the agent runs `agk review send D`
- **THEN** round 2 runs in the foreground without calling agterm and stdout ends with the JSON object whose `companion` is `null` and which has no `started` field

## ADDED Requirements

### Requirement: Codex reviewer submits typed lines
The interactive codex reviewer SHALL run with codex's paste-burst detection off (`-c tui.disable_paste_burst=true`), so that a line typed into it together with its Enter is submitted rather than kept as a multi-line draft. The flag SHALL come after the configured model, effort and `extra_args`.

#### Scenario: Codex reviewer argv
- **GIVEN** the agent runs inside agterm and the reviewer resolves to `codex` with `extra_args = ["-c", "tui.disable_paste_burst=false"]`
- **WHEN** the agent runs `agk review start`
- **THEN** the companion's codex command line carries `-c tui.disable_paste_burst=true` after the user's `-c tui.disable_paste_burst=false`, so paste-burst detection is off

#### Scenario: Claude reviewer unchanged
- **GIVEN** the agent runs inside agterm and the reviewer resolves to `claude`
- **WHEN** the agent runs `agk review start`
- **THEN** the claude command line carries no paste-burst setting

### Requirement: Typed reviewer line is submitted
After typing a round or fix-up line into the reviewer, agk SHALL wait up to 10 seconds for `active`. If it does not come, the reviewer's status is still `completed` or `idle` and the typed line is visible on the companion's screen, agk SHALL type one Enter on its own and wait for `active` or `blocked` for the rest of the 30-second handshake. It SHALL type nothing more, and no Enter when the status is anything else or the line is not on screen.

#### Scenario: Enter lost, retry submits
- **GIVEN** review run D whose round 1 completed, and the reviewer keeps the typed line for round 2 in its input box with status `completed`
- **WHEN** the agent runs `agk review send D`
- **THEN** after 10 seconds without `active` agk reads the companion's screen, finds the line, types one Enter alone, the reviewer reports `active`, and stdout carries `"started": true`

#### Scenario: Permission prompt after typing
- **GIVEN** the reviewer's status turns `blocked` after the round line is typed
- **WHEN** the first 10-second wait ends without `active`
- **THEN** agk types no Enter, the send reports `"started": false`, and the stderr warning tells the user to answer the permission prompt in the companion deliberately, without advising to press Enter

#### Scenario: Permission prompt after the retry Enter
- **GIVEN** the retry Enter submits the round line and the reviewer turns `blocked` before reporting `active`
- **WHEN** the last wait sees `blocked`
- **THEN** the send reports `"started": false` and the warning tells the user to answer the permission prompt deliberately, without advising to press Enter

#### Scenario: Line not on screen
- **GIVEN** the reviewer's status stays `completed` after the round line is typed, but the line is not on the last screen lines of the companion
- **WHEN** the first 10-second wait ends without `active`
- **THEN** agk types no Enter and keeps waiting for `active` or `blocked` until the 30-second handshake ends; if neither comes, the warning tells the user to look at the companion, without advising to press Enter

#### Scenario: Fix-up line retried
- **GIVEN** `agk review wait D` types fix-up 1 for round 1 and the reviewer keeps it in its input box with status `completed`
- **WHEN** the first 10-second wait ends without `active`
- **THEN** agk types one Enter alone and, once the reviewer reports `active`, records fix-up 1 as started

#### Scenario: Retry bounded by the wait budget
- **GIVEN** `agk review wait D --timeout 5` types a fix-up line with less than 10 seconds of its budget left
- **WHEN** the budget runs out without `active`
- **THEN** agk types no Enter and does not wait past the budget
