# Tasks

## 1. Collect and ancestry against a recorded merge base (real git)

- [x] 1.1 In `internal/review/scope_test.go` (or `fingerprint_test.go`) add real-git tests that pin
  the property the design relies on, before touching the CLI: (a) on `main` with uncommitted
  changes, `Collect(root, "main")` gives merge base `M` and fingerprint `F`; after committing the
  same content on `main`, `Collect(root, M)` returns merge base `M` and fingerprint `F`; (b) a
  feature branch rebased onto a `main` that gained a commit touching another file: `Collect(root, M)`
  gives a different fingerprint. Verify with `go test ./internal/review/ -run 'Collect.*MergeBase'`;
  if they pass without code changes, the property holds and `Collect` stays as it is
- [x] 1.2 RED: real-git tests for a new ancestry function in `internal/review` (name it in the
  implementation, for example `IsAncestor(ctx, dir, commit) (bool, error)`): `true` for an
  ancestor of `HEAD` and for `HEAD` itself; `false` with no error after `HEAD` moved to a diverged
  history that shares older commits, and to an unrelated orphan branch while the commit still
  exists; an error naming the commit when it does not exist in the repository. Verify the tests
  fail with `go test ./internal/review/ -run Ancestor`
- [x] 1.3 GREEN: implement it with `git cat-file -e <commit>^{commit}` (failure is the "missing"
  error) and `git merge-base --is-ancestor <commit> HEAD` (exit 0 true, exit 1 false, anything
  else an error), and add it to `Env` as an injected func next to `Collect`, wired to the real one
  in the production `Env`. Verify `go test ./internal/review/ ./internal/cli/` passes

## 2. Record the merge base per diff round

- [x] 2.1 RED: in `internal/cli` tests, assert that `agk review start --base main` and
  `agk review send` on a diff run write `rounds/<k>/merge-base` holding the scope's merge base
  before the reviewer starts (alongside `scope.sha256`), that round 2 after a different scripted
  merge base records its own, and that a plan run (`--change`) writes no `merge-base`. Verify the
  new tests fail with `go test ./internal/cli/ -run 'MergeBase'`
- [x] 2.2 GREEN: carry the merge base from `reviewScope`/`reviewPrompt` to `initRound` and write
  `rounds/<k>/merge-base` with `fsx.ReplaceFile` right after `scope.sha256` (diff rounds only);
  extend the rollback of a failed round to remove it like the other round files if the existing
  rollback does not already remove the round directory. Verify `go test ./internal/cli/` passes

## 3. `agk review fresh` compares against the recorded merge base

