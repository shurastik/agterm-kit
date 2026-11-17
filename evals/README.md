# agk skill evals

Evals for the `agk-propose`, `agk-explore` and `agk-archive` skills, run with `claude plugin eval` (Claude Code 2.1.319+). Every case
runs a real Claude child on your account against stand-ins for `agk` and `revdiff`: no real reviewer,
applier, agterm companion or revdiff TUI is ever started. The suite is not part of `go test` or the
release.

```bash
scripts/eval-skills.sh                                   # whole suite, 3 runs per case
scripts/eval-skills.sh --case plan-done --runs 1         # one case while iterating
AGK_EVAL_MAX_COST_USD=5 scripts/eval-skills.sh --case findings-first-round
```

The script runs `evals/_fixtures/test.sh` (fixture self-test, free), puts `evals/_fixtures/bin`
first on `PATH`, and runs `claude plugin eval evals --scaffold --trust-plugin --no-publish
--ablation none --allow-tools Bash Write Edit --judge-model sonnet` with threshold
`AGK_EVAL_THRESHOLD` (default 0.8) and cost ceiling `AGK_EVAL_MAX_COST_USD` (default 10). Other
arguments go to `claude plugin eval`. Reports land in `evals/cases/results/` (gitignored).

## Layout

| Path | Purpose |
|------|---------|
| `.claude-plugin/plugin.json` | Makes `evals/` the plugin under test; `experimental.evals: "cases"` |
| `skills` | Symlink to `../skills` (the suite plugin's skills) |
| `cases/<case>/` | One case per scenario of a workflow skill (see the table below): `case.yaml`, `prompt.md`, `scaffold.sh`, `graders/*.md` |
| `case-plugins/<case>/` | The plugin each case loads (`case.yaml` `plugins:`): manifest plus symlinks to the skill under test (`skills/agk-propose`, `skills/agk-explore` or `skills/agk-archive`) and the stubs that case needs |
| `_fixtures/bin/agk`, `_fixtures/bin/revdiff` | Stand-ins |
| `_fixtures/bin/git` | Runs the real git binary behind Apple's `/usr/bin/git` shim, which fails in the agent's Bash sandbox; the worktree cases need it because the agent runs git itself there |
| `_fixtures/stubs/openspec-propose`, `openspec-explore`, `openspec-verify-change`, `openspec-archive-change`, `revdiff`, `agk-apply` (under `_fixtures/stubs/`) | Stub skills |
| `_fixtures/write-change.sh` | Writes the fixed change `add-widget` for the `openspec-propose` stub |
| `_fixtures/verify.sh`, `_fixtures/archive.sh` | Check results and archive step for the `openspec-verify-change` and `openspec-archive-change` stubs |
| `_fixtures/scaffold-archive.sh` | Archive-case workspace: the change `add-widget`, an uncommitted code file, pre-created `.agk/` runs; `--worktree` makes the workspace a linked worktree of a main repository next to it (`<workspace>.main`) on branch `add-widget` with `branch.add-widget.agkBase = main`, `--main-branch` stands the workspace repository itself on that branch with the base recorded, `--branch` does the same without a recorded base |
| `_fixtures/outcomes/<case>/` | Per-case outcomes: review rounds (`round-<k>.json`), revdiff annotations (`revdiff-<n>.txt`), verify results (`verify.txt`), markers `edit-during-review`, `edit-during-verify` and `archive-fail`, and the worktree markers `sync-rebased-<n>`, `sync-conflict`, `edit-after-sync-<n>` and `finish-fail` |
| `_fixtures/scaffold-common.sh` | Shared scaffold: PATH guard, `.eval/` staging, git repo with an `openspec/` skeleton |
| `_fixtures/test.sh` | Self-test of the stand-ins and the PATH guard (no model, no cost) |

### Scenarios and cases

| Case | Skill | Scenarios |
|------|-------|-----------|
| `plain-explore-request` | `agk-explore` | Plain explore request |
| `explore-no-openspec-skills` | `agk-explore` | Project without OpenSpec skills |
| `idea-settled` | `agk-explore` | Exploration writes no code, Idea settled |
| `plain-archive-request` | `agk-archive` | Plain archive request |
| `archive-no-openspec-skills` | `agk-archive` | Project without OpenSpec skills |
| `fresh-session-archive` | `agk-archive` | One active change, Fresh session review approves, Gates pass, Applier and plan reviewer still open, Archive done |
| `approved-in-session` | `agk-archive` | Approved earlier in the session |
| `file-added-after-approve` | `agk-archive` | File added after the approve |
| `later-round-findings` | `agk-archive` | Later round found issues |
| `two-changes-in-session` | `agk-archive` | Review of another change, Diff reviewer of another change in the session |
| `verified-in-session` | `agk-archive` | Verified earlier in the session |
| `changed-during-review` | `agk-archive` | Code changed during the fallback review |
| `fallback-findings` | `agk-archive` | Fallback review has findings |
| `nothing-to-review` | `agk-archive` | Nothing to review |
| `merge-base-retry` | `agk-archive` | Nothing to review against the base, retried at the recorded merge base |
| `merge-base-retry-empty` | `agk-archive` | Retry at the merge base also has nothing to review |
| `changed-during-verify` | `agk-archive` | Code changed during verify |
| `verify-critical` | `agk-archive` | Verify reports CRITICAL (also the broken-skill check) |
| `verify-warnings` | `agk-archive` | Verify reports warnings only |
| `archive-failed` | `agk-archive` | Archive failed |
| `worktree-archive` | `agk-archive` | Change worktree detection, Fallback review in a change worktree, Code committed and synced first, Finish succeeds, Archive done in a change worktree |
| `worktree-sync-conflict` | `agk-archive` | Sync conflict |
| `worktree-gate-fails` | `agk-archive` | Gate fails in a change worktree |
| `worktree-verified-then-rebased` | `agk-archive` | Verified before a rebasing sync |
| `worktree-base-moved` | `agk-archive` | Base moved during the gates |
| `worktree-finish-fails` | `agk-archive` | Finish fails |
| `worktree-archive-failed` | `agk-archive` | Archive failed in a change worktree |
| `worktree-code-after-gates` | `agk-archive` | Code changed after the gates |
| `branch-in-main-checkout` | `agk-archive` | Change branch checked out in the main checkout |
| `branch-without-base` | `agk-archive` | Branch without a recorded base |

The worktree cases run in a real linked worktree: the scaffold builds a main repository next to the
workspace (on `main`, with the plan commit) and attaches the workspace to it on branch `add-widget`
with `branch.add-widget.agkBase = main`, which is what the skill detects. Branch state alone would put
the main checkout in the workspace, where a careful agent rightly refuses `agk finish`. The stand-in
`agk` finds the workspace through `git config eval.workspace`, so it logs and plays its outcomes there
even when the skill runs `agk finish` from the main checkout. The child's own `git commit` runs in the
repository (the harness disables git hooks, so the commits are not in `agk-calls.log`): the graders
match them in the trace, in order with the stand-in's calls, which the trace holds too.

In the in-session archive cases the prompt first asks the agent to set the session up (run the
review, run it and verify, create a file, run two rounds, or run a review as `other-change`'s code
review) and then to run the `agk-archive` skill.

### Stand-ins and stubs

- `agk` appends its argv to `agk-calls.log` in the workspace, plus indented notes the graders match:
  the change's files at `start`, `round <k>: <n> finding(s), verdict <v>` at `wait`, the note lines
  and `changed since round <k>: <files>` at `send`, `close: last verdict <v>` at `close`. It plays the
  case's outcomes chosen by `EVAL_AGK_CASE` (`case.yaml` `execution.env`): `wait` writes
  `rounds/<k>/review.json` from `round-<k>.json`, or an approve when there is none. `apply` is only
  logged. `sync <change> [--no-pull]` logs its call and prints `rebased: false`, or `true` on the
  n-th call when the case holds a `sync-rebased-<n>` marker (a rebasing sync also changes the
  workspace fingerprint, as a moved merge base does, so `review fresh` turns false); it exits 1 on a
  `sync-conflict` marker, and refuses uncommitted files and a branch without `agkBase` like the real
  command. `finish <change>` refuses uncommitted files and a change not archived in the committed
  tree, exits 1 on a `finish-fail` marker (the base moved) and otherwise prints its success JSON.
- `revdiff` logs its argv to the same file, notes whether `--only` covers every change file and
  whether the description file is the run dir's `revdiff-description.md`, and prints the case's
  `revdiff-<n>.txt` for launch n (nothing = the user quit without annotations).
- The `openspec-propose` stub runs `.eval/write-change.sh` and stops like the real skill; the
  `revdiff` stub tells the agent to run `revdiff` in the foreground and read its stdout as annotations.
- Diff reviews (`agk review start --base`) create a run dir with a `base` file; `start` and `send`
  store per round the merge base of the base and `HEAD` (`rounds/<k>/merge-base`) and a fingerprint
  of the workspace (each path between the merge base and the working tree with its content, so a
  commit that keeps the content keeps the fingerprint). `review fresh` recomputes it against the
  recorded merge base, answers `false` when that commit is no longer an ancestor of `HEAD`, exits 1
  when it is missing, and prints `merge_base`; a round without the file falls back to the run's base.
  `start` answers `nothing to review` (exit 1) with no diff and no untracked file, and, with the
  `retry-nothing-to-review` marker, also for a `--base` that is a 40-hex commit id (the retry at a merge
  base). With the `edit-during-review` marker, `review wait` appends a line to
  `widget.go` before it returns. `apply close` and `review close` log their run dir. Inside the
  agent's sandbox Apple's `/usr/bin/git` shim fails (xcrun cannot write its cache), so the stand-in
  falls back to the real binary under `/Library/Developer/CommandLineTools`.
- The `openspec-verify-change` stub runs `.eval/verify.sh <change>` (logs `verify <change>`, prints
  the check results of `verify.txt` or all-pass ones; with the `edit-during-verify` marker it first appends a line to `widget.go`) and has the agent write the report itself, like
  the real skill. The `openspec-archive-change` stub runs `.eval/archive.sh <change>` (logs `archive
  <change>`, moves the change under `openspec/changes/archive/`, or fails with `archive-fail`).
- The `openspec-explore` stub sets the explore stance (no writes outside `openspec/changes/`) and
  ends like the stock 2.0 skill, suggesting the `openspec-propose` skill and naming
  `openspec-apply-change`, which `agk-explore` must replace with `/agk-propose`.
- The `openspec-propose` stub runs the scripted change (`.eval/write-change.sh`) and ends like the
  stock 2.0 skill, suggesting the `openspec-apply-change` skill or "ask me to apply this change",
  which `agk-propose` must replace with `/agk-apply <change>`.
- The `agk-apply` stub carries the real skill's name and description, so it is as discoverable as in
  production; its body runs `agk apply start <change>` (only logged). Only the `plan-done` case plugin
  loads it, to check that `agk-propose` does not invoke it.
- "Project without OpenSpec skills" leaves the `openspec-propose` (or `openspec-explore`) stub out
  of its case plugin, "revdiff missing" the `revdiff` stub.

## What the spike settled (task 8.1)

- **Plugin resolution.** The repository root does not resolve as a plugin without a manifest, and
  the eval dir must sit inside the plugin root (`--eval-dir ..` and `experimental.evals: "../"` are
  refused). Hence `evals/` is the plugin, with cases in `evals/cases/`.
- **Stub skills.** Skills in the case workspace's `.claude/skills` are not loaded. `case.yaml`
  `plugins:` loads a plugin but replaces the plugin under test, so each case names a plugin that
  holds `agk-propose` and its stubs. Plugin skills load namespaced (`agk:agk-propose`,
  `agk:openspec-propose`); the skill accepts namespaced names.
- **Case plugins live outside `cases/`.** Each run scans the whole eval dir and refuses a symlink to
  a directory anywhere except inside the current case's own plugins. Case plugins full of symlinks
  inside `cases/<case>/` therefore break every other case, so they live in `case-plugins/<case>/`
  and `case.yaml` names them as `../../case-plugins/<case>`.
- **PATH.** `execution.env` accepts only `EVAL_*` keys, and a workspace `.claude/settings.json`
  `env.PATH` is ignored. The agent inherits the operator's `PATH`, so the script prepends the
  stand-ins. Without it a case would call the real `agk`: every scaffold fails unless `agk` and
  `revdiff` resolve to the stand-ins.
- **Sandbox.** The agent's Bash sandbox cannot read the repository's case and plugin dirs (it can
  execute the stand-ins on `PATH`). The scaffold runs outside the sandbox with the workspace as cwd,
  so it stages what the stubs and stand-ins read into `.eval/` in the workspace.
- **Graders.** `regex` matches `last_message`, `trace` (the tool calls and results) or a workspace
  file (`{source: file, path: agk-calls.log}`); `llm` takes the same `focus` values and votes three
  times; `tool_used` takes `input_match`, `min`, `max` (`arm: both` for "never called");
  `tool_order` compares tool names; `file_exists` sees only files created during the run.

## Baseline and cost

Baseline on the current skills, 3 runs per case (2026-11-08, claude-opus-5-5, sonnet judge): every
case scored 1.00 (3/3), the `agk-propose` ones included. The first pass hit the account's usage
limit part-way, so the cases it left unscored were re-run on their own; the table holds each case's
cost from the pass that scored it.

| Case | Score | Cost of 3 runs (USD) |
|------|-------|----------------------|
| annotations-changed-plan | 1.00 | 1.03 |
| approved-in-session | 1.00 | 0.58 |
| archive-failed | 1.00 | 0.58 |
| archive-no-openspec-skills | 1.00 | 0.32 |
| changed-during-review | 1.00 | 0.46 |
| changed-during-verify | 1.00 | 0.56 |
| disputed-finding | 1.00 | 0.56 |
| explore-no-openspec-skills | 1.00 | 0.27 |
| fallback-findings | 1.00 | 0.46 |
| file-added-after-approve | 1.00 | 0.73 |
| findings-first-round | 1.00 | 0.97 |
| fresh-session-archive | 1.00 | 0.81 |
| idea-settled | 1.00 | 0.46 |
| later-round-findings | 1.00 | 0.44 |
| no-openspec-skills | 1.00 | 0.34 |
| nothing-changed | 1.00 | 0.69 |
| nothing-to-review | 1.00 | 0.37 |
| plain-archive-request | 1.00 | 0.27 |
| plain-explore-request | 1.00 | 0.40 |
| plain-propose-request | 1.00 | 0.31 |
| plan-done | 1.00 | 0.68 |
| revdiff-available | 1.00 | 0.68 |
| revdiff-missing | 1.00 | 0.58 |
| review-follows-artifacts | 1.00 | 0.73 |
| two-changes-in-session | 1.00 | 0.69 |
| verified-in-session | 1.00 | 0.57 |
| verify-critical | 1.00 | 0.53 |
| verify-warnings | 1.00 | 0.60 |

A full 3-run pass costs about 15 USD (27 cases) and takes about 15 minutes with
`--concurrency 4`, so raise `AGK_EVAL_MAX_COST_USD` (default 10) for the whole suite; one run of a
case costs 0.10-0.35 USD. The threshold is 0.8: it tolerates one flaky grader in one of three runs.

Broken-skill check (task 8.4): with steps 3 and 4 removed from `skills/agk-propose/SKILL.md` and the
reviewer closed right after the first approve, `annotations-changed-plan` scored 0 in all 3 runs (no
revdiff, no send, closed after round 1) and `scripts/eval-skills.sh` exited 1. Leaving steps 3 and 4 in place while only saying "close right away" did
not break the behavior: the agent followed the remaining steps and scored 1.

Broken-skill check for `agk-archive` (task 4.4): with the verify gate removed from
`skills/agk-archive/SKILL.md` (the section, and every mention of verify in the description, intro
and preconditions), `verify-critical` scored 0.00 in all 3 runs (the agent never ran verify and archived
the change) and `scripts/eval-skills.sh` exited 1. The original skill was
restored afterwards (checked with `diff` against a copy).

After the code-review follow-ups (fresh check after verify, task 4.6) the new case
`changed-during-verify` scored 1.00 in 3/3 runs, and the archive cases whose skill text changed
were re-run once each at 1.00 (`fresh-session-archive` scored 0.89 once, closing another change's
run; it scored 1.00 in 3/3 runs on the re-run).
