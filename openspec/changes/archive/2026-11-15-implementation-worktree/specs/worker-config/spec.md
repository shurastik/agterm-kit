# Spec Delta

## MODIFIED Requirements

### Requirement: Worker roles and defaults
agk SHALL know two worker roles, `applier` and `reviewer`. Each SHALL resolve to a harness (`claude` or `codex`) plus optional `model`, `effort` and `extra_args`; a claude applier also has `permission_mode` (default `auto`), and the applier of either harness has `worktree` (default `false`), which a harness switch keeps. Without any configuration the applier SHALL be `claude` and the reviewer `codex`, both with the harness's own default model.

#### Scenario: No configuration at all
- **GIVEN** no config files exist and no worker flags are passed
- **WHEN** the user runs `agk config`
- **THEN** it reports applier harness `claude` with `permission_mode` `auto` and `worktree` `false`, and reviewer harness `codex`, each with no model, no effort and no extra args, and source `default`

#### Scenario: Harness switch keeps worktree
- **GIVEN** the user file sets `[applier] harness = "claude"` and `worktree = true`
- **WHEN** the user runs `agk config --applier codex`
- **THEN** the applier is `codex` with `worktree` `true` from the user file

### Requirement: Role flag
`agk apply start` SHALL accept `--applier HARNESS[:MODEL]` and `agk review start` and `agk config` SHALL accept `--reviewer HARNESS[:MODEL]` (`agk config` both). When the flag names a harness other than the one the files resolve to, the file values for `model`, `effort`, `extra_args` and `permission_mode` SHALL be dropped for that role. `agk apply start` SHALL also accept `--worktree` and `--no-worktree`, which set the applier's `worktree` to true or false with source `flag`; passing both SHALL be refused.

#### Scenario: Model override for one run
- **GIVEN** the user file sets `[reviewer] harness = "codex"` and `effort = "high"`
- **WHEN** the user runs `agk review start --reviewer codex:gpt-5.5-mini`
- **THEN** the reviewer runs codex with model `gpt-5.5-mini` and effort `high`

#### Scenario: Harness switch drops foreign options
- **GIVEN** the user file sets `[reviewer] harness = "codex"`, `model = "gpt-5.5"` and `extra_args = ["--profile", "review"]`
- **WHEN** the user runs `agk review start --reviewer claude`
- **THEN** the reviewer runs claude with its default model and without `--profile review`

#### Scenario: Malformed flag
- **WHEN** the user passes `--applier gemini` or `--applier claude:`
- **THEN** the command exits 1 with an error naming the flag and the accepted harnesses, before any companion or run directory is created

#### Scenario: Worktree off for one run
- **GIVEN** the user file sets `[applier] worktree = true`
- **WHEN** the user runs `agk apply start add-auth --no-worktree`
- **THEN** the applier runs in the main checkout, and no worktree, pull or commit happens

#### Scenario: Both worktree flags
- **WHEN** the user runs `agk apply start add-auth --worktree --no-worktree`
- **THEN** the command exits 1 naming both flags, before any pull, commit, worktree, companion or run directory

### Requirement: Configuration validation
agk SHALL reject a config file that is not valid TOML, has a table other than `[applier]` and `[reviewer]`, a key it does not know, an unknown harness, a value of the wrong type, `permission_mode` outside `[applier]` or for a codex applier, `worktree` outside `[applier]`, or an `extra_args` entry containing a backslash or a line break. The error SHALL name the file and the offending table or key, and the command SHALL exit 1 without starting a worker.

#### Scenario: Typo in a key
- **GIVEN** `.agk.toml` contains `[reviewer] modle = "x"`
- **WHEN** the user runs `agk review start`
- **THEN** it exits 1 with an error naming `.agk.toml` and `reviewer.modle`, and no run directory is created

#### Scenario: Unquotable extra argument
- **GIVEN** the user file sets `[applier] extra_args = ["--x\nrm"]`
- **WHEN** the user runs `agk apply start c`
- **THEN** it exits 1 with an error naming `applier.extra_args`

#### Scenario: Worktree on the reviewer
- **GIVEN** `.agk.toml` contains `[reviewer] worktree = true`
- **WHEN** the user runs `agk config`
- **THEN** it exits 1 with an error naming `.agk.toml` and `reviewer.worktree`

#### Scenario: Worktree of the wrong type
- **GIVEN** `.agk.toml` contains `[applier] worktree = "yes"`
- **WHEN** the user runs `agk apply start c`
- **THEN** it exits 1 with an error naming `.agk.toml` and `applier.worktree`, and no worktree, commit or run directory is created

### Requirement: Show the resolved configuration
`agk config [--applier H[:M]] [--reviewer H[:M]]` SHALL print one JSON object with an `applier` and a `reviewer` entry, each listing `harness`, `model`, `effort`, `extra_args`, for a claude applier `permission_mode`, and for the applier `worktree`, and for every key the source it came from (`flag`, a file path, or `default`). It SHALL exit 1 on any validation error.

#### Scenario: Sources shown per key
- **GIVEN** `.agk.toml` sets `[applier] effort = "high"` and nothing else is configured
- **WHEN** the user runs `agk config`
- **THEN** the applier entry has `effort` `high` with source `<repo>/.agk.toml` and `harness` `claude` with source `default`

#### Scenario: Worktree source shown
- **GIVEN** the user file sets `[applier] worktree = true`
- **WHEN** the user runs `agk config`
- **THEN** the applier entry has `worktree` `true` with the user file as its source
