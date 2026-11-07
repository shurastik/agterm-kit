# agk skill evals

Evals for the `agk-propose` skill, run with `claude plugin eval` (Claude Code 2.1.319+). Every case
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
| `cases/<case>/` | One case per `propose-workflow` scenario: `case.yaml`, `prompt.md`, `scaffold.sh`, `graders/*.md` |
| `case-plugins/<case>/` | The plugin each case loads (`case.yaml` `plugins:`): manifest plus symlinks to `skills/agk-propose` and the stubs that case needs |
| `_fixtures/bin/agk`, `_fixtures/bin/revdiff` | Stand-ins |
| `_fixtures/stubs/openspec-propose`, `_fixtures/stubs/revdiff` | Stub skills |
| `_fixtures/write-change.sh` | Writes the fixed change `add-widget` for the `openspec-propose` stub |
| `_fixtures/outcomes/<case>/` | Per-case review outcomes (`round-<k>.json`) and revdiff annotations (`revdiff-<n>.txt`) |
| `_fixtures/scaffold-common.sh` | Shared scaffold: PATH guard, `.eval/` staging, git repo with an `openspec/` skeleton |
| `_fixtures/test.sh` | Self-test of the stand-ins and the PATH guard (no model, no cost) |

### Stand-ins and stubs

- `agk` appends its argv to `agk-calls.log` in the workspace, plus indented notes the graders match:
  the change's files at `start`, `round <k>: <n> finding(s), verdict <v>` at `wait`, the note lines
  and `changed since round <k>: <files>` at `send`, `close: last verdict <v>` at `close`. It plays the
  case's outcomes chosen by `EVAL_AGK_CASE` (`case.yaml` `execution.env`): `wait` writes
  `rounds/<k>/review.json` from `round-<k>.json`, or an approve when there is none. `apply` is only
  logged.
- `revdiff` logs its argv to the same file, notes whether `--only` covers every change file and
  whether the description file is the run dir's `revdiff-description.md`, and prints the case's
  `revdiff-<n>.txt` for launch n (nothing = the user quit without annotations).
- The `openspec-propose` stub runs `.eval/write-change.sh` and stops like the real skill; the
  `revdiff` stub tells the agent to run `revdiff` in the foreground and read its stdout as annotations.
- "Project without OpenSpec skills" leaves the `openspec-propose` stub out of its case plugin,
  "revdiff missing" the `revdiff` stub.

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

Baseline on the current skill, 3 runs per case (2026-11-07, claude-opus-5-5, sonnet judge): every
case scored 1.00 (3/3).

| Case | Score | Cost of 3 runs (USD) |
|------|-------|----------------------|
| annotations-changed-plan | 1.00 | 0.98 |
| disputed-finding | 1.00 | 0.57 |
| findings-first-round | 1.00 | 0.94 |
| no-openspec-skills | 1.00 | 0.33 |
| nothing-changed | 1.00 | 0.70 |
| plain-propose-request | 1.00 | 0.32 |
| plan-done | 1.00 | 0.69 |
| revdiff-available | 1.00 | 0.69 |
| revdiff-missing | 1.00 | 0.60 |
| review-follows-artifacts | 1.00 | 0.73 |

A full 3-run pass costs about 7 USD and takes a few minutes with `--concurrency 4`; one run of a
case costs 0.10-0.35 USD. The threshold is 0.8: it tolerates one flaky grader in one of three runs.

Broken-skill check (task 8.4): with steps 3 and 4 removed from `skills/agk-propose/SKILL.md` and the
reviewer closed right after the first approve, `annotations-changed-plan` scored 0 in all 3 runs (no
revdiff, no send, closed after round 1) and `scripts/eval-skills.sh` exited 1. Leaving steps 3 and 4 in place while only saying "close right away" did
not break the behavior: the agent followed the remaining steps and scored 1.
