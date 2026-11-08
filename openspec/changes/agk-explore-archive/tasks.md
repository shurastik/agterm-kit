# Tasks

## 1. Scope fingerprint and agk review fresh

- [x] 1.1 Write failing tests in `internal/review/scope_test.go` (real temp git repos, like the existing `TestCollect*`) for a `Scope.Fingerprint` returned by `Collect` (design decision 10):
  - identical for an untracked file before and after committing it unchanged;
  - identical for a renamed tracked file before staging, after `git add -A` and after the commit (no content change);
  - different after a one-byte content edit with the same file list;
  - different after adding an untracked file or deleting an in-scope file;
  - different when the merge base moves;
  - different after a chmod that drops the executable bit, and after replacing a file with a symlink whose target text equals the file's bytes;
  - with an updated submodule: `Collect` still succeeds (alongside `TestCollectSeesSubmoduleUpdateDespiteIgnoreConfig`), and the fingerprint changes after a new commit in the submodule, after an uncommitted edit in it, and after a second edit of an already modified tracked file and of an existing untracked file in it (same HEAD, same status lines);
  - set for omitted untracked files too (binary, too large, symlink);
  - consistent snapshot: with the test hook editing a file once between the first fingerprint and the content reads, the result has the edited content and the matching fingerprint; with the hook editing on every attempt, `Collect` fails with the "changed during collection" error.

  Verify `go test ./internal/review -run Fingerprint` fails
- [x] 1.2 Implement the fingerprint in `internal/review/scope.go` (design decision 10):
  - SHA-256 over the merge base and the sorted lines `path NUL kind NUL mode NUL value` for the union of `git diff --name-only --no-renames` (same `DiffFlags` and `.agk` exclusion; also in the submodule pass) and the untracked list;
  - kinds `file` (mode 644/755, content hash), `symlink` (target), `submodule` (HEAD plus the fingerprint computed recursively inside it against its HEAD, or `uninitialized`) and `deleted`;
  - before/after fingerprints around the content collection, at most three attempts, and the unexported test hook.

  Verify the 1.1 tests and the existing `go test ./internal/review` pass
- [x] 1.3 Write failing tests in `internal/cli/review_start_test.go` and `review_send_test.go` with the faked `Env.Collect`: a diff round writes `rounds/<k>/scope.sha256` holding the faked scope's fingerprint before the reviewer starts; a plan round writes none. Verify they fail, then write the file next to `prompt.md` for diff rounds in `internal/cli/review.go`, and verify they pass
- [x] 1.4 Write failing tests in a new `internal/cli/review_fresh_test.go` for `agk review fresh <dir>`:
  - `{"fresh":true,"round":K}` when the faked scope matches the current round;
  - `fresh:false` when it differs;
  - `fresh:false` on `ErrNothingToReview`;
  - exit 1 with no JSON for a plan run, for a round without `scope.sha256`, and for a bad run dir;
  - no file in the run dir changes and the fake `Ctl` gets no call.

  Verify they fail, then implement `ReviewFresh` and its `fresh` dispatch in `internal/cli/main.go` (plus usage text), and verify `go test ./internal/cli` passes
- [x] 1.5 Document `agk review fresh <dir>` in `AGENTS.md` (Usage block) and in `skills/agk-review/SKILL.md` (one line: what it answers and that it is read-only); verify `grep -n "review fresh" AGENTS.md skills/agk-review/SKILL.md` shows both and `go build ./... && go vet ./...` pass

## 2. agk-explore skill

- [x] 2.1 Add `agk-explore` to the expected skills in `internal/setup/repo_skills_test.go`; verify `go test ./internal/setup -run TestRepoSkillsHaveMatchingNames` fails with `skill agk-explore missing`
- [x] 2.2 Write `skills/agk-explore/SKILL.md` (spec `explore-workflow`):
  - front matter `name: agk-explore`, with a description that says "Use only when explicitly invoked";
  - the `openspec-explore` precondition: look in the skill list, a plugin namespace or `.claude/skills/`, and give `openspec init`/`openspec update` advice when it is missing;
  - follow `openspec-explore` unchanged;
  - at every handoff name `/agk-propose`, never `/opsx:propose`, and never start it.

  Verify the test from 2.1 passes
