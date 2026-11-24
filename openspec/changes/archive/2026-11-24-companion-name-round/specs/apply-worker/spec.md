## MODIFIED Requirements

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

## ADDED Requirements

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
