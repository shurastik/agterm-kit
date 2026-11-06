## Purpose

Decides which agent harness, model and options each agk worker role (applier, reviewer) runs with,
from command-line flags, an optional per-repository file, a per-user file and built-in defaults.

## ADDED Requirements

### Requirement: Worker roles and defaults
agk SHALL know two worker roles, `applier` and `reviewer`. Each SHALL resolve to a harness (`claude` or `codex`) plus optional `model`, `effort` and `extra_args`; a claude applier also has `permission_mode` (default `auto`). Without any configuration the applier SHALL be `claude` and the reviewer `codex`, both with the harness's own default model.

#### Scenario: No configuration at all
- **GIVEN** no config files exist and no worker flags are passed
- **WHEN** the user runs `agk config`
- **THEN** it reports applier harness `claude` with `permission_mode` `auto` and reviewer harness `codex`, each with no model, no effort and no extra args, and source `default`

### Requirement: Configuration sources and precedence
For each role key, agk SHALL take the value from the first source that sets it: the role flag, then `.agk.toml` in the repository root, then `$XDG_CONFIG_HOME/agk/config.toml` (default `~/.config/agk/config.toml`), then the built-in default. A missing file SHALL be skipped silently.

#### Scenario: Repository file overrides the user file
- **GIVEN** the user file sets `[reviewer] model = "gpt-5.5"` and `effort = "high"`, and `.agk.toml` sets `[reviewer] effort = "medium"`
- **WHEN** a review is started in that repository
- **THEN** the reviewer runs with model `gpt-5.5` and effort `medium`

#### Scenario: XDG config home
- **GIVEN** `XDG_CONFIG_HOME=/x` and `/x/agk/config.toml` sets `[applier] model = "claude-sonnet-5-5"`
- **WHEN** the user runs `agk config`
- **THEN** the applier model is `claude-sonnet-5-5` with source `/x/agk/config.toml`

### Requirement: Role flag
`agk apply start` SHALL accept `--applier HARNESS[:MODEL]` and `agk review start` and `agk config` SHALL accept `--reviewer HARNESS[:MODEL]` (`agk config` both). When the flag names a harness other than the one the files resolve to, the file values for `model`, `effort`, `extra_args` and `permission_mode` SHALL be dropped for that role.

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

### Requirement: Configuration validation
agk SHALL reject a config file that is not valid TOML, has a table other than `[applier]` and `[reviewer]`, a key it does not know, an unknown harness, a value of the wrong type, `permission_mode` outside `[applier]` or for a codex applier, or an `extra_args` entry containing a backslash or a line break. The error SHALL name the file and the offending table or key, and the command SHALL exit 1 without starting a worker.

#### Scenario: Typo in a key
- **GIVEN** `.agk.toml` contains `[reviewer] modle = "x"`
- **WHEN** the user runs `agk review start`
- **THEN** it exits 1 with an error naming `.agk.toml` and `reviewer.modle`, and no run directory is created

#### Scenario: Unquotable extra argument
- **GIVEN** the user file sets `[applier] extra_args = ["--x\nrm"]`
- **WHEN** the user runs `agk apply start c`
- **THEN** it exits 1 with an error naming `applier.extra_args`

### Requirement: Run pinned to its resolved configuration
Each run directory SHALL record the worker configuration resolved when it was started, and every later step of that run (follow-up rounds, resume, inline continuation) SHALL use the recorded configuration, ignoring later changes to the files.

#### Scenario: Config changed mid-run
- **GIVEN** a review started with reviewer `codex`
- **WHEN** the user edits the user file to `harness = "claude"` and then runs `agk review send <dir>`
- **THEN** round 2 still resumes the codex session

### Requirement: Show the resolved configuration
`agk config [--applier H[:M]] [--reviewer H[:M]]` SHALL print one JSON object with an `applier` and a `reviewer` entry, each listing `harness`, `model`, `effort`, `extra_args` and, for a claude applier, `permission_mode`, and for every key the source it came from (`flag`, a file path, or `default`). It SHALL exit 1 on any validation error.

#### Scenario: Sources shown per key
- **GIVEN** `.agk.toml` sets `[applier] effort = "high"` and nothing else is configured
- **WHEN** the user runs `agk config`
- **THEN** the applier entry has `effort` `high` with source `<repo>/.agk.toml` and `harness` `claude` with source `default`

### Requirement: Same worker on both roles is flagged
When the applier and the reviewer resolve to the same harness and the same model (an unset model counts as the same), `agk config`, `agk apply start` and `agk review start` SHALL print a warning on stderr that the review is not independent, and SHALL still proceed.

#### Scenario: Claude reviews claude
- **GIVEN** both roles resolve to `claude` with no model
- **WHEN** the user runs `agk review start`
- **THEN** stderr carries the warning and the review starts
