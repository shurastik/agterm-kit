# Design

## Context

See proposal.md for why. The relevant mechanics today:

- `review.Collect(ctx, dir, base)` resolves `merge-base <base> HEAD` into `Scope.MergeBase`, then
  collects the diff, the untracked files and `Scope.Fingerprint` against it. `fingerprintOf` hashes
  the merge base id first, then one line per in-scope path with its kind, mode and content. Whether
  a change is committed, staged or untracked does not enter.
- `initRound(dir, k, prompt, fingerprint)` writes `rounds/<k>/scope.sha256` (diff rounds only), then
  `prompt.md`, then the `round` file. `reviewScope`/`reviewPrompt` hand the fingerprint up from the
  scope; the merge base stays inside the prompt.
- `ReviewFresh` reloads the run, reads the current round's `scope.sha256`, calls
  `e.Collect(ctx, run.root, run.state.Base)` and compares fingerprints. `ErrNothingToReview` maps to
  `fresh: false`.
- The `agk-archive` fallback runs `agk review start --base <base> --for <change>`. On
  `nothing to review` it asks the user for a base.
- The eval stand-in `evals/_fixtures/bin/agk` plays `review fresh` with its own fingerprint over
  `git diff <base>`.

## Goals / Non-Goals

**Goals:**
- A commit that keeps the reviewed tree keeps the round fresh, on any branch, the base branch included.
- An edit, a rebase or merge that brings other content in, or a history that drops the recorded
  merge base (an unrelated one included) still reads stale.
- Rounds recorded by an older agk keep their current behavior.

**Non-Goals:**
- Changing what a diff round reviews or how `agk review start --base` chooses its merge base.
- Teaching `/agk-archive` to pick a base on its own in a fresh session without a run of its own.
- Changing the worktree-mode flow (`agk sync`, `agk finish`, the recorded `agkBase`).

## Decisions

### Record the merge base in its own round file

`rounds/<k>/merge-base` holds the full commit id, one line, written by `initRound` right after
`scope.sha256` and before `prompt.md`, through `fsx.ReplaceFile` like the others. The merge base
travels from the scope next to the fingerprint (`reviewScope` and `reviewPrompt` return it; plan
rounds return an empty one and write no file).

Alternative: a second line in `scope.sha256`. Rejected: it changes the format of a file that older
agk and the docs read as one hash, and a reader of a mixed run would need to guess the format.
Alternative: one merge base per run in `worker.json`. Rejected: `agk review send` collects again,
and a rebase between rounds changes the merge base, so it belongs to the round.

### Fresh collects against the recorded commit id

`ReviewFresh` reads `rounds/<k>/merge-base`, first asks whether that commit is an ancestor of
`HEAD`, and only then passes the id as the base to the same `e.Collect`:

```
M missing from the repo      -> error                     -> exit 1 (the gate counts it as unconfirmed)
M not an ancestor of HEAD    -> fresh:false, merge_base M -> no Collect
  (other branch, rewritten history, unrelated history / orphan branch)
M ancestor of HEAD           -> Collect(root, M): merge base is M -> fingerprint over diff(M -> worktree)
```

The ancestry check is a new function in `internal/review`, next to `Collect` and tested the same
way on real temporary repositories: `git cat-file -e <M>^{commit}` failing is an error, then
`git merge-base --is-ancestor <M> HEAD` answers (exit 0 yes, exit 1 no, anything else an error). It
is injected into `Env` like `Collect`, so the `internal/cli` tests script it. The check is needed
because `merge-base <M> HEAD` has no answer for unrelated histories: git exits 1 without a commit,
and `Collect` turns that into an error, not a stale result.

A rebase onto a moved base keeps `M` an ancestor, but `diff(M -> worktree)` now holds the upstream
commits as well, so the path set or contents differ and the round is stale. That is what the
archive gate needs: a rebased tree was not reviewed.

Alternative: keep recomputing from the run's base and add a special case for "HEAD is the merge
base". Rejected: it only covers the base branch and still needs its own rule for rebases.
Alternative: no ancestry check, relying on `merge-base <M> HEAD` resolving another commit whose
hash cannot match. Rejected: it holds for diverged history but not for unrelated history, where
`fresh` would exit 1 instead of answering `fresh: false`.

### Old rounds fall back to the run's base

A round with `scope.sha256` and no `merge-base` is collected against `run.state.Base`, as today, and
`merge_base` prints the merge base that collection resolved. Rejected alternative: exit 1 like a
round without a fingerprint. That would turn every in-flight run into an error on upgrade for a
file that only improves one case.

### `merge_base` in the output

`freshResult` gains `MergeBase string \`json:"merge_base"\``: the recorded id, or for an old round
the one `Collect` resolved. On `ErrNothingToReview` the recorded id is still known; for an old
round it is the one resolved before the scope came back empty, so `Collect`'s error path needs to
keep it available (return the partial scope with the error, or resolve the merge base in
`ReviewFresh` first). The implementation picks the smaller change; the spec only fixes the output.

### The archive fallback retries once, outside a change worktree

In step 3 of `skills/agk-archive/SKILL.md`, after `nothing to review` and not in a change worktree,
the skill takes the `merge_base` from the `agk review fresh` output it already got for this
session's own diff review run of the change (step 2). If there is one and it differs from the base
it just tried, it runs `agk review start --base <merge_base> --for <change>` once and continues as
with any fallback round. Otherwise, or on a second `nothing to review`, it asks the user as today.

The retry is not the first choice: in worktree mode, after a rebasing `agk sync`, the recorded
merge base is the old base, and a review from it would include the upstream commits. In a change
worktree the recorded `agkBase` stays the only base. On a feature branch `--base main` already has
a scope, so the retry never runs there.

### The eval stand-in records the merge base too

`evals/_fixtures/bin/agk` writes `rounds/<k>/merge-base` (the resolved `git merge-base <base> HEAD`)
and `review fresh` fingerprints against it and prints `merge_base`. A new case plays the retry:
review on `main`, approve, edit, commit everything on `main`, `/agk-archive`.

## Risks / Trade-offs

- [The retry at the recorded merge base on `main` also reviews other people's commits pulled
  between `M` and `HEAD`] → acceptable: that is the base the user would name when asked; the
  reviewer sees the whole scope in its prompt, and the skill reports what it reviewed against.
- [A pruned or garbage-collected merge base makes `fresh` exit 1] → the gate treats an error as
  unconfirmed and runs the fallback; nothing archives on stale code.
- [`merge_base` changes the JSON shape of `fresh`] → an added field; the skills and README read
  `fresh` and `round` only, and both keep their meaning.

## Migration Plan

None beyond shipping: new rounds write `merge-base`, old rounds use the fallback. Rollback is a
downgrade; an older agk ignores the extra file.