- [x] 2.3 Add the stub `evals/_fixtures/stubs/openspec-explore/SKILL.md`: explore stance, no writes, and an ending that suggests `/opsx:propose` (design decision 9). Verify `bash evals/_fixtures/test.sh` still passes
- [x] 2.4 Add the cases `plain-explore-request`, `explore-no-openspec-skills` and `idea-settled` (`case.yaml`, `prompt.md`, `scaffold.sh`, `graders/`), with their `case-plugins/<case>/` symlinks. The no-skills case leaves out the stub. The `idea-settled` graders check:
  - `/agk-propose` in the last message;
  - no `/opsx:propose`;
  - no `agk` line in `agk-calls.log`;
  - no write outside `openspec/changes/`.

  Verify `scripts/eval-skills.sh --case <case> --runs 1` scores at least 0.8 for each
- [x] 2.5 Document `/agk-explore` in `AGENTS.md` (Overview and the skills paragraph) and add the three cases to `evals/README.md` (layout and the scenario-to-case table); verify `grep -n agk-explore AGENTS.md evals/README.md` shows both

## 3. Eval fixtures for the archive cases

- [x] 3.1 Extend the stand-in `evals/_fixtures/bin/agk`:
  - `review start --base <ref>` creates a run dir with a `base` file and prints `{"companion","dir"}`. It answers `nothing to review` (exit 1) when the workspace has no diff against the ref and no untracked file;
  - `run_dir` accepts a `base` or a `change` file, and `review send` works on a `base` run (no change-dir check), so `later-round-findings` can run a second round;
  - diff `start` and `send` store a shell fingerprint of the workspace in the round (`git diff <base>` plus each untracked path and content), and `review fresh <dir>` recomputes it and prints `{"fresh","round"}` (design decision 9);
  - with `outcomes/<case>/edit-during-review`, `review wait` appends a line to the code file before returning;
  - `apply close <dir>` and `review close <dir>` log their run dir.

  Add matching checks to `evals/_fixtures/test.sh`, including `fresh` turning false after a new file. Verify it passes
- [x] 3.2 Add `evals/_fixtures/verify.sh` and `evals/_fixtures/archive.sh`, staged into `.eval/` by the scaffold:
  - `verify.sh <change>` logs `verify <change>` and prints the check results from `outcomes/<case>/verify.txt`, or all-pass results when there is none. The stub, like the real skill, has the agent write the report itself;
  - `archive.sh <change>` logs `archive <change>`. It moves the change under `openspec/changes/archive/`, or exits 1 when `outcomes/<case>/archive-fail` exists.

  Add the stubs `openspec-verify-change` and `openspec-archive-change` that run them. Verify `test.sh` covers both scripts and passes
- [x] 3.3 Add `evals/_fixtures/scaffold-archive.sh`, which the archive cases' `scaffold.sh` runs after `scaffold-common.sh`. Its options:
  - write the change `add-widget`, committed;
  - leave an uncommitted code file, or none for `nothing-to-review`;
  - pre-create `.agk/applies/<n>/` and `.agk/reviews/<n>/` with `worker.json` recording a given change.

  Verify `test.sh` stages a workspace with each option and checks the result

## 4. agk-archive skill

- [x] 4.1 Add `agk-archive` to the expected skills in `internal/setup/repo_skills_test.go`; verify the test fails with `skill agk-archive missing`
- [x] 4.2 Write `skills/agk-archive/SKILL.md` (spec `archive-workflow`, design decisions 3-8):
  - front matter "Use only when explicitly invoked";
  - preconditions: `openspec-archive-change`, `openspec-verify-change`, and `agk` on `PATH`;
  - change selection;
  - review gate: the latest diff review round this session ran for the change approved, and `agk review fresh <dir>` reports `fresh: true`. A later round with findings cancels an earlier approve and stops the skill. `fresh: false` or a `fresh` error counts as unconfirmed. The fallback is `agk review start --base <base>` + `agk review wait`, then `agk review fresh` on the approving round. It stops on findings with the reviewer left open, stops on `fresh: false` or a `fresh` error, and asks on `nothing to review`;
  - verify gate: a report the agent produced in this session by following `openspec-verify-change` for the change. The fallback runs that skill, stops on CRITICAL, and only lists WARNING/SUGGESTION;
  - archive through `openspec-archive-change`;
  - after a successful archive, close the apply and review runs whose `worker.json` records the change, plus the diff review runs this session ran for this change. Runs of other or unclear changes stay open;
  - final report, with no commit, push or fixes.

  Verify the test from 4.1 passes
