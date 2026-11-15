# Spec Delta

## MODIFIED Requirements

### Requirement: Applier companion inside agterm
Inside agterm, `agk apply start` SHALL create a background companion of the current session named `<change> · <harness>-apply #N`, running the configured harness interactively in the root the applier works in, with the first round started by a single-line instruction to read `rounds/1/prompt.md`. It SHALL print `{"companion": "<id>", "dir": "<run dir>", "started": <bool>, "worktree": <path or null>, "pulled": <bool>}` and exit 0.

#### Scenario: Claude applier starts
- **GIVEN** the applier resolves to `claude` and worktree mode is off
- **WHEN** the agent in session S runs `agk apply start add-auth`
- **THEN** a companion `add-auth · claude-apply #<N>` appears under S, S stays selected, the companion runs an interactive claude session in the repository root whose first prompt points at `rounds/1/prompt.md`, and stdout carries the companion id, the run dir, `"started": true`, `"worktree": null` and `"pulled": false`

#### Scenario: Applier in the change worktree
- **GIVEN** worktree mode and the applier resolving to `claude`
- **WHEN** the agent in session S runs `agk apply start add-auth` from the main checkout
- **THEN** the companion is named `add-auth · claude-apply #<N>`, its working directory is `.worktrees/add-auth`, and stdout carries `"worktree"` with the absolute path of `.worktrees/add-auth`

#### Scenario: Agent session keeps its name
- **GIVEN** the agent runs in agterm session S
- **WHEN** it runs `agk apply start add-auth`
- **THEN** agk renames no session: S keeps its name, and only the new companion carries the change name