- [x] 3.1 RED: in `internal/cli/review_fresh_test.go`, with the ancestry func and `Collect`
  scripted, assert: when `rounds/<k>/merge-base` exists, `fresh` checks ancestry of that id and
  calls `Collect` with it (not the run's base); a scripted "not an ancestor" prints
  `{"fresh":false,"round":K,"merge_base":"<M>"}`, exits 0 and calls no `Collect`; a scripted
  ancestry error exits 1 with no JSON; without the file it calls `Collect` with the run's base and
  no ancestry check; `merge_base` is the recorded id, the id resolved for an old round, and the id
  on the `nothing to review` path. Keep the existing read-only, refusal and usage tests green.
  Verify the new tests fail with `go test ./internal/cli/ -run ReviewFresh`
- [x] 3.2 GREEN: in `ReviewFresh` read the recorded merge base, run the ancestry check, then pass
  the id to `e.Collect`; add `MergeBase string \`json:"merge_base"\`` to `freshResult`, and make the
  merge base available on the `ErrNothingToReview` path for an old round (smallest change: resolve
  it in `ReviewFresh` or have `Collect` return the partial scope with the error). Verify
  `go test ./internal/cli/ ./internal/review/` passes
- [x] 3.3 End to end in `internal/cli` with the real `review.Collect` on a temporary git repository
  (`h.env.Collect = review.Collect`, as `review_send_test.go` already does for a plan run): on
  `main` with an uncommitted file, start a diff review (inline harness), commit the file on `main`
  without changing it, run `agk review fresh` and get `"fresh":true` with the recorded
  `merge_base`; then edit the file and get `"fresh":false`; then check out an orphan branch and get
  `"fresh":false` with exit 0. Verify with `go test ./internal/cli/ -run ReviewFresh`
- [x] 3.4 Docs: update the `agk review fresh` line in `AGENTS.md` (Commands) and the paragraph in
  `README.md` to the new output `{"fresh":…,"round":K,"merge_base":"<sha>"}`, the comparison
  against the round's recorded merge base, and that a commit on the base branch keeps a round
  fresh while an edit or a rebase does not. Add `merge-base` next to `scope.sha256` in the
  `AGENTS.md` run-artifact paragraph. Verify with `grep -n merge_base README.md AGENTS.md`

## 4. `/agk-archive` retries at the recorded merge base

- [x] 4.1 Eval stand-in: in `evals/_fixtures/bin/agk` record `rounds/<k>/merge-base`
  (`git merge-base <base> HEAD`) for each diff round at `review start` and `review send`, make
  `review fresh` fingerprint against the recorded merge base (falling back to the run's base when
  the file is missing) and print `"merge_base"`. Verify `evals/_fixtures/test.sh` passes and the
  existing archive cases' scaffolds still run
- [x] 4.2 RED: add the eval case `evals/cases/merge-base-retry/` with its case plugin
  `evals/case-plugins/merge-base-retry/` (same skill links as `file-added-after-approve`): the
  workspace stays on `main` with uncommitted `widget.go`; the prompt runs
  `agk review start --base main` and `agk review wait` for `add-widget`, then appends a line to
  `widget.go`, commits everything on `main`, then `/agk-archive add-widget`. Graders: the log shows
  `agk review start --base <40-hex sha> --for add-widget` after a failed
  `agk review start --base main`, followed by verify and archive; the last message does not ask
  for a base. Add a case where the retry is not possible to cover "Retry at the merge base also
  has nothing to review" (for example the stand-in outcome marker makes the retry answer
  `nothing to review`) and grade that the user is asked and nothing is archived. Run
  `scripts/eval-skills.sh --case merge-base-retry --runs 1` against the unchanged skill and confirm
  it fails
- [x] 4.3 GREEN: in `skills/agk-archive/SKILL.md` step 3, on `nothing to review` outside a change
  worktree, retry once with `--base <merge_base>` taken from the `agk review fresh` output of this
  session's own diff review run for the change, when it exists and differs from the base just
  tried; ask the user otherwise or after a second `nothing to review`. Fix the step 3 intro and the
  Notes so they no longer say a plain commit never moves the merge base (it is the recorded merge
  base that keeps it fresh), and keep the change-worktree base rule as is. Verify
  `scripts/eval-skills.sh --case merge-base-retry` passes the threshold, and the cases
  `nothing-to-review`, `file-added-after-approve`, `fresh-session-archive` and
  `worktree-verified-then-rebased` still pass with `--runs 1`
- [x] 4.4 Docs: update the `/agk-archive` description in `README.md` and `AGENTS.md` if they
  describe the fallback's `nothing to review` handling, and list the new case in `evals/README.md`
  if it enumerates cases. Verify with `grep -n 'nothing to review' README.md AGENTS.md evals/README.md`

## 5. Backlog and integration

- [x] 5.1 Remove the entry "`agk review fresh` goes stale after committing on the base branch" from
  `BACKLOG.md`; verify with `grep -c 'goes stale after committing' BACKLOG.md` printing 0
- [x] 5.2 Run `go build -o .bin/agk ./cmd/agk && go vet ./... && go test ./...` and
  `openspec validate review-fresh-recorded-merge-base --strict`; all pass

## Workflow follow-up

- Codex review of the diff until clean, then `/opsx:verify` with 0 CRITICAL.
- Archive with `/agk-archive review-fresh-recorded-merge-base`.
