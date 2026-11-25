# apply-worker Specification

## Purpose
Runs the applier, the worker that implements an OpenSpec change and later fixes review findings or user notes,
as an autonomous agent session in a visible agterm companion (or inline outside agterm) that the main session
can start, wait for, send follow-up rounds to and close.

## Requirements

### Requirement: Apply run directory
Each `agk apply start` SHALL create a new run directory `.agk/applies/<timestamp>/` under the root the applier works in. That root SHALL be the change's worktree in worktree mode or when started inside a linked worktree, and the repository root otherwise. The run directory SHALL hold the resolved worker configuration, the change name, the current round number, the companion id when one exists, and for every round k a `rounds/<k>/` directory with that round's `prompt.md` and, once the applier reports, `report.json`.

#### Scenario: Directory after start
- **GIVEN** a repository with two earlier apply runs
- **WHEN** the agent runs `agk apply start add-auth` inside agterm
- **THEN** a new, third run directory exists with the change name, round `1`, the companion id and `rounds/1/prompt.md`

#### Scenario: Directory in worktree mode
- **GIVEN** worktree mode and a new worktree `.worktrees/add-auth`
- **WHEN** the agent runs `agk apply start add-auth` from the main checkout
- **THEN** the run directory is created under `.worktrees/add-auth/.agk/applies/` and none under the main checkout's `.agk/applies/`

### Requirement: Change must be ready to apply
`agk apply start <change>` SHALL refuse a change name that is not kebab-case, and SHALL ask `openspec instructions apply --change <change> --json` for the change's state, refusing with the CLI's message when the change is missing or blocked and with `nothing to apply` when all tasks are done. A refusal SHALL exit 1 without creating a run directory or companion.

#### Scenario: All tasks done
- **GIVEN** change `add-auth` whose tasks are all checked
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** stderr says `nothing to apply`, the exit code is 1 and no run directory exists

#### Scenario: Unknown change
- **WHEN** the agent runs `agk apply start no-such-change`
- **THEN** the command exits 1 with openspec's error on stderr

### Requirement: Applier companion inside agterm
Inside agterm, `agk apply start` SHALL create a background companion of the current session named `<change> · <harness>-apply r1`, running the configured harness interactively in the root the applier works in, with the first round started by a single-line instruction to read `rounds/1/prompt.md`. It SHALL print `{"companion": "<id>", "dir": "<run dir>", "started": <bool>, "worktree": <path or null>, "pulled": <bool>}` and exit 0.

#### Scenario: Claude applier starts
- **GIVEN** the applier resolves to `claude` and worktree mode is off
- **WHEN** the agent in session S runs `agk apply start add-auth`
- **THEN** a companion `add-auth · claude-apply r1` appears under S, S stays selected, the companion runs an interactive claude session in the repository root whose first prompt points at `rounds/1/prompt.md`, and stdout carries the companion id, the run dir, `"started": true`, `"worktree": null` and `"pulled": false`

#### Scenario: Applier in the change worktree
- **GIVEN** worktree mode and the applier resolving to `claude`
- **WHEN** the agent in session S runs `agk apply start add-auth` from the main checkout
- **THEN** the companion is named `add-auth · claude-apply r1`, its working directory is `.worktrees/add-auth`, and stdout carries `"worktree"` with the absolute path of `.worktrees/add-auth`

#### Scenario: Agent session keeps its name
- **GIVEN** the agent runs in agterm session S
- **WHEN** it runs `agk apply start add-auth`
- **THEN** agk renames no session: S keeps its name, and only the new companion carries the change name

### Requirement: Start handshake
After creating the companion, `agk apply start` SHALL wait up to 60 seconds for the companion's agent status to become `active`, `completed` or `blocked`. If none arrives, it SHALL report `"started": false`, print to stderr a warning with the last lines of the companion's screen, and leave the companion open for the user.

#### Scenario: Folder trust prompt
- **GIVEN** claude shows its workspace-trust prompt in the companion and sets no status
- **WHEN** 60 seconds pass after the companion was created
- **THEN** stdout carries `"started": false`, stderr carries the warning with the screen lines showing the trust prompt, the exit code is 0 and the companion stays open

### Requirement: Applier instructions
The applier's prompts SHALL come from templates built into agk. The first round SHALL tell it to implement the pending tasks of the named change with `openspec instructions apply`, test first, run the project's gates as `AGENTS.md` describes, check off each finished task, never ask the user, and stop with a `blocked` report instead of guessing.

