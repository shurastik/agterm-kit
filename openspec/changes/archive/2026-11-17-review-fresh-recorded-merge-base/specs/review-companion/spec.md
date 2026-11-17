## ADDED Requirements

### Requirement: Merge base of a diff round
Each diff review round SHALL record in `rounds/<k>/merge-base` the full commit id of the merge base its scope was built from, written before the round's reviewer starts and alongside `scope.sha256`. Plan rounds SHALL record none.

#### Scenario: Merge base written with the fingerprint
- **GIVEN** a branch whose merge base with `main` is commit `M`
- **WHEN** `agk review start --base main` or `agk review send` on that diff review builds round k
- **THEN** `rounds/<k>/merge-base` holds the full id of `M` before the round's reviewer starts

#### Scenario: Each round records its own merge base
- **GIVEN** a diff review whose round 1 was built at merge base `M1`, after which the branch was rebased onto `M2`
- **WHEN** `agk review send` builds round 2
- **THEN** `rounds/1/merge-base` still holds `M1` and `rounds/2/merge-base` holds `M2`

#### Scenario: Plan round has no merge base
- **WHEN** round 1 of `agk review start --change add-x` is built
- **THEN** `rounds/1/` has no `merge-base`

## MODIFIED Requirements

### Requirement: Freshness check
`agk review fresh <run-dir>` SHALL compare the scope between the current round's recorded merge base and the working tree with that round's fingerprint. A round without a recorded merge base SHALL be compared as before, against the merge base of the run's recorded base and `HEAD`. It SHALL print one JSON object `{"fresh": true|false, "round": K, "merge_base": "<sha>"}` and exit 0, where `merge_base` is the merge base it compared against. An empty scope, or a recorded merge base that is no longer an ancestor of `HEAD` (unrelated history included), SHALL count as not fresh. It SHALL exit 1 for a plan review run, a round without a fingerprint, a recorded merge base missing from the repository, or an invalid run directory.

#### Scenario: Unchanged scope
- **GIVEN** a diff review whose current round 2 was built from the current scope at merge base `M`
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it prints `{"fresh":true,"round":2,"merge_base":"<M>"}` and exits 0

#### Scenario: Scope changed after the round
- **GIVEN** a diff review whose current round 1 was built before an in-scope file was edited
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it prints `"fresh":false` with `"round":1` and exits 0

#### Scenario: Committed on the base branch
- **GIVEN** a diff review run with `--base main` while `main` is checked out, whose current round 1 approved uncommitted changes at merge base `M`
- **WHEN** those changes are committed on `main` without changing their content, and the agent runs `agk review fresh <dir>`
- **THEN** it prints `{"fresh":true,"round":1,"merge_base":"<M>"}` and exits 0

#### Scenario: Rebased onto a moved base
- **GIVEN** a diff review on a feature branch whose current round 1 was built at merge base `M`, after which `main` gained a commit touching another file and the branch was rebased onto it
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it prints `"fresh":false` and exits 0

#### Scenario: Recorded merge base no longer an ancestor
- **GIVEN** a diff review whose current round 1 recorded merge base `M`, after which `HEAD` was moved to a history that does not contain `M`, either one that shares older commits with it or an unrelated one (an orphan branch) with no common ancestor, while the commit `M` still exists in the repository
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it prints `{"fresh":false,"round":1,"merge_base":"<M>"}` and exits 0

#### Scenario: Recorded merge base missing from the repository
- **GIVEN** a diff review whose current round recorded merge base `M`, and the commit `M` no longer exists in the repository
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it exits 1 with an error naming the missing merge base and prints no JSON

#### Scenario: Scope became empty
- **GIVEN** a diff review whose changes have since been reverted, so nothing is left between its recorded merge base and the working tree
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it prints `"fresh":false` with `"round":1` and exits 0

#### Scenario: Round without a recorded merge base
- **GIVEN** a diff run whose current round has `scope.sha256` but no `merge-base`, written by an agk before this change
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it compares against the merge base of the run's base and `HEAD`, prints that merge base as `merge_base` and exits 0

#### Scenario: Not a diff round with a fingerprint
- **WHEN** the agent runs `agk review fresh` on a plan review run, or on a diff run whose current round was written by an agk without fingerprints
- **THEN** it exits 1 with an error naming the reason and prints no JSON

#### Scenario: Read only
- **WHEN** the agent runs `agk review fresh <dir>` while the reviewer companion is open
- **THEN** it does not change the run directory, start a round or call agterm
