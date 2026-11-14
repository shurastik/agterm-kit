## MODIFIED Requirements

### Requirement: Follow-up round
`agk apply send <run-dir> (<file> | --stdin) [--timeout SECONDS]` SHALL start round k+1 in the live applier session: wait (default 600 s) until its current turn is over, write `rounds/<k+1>/prompt.md` from the built-in fix template and the given message, record round k+1, type a single-line instruction to read that prompt into the session, and wait up to 30 seconds for the session to become `active`, warning on stderr when it does not. It SHALL print `{"round": k+1, "started": <bool>}`, where `started` tells whether the applier reported the round as started.

#### Scenario: Review findings sent to the applier
- **GIVEN** apply run D finished round 1 with `done`
- **WHEN** the agent runs `agk apply send D findings.md`
- **THEN** `rounds/2/prompt.md` contains the content of `findings.md` and the absolute path of `rounds/2/report.json`, the companion receives one line pointing at `rounds/2/prompt.md` followed by Enter, and stdout is `{"round":2,"started":true}`

#### Scenario: Round start not confirmed
- **GIVEN** apply run D finished round 1 and its applier does not report `active` after the line is typed, even after the Enter retry
- **WHEN** the agent runs `agk apply send D notes.md`
- **THEN** round 2 stays recorded, stderr carries a warning that the applier did not report round 2 as started and tells the user to look at the companion (press Enter if the line is still in its input box, answer a permission prompt deliberately), stdout is `{"round":2,"started":false}` and the exit code is 0, so `agk apply wait D` can still finish the round through its report file

#### Scenario: Applier still busy
- **GIVEN** the applier of D is mid-turn
- **WHEN** the agent runs `agk apply send D notes.md --timeout 30` and the turn does not end within 30 seconds
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the applier is busy

#### Scenario: Applier waits for a permission answer
- **GIVEN** the applier of D shows a permission prompt (agent status `blocked`)
- **WHEN** the agent runs `agk apply send D notes.md`
- **THEN** nothing is typed into the companion, no round is recorded and the command exits 1 saying the applier waits for the user

#### Scenario: Applier gone
- **GIVEN** the companion of D was closed
- **WHEN** the agent runs `agk apply send D notes.md`
- **THEN** the command exits 1 saying the applier is gone and no round is recorded

#### Scenario: Follow-up round outside agterm
- **GIVEN** an apply run D with no companion
- **WHEN** the agent runs `agk apply send D notes.md`
- **THEN** round 2 runs headless in the foreground and stdout is `{"round":2}`, with no `started` field

## ADDED Requirements

### Requirement: Codex applier submits typed lines
The interactive codex applier SHALL run with codex's paste-burst detection off (`-c tui.disable_paste_burst=true`), so that a line typed into it together with its Enter is submitted rather than kept as a multi-line draft. The flag SHALL come after the configured model, effort and `extra_args`.

#### Scenario: Codex applier argv
- **GIVEN** the agent runs inside agterm and the applier resolves to `codex` with `extra_args = ["-c", "tui.disable_paste_burst=false"]`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the companion's codex command line carries `-c tui.disable_paste_burst=true` after the user's `-c tui.disable_paste_burst=false`, so paste-burst detection is off

#### Scenario: Claude applier unchanged
- **GIVEN** the agent runs inside agterm and the applier resolves to `claude`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the claude command line carries no paste-burst setting

### Requirement: Typed applier line is submitted
After typing a round line into the applier, agk SHALL wait up to 10 seconds for `active`. If it does not come, the applier's status is still `completed` or `idle` and the typed line is visible on the companion's screen, agk SHALL type one Enter on its own and wait for `active` or `blocked` for the rest of the 30-second handshake. It SHALL type nothing more, and no Enter when the status is anything else or the line is not on screen.

#### Scenario: Enter lost, retry submits
- **GIVEN** apply run D finished round 1, and the applier keeps the typed line for round 2 in its input box with status `completed`
- **WHEN** the agent runs `agk apply send D notes.md`
- **THEN** after 10 seconds without `active` agk reads the companion's screen, finds the line, types one Enter alone, the applier reports `active`, and stdout is `{"round":2,"started":true}`

#### Scenario: No retry without the line on screen
- **GIVEN** the applier's status stays `completed` after the round line is typed, but the line is not on the last screen lines of the companion
- **WHEN** the first 10-second wait ends without `active`
- **THEN** agk types no Enter, after the 30-second handshake stdout carries `"started": false`, and the stderr warning tells the user to look at the companion, without advising to press Enter
