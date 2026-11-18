# autopilot-workflow Specification

## Purpose
The `agk-autopilot` agent skill: an unattended run of the agk workflow from an idea to an
implemented, reviewed and verified OpenSpec change, in which the model decides every point the
other skills would ask the user about, records each decision, and stops before archiving.

## Requirements

### Requirement: Explicit invocation
The `agk-autopilot` skill SHALL run only when invoked by name (`/agk-autopilot <idea>`, "agk autopilot"). A plain request to build or propose something SHALL NOT start an autopilot run.

#### Scenario: Plain build request
- **WHEN** the user asks the agent to implement a feature without naming `agk-autopilot`
- **THEN** the agent runs no `agk apply start` and no `agk review start` on its own initiative

### Requirement: Preconditions
Before writing anything the skill SHALL check that the project has the `openspec-propose` and `openspec-verify-change` skills and that `agk` is on `PATH`, and SHALL stop with setup advice when one is missing.

#### Scenario: Project without OpenSpec skills
- **GIVEN** a repository with no `openspec-propose` skill
- **WHEN** the user invokes `/agk-autopilot add a widget`
- **THEN** the agent tells the user to run `openspec init` or `openspec update`, creates no change and runs no `agk` command

### Requirement: Change written without asking
The skill SHALL write the change's artifacts with the project's `openspec-propose` workflow without asking the user anything. Where that workflow would ask (an ambiguity, the change name, a name that already exists), the model SHALL choose and record the choice as a decision.

#### Scenario: Ambiguous idea
- **GIVEN** an idea that leaves the output format open
- **WHEN** the user invokes `/agk-autopilot` with it
- **THEN** the change's artifacts are written without a question to the user, and `decisions.md` holds a `[scope]` entry naming the chosen format and why

#### Scenario: Name already taken
- **GIVEN** an active change whose name the idea would produce
- **WHEN** the skill creates the change
- **THEN** it creates the change under another, unused name, leaves the existing change untouched, and records the name as a decision

### Requirement: Decision log
Every decision the model takes in place of the user SHALL be appended to `decisions.md` in the change's directory as an entry `D<n>` tagged `[scope]` or `[impl]`, with what was open, what was chosen and why. The file SHALL exist from the first plan review round on, also when it holds no entry.

#### Scenario: Entry format
- **WHEN** the model sets aside a review finding
- **THEN** `decisions.md` gains a numbered entry with the tag, the finding it concerns, the decision and the reason

#### Scenario: Log reviewed with the plan
- **WHEN** the first plan review round starts
- **THEN** `openspec/changes/<change>/decisions.md` exists and is part of the reviewed files

#### Scenario: Log follows the worktree
- **GIVEN** the applier runs in a change worktree
- **WHEN** a decision is taken after `agk apply start`
- **THEN** it is appended to `decisions.md` of the change inside that worktree

### Requirement: Plan review without revdiff
The skill SHALL run the plan review loop of `agk-propose` (`agk review start --change`, fix, `agk review send`) until a round approves the current artifacts, and SHALL NOT launch revdiff at any point of the run.

#### Scenario: Plan approved
- **GIVEN** the plan review round approves the current artifacts
- **WHEN** the skill continues
- **THEN** it starts the applier with `agk apply start <change>` and never runs revdiff

### Requirement: Disputed findings decided by the model
A plan or code review finding the model disagrees with, and a `needs-attention` verdict without findings, SHALL be decided by the model instead of the user: it either acts on it or sets it aside, records a decision, and names that decision in the note of the next review round.

#### Scenario: Disputed plan finding
- **GIVEN** a plan review finding that contradicts the idea
- **WHEN** the skill handles the round
- **THEN** it asks the user nothing, appends a decision to `decisions.md`, and runs `agk review send` with a note naming that decision

### Requirement: Implementation with the applier
After the plan passes, the skill SHALL implement the change with `agk apply start <change>` and wait for its report. After each `done` report it SHALL review the diff with `agk review start --base <base> --for <change>` (re-reviews with `agk review send`), hand the findings it accepts to the applier with `agk apply send`, and repeat until a code review round approves.

#### Scenario: Findings go to the applier
- **GIVEN** a code review round with a finding the model accepts
- **WHEN** the skill handles it
- **THEN** it sends the finding with `agk apply send`, waits for the applier's next report, and then runs `agk review send` on the same reviewer