#### Scenario: First-round prompt content
- **WHEN** `agk apply start add-auth` writes `rounds/1/prompt.md`
- **THEN** it names change `add-auth`, the `openspec instructions apply --change add-auth --json` command, and the absolute path of `rounds/1/report.json` the applier must write

### Requirement: Applier guardrails
The applier SHALL be denied committing, pushing, tagging and rewriting git history: a claude applier through deny rules for those git commands added to its settings and its configured permission mode, a codex applier through a `workspace-write` sandbox with approvals set to `never`. The prompt SHALL also forbid commits, pushes, deploys and archiving.

#### Scenario: Claude applier tries to commit
- **GIVEN** a claude applier
- **WHEN** it attempts `git commit`
- **THEN** the command is denied by its settings and nothing is committed

#### Scenario: Codex applier flags
- **GIVEN** the applier resolves to `codex` with model `gpt-5.5`
- **WHEN** the companion is started
- **THEN** codex runs interactively with `--sandbox workspace-write`, `--ask-for-approval never`, model `gpt-5.5` and the repository root as working directory

### Requirement: Round report
At the end of every round the applier SHALL be instructed to write `rounds/<k>/report.json`: a JSON object with `status` `done` or `blocked` and `summary`; a blocked report SHALL also carry `problem` and `artifact` (`proposal`, `specs`, `design`, `tasks` or `finding`). agk SHALL treat any other content as an invalid report.

#### Scenario: Blocked on an ambiguous task
- **GIVEN** the applier stops because task 3.2 has two readings
- **WHEN** it ends the round
- **THEN** `rounds/1/report.json` has `status` `blocked`, `artifact` `tasks` and a `problem` naming both readings

### Requirement: Claude applier turn state
A claude applier SHALL have hooks in its settings that record its turn state in the run's `turn.json` with the round the turn belongs to: on a turn start, `running` with round k when the prompt is round k's kickoff line, else with the round already recorded (none: nothing); on a turn end, `stopped` with that round and the number of background tasks in flight and of scheduled wakeups. The hooks SHALL NOT set the agent status, print anything or fail the turn.

#### Scenario: Turn ends with a background shell in flight
- **GIVEN** a claude applier in round 1 runs a test suite as a background shell and ends its turn
- **WHEN** its `Stop` hook runs
- **THEN** `turn.json` records round 1, `stopped`, 1 background task and 0 wakeups

#### Scenario: Applier resumes by itself
- **GIVEN** `turn.json` records round 1, `stopped`, 1 background task
- **WHEN** the background shell finishes and claude starts a new turn with its task notification
- **THEN** `turn.json` records round 1, `running`

#### Scenario: Turn ends with nothing in flight
- **WHEN** the applier ends a turn of round k with no background task and no scheduled wakeup
- **THEN** `turn.json` records round k, `stopped`, 0 background tasks and 0 wakeups

#### Scenario: Turn of the previous round ends after the next round was published
- **GIVEN** the applier's round-1 turn is still running, `turn.json` records round 1, `running`, and `agk apply send` has already made round 2 the run's current round
- **WHEN** that turn's `Stop` hook runs
- **THEN** `turn.json` records round 1, `stopped`, and nothing is recorded for round 2

#### Scenario: Round 1 resumes while round 2 is being sent
- **GIVEN** `turn.json` records round 1, `stopped` with 1 background task, and the applier starts a turn with its task notification
- **WHEN** `agk apply send` makes round 2 the run's current round before that turn's start hook runs, the send then gives up because the turn is busy and puts the current round back to 1, and the turn ends with another background task in flight
- **THEN** `turn.json` records round 1 at every step, and `agk apply wait D` for round 1 keeps treating the applier as paused

#### Scenario: Round's kickoff line starts the turn
- **GIVEN** `turn.json` records round 1, `stopped` with nothing in flight
- **WHEN** the applier starts a turn whose prompt is the line `Read <run dir>/rounds/2/prompt.md and follow it.`
- **THEN** `turn.json` records round 2, `running`

#### Scenario: Line the user typed into the applier
- **GIVEN** `turn.json` records round 2, `stopped` with nothing in flight
- **WHEN** the user types a question into the companion and a turn starts
- **THEN** `turn.json` records round 2, `running`

