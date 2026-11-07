## Purpose

Runs the applier, the worker that implements an OpenSpec change and later fixes review findings or user notes,
as an autonomous agent session in a visible agterm companion (or inline outside agterm) that the main session
can start, wait for, send follow-up rounds to and close.

## ADDED Requirements

### Requirement: Apply run directory
Each `agk apply start` SHALL create a new run directory `.agk/applies/<timestamp>/` under the repository root, numbered N like review runs (1-based position among the repository's apply runs). It SHALL hold the resolved worker configuration, the change name, the current round number, the companion id when one exists, and for every round k a `rounds/<k>/` directory with that round's `prompt.md` and, once the applier reports, `report.json`.

#### Scenario: Directory after start
- **GIVEN** a repository with two earlier apply runs
- **WHEN** the agent runs `agk apply start add-auth` inside agterm
- **THEN** a new run directory exists for apply #3 with the change name, round `1`, the companion id and `rounds/1/prompt.md`

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
Inside agterm, `agk apply start` SHALL create a background companion of the current session named `<harness>-apply #N`, running the configured harness interactively in the repository root, with the first round started by a single-line instruction to read `rounds/1/prompt.md`. It SHALL print `{"companion": "<id>", "dir": "<run dir>", "started": <bool>}` and exit 0.

#### Scenario: Claude applier starts
- **GIVEN** the applier resolves to `claude`
- **WHEN** the agent in session S runs `agk apply start add-auth`
- **THEN** a companion `claude-apply #<N>` appears under S, S stays selected, the companion runs an interactive claude session whose first prompt points at `rounds/1/prompt.md`, and stdout carries the companion id, the run dir and `"started": true`

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

### Requirement: Wait for the applier
`agk apply wait <run-dir> [--timeout SECONDS]` (default 3600) SHALL print one JSON object with `status` and `round` and exit 0. The status SHALL be `done` or `blocked` from a valid report of the current round (with the report's fields), `needs-human` when the turn ended without a valid report or the agent asks for permission, `cancelled` when the companion is gone, or `running` on timeout. It SHALL NOT close the companion.

#### Scenario: Round finished
- **GIVEN** the applier of run dir D wrote a valid `done` report for round 1
- **WHEN** the agent runs `agk apply wait D`
- **THEN** stdout is a JSON object with `status` `done`, `round` 1 and the report's `summary`, and the companion stays open

#### Scenario: Permission prompt
- **GIVEN** the applier's agent status becomes `blocked` before it wrote a report
- **WHEN** `agk apply wait D` is waiting
- **THEN** it prints `status` `needs-human` with `reason` `permission`

#### Scenario: Turn ended with a question
- **GIVEN** the applier's turn ends (status `completed` or `idle`) and the current round has no report
- **WHEN** `agk apply wait D` sees it
- **THEN** it prints `status` `needs-human` with `reason` `no-report`

#### Scenario: Garbage report
- **GIVEN** `rounds/1/report.json` is not a JSON object with a valid `status`
- **WHEN** `agk apply wait D` reads it
- **THEN** it prints `status` `needs-human` with `reason` `invalid-report`

#### Scenario: User closed the applier
- **GIVEN** the companion of D is closed before a report exists
- **WHEN** `agk apply wait D` is waiting
- **THEN** it prints `status` `cancelled`

### Requirement: Follow-up round
`agk apply send <run-dir> (<file> | --stdin) [--timeout SECONDS]` SHALL start round k+1 in the live applier session: wait (default 600 s) until its current turn is over, write `rounds/<k+1>/prompt.md` from the built-in fix template and the given message, record round k+1, type a single-line instruction to read that prompt into the session, and wait up to 30 seconds for the session to become `active`, warning on stderr when it does not. It SHALL print `{"round": k+1}`.

#### Scenario: Review findings sent to the applier
- **GIVEN** apply run D finished round 1 with `done`
- **WHEN** the agent runs `agk apply send D findings.md`
- **THEN** `rounds/2/prompt.md` contains the content of `findings.md` and the absolute path of `rounds/2/report.json`, the companion receives one line pointing at `rounds/2/prompt.md` followed by Enter, and stdout is `{"round":2}`

#### Scenario: Round start not confirmed
- **GIVEN** apply run D finished round 1 and its applier does not report `active` after the line is typed
- **WHEN** the agent runs `agk apply send D notes.md`
- **THEN** round 2 stays recorded, stderr carries a warning that the applier did not report round 2 as started, stdout is `{"round":2}` and the exit code is 0, so `agk apply wait D` can still finish the round through its report file

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

### Requirement: Close the applier
`agk apply close <run-dir>` SHALL close the run's companion so it can be restored with agterm's undo, and SHALL exit 0 when the companion is already gone.

#### Scenario: Close after the loop
- **GIVEN** apply run D with an open companion
- **WHEN** the agent runs `agk apply close D`
- **THEN** the companion is closed with undo available and the exit code is 0

### Requirement: Inline applier outside agterm
When `AGTERM_ENABLED` is unset, `agk apply start` SHALL run the first round headless in the foreground (claude in print mode with a fixed session id, or `codex exec` in a `workspace-write` sandbox), streaming sanitised output to stderr, then print `{"companion": null, "dir": "<run dir>", "started": true}`; `agk apply send` SHALL resume the same harness session headless for the next round; `agk apply wait` SHALL read the report without calling agterm.

#### Scenario: Apply outside agterm
- **GIVEN** `AGTERM_ENABLED` is unset
- **WHEN** the agent runs `agk apply start add-auth` and then `agk apply wait <dir>`
- **THEN** no agtermctl command is executed, round 1 runs to completion before `start` returns, and `wait` prints the round 1 report outcome

#### Scenario: Follow-up resumes the session
- **GIVEN** an inline claude apply run D after round 1
- **WHEN** the agent runs `agk apply send D notes.md`
- **THEN** claude is resumed with the session id recorded for D and round 2 runs in the foreground
