# installation Specification

## Purpose
Puts agk and the agent skills that drive it onto a machine and keeps the two in sync: the release
layout, the Homebrew install, `agk setup` wiring the skills into Claude Code and Codex, and
`agk version`. Projects receive nothing from installation.

## Requirements

### Requirement: Version reporting
`agk version` SHALL print the build version on one line and exit 0. A release build prints its
release version; a build without a stamped version prints `dev`.

#### Scenario: Release build
- **GIVEN** agk built for release `0.3.0`
- **WHEN** the user runs `agk version`
- **THEN** stdout is `0.3.0` and the exit status is 0

#### Scenario: Local build
- **GIVEN** agk built with `go build` and no version stamp
- **WHEN** the user runs `agk version`
- **THEN** stdout is `dev`

### Requirement: Release layout
A release SHALL install the `agk` binary and a skills directory `share/agk/skills/` that holds one
subdirectory per agent skill, each with a `SKILL.md`. Every skill directory name and skill `name`
SHALL start with `agk-`.

#### Scenario: Skills ship with the binary
- **WHEN** a release is installed with Homebrew
- **THEN** `$(brew --prefix)/opt/agk/share/agk/skills/agk-review/SKILL.md` exists and its `name` is `agk-review`

### Requirement: Homebrew install
agk SHALL be installable with `brew install shurastik/tap/agk` from a public tap. The formula SHALL
build agk with its release version stamped, run `agk version` as its test, and tell the user to run
`agk setup` after install and after every upgrade.

#### Scenario: Fresh install
- **GIVEN** a Mac with Homebrew and no agk
- **WHEN** the user runs `brew install shurastik/tap/agk`
- **THEN** `agk version` prints the formula's version and the install output mentions `agk setup`

### Requirement: Skills directory resolution
`agk setup` SHALL take the skills directory from `--skills-dir`, else `$AGK_SKILLS_DIR`, else the
Homebrew `opt/agk/share/agk/skills` path when agk runs from a Homebrew Cellar, else
`share/agk/skills` beside the binary's parent directory. It SHALL fail with a message naming
`--skills-dir` when none exists, and SHALL fail without changing anything when the chosen directory holds no `agk-*`
skill.

#### Scenario: Homebrew install links through the stable opt path
- **GIVEN** agk runs from `/opt/homebrew/Cellar/agk/0.3.0/bin/agk`
- **WHEN** the user runs `agk setup`
- **THEN** the skill links point into `/opt/homebrew/opt/agk/share/agk/skills/`, not into the versioned Cellar directory

#### Scenario: Development checkout
- **WHEN** the user runs `agk setup --skills-dir ~/Projects/agterm-kit/skills`
- **THEN** the skill links point into that checkout's `skills/` directory

#### Scenario: Skills directory without skills
- **GIVEN** agk skill links are installed and `--skills-dir` names a directory with no `agk-*` skill
- **WHEN** the user runs `agk setup --skills-dir <that directory>`
- **THEN** it exits 1 naming the directory, and the installed links are unchanged

#### Scenario: No skills directory found
- **GIVEN** agk was installed with `go install` and neither `--skills-dir` nor `$AGK_SKILLS_DIR` is set
- **WHEN** the user runs `agk setup`
- **THEN** it exits 1, changes nothing, and stderr names `--skills-dir`

### Requirement: Skill linking
`agk setup` SHALL create, for every skill in the skills directory, a symlink of the same name in the
Claude Code skills directory (`$CLAUDE_CONFIG_DIR/skills`, default `~/.claude/skills`) and in the
Codex skills directory (`$CODEX_HOME/skills`, default `~/.codex/skills`), creating a missing
`skills` directory. An agent whose config directory does not exist SHALL be skipped and reported.

#### Scenario: First setup
- **GIVEN** `~/.claude` and `~/.codex` exist and neither has agk skill links
- **WHEN** the user runs `agk setup`
- **THEN** `~/.claude/skills/agk-review` and `~/.codex/skills/agk-review` are symlinks to the `agk-review` directory in the skills directory

#### Scenario: Codex not set up
- **GIVEN** `~/.codex` does not exist
- **WHEN** the user runs `agk setup`
- **THEN** the Claude Code links are created, no `~/.codex` is created, and the output reports Codex as skipped

#### Scenario: Agents share one skills directory
- **GIVEN** `CODEX_HOME` points at `~/.claude`, so both agents read `~/.claude/skills`, which holds the legacy `agterm-codex-review` link
- **WHEN** the user runs `agk setup`
- **THEN** each link is created or removed once, Codex is reported as skipped for sharing Claude Code's skills directory, and the exit status is 0

### Requirement: Setup is idempotent
Running `agk setup` again SHALL leave correct links untouched and repoint an agk-named symlink that
points elsewhere. It SHALL NOT replace a regular file or directory with a link; such a path is
reported as a conflict, the other actions still run, and the command exits 1.