- [x] 4.3 Add the archive cases, with their case plugins and outcomes (design decision 9 table): `plain-archive-request`, `archive-no-openspec-skills`, `fresh-session-archive`, `approved-in-session`, `file-added-after-approve`, `later-round-findings`, `two-changes-in-session`, `verified-in-session`, `changed-during-review`, `fallback-findings`, `nothing-to-review`, `verify-critical`, `verify-warnings` and `archive-failed`. The graders match `agk-calls.log`:
  - ordering: review start before verify before archive;
  - `approved-in-session`: one `agk review fresh` and one `agk review start`;
  - `file-added-after-approve` and `two-changes-in-session`: two `agk review start`;
  - `later-round-findings` and `changed-during-review`: no `verify` and no `archive`;
  - `verified-in-session`: one `verify`;
  - `two-changes-in-session`: `agk review close` names only the fallback run dir.

  Verify `scripts/eval-skills.sh --case <case> --runs 1` scores at least 0.8 for each
- [x] 4.4 Broken-skill check: put a copy of `skills/agk-archive/SKILL.md` with the verify gate removed in place of the original, run `scripts/eval-skills.sh --case verify-critical --runs 3`, and check that it scores below the threshold and the script exits 1. Restore the skill and record the result in `evals/README.md`. Verify `git diff skills/agk-archive` is empty after the restore
- [x] 4.5 Document `/agk-archive` in `AGENTS.md` (Overview, the skills paragraph, and the `evals/` layout row, which now names all three workflow skills), and add the archive cases to `evals/README.md`. Verify `grep -n agk-archive AGENTS.md evals/README.md` shows both
- [x] 4.6 Fingerprint and skill follow-ups from the code review: an uninitialized submodule's fingerprint value includes its gitlink SHA (index, else merge base), the executable bit follows git's owner-bit rule (`0o100`), and `agk-archive` re-runs `agk review fresh` on the approving round after the verify gate and right before the archive (spec scenario "Code changed during verify"). Add the eval case `changed-during-verify` (`verify.sh` appends a line to `widget.go` under the `edit-during-verify` marker) with its case plugin; verify `scripts/eval-skills.sh --case changed-during-verify --runs 3` scores at least 0.8 and record the baseline in `evals/README.md`

## 5. Integration

- [x] 5.1 Run the whole suite (`scripts/eval-skills.sh`, 3 runs per case) and record the new baseline and the cost per case in `evals/README.md`. Verify every case scores at least 0.8, the existing `agk-propose` ones included
- [x] 5.2 Run `go test ./...` and `go vet ./...`; verify both pass
- [x] 5.3 If `README.md` exists (the `readme` change has landed), add `/agk-explore`, `/agk-archive` and `agk review fresh` to it. Otherwise add a line to `openspec/changes/readme/tasks.md` naming them. Verify with `grep -n "agk-archive" README.md openspec/changes/readme/tasks.md`
- [x] 5.4 Point the user's `~/.claude/CLAUDE.md` workflow at the new skills: step 1 becomes `/agk-explore`, and step 6 becomes `/agk-archive`, without `bd close`. Verify `grep -n "agk-explore\|agk-archive" ~/.claude/CLAUDE.md`

## Workflow follow-up

- Codex review of the diff, then `/opsx:verify` with 0 CRITICAL.
- Archive the change with `/agk-archive` itself, once `agk setup --skills-dir ./skills` has linked it.
