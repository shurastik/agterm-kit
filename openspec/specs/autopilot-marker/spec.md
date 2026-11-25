# autopilot-marker Specification

## Purpose
Marks the agterm sidebar rows of an autopilot run, so the user sees at a glance which session the
autopilot is driving right now and which companions an autopilot started.

## Requirements

### Requirement: Autopilot line on
Inside agterm, `agk autopilot on` SHALL set the info line of the caller's session (`AGTERM_SESSION_ID`) to the autopilot line, a fixed text starting with `🤖 autopilot`, set to expire after 1200 seconds, and SHALL record the session's autopilot as live until that moment. It SHALL print `{"session": "<id>", "marked": true}` and exit 0. Running it again SHALL set the line again and restart both the expiry and the live period.

#### Scenario: Line set on the main session
- **GIVEN** the agent runs in agterm session S
- **WHEN** it runs `agk autopilot on`
- **THEN** S's info line is the autopilot line with an expiry of 1200 seconds, stdout is `{"session":"S","marked":true}` and the exit code is 0

#### Scenario: Second on restarts the expiry
- **GIVEN** `agk autopilot on` ran in session S ten minutes ago
- **WHEN** it runs again in S
- **THEN** S's line is set again with an expiry of 1200 seconds and the autopilot of S counts as live for 1200 seconds from now

#### Scenario: No session is renamed or tinted
- **WHEN** `agk autopilot on` runs in session S
- **THEN** S keeps its name and its tint

### Requirement: Autopilot line off
Inside agterm, `agk autopilot off` SHALL clear the info line of the caller's session and end its live autopilot. It SHALL print `{"session": "<id>", "marked": false}` and exit 0, also when no autopilot was on. It SHALL NOT rename, tint or close any companion.

#### Scenario: Line cleared
- **GIVEN** `agk autopilot on` ran in session S
- **WHEN** the agent runs `agk autopilot off` in S
- **THEN** S's info line is cleared, stdout is `{"session":"S","marked":false}`, the exit code is 0, and a later refresh of S does not set the line again

#### Scenario: Off without on
- **GIVEN** no autopilot was turned on in session S
- **WHEN** the agent runs `agk autopilot off` in S
- **THEN** S's info line is cleared and the exit code is 0

#### Scenario: Companions keep their marks
- **GIVEN** session S has the companion `🤖 add-auth · claude-apply r2` of an autopilot run
- **WHEN** the agent runs `agk autopilot off` in S
- **THEN** the companion keeps its name and its tint

### Requirement: Autopilot command outside agterm
Outside agterm (`AGTERM_ENABLED` or `AGTERM_SESSION_ID` unset), `agk autopilot on` and `agk autopilot off` SHALL call no agterm command, record nothing, print `{"session": null, "marked": false}` and exit 0.

#### Scenario: Inline session
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the agent runs `agk autopilot on`
- **THEN** agk calls no agterm command, stdout is `{"session":null,"marked":false}` and the exit code is 0

### Requirement: Autopilot command usage
`agk autopilot` SHALL take exactly one argument, `on` or `off`. Any other argument list SHALL print a usage message to stderr and exit 1, and `agk autopilot --help` SHALL print the usage to stdout and exit 0.

#### Scenario: Unknown argument
- **WHEN** the agent runs `agk autopilot start`
- **THEN** stderr carries the usage of `agk autopilot on|off`, nothing changes and the exit code is 1

### Requirement: Marks are best effort
A mark that agterm refuses (setting or clearing the line, renaming or tinting a companion) SHALL be a warning on stderr and SHALL NOT change a command's other output or its exit code. `agk autopilot on` whose line agterm refuses SHALL print `"marked": false`.

#### Scenario: Line refused
- **GIVEN** agterm refuses the info line of session S
- **WHEN** the agent runs `agk autopilot on` in S
- **THEN** stderr carries a warning, stdout is `{"session":"S","marked":false}` and the exit code is 0

#### Scenario: Tint refused
- **GIVEN** agterm refuses the tint of a new companion
- **WHEN** the agent runs `agk apply start add-auth --autopilot`
- **THEN** stderr carries a warning and stdout and the exit code are those of a start whose tint succeeded

### Requirement: Autopilot runs of the workers
`agk apply start` and `agk review start` SHALL accept `--autopilot`. A run started with it SHALL record in its `worker.json` that it is an autopilot run and, inside agterm, the caller's session as the session its autopilot line belongs to. A run started without it SHALL record neither and SHALL get no autopilot mark.

