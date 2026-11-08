# Spec Delta

## ADDED Requirements

### Requirement: Scope fingerprint of a diff round
Each diff review round SHALL record in `rounds/<k>/scope.sha256` a fingerprint of the scope its prompt was built from: the merge base and, per in-scope path, its kind (file, symlink, submodule, skip-worktree entry, deleted), git's effective executable bit, and its content hash, link target, index object, or submodule commit plus own fingerprint. It SHALL NOT depend on whether a change is committed, staged or untracked. Plan rounds record none.

#### Scenario: Fingerprint written with the prompt
- **WHEN** `agk review start --base main` or `agk review send` on a diff review builds round k
- **THEN** `rounds/<k>/scope.sha256` exists before the round's reviewer starts

#### Scenario: Committing keeps the fingerprint
- **GIVEN** a diff scope with one untracked file `a.go`
- **WHEN** `a.go` is committed without changing its content
- **THEN** the fingerprint of the scope is unchanged

#### Scenario: Staging a rename keeps the fingerprint
- **GIVEN** a diff scope where a tracked file `old.go` was renamed to `new.go` in the working tree, not yet staged
- **WHEN** the rename is staged and then committed without changing any content
- **THEN** the fingerprint of the scope is the same after each step

#### Scenario: Content edit changes the fingerprint
- **GIVEN** a diff scope with an untracked file `a.go`
- **WHEN** one byte of `a.go` changes and the file list stays the same
- **THEN** the fingerprint of the scope changes

#### Scenario: Path set changes the fingerprint
- **WHEN** a new untracked file is added to the scope, or an in-scope file is deleted
- **THEN** the fingerprint of the scope changes

#### Scenario: Mode or kind change changes the fingerprint
- **WHEN** an in-scope script loses its executable bit, or an in-scope file is replaced by a symlink whose target text equals the file's old bytes
- **THEN** the fingerprint of the scope changes

#### Scenario: Submodule in scope
- **GIVEN** a diff scope with an updated submodule
- **WHEN** the scope is collected, and later the submodule moves to another commit or gets uncommitted changes
- **THEN** collection succeeds as before, and the fingerprint changes after the move or the edit

#### Scenario: Second edit inside a dirty submodule
- **GIVEN** a diff scope with a submodule whose tracked file `a.go` is already modified and whose untracked file `new.go` exists
- **WHEN** `a.go` or `new.go` is edited again, so the submodule's HEAD and status lines stay the same
- **THEN** the fingerprint of the scope changes

#### Scenario: Plan round has no fingerprint
- **WHEN** round 1 of `agk review start --change add-x` is built
- **THEN** `rounds/1/` has no `scope.sha256`

### Requirement: Fingerprint matches the prompt
The fingerprint recorded for a round SHALL describe the same working-tree state as that round's prompt. agk SHALL compute it before and after it collects the prompt's content, and SHALL collect again when the two differ. When they still differ after three attempts, the round SHALL fail with an error saying the files changed during collection, and no prompt is written.

#### Scenario: File edited during collection
- **GIVEN** an in-scope file that changes once between the first fingerprint and the prompt's read of it
- **WHEN** a diff round is built
- **THEN** the scope is collected again, and the recorded fingerprint and the prompt both show the edited file

#### Scenario: Files keep changing
- **GIVEN** an in-scope file that changes during every collection attempt
- **WHEN** a diff round is built
- **THEN** the command exits 1 with an error that files changed during collection, and writes no `prompt.md` or `scope.sha256` for that round

### Requirement: Freshness check
`agk review fresh <run-dir>` SHALL compare the current diff scope against the run's recorded base with the current round's fingerprint. It SHALL print one JSON object `{"fresh": true|false, "round": K}` and exit 0. An empty scope SHALL count as not fresh. It SHALL exit 1 for a plan review run, a round without a fingerprint, or an invalid run directory.

#### Scenario: Unchanged scope
- **GIVEN** a diff review whose current round 2 was built from the current scope
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it prints `{"fresh":true,"round":2}` and exits 0

#### Scenario: Scope changed after the round
- **GIVEN** a diff review whose current round 1 was built before an in-scope file was edited
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it prints `{"fresh":false,"round":1}` and exits 0

#### Scenario: Scope became empty
- **GIVEN** a diff review whose changes have since been reverted, so nothing is left to review against its base
- **WHEN** the agent runs `agk review fresh <dir>`
- **THEN** it prints `{"fresh":false,"round":1}` and exits 0

#### Scenario: Not a diff round with a fingerprint
- **WHEN** the agent runs `agk review fresh` on a plan review run, or on a diff run whose current round was written by an agk without fingerprints
- **THEN** it exits 1 with an error naming the reason and prints no JSON

#### Scenario: Read only
- **WHEN** the agent runs `agk review fresh <dir>` while the reviewer companion is open
- **THEN** it does not change the run directory, start a round or call agterm
