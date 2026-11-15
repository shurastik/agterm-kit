# Spec Delta

## MODIFIED Requirements

### Requirement: Apply run directory
Each `agk apply start` SHALL create a new run directory `.agk/applies/<timestamp>/` under the root the applier works in, numbered N like review runs (1-based position among that root's apply runs). That root SHALL be the change's worktree in worktree mode or when started inside a linked worktree, and the repository root otherwise. The run directory SHALL hold the resolved worker configuration, the change name, the current round number, the companion id when one exists, and for every round k a `rounds/<k>/` directory with that round's `prompt.md` and, once the applier reports, `report.json`.

#### Scenario: Directory after start
- **GIVEN** a repository with two earlier apply runs
- **WHEN** the agent runs `agk apply start add-auth` inside agterm
- **THEN** a new run directory exists for apply #3 with the change name, round `1`, the companion id and `rounds/1/prompt.md`

#### Scenario: Directory in worktree mode
- **GIVEN** worktree mode and a new worktree `.worktrees/add-auth`
- **WHEN** the agent runs `agk apply start add-auth` from the main checkout
- **THEN** the run directory is created under `.worktrees/add-auth/.agk/applies/` and none under the main checkout's `.agk/applies/`

### Requirement: Applier companion inside agterm
Inside agterm, `agk apply start` SHALL create a background companion of the current session named `<harness>-apply #N`, running the configured harness interactively in the root the applier works in, with the first round started by a single-line instruction to read `rounds/1/prompt.md`. It SHALL print `{"companion": "<id>", "dir": "<run dir>", "started": <bool>, "worktree": <path or null>, "pulled": <bool>}` and exit 0.

#### Scenario: Claude applier starts
- **GIVEN** the applier resolves to `claude` and worktree mode is off
- **WHEN** the agent in session S runs `agk apply start add-auth`
- **THEN** a companion `claude-apply #<N>` appears under S, S stays selected, the companion runs an interactive claude session in the repository root whose first prompt points at `rounds/1/prompt.md`, and stdout carries the companion id, the run dir, `"started": true`, `"worktree": null` and `"pulled": false`

#### Scenario: Applier in the change worktree
- **GIVEN** worktree mode and the applier resolving to `claude`
- **WHEN** the agent in session S runs `agk apply start add-auth` from the main checkout
- **THEN** the companion's working directory is `.worktrees/add-auth`, and stdout carries `"worktree"` with the absolute path of `.worktrees/add-auth`

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
