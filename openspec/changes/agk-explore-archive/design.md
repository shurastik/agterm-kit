# Design

## Context

- `agk-propose` set the pattern. It is a skill invoked only by name, it delegates the OpenSpec part
  to the project's generated skill (`openspec-propose`), and it adds only the agk steps around it.
  It finds that skill in three places: the skill list, a plugin namespace, or
  `.claude/skills/<name>/SKILL.md`. It is checked by eval cases in `evals/` that run against a
  stand-in `agk` and stub skills.
- `agk setup` links every `agk-*` directory under the skills dir. New skills need no setup change.
  `internal/setup/repo_skills_test.go` pins the shipped set.
- Run state on disk: `.agk/applies/<run>/worker.json` and plan review runs in
  `.agk/reviews/<run>/worker.json` record `"change"`. Diff review runs record only `"base"`.
  `agk apply close` and `agk review close` are idempotent: no companion, or a companion that is
  already gone, still exits 0.
- `openspec/config.yaml` `operations.archive.guidance` already states the verify precondition, but
  only as advice that `openspec-archive-change` reads.

## Goals / Non-Goals

**Goals:**
- Both skills stay thin. They own only what agk adds, and the OpenSpec behavior stays in the
  project's generated skills, which `openspec update` keeps current.
- The archive gates are safe by default: when in doubt, re-run the review or verify instead of
  trusting a claim.

**Non-Goals:**
- Go changes are limited to the scope fingerprint and `agk review fresh` (decision 10). Diff
  review runs stay unlinked to a change, and there is no `agk archive` subcommand.
- `agk-archive` does not fix findings or verify issues, and it does not commit.
- OpenSpec stores (`--store`) are not supported, as with `agk-propose`.

## Decisions

1. **Delegate, don't copy.** Each skill tells the agent to follow the project's
   `openspec-explore`, `openspec-verify-change` or `openspec-archive-change` skill, and states only
   the agk overrides: the handoff target, the gates, and the closes. A copied skill would drift from
   OpenSpec on every `openspec update`. *Alternative:* `operations.explore.guidance` in each
   project's `config.yaml`. That would be per project, and nothing would ship with agk.

2. **Explicit invocation only.** Both descriptions say "Use only when explicitly invoked", as
   `agk-propose` does. Archive cannot be undone, and an auto-triggered explore would take every
   brainstorm away from the stock skill. `agk-apply` and `agk-review` stay auto-triggered. They are
   worker drivers, not workflow steps.