### Requirement: Applier blocked
When the applier reports `blocked`, the skill SHALL resolve it itself: edit the named artifact (or decide a `finding`), record a decision, have every planning-artifact edit pass a plan review round, and only then send the applier a note to continue.

#### Scenario: Blocked on tasks
- **GIVEN** the applier reports `blocked` with artifact `tasks`
- **WHEN** the skill handles it
- **THEN** `tasks.md` is edited, a decision is recorded, a plan review round approves the edited plan, and after that `agk apply send` delivers the note

### Requirement: Verify gate
After the code review approves, the skill SHALL run `openspec-verify-change`. CRITICAL issues SHALL go to the applier, then through a code review round and verify again. A WARNING SHALL be fixed the same way or set aside with a recorded decision. The run SHALL finish only when the last code review round approved and the verify after it reported 0 CRITICAL, both on the current files.

#### Scenario: Code changed during verify
- **GIVEN** the code review approved and a file changes while verify runs
- **WHEN** verify reports 0 CRITICAL
- **THEN** `agk review fresh <dir>` reports `fresh: false`, and a code review round and verify run again before the final report

#### Scenario: Verify CRITICAL
- **GIVEN** verify reports one CRITICAL issue
- **WHEN** the skill handles it
- **THEN** the issue goes to the applier with `agk apply send`, a code review round runs on the fix, and verify runs again

#### Scenario: Decision written after verify
- **GIVEN** verify reports only a WARNING the model sets aside
- **WHEN** the skill records the decision in `decisions.md`
- **THEN** a code review round and verify run again before the final report

### Requirement: Round limits
The skill SHALL run at most 5 rounds in one plan reviewer and at most 5 rounds in one code reviewer. When a round would exceed its limit, the run SHALL stop with the report of what was done and the open findings, and ask the user how to go on.

#### Scenario: Code review does not converge
- **GIVEN** the code reviewer's round 5 still has findings
- **WHEN** the skill handles the round
- **THEN** it runs no sixth round, stops, and reports the open findings and the decisions so far

### Requirement: Human-only stops
The skill SHALL turn to the user only when a worker needs a human (`needs-human`, a `started: false` start or send), when a companion was closed (`cancelled`), or on a round limit. On `needs-human` it SHALL tell the user and wait again; on `cancelled` it SHALL stop with the report.

#### Scenario: Permission prompt in a companion
- **GIVEN** `agk apply wait` returns `needs-human` with reason `permission`
- **WHEN** the skill handles it
- **THEN** it tells the user to answer in the applier companion and runs the wait again, without answering the prompt itself

#### Scenario: Companion closed
- **GIVEN** `agk review wait` returns `cancelled`
- **WHEN** the skill handles it
- **THEN** it stops, starts no new reviewer, and reports the run so far

### Requirement: Final report and stop before archive
At the end of a run, finished or stopped, the skill SHALL report in chat: the change, the `[scope]` decisions first, then the `[impl]` decisions, the plan and code review round counts, the verify result and the worktree path when there is one. A finished run SHALL name `/agk-archive <change>` as the only next step. The skill SHALL NOT archive, run `git commit`, push or run `agk finish`; the plan commit `agk apply start` makes in worktree mode is the only commit of a run.

#### Scenario: Finished run
- **GIVEN** the last code review approved and verify after it reported 0 CRITICAL
- **WHEN** the skill ends
- **THEN** the report lists the decisions grouped `[scope]` before `[impl]`, names `/agk-archive <change>`, and no archive, `git commit`, push or `agk finish` ran

#### Scenario: Plan commit in worktree mode
- **GIVEN** worktree mode is configured
- **WHEN** the skill runs `agk apply start <change>`
- **THEN** the plan commit agk makes there (the change directory with `decisions.md`) is the run's only commit, and the final report names it

### Requirement: Eval suite
The repository SHALL hold eval cases for `agk-autopilot` runnable with `claude plugin eval`, with at least one case per scenario of this capability, run against stand-ins for `agk` that record their arguments, never a real applier, reviewer or agterm.

#### Scenario: Suite covers the skill
- **WHEN** `scripts/eval-skills.sh` runs
- **THEN** it includes the `agk-autopilot` cases, and each scenario of this capability maps to at least one case in `evals/README.md`
