# agk

Applier and reviewer companions for coding agents in the agterm terminal

`agk` runs two workers next to your coding-agent session, each in its own visible terminal
companion in [agterm](https://agterm.app) (a native macOS terminal):

- the **applier** implements an [OpenSpec](https://github.com/Fission-AI/OpenSpec) change on its
  own, test first, and reports every round in a file;
- the **reviewer** reviews the diff (or the plan of a change) read-only and returns structured
  findings, and stays alive between rounds so a re-review remembers the earlier ones. In agterm it
  is a live Claude Code or Codex session you can watch and talk to; its harness sandbox lets it
  write only its findings file.

Both can be Claude Code or Codex. You keep working in your main session; `agk` is the glue that
starts the workers, waits for their results and hands findings back. Outside agterm the workers run
inline in the foreground instead.

## How it works

```
  you
   |
   v
+--------------------+   agk apply start / wait / send   +--------------------+
|    main session    | --------------------------------> |  applier companion |
| (Claude Code/Codex)|   implements the change, test     | claude or codex    |
|                    | <-------------------------------- | writes a report    |
|                    |   round report: done | blocked    +--------------------+
|                    |
|                    |   agk review start / wait / send  +--------------------+
|                    | --------------------------------> | reviewer companion |
|                    |   reviews the diff or the plan,   | codex or claude    |
|                    | <-------------------------------- | read-only          |
+--------------------+   findings (review.json)          +--------------------+
          |                                                        |
          +---------> .agk/applies/<run>/rounds/<k>/ <-------------+
                      .agk/reviews/<run>/rounds/<k>/
                      (what each round was asked and what it answered)
```

The loop, with the agent skills driving it:

0. `/agk-explore` thinks an idea through with the project's OpenSpec explore workflow and, when it
   is settled, hands it to `/agk-propose` instead of the `openspec-propose` skill's handoff.
1. `/agk-propose` writes an OpenSpec change (proposal, specs, design, tasks), has the reviewer check
   the plan until it approves, lets you annotate it in revdiff, and stops.
2. `/agk-apply <change>` runs the applier loop: `agk apply start` launches the applier, which works
   through the tasks and writes a report; `agk apply wait` returns it, `agk apply send` hands it
   findings for the next round.
3. `agk review start` reviews the diff in the reviewer companion; `agk review wait` returns the
   findings.
4. Findings go to the applier with `agk apply send` for the next round, then `agk review send`
   re-reviews in the same reviewer session. Repeat until the review approves.
5. `/agk-archive` archives the change once the last review approved the code as it is now
   (`agk review fresh` says whether it still is) and verify passed after it, then closes the
   change's applier and reviewer companions. In worktree mode (see below) it first commits the code
   and runs `agk sync` so the gates check the rebased code, syncs again before archiving, commits the
   archive and runs `agk finish`; in the main checkout it never commits. Without the skill,
   `agk apply close` and `agk review close` end the workers.

Every round is a directory under `.agk/` in the repository (gitignored once `agk setup` has added
`.agk/` to your global git ignore), so the state survives a restart of any of the sessions.

**agterm** is a macOS terminal with a control CLI, `agtermctl`. A *companion* is a terminal session
opened under another one; `agk` starts each worker in one and learns that a turn is over from the
session status. **OpenSpec** keeps a feature as a *change*: a folder of artifacts (`proposal.md`,
spec deltas, `design.md`, `tasks.md`) that is written, reviewed and implemented before it is
archived into the project's specs.

## Requirements

- macOS and [agterm](https://agterm.app), which provides `agtermctl`. Without it `agk` still works,
  with the workers running inline in the foreground.
- `git`.
- [`openspec`](https://github.com/Fission-AI/OpenSpec), the CLI the applier and `/agk-propose` use.
- [`claude`](https://claude.com/claude-code) (Claude Code) and [`codex`](https://github.com/openai/codex)
  (Codex CLI).
- [`revdiff`](https://github.com/umputun/revdiff), optional: `/agk-propose` uses it for your pass
  over the plan and skips that step without it.

`agk setup` checks these. `git`, `codex`, `claude` and `openspec` are required: when one is missing
`agk setup` exits 1, after it has already linked the skills. `agtermctl` and `revdiff` are reported
as optional. That check is stricter than what a run needs: a run only starts the harnesses its
workers are configured with (the applier is `claude` and the reviewer `codex` by default, see
[Configuration](#configuration)), so the other CLI is needed to make `agk setup` pass, not to run a
worker. The README names no minimum versions: none is verified.

## Installation

```bash
brew install shurastik/tap/agk
agk setup
```

`agk setup` is machine-level and idempotent; it never touches the repository you run it in. It:

- links every `agk-*` skill into `~/.claude/skills` (`$CLAUDE_CONFIG_DIR/skills`) and
  `~/.codex/skills` (`$CODEX_HOME/skills`); an agent whose config directory does not exist is
  skipped, and a real file or directory in the way is reported as a conflict (exit 1), never
  replaced;
- removes `agk-*` links whose skill is no longer shipped, and the legacy `agterm-codex-review` link;
- adds `.agk/` to your global git ignore file (`core.excludesFile`, else
  `$XDG_CONFIG_HOME/git/ignore`, else `~/.config/git/ignore`), keeping its other lines;
- reports which dependencies it found.

`agk setup --dry-run` prints those actions and changes nothing. Rerun `agk setup` after every
`brew upgrade agk`, because the skills ship with the release. `agk version` prints the installed
version (`dev` for a local build).

The skills come from `--skills-dir`, else `$AGK_SKILLS_DIR`, else the Homebrew install
(`$(brew --prefix)/opt/agk/share/agk/skills`).

From a checkout:

```bash
go build -o .bin/agk ./cmd/agk
.bin/agk setup --skills-dir ./skills
```

Put `.bin/agk` on your `PATH` (or call it by path) so the skills find it.

## Quick start

`agk setup` does not prepare your projects. In the repository you want to work on, set up OpenSpec
first, so the project gets its `openspec-propose`, `openspec-explore`, `openspec-verify-change` and
`openspec-archive-change` skills (`/agk-propose`, `/agk-explore` and `/agk-archive` stop without
doing anything when theirs is missing):

```bash
openspec init --tools claude    # a repository without openspec/; --tools codex for Codex
openspec update                 # a repository that already has openspec/
```

### Through the agent

Open Claude Code (or Codex) in an agterm session in that repository and describe the work:

```
/agk-propose add a --json flag to the export command
```

The agent writes the change, has the reviewer approve it and then opens it for you in revdiff;
annotate what you want changed and quit. When you are happy with the plan, ask for the applier
("implement the change with the applier") and, once it reports, for a review of the diff
("review the diff"). The `agk-apply` and `agk-review` skills run the loops below for you, send the
review findings to the applier and re-review until the review approves.

### With the CLI

The commands print JSON; keep the `dir` they return.

```bash
agk apply start add-json-export          # {"companion":"<id>","dir":"<run dir>","started":true,"worktree":null,"pulled":false}
agk apply wait <run dir>                 # {"status":"done|blocked|needs-human|cancelled|running","round":1,...}

agk review start --base main --for add-json-export   # {"companion":"<id>","dir":"<run dir>","started":true}
agk review wait <run dir>                # {"status":"completed|blocked|needs-human|cancelled|running","round":1,...}
```

`"started": false` means the worker did not report within a minute, usually a folder-trust or login
prompt in its companion: answer it there, then wait. `needs-human` means the worker needs you in its
companion: a question, a permission prompt (`reason` `permission`), or a turn that ended without its
report or findings file (`no-report` / `no-output`). Nothing is recorded; wait again once it is done.

A claude applier often starts long work (a test suite, an eval run) as a background shell and ends
its turn, then resumes by itself when the work finishes. `agk apply wait` does not read that as a
question: the applier's hooks record in `<run dir>/turn.json` whether its turn ended with background
tasks or scheduled wakeups in flight, and while they are, the wait goes on until the report appears.
When the timeout passes during such a pause it prints `{"status":"running","round":1,"reason":"background","background":2}`,
where `background` counts the tasks and wakeups in flight; wait again. A turn that ended with nothing
in flight and no report is still `needs-human` / `no-report`. Codex appliers and runs started by an
older agk have no such hooks and keep deciding from the agent status alone.

A completed review has its findings in `<run dir>/rounds/<round>/review.json`. Send them on and
re-review:

```bash
agk apply send <apply run dir> findings.md    # {"round":2,"started":true}; or --stdin
agk apply wait <apply run dir>
agk review send <review run dir> --note fixed.md   # {"companion":...,"dir":...,"round":2,"started":true}
agk review wait <review run dir>
```

Inside agterm, `send` reports `started` as `start` does. `"started": false` means the line was typed
but the worker never reported the round as started (the round stays recorded and the exit code stays
0). Look at the companion; the stderr warning names the case. If the line sits unsent in the input
box, press Enter there; if a permission prompt is open, answer it deliberately. Then wait as usual.
Before giving up, agk types one Enter of its own when the line is visible on the screen and the
worker is idle. Outside agterm the output has no `started` field.

`agk review start --change add-json-export` reviews the plan of a change instead of a diff.
`--for <change>` only names the change a diff review implements (kebab-case, not together with
`--change`): the review is the same, and the run records the name.

The sidebar names of the companions carry the change, so several changes in flight stay apart:
`<change> · <harness>-apply #N` for the applier, `<change> · <harness>-plan #N` for a plan review
(`--change`), `<change> · <harness>-review #N` for a diff review started with `--for`, and
`<harness>-review #N` for one without. agk names only the companions it creates; your own session
keeps its name.

`agk review fresh <review run dir>` is read-only: it prints `{"fresh":true|false,"round":K,"merge_base":"<sha>"}`,
whether the current diff still matches what the run's current round reviewed. Each diff round records
the merge base it was built from, and `fresh` compares against that one, so committing the reviewed
work, on the base branch too, keeps the round fresh, while an edit, a rebase onto a moved base or
history that no longer contains the recorded merge base makes it stale. A round from an older agk
without a recorded merge base is compared against the merge base of the run's base and `HEAD`. A plan
review, a round without a fingerprint or a recorded merge base missing from the repository exits 1.
When the loop is over:

```bash
agk apply close <apply run dir>
agk review close <review run dir>
```

`agk --help`, `agk apply --help` and `agk review --help` list the commands and their flags; the
timeouts (`--timeout SECONDS`) default to 3600 s for `apply wait`, 600 s for `apply send` and
`review send` (how long they wait for the worker's turn to end before typing the next round) and
1800 s for `review wait`. Exit codes: 0 ok, 1 error.

## Worktree mode

By default the applier works in the checkout you start it from. With worktree mode it gets a git
worktree and a branch of its own, so one change's code is isolated from your edits and from other
changes in flight. It is off by default: set `[applier] worktree = true` (in
`~/.config/agk/config.toml` to have it in every repository), or pass `--worktree` /
`--no-worktree` to one `agk apply start` (both at once is an error).

`agk apply start <change>` in worktree mode, run from the main checkout:

1. refuses, before anything changes, a detached HEAD (the checked-out branch is the *base*), an
   existing branch `<change>` that has no worktree, and a `.worktrees/<change>` that is not a
   worktree of branch `<change>`;
2. checks that the change is ready to apply;
3. updates the base with `git pull --ff-only`. The pull is skipped when the base has no upstream or
   `--no-pull` is given (offline work), and a failed pull (a diverged base, an unreachable remote,
   local edits in the way) stops the start with git's message. After a pull the change is checked
   again, since the pull may have finished or removed it;
4. commits the change's artifacts, `openspec/changes/<change>/`, on the base with the message
   `docs(openspec): plan <change>`. The commit holds only those paths: your other staged or
   unstaged edits stay as they are, hooks run normally, and nothing is committed when the artifacts
   are committed already. Parallel sessions then see the change on the base branch;
5. creates the worktree `.worktrees/<change>` on a new branch `<change>` at the base tip, records
   the base in `git config branch.<change>.agkBase` and hides `.worktrees/` through the
   repository's `info/exclude`;
6. runs the applier, its run directory (`.agk/applies/<run>/` under the worktree) and its companion
   there.

stdout gains `"worktree"` (the worktree's absolute path, or `null` when the mode is off or the
start runs in some other linked worktree) and `"pulled"` (whether a pull ran). A second
`agk apply start` for the same change reuses the existing worktree: no pull, no plan commit, a new
run directory under it. Started from inside a linked worktree, the command creates nothing and runs
the applier right there. agk commits to your base branch only in this mode (the plan commit), and
never pushes.

### agk sync and agk finish

Both take the change name and work from the main checkout or from any worktree of the repository.
They refuse, changing nothing, unless `.worktrees/<change>` is a worktree of branch `<change>`,
`branch.<change>.agkBase` is set and the main checkout has that base branch checked out. (`agk
finish` makes one exception, for a rerun after the merge, below.)

`agk sync <change> [--no-pull]` brings the branch up to date with its base. It needs a clean
worktree (files under the ignored `.agk/` do not count), pulls the base with `git pull --ff-only`
under the same rules as `apply start`, then rebases branch `<change>` onto the base in the worktree.
It prints `{"change","base","pulled","rebased","head"}`; `rebased` says whether the branch tip
moved. A failed pull stops it before the rebase. A rebase conflict aborts the rebase, leaves the
branch exactly as it was, names the conflicting files and exits 1; resolving it is up to you.

The rebase is its own command, and runs *before* the review and verify gates, because a rebase
without a textual conflict can still break the build (a call to a function whose signature changed
upstream). `/agk-archive` therefore syncs first, runs the gates on the rebased code, and syncs again
before archiving: only a tree the gates saw gets merged.

`agk finish <change>` merges a finished change and removes its worktree. It refuses unless the
worktree is clean, the change is archived on its branch (`openspec/changes/<change>/` gone,
`openspec/changes/archive/<date>-<change>/` committed) and the base has no commits the branch lacks
(it names `agk sync`). Then:

1. it fast-forwards the base in the main checkout with `git merge --ff-only`. It never pulls or
   rebases, so the merged tree is exactly the one the gates checked, and the history stays linear;
2. it closes the companion of every run under the worktree's `.agk/applies/` and `.agk/reviews/`
   (a companion that is already gone counts as closed);
3. it removes the worktree, without forcing, and deletes branch `<change>` with `git branch -d`
   (which also drops `agkBase`);
4. it prints `{"change","base","head","closed"}`: the base branch, its new tip and the run
   directories it closed.

A refusal or a failed fast-forward (for example an uncommitted edit of yours to a file the branch
changes) changes nothing. When a cleanup step fails, the merge stays, the remaining steps are
skipped and the error names what is left. Fix the cause and run `agk finish <change>` again: when
the branch is already contained in the base it skips the merge (and the sync check, since the base
may have moved on), does only the cleanup still due, and treats a worktree that is already gone as
removed. The run logs under the removed worktree's `.agk/` go with it; the code and the archive are
in git history. agk never pushes: pushing the base is yours.

## Configuration

Which harness, model and options each worker runs with comes from, strongest first:

1. the flags `--applier` and `--reviewer` (`agk apply start`, `agk review start`, `agk config`);
2. `.agk.toml` in the repository root;
3. `~/.config/agk/config.toml` (`$XDG_CONFIG_HOME/agk/config.toml` when `XDG_CONFIG_HOME` is an
   absolute path);
4. the defaults: applier `claude` with `permission_mode = "auto"`, reviewer `codex`.

```toml
[applier]
harness = "claude"             # claude | codex
model = "claude-sonnet-5-5"    # optional; the harness default otherwise
effort = "high"                # optional
permission_mode = "auto"       # claude applier only; default auto
extra_args = ["--add-dir", "../shared"]
worktree = false               # applier only; default false: implement in a git worktree

[reviewer]
harness = "codex"
model = "gpt-5.5"
```

Only the tables `[applier]` and `[reviewer]` and the keys above are accepted; anything else is an
error. Values must not contain a backslash or a line break. `permission_mode` exists only on a
`claude` applier. `worktree` exists only in `[applier]`, for either harness: `true` turns on
[worktree mode](#worktree-mode) for `agk apply start` (the flags `--worktree` and `--no-worktree`
override it for one run). It defaults to `false`, so nothing changes until you set it; put
`worktree = true` in `~/.config/agk/config.toml` to have it in every repository.

`agk config` prints the resolved settings of both workers as JSON, with the source of every value
(`flag`, a file path or `default`), so you can see which layer won:

```bash
agk config
agk config --reviewer claude:claude-opus-5-5
```

Two things to know:

- **Switching the harness drops the options of the other one.** A layer that sets `harness` to a
  different harness than the one below it discards `model`, `effort` and `extra_args` set by the
  lower layers (and `permission_mode` when the new harness is not a `claude` applier). Set
  `harness` together with the options it should keep in the same layer. `worktree` is not tied to
  a harness and survives the switch.
- **`--applier` and `--reviewer` take `harness[:model]` and nothing else.** `--reviewer
  codex:gpt-5.5` sets the harness and the model; effort, `extra_args` and `permission_mode` come
  from a config file.
- **The reviewer's `extra_args` cannot widen what it may do.** `agk review start` refuses ones that
  change its sandbox, approvals, permission mode, allowed tools, extra directories or settings
  (for example `--sandbox`, `-c sandbox_mode=…`, `--dangerously-skip-permissions`, `--add-dir`).

`agk` warns on stderr when the applier and the reviewer are the same harness and model: such a
review is not independent.

## Development

```bash
go build -o .bin/agk ./cmd/agk   # build
go test ./...                    # unit tests
go vet ./...                     # vet
scripts/eval-skills.sh           # skill evals
```

The unit tests never start agterm, claude or codex: every external call goes through an interface
with a fake. The skill evals run `claude plugin eval` (Claude Code 2.1.319 or newer) with real
Claude children on your account, so **they spend API usage**; they are not part of `go test`. See
`evals/README.md`.

Releases: `scripts/release.sh X.Y.Z` tags the version and updates the Homebrew formula in the
`shurastik/homebrew-tap` checkout (`$AGK_TAP_DIR`, default `~/Projects/homebrew-tap`).
`AGENTS.md` describes the repository layout and conventions for coding agents; changes are planned
and tracked with OpenSpec under `openspec/`.

## License

MIT, see [LICENSE](LICENSE).