#### Scenario: Second run changes nothing
- **GIVEN** `agk setup` has completed
- **WHEN** the user runs `agk setup` again
- **THEN** no link or file is modified and every action is reported as already done

#### Scenario: Upgrade repoints a stale link
- **GIVEN** `~/.claude/skills/agk-review` is a symlink into a different directory
- **WHEN** the user runs `agk setup`
- **THEN** the link points into the resolved skills directory

#### Scenario: A real directory with a skill's name
- **GIVEN** `~/.claude/skills/agk-review` is a regular directory
- **WHEN** the user runs `agk setup`
- **THEN** that directory is left unchanged, the output reports a conflict for it, the other links are created, and the exit status is 1

### Requirement: Stale and legacy links are removed
`agk setup` SHALL remove symlinks named `agk-*` in the agent skills directories whose skill no
longer exists in the skills directory, and the legacy `agterm-codex-review` link. It SHALL only
remove symlinks, never regular files or directories.

#### Scenario: Skill dropped in a new release
- **GIVEN** `~/.claude/skills/agk-old` is a symlink and the skills directory has no `agk-old`
- **WHEN** the user runs `agk setup`
- **THEN** `~/.claude/skills/agk-old` is removed

#### Scenario: Legacy review skill link
- **GIVEN** `~/.claude/skills/agterm-codex-review` is a symlink into a checkout
- **WHEN** the user runs `agk setup`
- **THEN** the link is removed and `~/.claude/skills/agk-review` exists

#### Scenario: Unrelated skills untouched
- **GIVEN** `~/.claude/skills/agterm` is a regular directory and `~/.claude/skills/pohuy` a symlink
- **WHEN** the user runs `agk setup`
- **THEN** both are unchanged

### Requirement: Global git ignore
`agk setup` SHALL ensure the user's global git excludes file (git's `core.excludesFile`, else
`$XDG_CONFIG_HOME/git/ignore`, else `~/.config/git/ignore`) contains a line ignoring `.agk/`,
creating the file if missing and keeping its existing lines. A symlinked excludes file is edited at
its target.

#### Scenario: Line appended once
- **GIVEN** `~/.config/git/ignore` contains `**/.claude/settings.local.json` and no `.agk` line
- **WHEN** the user runs `agk setup` twice
- **THEN** the file contains its original line followed by exactly one `.agk/` line

#### Scenario: Global ignore explicitly disabled
- **GIVEN** the git config sets `core.excludesFile` to an empty value, which turns the global excludes file off
- **WHEN** the user runs `agk setup`
- **THEN** no excludes file is written, the output reports a conflict asking for an absolute path, the skill links are still set up, and the exit status is 1

#### Scenario: Dotfiles symlink preserved
- **GIVEN** the excludes file is a symlink into a dotfiles repository
- **WHEN** the user runs `agk setup`
- **THEN** the symlink still exists and the dotfiles copy contains `.agk/`

### Requirement: Dependency report
`agk setup` SHALL report whether `git`, `codex`, `claude` and `openspec` are found on `PATH`, and
whether `agtermctl` is found the way agterm's hooks resolve it (`$AGTERMCTL`, the app bundle,
`PATH`). A missing required tool SHALL make the command exit 1 after the wiring is done; a missing
`agtermctl` is reported as optional and does not.

#### Scenario: Missing openspec
- **GIVEN** `openspec` is not on `PATH`
- **WHEN** the user runs `agk setup`
- **THEN** the links and ignore line are still set up, the output names `openspec` as missing, and the exit status is 1

#### Scenario: No agterm
- **GIVEN** agterm is not installed and every required tool is present
- **WHEN** the user runs `agk setup`
- **THEN** the output reports `agtermctl` as optional and missing, and the exit status is 0

### Requirement: Dry run
`agk setup --dry-run` SHALL print every action it would take and SHALL NOT create, change or remove
any file or link.

#### Scenario: Preview before first setup
- **GIVEN** no agk skill links exist
- **WHEN** the user runs `agk setup --dry-run`
- **THEN** the output lists the links and the ignore line it would add, and the filesystem is unchanged

### Requirement: Machine-level only
`agk setup` SHALL NOT create or modify anything inside the current working directory or any
repository, except a global excludes file that happens to live in one.

#### Scenario: Relative excludes file
- **GIVEN** the global git config sets `core.excludesFile = .gitignore`
- **WHEN** the user runs `agk setup` inside a repository
- **THEN** the repository's `.gitignore` is unchanged, the output reports a conflict asking for an absolute path, the skill links are still set up, and the exit status is 1

#### Scenario: Run inside a project
- **GIVEN** the current directory is a git repository
- **WHEN** the user runs `agk setup`
- **THEN** `git status` in that repository shows no change