#### Scenario: Flag recorded
- **GIVEN** the agent runs in agterm session S
- **WHEN** it runs `agk review start --base main --for add-auth --autopilot`
- **THEN** the run's `worker.json` records an autopilot run with session S

#### Scenario: Inline autopilot run
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the agent runs `agk apply start add-auth --autopilot`
- **THEN** the run's `worker.json` records an autopilot run without a session, the applier runs inline and agk calls no agterm command

#### Scenario: Run without the flag
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the run's `worker.json` records no autopilot run, and its companion is named and tinted as without this capability

### Requirement: Companion mark of an autopilot run
Inside agterm, the companion of an autopilot run SHALL carry the name the run would get without `--autopilot`, prefixed with `🤖 `, from its creation on and after every per-round rename, and its sidebar icon SHALL be tinted purple right after it is created. The companion SHALL keep both marks until it is closed.

#### Scenario: Applier companion marked
- **GIVEN** the applier resolves to `claude`
- **WHEN** the agent in session S runs `agk apply start add-auth --autopilot`
- **THEN** the companion under S is named `🤖 add-auth · claude-apply r1` and its icon is tinted purple

#### Scenario: Plan reviewer marked
- **GIVEN** the reviewer resolves to `codex`
- **WHEN** the agent in session S runs `agk review start --change add-auth --autopilot`
- **THEN** the companion under S is named `🤖 add-auth · codex-plan r1` and its icon is tinted purple

#### Scenario: Rename keeps the prefix
- **GIVEN** autopilot review run D with companion `🤖 add-auth · codex-review r1` whose round 1 completed
- **WHEN** the agent runs `agk review send D`
- **THEN** after the line for round 2 is typed, the companion is named `🤖 add-auth · codex-review r2`

### Requirement: Autopilot line refreshed by its runs
While the autopilot of the session recorded by an autopilot run is live, `agk apply wait`, `agk review wait`, `agk apply send` and `agk review send` on that run SHALL set the session's autopilot line again with a fresh 1200-second expiry and extend the live period to match: once when the command begins, and at least once per 60 seconds while it waits. Once the autopilot is no longer live (turned off, or its period ran out), they SHALL NOT set the line.

#### Scenario: Long applier round keeps the line
- **GIVEN** `agk autopilot on` ran in session S and autopilot apply run D of S is in a round that lasts 2 hours
- **WHEN** the agent runs `agk apply wait D`
- **THEN** S's autopilot line is set again at least once per 60 seconds for as long as the wait runs, so it never expires during the round

#### Scenario: Send refreshes the line
- **GIVEN** the autopilot of session S is live and D is an autopilot review run of S
- **WHEN** the agent runs `agk review send D`
- **THEN** S's autopilot line is set again with a 1200-second expiry

#### Scenario: Long send keeps the line
- **GIVEN** the autopilot of session S is live and the applier of autopilot apply run D of S stays busy for 50 minutes
- **WHEN** the agent runs `agk apply send D notes.md --timeout 3600`
- **THEN** S's autopilot line is set again at least once per 60 seconds while the send waits for the turn to end

#### Scenario: Wait after off
- **GIVEN** `agk autopilot off` ran in session S, and D is an autopilot apply run of S whose companion is still open
- **WHEN** the user runs `agk apply wait D`
- **THEN** S's info line is not set

#### Scenario: Crashed autopilot does not light up again
- **GIVEN** the autopilot of session S stopped without `agk autopilot off` more than 1200 seconds ago, so its line has expired
- **WHEN** the user runs `agk apply send D notes.md` on S's autopilot run D
- **THEN** S's info line is not set

#### Scenario: Run without the flag refreshes nothing
- **GIVEN** the autopilot of session S is live and D is an apply run started without `--autopilot`
- **WHEN** the agent runs `agk apply wait D`
- **THEN** agk sets no info line on S

#### Scenario: Refresh refused
- **GIVEN** agterm refuses S's info line during `agk review wait D`
- **WHEN** the review round completes
- **THEN** stderr carries a warning and stdout and the exit code are those of a wait without the refusal

### Requirement: Autopilot state changes do not interleave
`agk autopilot on`, `agk autopilot off` and a refresh of the same session SHALL NOT interleave: a refresh SHALL NOT set the line or extend the live period after an `off` of that session returned.

#### Scenario: Off during a refresh
- **GIVEN** a background `agk review wait D` of autopilot run D of session S has found the autopilot live and is setting S's line
- **WHEN** the agent runs `agk autopilot off` in S at that moment
- **THEN** once `off` has returned, S's info line is cleared, the autopilot of S is not live, and the wait sets the line no more