3. **Gates are judged from session context (user's choice), with a conservative fallback.** Only
   this session's own evidence counts. A user's claim does not count, and neither does a compaction
   summary.
   - **Review:** `agk review wait` and `review.json` output of the *latest* diff review round run
     for the change. A later round with findings cancels an earlier approve. Whether the code is
     still what that round saw is not judged by the agent: `agk review fresh <dir>` answers it from
     the round's scope fingerprint (decision 10). That also catches edits made outside the session.
     A `fresh: false` result or a `fresh` error counts as unconfirmed.
   - **Verify:** the stock `openspec-verify-change` skill writes its report as the agent's own
     message, not as tool output. So the evidence is a report the agent produced in this session by
     following that skill for the change, with its CRITICAL count, after the passing review round
     and the last code change.

   If the session cannot confirm a gate, the skill runs that step itself. A gate the session
   confirms as failing, meaning the latest round has unresolved findings, stops the skill with no
   new review.
   *Alternative considered:* also record the change in every diff review run, so a new session
   could find the run on disk. It is deferred: it needs a new start flag and a decision about which
   change a diff belongs to. A fresh session pays one review round instead.

4. **Order: review gate, verify gate, fresh check, archive, closes.** Verify must come after the
   last review, because the review can lead to code changes. Verify takes time and files can change
   while it runs (an open applier, the user's editor), so `agk review fresh` runs once more on the
   approving round right before the archive; `fresh: false` or an error stops with nothing
   archived and the reviewer open. Companions are closed only after the archive
   succeeds: if the archive fails, the user may still need the applier or the reviewer.

5. **The fallback review stops on findings or on a stale approve.** An approving fallback round
   is followed by `agk review fresh` on the same round. Files can change while the reviewer runs,
   for example from the user's editor or a still-running applier, so `fresh: false` or an error
   stops the skill, with nothing archived. `agk-archive` is a gate, not a fixer. Fixing belongs
   to the apply and review loop (the main session, or `agk apply send`). The reviewer stays open so
   the next round resumes its session. The user then runs `/agk-archive` again, and in the same
   session the gate passes once a round approves. The same stop applies to the fresh check after
   verify (decision 4).

6. **Fallback base.** The review runs `agk review start --base <base>`. The base is the branch the
   work forked from, as in `agk-review`, and defaults to `main`. On a branch where everything is
   already committed into `main`, the diff is empty (`nothing to review`). The skill then asks the
   user: archive without a code review, or name a base such as the commit before the
   implementation. It does not guess.

7. **Companion discovery.** The skill closes:
   - apply runs and plan review runs whose `worker.json` has `"change": "<change>"`, found by
     scanning `.agk/applies/*/` and `.agk/reviews/*/` under the repo root;
   - the diff review run dirs this session ran for this change: the gate's run, and the code review
     of this change's work. Those runs record no change, so the link comes from the session. A
     session that also reviewed another change, or a run of unclear purpose, keeps those reviewers
     open. Older diff reviews from other sessions are never touched.

   Closes are idempotent, so closing an already-closed run is harmless. A failed close is reported
   and does not stop the others.

8. **Verify outcome handling.** CRITICAL stops the archive. WARNING and SUGGESTION do not block,
   and they are listed in the final message. The user's rule "fix WARNING or reject it with a
   reason" belongs to the apply step, which already ran.

9. **Evals extend the existing suite.** The new cases go in `evals/cases/` and `case-plugins/`.
   Stub skills:
   - `openspec-explore` ends by suggesting `/opsx:propose`. The handoff case checks the override.
   - `openspec-verify-change` runs `.eval/verify.sh <change>`. It logs `verify <change>` to
     `agk-calls.log` and prints the case's check results from `verify.txt`, or all-pass results
     when there is none. Like the real skill, the stub then has the agent write the report itself,
     with CRITICAL/WARNING/SUGGESTION counts. The in-session case thus checks reuse of the agent's
     own report, not of a tool's verdict.
   - `openspec-archive-change` runs `.eval/archive.sh <change>`. It logs `archive <change>`, and it
     moves the change under `openspec/changes/archive/` or fails with the case's `archive-fail`
     marker.

   Stand-in `agk` changes:
   - `review start --base` creates a run dir with a `base` file. It answers `nothing to review` when
     the workspace has no diff against the base and no untracked file.
   - `review start --base` and `review send` on a diff run store a shell fingerprint of the
     workspace in the round: `git diff <base>` plus each untracked path and its content.
     `review fresh` recomputes it and prints the same JSON as the real command. With the case
     marker `outcomes/<case>/edit-during-review`, `review wait` appends a line to the code file
     before it returns, the way a file changes while a reviewer runs.
   - `run_dir` accepts a `base` or a `change` file.
   - `apply close` and `review close` log their run dir.

   The scaffold gains options for an uncommitted code file, the change `add-widget`, and
   pre-created `.agk/` run dirs with `worker.json`. One case can cover several scenarios, and
   `evals/README.md` maps scenarios to cases:

   | Case | Scenarios |
   |------|-----------|
   | `plain-explore-request` | explore: Plain explore request |
   | `explore-no-openspec-skills` | explore: Project without OpenSpec skills |
   | `idea-settled` | explore: Exploration writes no code, Idea settled |
   | `plain-archive-request` | archive: Plain archive request |
   | `archive-no-openspec-skills` | archive: Project without OpenSpec skills |
   | `fresh-session-archive` | One active change, Fresh session review approves, Gates pass, Applier and plan reviewer still open, Archive done |
   | `approved-in-session` | Approved earlier in the session |
   | `file-added-after-approve` | File added after the approve |
   | `later-round-findings` | Later round found issues |
   | `two-changes-in-session` | Review of another change, Diff reviewer of another change in the session |
   | `verified-in-session` | Verified earlier in the session |
   | `changed-during-review` | Code changed during the fallback review |
   | `fallback-findings` | Fallback review has findings |
   | `nothing-to-review` | Nothing to review |
   | `changed-during-verify` | Code changed during verify |
   | `verify-critical` | Verify reports CRITICAL (also the broken-skill check) |
   | `verify-warnings` | Verify reports warnings only |
   | `archive-failed` | Archive failed |

   In the in-session cases, the prompt first asks the agent to set the session up, and then to
   invoke `/agk-archive`. Depending on the case, setting up means one of:
   - run the review;
   - run the review and `/opsx:verify`;
   - run the review and then create a file;
   - run two review rounds;
   - run a review named as `other-change`'s code review.

   The graders count the `agk review start`, `agk review fresh`, `agk review close` and `verify`
   lines in `agk-calls.log`.

10. **Scope fingerprint and `agk review fresh`.** `review.Collect` already reads the whole diff
    scope. It also returns a fingerprint: SHA-256 over the merge base, then one sorted line per
    in-scope path. The paths are the union of `git diff --name-only --no-renames` against the merge
    base (same flags and `.agk` exclusion as the diff) and the untracked list. Without
    `--no-renames`, staging a rename would collapse the deleted source path into the destination,
    and a `git add` or commit would change the fingerprint with no file changed. The recursive
    submodule pass uses the same flag. Each line is
    `path NUL kind NUL mode NUL value`, from `Lstat` of the working tree:
    - a regular file: kind `file`, value the SHA-256 of the content. The mode is git's effective
      one: `755` when the owner execute bit is set, else `644`. With `core.filemode=false` (read
      per repository) a tracked file takes the mode of its index entry, and a file without an index
      entry is `644`, as `git add` would store it;
    - a symlink: kind `symlink`, value the link target;
    - a submodule (a directory at a gitlink path): kind `submodule`, value
      `git -C <path> rev-parse HEAD` plus the same fingerprint computed recursively inside it, with
      its `HEAD` as the base. That covers the paths of `git -C <path> diff --name-only HEAD` and its
      untracked files, each with kind, mode and content, and nested submodules the same way. New
      commits, every content edit (a second edit of an already dirty file included), mode changes
      and untracked files in the submodule therefore all count. That is stricter than
      `--ignore-submodules=none`, which only marks the submodule `-dirty`. An uninitialized
      submodule (no `.git`) gets the value `uninitialized:<gitlink sha>`, from its index entry or
      else the merge-base tree;
    - a path missing on disk whose index entry is skip-worktree (sparse checkout): kind `index`,
      with the index mode and object id, since git diffs the index content for it;
    - any other directory, or a missing path: kind `deleted`.

    Every per-path git lookup passes the path as `:(literal)<rel>` and takes the record whose path
    equals it exactly, so names with glob characters cannot match another file.

    The kind and mode are part of the line, so a chmod, or a file replaced by a symlink with the
    same bytes, changes the fingerprint. Omitted untracked files (binary, too large) are hashed
    too: the fingerprint covers the scope, not the prompt.
    - Committing changes nothing in that input, so the usual "commit, then archive" order does not
      force a re-review. Moving the merge base (rebasing on or merging `main`) does force one, and
      should.
    - **Consistent snapshot.** `Collect` reads the diff and the untracked files in separate git
      calls and file reads, so one pass cannot be atomic. It computes the fingerprint before and
      after collecting the content, and collects again when they differ, up to three attempts. If
      they still differ, it fails with an error saying the files changed during collection. On
      success, the before value (equal to the after value) is `Scope.Fingerprint`. An edit that
      lands between two reads within one pass changes one of the two fingerprints, so it is caught.
      Tests reach the window through an unexported hook in `internal/review` that runs between the
      first fingerprint and the content reads.
    - `initRound` callers for diff runs write `rounds/<k>/scope.sha256` next to `prompt.md`, from
      the same `Scope` the prompt was rendered from.
    - `agk review fresh <dir>` loads the run and refuses plan runs. It reads the current round's
      `scope.sha256`, and refuses a missing one with an error naming an older agk. It calls
      `Env.Collect` with the recorded base, which tests can fake, and maps `ErrNothingToReview` to
      `fresh: false`. Then it compares and prints `{"fresh","round"}`. It writes nothing and does
      not touch agterm.

    *Alternatives:* hashing `scope.Diff` plus the untracked contents is simpler, but it changes on
    commit (untracked files move into the diff) and would force a re-review on every archive after
    a commit. Comparing with `prompt.md` text depends on the template and on truncation.

## Risks / Trade-offs

- [The agent links the wrong review run to the change] → The skill uses only runs this session
  started or drove for the change's work. Any doubt counts as unconfirmed, and the fallback re-runs
  the review. `two-changes-in-session` checks this.
- [Fingerprint cost on large scopes] → It reads every in-scope file once per round and once per
  `fresh` call. That is the same order of work as the existing untracked read, and acceptable for a
  review.
- [A fresh session always pays one review round] → Accepted. The fingerprint design (decision 3) is
  the follow-up if this gets expensive.
- [Closing companions that another session still drives for the same change] → Accepted. An
  archived change is done. Runs of other changes are never touched.
- [Runs started in another worktree live under that worktree's `.agk/`] → The skill scans only the
  current repo root and says so in its report. Companions from elsewhere stay open.
- [The eval suite cost grows by 18 cases] → The cases are cheap prompts. `--case` runs one case
  while iterating. The baseline for the new cases is recorded in `evals/README.md`.

## Migration Plan

Ship in the next release. Users get the skills with `brew upgrade agk && agk setup` (setup links new
`agk-*` dirs). Nothing to roll back beyond removing the two skill dirs. `agk setup` then removes the
stale links.
