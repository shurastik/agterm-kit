# Spec Delta

## MODIFIED Requirements

### Requirement: Applier companion inside agterm
Inside agterm, `agk apply start` SHALL create a background companion of the current session named `<change> · <harness>-apply r1`, prefixed with `🤖 ` for a run started with `--autopilot` (see the `autopilot-marker` capability), running the configured harness interactively in the root the applier works in, with the first round started by a single-line instruction to read `rounds/1/prompt.md`. It SHALL print `{"companion": "<id>", "dir": "<run dir>", "started": <bool>, "worktree": <path or null>, "pulled": <bool>}` and exit 0.

#### Scenario: Claude applier starts
- **GIVEN** the applier resolves to `claude` and worktree mode is off
- **WHEN** the agent in session S runs `agk apply start add-auth`
- **THEN** a companion `add-auth · claude-apply r1` appears under S, S stays selected, the companion runs an interactive claude session in the repository root whose first prompt points at `rounds/1/prompt.md`, and stdout carries the companion id, the run dir, `"started": true`, `"worktree": null` and `"pulled": false`

#### Scenario: Applier in the change worktree
- **GIVEN** worktree mode and the applier resolving to `claude`
- **WHEN** the agent in session S runs `agk apply start add-auth` from the main checkout
- **THEN** the companion is named `add-auth · claude-apply r1`, its working directory is `.worktrees/add-auth`, and stdout carries `"worktree"` with the absolute path of `.worktrees/add-auth`

#### Scenario: Autopilot applier starts
- **GIVEN** the applier resolves to `claude`
- **WHEN** the agent in session S runs `agk apply start add-auth --autopilot`
- **THEN** the companion under S is named `🤖 add-auth · claude-apply r1` and stdout has the same fields as without the flag

#### Scenario: Agent session keeps its name
- **GIVEN** the agent runs in agterm session S
- **WHEN** it runs `agk apply start add-auth`
- **THEN** agk renames no session: S keeps its name, and only the new companion carries the change name

### Requirement: Applier companion renamed for each round
Once `agk apply send` has typed round k+1 into the applier companion, it SHALL rename the companion to the name `agk apply start` gives it with `r<k+1>` in place of `r1`: `<change> · <harness>-apply r<k+1>`, prefixed with `🤖 ` for an autopilot run. A failed rename SHALL be a warning on stderr and SHALL NOT change the command's stdout or exit code. A round that was not typed (busy, permission prompt, gone) SHALL NOT rename it.

#### Scenario: Findings round renames the applier
- **GIVEN** apply run D of `add-auth` with companion `add-auth · claude-apply r1` that finished round 1 with `done`
- **WHEN** the agent runs `agk apply send D findings.md`
- **THEN** after the line for round 2 is typed, the companion is named `add-auth · claude-apply r2`

#### Scenario: Autopilot applier renamed
- **GIVEN** autopilot apply run D of `add-auth` with companion `🤖 add-auth · claude-apply r1` that finished round 1 with `done`
- **WHEN** the agent runs `agk apply send D findings.md`
- **THEN** after the line for round 2 is typed, the companion is named `🤖 add-auth · claude-apply r2`

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
