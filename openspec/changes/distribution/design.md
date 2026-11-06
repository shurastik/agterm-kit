# Design

## Context

See proposal.md for motivation and `specs/installation/spec.md` for the behavior. Current state:

- `cmd/agk/main.go` builds an `internal/cli.Env` (every side effect injectable) and dispatches
  `review` and `triage`; there is no version and no setup command.
- The only skill is `skill/agterm-codex-review/SKILL.md`, linked by hand into `~/.claude/skills/`.
  `~/.codex/skills/` exists on this machine and already holds a directory skill (`agterm`), so Codex
  loads skills from there.
- The user's global excludes file is the git default `~/.config/git/ignore` (no
  `core.excludesFile` set); it holds one line.
- `go.mod` says `go 1.27.3`, the exact local toolchain, which forces every builder (including
  Homebrew's Go) to be at least that patch release.
- `internal/agterm.ResolveBin` already resolves `agtermctl` like agterm's hooks; `internal/fsx`
  has `ReplaceFile` (temp file + rename, never writes through a symlink).
- The repository is private and tracks `.beads/` data; its remote also carries `refs/dolt/data`.

## Goals / Non-Goals

**Goals:**
- One command after `brew install`/`brew upgrade` gives the machine matching skills for both agents.
- Upgrades need no relinking: links resolve through a path Homebrew keeps stable.
- Keep the dev loop: a checkout can be wired with `--skills-dir`.

**Non-Goals:**
- Linux packages or a Homebrew cask; agterm is macOS-only and so is this formula.
- CI-driven releases; one maintainer releases from this Mac.
- Any per-project install step (`agk init`): nothing goes into projects.
- Uninstall command; `brew uninstall` plus deleting dangling links by hand is enough for now
  (the next `agk setup` of a reinstall cleans them anyway).

## Decisions

### D1. A source-built formula in our own tap, not GoReleaser

The formula downloads the GitHub tag tarball, builds with `go build -ldflags "-X …Version=…"`,
installs `skills/` with `pkgshare.install`, and runs `agk version` in `test do`.

Alternatives: GoReleaser with its Homebrew output. GoReleaser is steering Homebrew publishing
toward casks, and a cask has no `share/` that Homebrew links into the prefix, which is exactly what
the stable skills path relies on. It also needs CI and a cross-repo token to push the tap. For a
single Go binary with no cgo, building from source costs Homebrew a few seconds and removes both.

### D2. The formula template lives in this repo, the release script renders it into the tap

`packaging/homebrew/agk.rb.tmpl` (version, URL and sha256 placeholders) is versioned with the code
it builds, so a layout change (say, a new `share/` subdirectory) lands in the same commit as the
code. `scripts/release.sh VERSION` checks it is on a clean `main` in sync with origin, runs
`go test ./...` and `go vet ./...`, checks the tap checkout (on `main`, clean, fast-forwarded
from `origin main`) before anything is published, tags `vVERSION` (reusing an existing tag only if
it is on `HEAD`, so a failed run can be repeated), pushes the tag, downloads the tag tarball,
computes its sha256, renders the template into the tap checkout (`$AGK_TAP_DIR`, default
`~/Projects/homebrew-tap`) as `Formula/agk.rb`, commits and pushes the tap to `origin main` explicitly.

Alternative: hand-editing the formula in the tap each release; easy to forget the sha or the
skills install line.

### D3. Version stamped with `-ldflags -X`

`internal/buildinfo.Version` defaults to `dev`; the formula and release script set it. Alternative:
`debug.ReadBuildInfo()`; a build from a source tarball has no VCS or module version information, so
it would print `(devel)` for every release.

### D4. Skills directory: explicit, then Homebrew opt path from the Cellar layout

Order: `--skills-dir`, `$AGK_SKILLS_DIR`, then resolve the executable with `filepath.EvalSymlinks`.
If it matches `<prefix>/Cellar/agk/<version>/bin/agk`, use `<prefix>/opt/agk/share/agk/skills`.
Otherwise use `<dir of exe>/../share/agk/skills`. The first candidate that is a directory wins;
none is an error. An explicit `--skills-dir` or `$AGK_SKILLS_DIR` is made absolute and must be a
directory, else it is an error naming its source: a typo must not silently fall back to the
Homebrew skills.

Why the opt path: `opt/agk` is the symlink Homebrew repoints on every upgrade, so links into it
stay valid. The versioned Cellar directory is deleted by `brew cleanup`, so links into it would
dangle after an upgrade. Alternatives: `brew --prefix agk` (spawns Ruby, about a second, needs
`brew` on PATH); the unresolved `os.Executable()` path (on macOS it may already be resolved into
the Cellar, so it is not reliable).

### D5. `internal/setup`: plan, then apply

`setup.Plan(cfg) ([]Action, error)` refuses a skills directory without `agk-*` skills (it
would plan every installed link as stale), then inspects the filesystem with `Lstat`/`Readlink` and returns
actions with a kind (`ok`, `create`, `repoint`, `remove`, `conflict`, `skip`, `append`,
`missing`, `found`). `setup.Apply(actions)` performs the mutating ones. `--dry-run` prints the
plan and stops. `cfg` carries the home directory, `getenv`, a `lookPath` and a `gitConfig` func,
so tests run against a temp `HOME` with fakes and never touch the real one. The CLI prints one
line per action and decides the exit status (1 when any `conflict`, or a `missing` for a required
tool).

Alternative: act while scanning. Simpler code, but dry run would need a second code path, and
"wire everything, then fail on missing deps" would be harder to keep true.

### D6. Link mechanics

A skill link is a symlink to an absolute path. Creating or repointing writes the symlink under a
temporary name in the same directory and renames it over the destination, so a concurrent agent
never sees the link missing. `remove` only ever applies to paths whose `Lstat` says symlink.
A destination that is a regular file or directory is a `conflict`. Stale-link cleanup scans only
names matching `agk-*` plus the literal legacy name `agterm-codex-review`.

### D7. Excludes file

Path: `git config --global --includes --path core.excludesFile` (via the injected `gitConfig`;
`--includes` because `--global` skips `[include]` files by default, `--path` expands `~`), else
`$XDG_CONFIG_HOME/git/ignore`, else `~/.config/git/ignore`. `.agk/` counts as ignored only when
a literal `.agk`, `.agk/`, `/.agk` or `/.agk/` rule (trailing, not leading, whitespace trimmed as
gitignore does) comes after every `!` rule in the file: git's last matching rule wins, and any
negation (`!**/.agk/`, `!*`) might re-include it. Otherwise `.agk/` is appended, which makes it
last again. A relative `core.excludesFile` is a `conflict`: git resolves it per repository, so
appending would edit the current project's file. Positive globs that might already match are not evaluated; missing one only costs a
redundant line. To append: resolve symlinks
with `filepath.EvalSymlinks`, read the target, add a newline first if the file does not end with
one, add `.agk/`, write with `fsx.ReplaceFile` keeping the file mode. A missing file is created
with its parent directory. Editing the resolved target keeps a dotfiles symlink intact;
`ReplaceFile` alone would replace the symlink with a regular file.

### D8. Dependency checks

`git`, `codex`, `claude`, `openspec` via `exec.LookPath` (injected). `agtermctl` via
`agterm.ResolveBin`, then `lookPath` on its result (the fallback is the bare name `agtermctl`).
Only presence is checked; version floors are left to the commands that need them.

### D9. Skills move to `skills/`, prefix `agk-`

`skill/agterm-codex-review/` becomes `skills/agk-review/` (front matter `name: agk-review`). The
body stays the same apart from the run-dir note and anything naming the old skill; its triage
section stays until the change that removes `agk triage`. The plural top-level `skills/` mirrors
the installed `share/agk/skills/`, so `--skills-dir ./skills` works from a checkout.

### D10. `go` directive lowered to what the code needs

The task builds and tests with `GOTOOLCHAIN=go1.26.0`; if that passes, `go.mod` says `go 1.26.0`
(the stack in the project context), otherwise `go 1.27.0`. Either way no patch-level pin, so
Homebrew's current Go builds it without fetching a toolchain.

## Risks / Trade-offs

- [Publishing exposes history: `.beads/` data, `refs/dolt/data` on the remote, any stray secret]
  → Before switching visibility, scan the full history (`gitleaks detect` or an equivalent grep
  over `git log -p --all`) and list refs with `git ls-remote`; the owner decides on what is found.
  Making the repo public happens only after that, with the owner's explicit go-ahead.
- [Homebrew's Go is older than the `go` directive] → D10 keeps the directive at a minor release;
  the release script's `brew install --build-from-source` check (task) catches it before users do.
- [A user never runs `agk setup` after `brew upgrade`] → Links point at the opt path, so upgraded
  skill content is picked up anyway; only added or removed skills need the rerun. The formula
  caveat says to rerun it.
- [Codex or Claude change their user skills directory] → Both honour their config-dir variables
  (`CODEX_HOME`, `CLAUDE_CONFIG_DIR`); a move is a one-line change in the plan.
- [Agents not following symlinked skill directories] → Both do on this machine today
  (`~/.claude/skills/agterm-codex-review` is a link); covered by a manual check in tasks.

## Migration Plan

On this machine, after the first release:

1. `brew install shurastik/tap/agk`, then `agk setup` (replaces the legacy
   `agterm-codex-review` link with `agk-review` links for both agents, adds `.agk/` to the global
   ignore).
2. Remove `~/go/bin/agk` so the brew binary is the one on `PATH`.
3. Start a new Claude Code session and check that `/agk-review` is listed.

Rollback: `brew uninstall agk`, `go install ./cmd/agk` and
`agk setup --skills-dir ~/Projects/agterm-kit/skills` from a checkout.