#### Scenario: Turn end without a recorded start
- **GIVEN** the run has no `turn.json`
- **WHEN** a `Stop` hook runs
- **THEN** it records nothing

#### Scenario: Hook cannot record
- **GIVEN** the run directory is gone or the hook input is not valid JSON
- **WHEN** a turn-state hook runs
- **THEN** it writes nothing, prints nothing and exits 0, and the applier's turn goes on

#### Scenario: Codex applier
- **GIVEN** the applier resolves to `codex`
- **WHEN** `agk apply start` starts it
- **THEN** no turn-state hooks are installed and no turn state is recorded

#### Scenario: Run directory path that cannot go into a hook command
- **GIVEN** the applier resolves to `claude` and the run directory path contains a backslash or a line break
- **WHEN** `agk apply start` prepares the run
- **THEN** it exits 1 with an error naming the path and starts no companion

### Requirement: Wait for the applier
`agk apply wait <run-dir> [--timeout SECONDS]` (default 3600) SHALL print one JSON object with `status` and `round` and exit 0. The status SHALL be `done` or `blocked` from a valid report of the current round (with the report's fields), `needs-human` when the turn ended without a valid report or the agent asks for permission, `cancelled` when the companion is gone, or `running` on timeout. It SHALL NOT close the companion.

For a run whose applier records its turn state, only a turn state of the current round counts; one of an earlier round counts as missing. A recorded turn state of the current round SHALL count as the round having started. A turn that ended with background tasks in flight or a scheduled wakeup SHALL count as a pause, not as the end of the round: `agk apply wait` SHALL keep waiting until the report appears, the applier resumes and ends a turn, a permission prompt opens, the companion is gone or the timeout passes. A timeout during a pause SHALL print `running` with `reason` `background` and `background` set to the number of tasks and wakeups in flight. While the recorded turn state is `running`, a `completed` or `idle` status SHALL NOT end the wait. A turn state that is still missing after the round started and the status shows the turn over SHALL be waited for up to 30 seconds and then treated as a turn that ended with nothing in flight. Before printing `no-report`, the wait SHALL check the companion once more: gone gives `cancelled`, a permission prompt gives `permission`. Runs without recorded turn state SHALL keep deciding from the agent status alone.

#### Scenario: Round finished
- **GIVEN** the applier of run dir D wrote a valid `done` report for round 1
- **WHEN** the agent runs `agk apply wait D`
- **THEN** stdout is a JSON object with `status` `done`, `round` 1 and the report's `summary`, and the companion stays open

#### Scenario: Permission prompt
- **GIVEN** the applier's agent status becomes `blocked` before it wrote a report
- **WHEN** `agk apply wait D` is waiting
- **THEN** it prints `status` `needs-human` with `reason` `permission`

#### Scenario: Turn ended with a question
- **GIVEN** the applier's turn ends (status `completed` or `idle`), the current round has no report and its turn state records `stopped` with nothing in flight
- **WHEN** `agk apply wait D` sees it
- **THEN** it prints `status` `needs-human` with `reason` `no-report`

#### Scenario: Paused on a background task, then done
- **GIVEN** the applier's status is `completed`, round 1 has no report and its turn state records `stopped` with 1 background task
- **WHEN** `agk apply wait D` runs and the applier later resumes and writes a valid `done` report
- **THEN** the wait does not return before the report exists, and then prints `status` `done`

#### Scenario: Paused past the timeout
- **GIVEN** round 1 has no report and its turn state records `stopped` with 2 background tasks and 0 wakeups
- **WHEN** `agk apply wait D --timeout 60` runs and nothing changes for 60 seconds
- **THEN** it prints `{"status":"running","round":1,"reason":"background","background":2}`

#### Scenario: Resumed turn ends with a question
- **GIVEN** the applier was paused on a background task in round 1
- **WHEN** it resumes, ends that turn with nothing in flight and writes no report
- **THEN** `agk apply wait D` prints `status` `needs-human` with `reason` `no-report`

#### Scenario: Status shows the turn over before the hook ran
- **GIVEN** the applier's status turns `completed` while its turn state still records `running`
- **WHEN** the `Stop` hook then records `stopped` with nothing in flight and there is no report
- **THEN** `agk apply wait D` prints `needs-human` with `reason` `no-report` only after the turn state records `stopped`

#### Scenario: Turn state never recorded
- **GIVEN** a claude run with turn-state hooks whose round 2 has been seen active, has no report and no turn state of round 2
- **WHEN** the status shows the turn over and no turn state of round 2 appears within 30 seconds
- **THEN** `agk apply wait D` prints `needs-human` with `reason` `no-report`

#### Scenario: Previous round's turn state is not the current round's
- **GIVEN** the run is in round 2, `turn.json` records round 1, `stopped` with nothing in flight, and the status still shows round 1's turn over
- **WHEN** `agk apply wait D` runs
- **THEN** it does not print `no-report` on that turn state, and keeps waiting for round 2 to start

#### Scenario: Round started unseen, applier paused
- **GIVEN** `agk apply send` returned `"started": false` for round 2, the applier ran a short turn that was never seen `active`, and `turn.json` records round 2, `stopped` with 1 background task
- **WHEN** `agk apply wait D --timeout 60` runs and nothing changes
- **THEN** it prints `{"status":"running","round":2,"reason":"background","background":1}`

#### Scenario: Round started unseen, turn ended with a question
- **GIVEN** `agk apply send` returned `"started": false` for round 2, and `turn.json` records round 2, `stopped` with nothing in flight and no report exists
- **WHEN** `agk apply wait D` runs
- **THEN** it prints `status` `needs-human` with `reason` `no-report` without waiting for the timeout

#### Scenario: Companion closed after a turn ended without a report
- **GIVEN** `turn.json` records the current round, `stopped` with nothing in flight, there is no report, and the user closed the companion
- **WHEN** `agk apply wait D` runs
- **THEN** it prints `status` `cancelled`

#### Scenario: Run without turn-state hooks
- **GIVEN** a codex applier, or a claude run started by an agk without turn-state hooks
- **WHEN** its turn ends without a report
- **THEN** `agk apply wait D` prints `needs-human` with `reason` `no-report` at once

#### Scenario: Garbage report
- **GIVEN** `rounds/1/report.json` is not a JSON object with a valid `status`
- **WHEN** `agk apply wait D` reads it
- **THEN** it prints `status` `needs-human` with `reason` `invalid-report`

#### Scenario: User closed the applier
- **GIVEN** the companion of D is closed before a report exists
- **WHEN** `agk apply wait D` is waiting
- **THEN** it prints `status` `cancelled`

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

### Requirement: Close the applier
`agk apply close <run-dir>` SHALL close the run's companion so it can be restored with agterm's undo, and SHALL exit 0 when the companion is already gone.

#### Scenario: Close after the loop
- **GIVEN** apply run D with an open companion
- **WHEN** the agent runs `agk apply close D`
- **THEN** the companion is closed with undo available and the exit code is 0

### Requirement: Inline applier outside agterm
When `AGTERM_ENABLED` is unset, `agk apply start` SHALL run the first round headless in the foreground in the root the applier works in (claude in print mode with a fixed session id, or `codex exec` in a `workspace-write` sandbox), streaming sanitised output to stderr, then print `{"companion": null, "dir": "<run dir>", "started": true, "worktree": <path or null>, "pulled": <bool>}`; `agk apply send` SHALL resume the same harness session headless for the next round; `agk apply wait` SHALL read the report without calling agterm.

#### Scenario: Apply outside agterm
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the agent runs `agk apply start add-auth` and then `agk apply wait <dir>`
- **THEN** no agtermctl command is executed, round 1 runs to completion before `start` returns, and `wait` prints the round 1 report outcome

#### Scenario: Follow-up resumes the session
- **GIVEN** an inline claude apply run D after round 1
- **WHEN** the agent runs `agk apply send D notes.md`
- **THEN** claude is resumed with the session id recorded for D and round 2 runs in the foreground

#### Scenario: Inline run in the change worktree
- **GIVEN** `AGTERM_ENABLED` is unset and worktree mode is on
- **WHEN** the agent runs `agk apply start add-auth` from the main checkout
- **THEN** round 1 runs with `.worktrees/add-auth` as its working directory, and stdout carries `"companion": null` and that worktree's path as `"worktree"`

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

### Requirement: Applier companion renamed for each round
Once `agk apply send` has typed round k+1 into the applier companion, it SHALL rename the companion to `<change> · <harness>-apply r<k+1>`. A failed rename SHALL be a warning on stderr and SHALL NOT change the command's stdout or exit code. A round that was not typed (busy, permission prompt, gone) SHALL NOT rename it.

#### Scenario: Findings round renames the applier
- **GIVEN** apply run D of `add-auth` with companion `add-auth · claude-apply r1` that finished round 1 with `done`
- **WHEN** the agent runs `agk apply send D findings.md`
- **THEN** after the line for round 2 is typed, the companion is named `add-auth · claude-apply r2`

#### Scenario: Unconfirmed round still renames
- **GIVEN** the applier of D does not report round 2 as started after the line is typed
- **WHEN** `agk apply send D notes.md` prints `{"round":2,"started":false}`
- **THEN** the companion is named after round 2, because round 2 stays recorded

#### Scenario: Rename fails
- **GIVEN** agterm refuses the rename of D's companion
- **WHEN** the agent runs `agk apply send D notes.md` and round 2 is typed and starts
- **THEN** stderr carries a warning naming the rename failure, stdout is `{"round":2,"started":true}` and the exit code is 0

#### Scenario: Applier busy keeps the name
- **GIVEN** the applier of D is mid-turn
- **WHEN** `agk apply send D notes.md --timeout 30` gives up after 30 seconds
- **THEN** no rename is attempted and the companion keeps its name

#### Scenario: Inline applier has nothing to rename
- **GIVEN** an apply run D with no companion
- **WHEN** the agent runs `agk apply send D notes.md`
- **THEN** agk calls no agterm rename

### Requirement: Codex applier runs without the shared daemon
The interactive codex applier SHALL run with `--no-daemon`, so its hooks run in its own process with its own `AGTERM_*` variables and not in a shared codex app-server started from another session. The flag SHALL come after the configured model, effort and `extra_args`, and SHALL appear once, even when `extra_args` already has it. It needs codex 0.156.0 or newer.

#### Scenario: Codex applier argv
- **GIVEN** the agent runs inside agterm and the applier resolves to `codex` with no `extra_args`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the companion's codex command line carries `--no-daemon` once, after the model and effort

#### Scenario: Flag already in extra_args
- **GIVEN** the applier resolves to `codex` with `extra_args = ["--no-daemon"]`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the companion's codex command line carries `--no-daemon` exactly once

#### Scenario: Claude applier unchanged
- **GIVEN** the agent runs inside agterm and the applier resolves to `claude`
- **WHEN** the agent runs `agk apply start add-auth`
- **THEN** the claude command line carries no `--no-daemon`

### Requirement: Codex applier turn end from a settled screen
Before typing a follow-up round into a codex applier whose status does not show the turn over (`completed`, `idle`) or a prompt (`blocked`), `agk apply send` SHALL treat the turn as over when the current round's report file is on disk, two reads of the companion's screen taken 5 s apart are identical, and the screen shows no codex dialog asking to confirm, submit an answer or allow a command. It SHALL then type the line and warn on stderr.

#### Scenario: Status stuck at active after round 1
- **GIVEN** apply run D with a codex applier whose round 1 has `report.json`, whose agent status stays `active`, and whose screen does not change
- **WHEN** the agent runs `agk apply send D findings.md`
- **THEN** the line for round 2 is typed into the companion within about two poll intervals, stderr carries a warning that the applier still reports `active` but its round is on disk and its screen has settled, stdout carries `"round":2`, and the exit code is 0

#### Scenario: No report yet
- **GIVEN** apply run D with a codex applier whose round 1 has no `report.json`, whose status stays `active`, and whose screen does not change
- **WHEN** the agent runs `agk apply send D notes.md --timeout 30`
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the applier is busy

#### Scenario: Screen still changing
- **GIVEN** apply run D with a codex applier whose round 1 has `report.json`, whose status stays `active`, and whose screen changes between every two reads
- **WHEN** the agent runs `agk apply send D notes.md --timeout 30`
- **THEN** nothing is typed into the companion, no new round is recorded and the command exits 1 saying the applier is busy

#### Scenario: Claude applier keeps the status rule
- **GIVEN** apply run D with a claude applier whose round 1 has `report.json`, whose status stays `active`, and whose screen does not change
- **WHEN** the agent runs `agk apply send D notes.md --timeout 30`
- **THEN** nothing is typed, no new round is recorded and the command exits 1 saying the applier is busy
