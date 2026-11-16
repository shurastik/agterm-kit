# Proposal

## Why

A claude applier often starts long work (eval runs, test suites) as a background shell or Monitor,
ends its turn, and resumes by itself when that work finishes. `agk apply wait` reads the end of a
turn without a report as "the applier needs the user" and returns `needs-human` / `no-report` at
once. It keeps doing so on every later call while the applier is only paused. The main session then
either bothers the user for nothing or has to fall back to watching `rounds/<k>/report.json` by hand.
This happened in two apply runs (backlog entries "agk apply wait reports needs-human / no-report while
the applier is still working" and "(was agk-cii)").

Claude Code now tells its `Stop` hook what is still in flight: the hook input carries
`background_tasks` and `session_crons`, "to distinguish 'session is done' from 'session is paused
waiting for background work to wake it back up'". A spike on claude 2.1.325 confirmed this: while
paused on a background shell or a Monitor, `background_tasks` lists the running task; the final
`Stop` has it empty; the resumed turn fires `UserPromptSubmit` with a `<task-notification>` prompt.

## What Changes

- The claude applier's settings layer, written by `agk apply start` into the run directory, gains
  two hooks next to the existing deny rules. They run a new hidden command `agk apply hook` that
  records the applier's turn state in the run's `turn.json`: `running` when a turn starts
  (`UserPromptSubmit`, also on a resumed turn), `stopped` with the count of in-flight background
  tasks and scheduled wakeups when a turn ends (`Stop`), each with the round the turn belongs to,
  taken from the round's kickoff line the applier received. The hook never sets the agent
  status and never fails the turn.
- `agk apply wait` uses that file for a claude applier run that has the hooks: a turn that ended with
  background work in flight is a pause, not a question. It keeps waiting, by the files, until the
  report appears, the turn state changes, a permission prompt opens or the companion is gone. On
  timeout during a pause it prints `running` with `reason` `background` and the count of tasks.
  A turn that ended with nothing in flight and no report is still `needs-human` / `no-report`.
- Runs without the hooks (codex appliers, runs started by an older agk) keep the current behavior.
- The `agk-apply` skill and README describe `running` with `reason` `background`.
- The two backlog entries were removed from `BACKLOG.md` when this change was proposed.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `apply-worker`: the claude applier gets turn hooks (new requirement), and "Wait for the applier"
  treats a turn that ended with background work in flight as a pause rather than `no-report`.

## Impact

- `internal/cli/apply.go` (`applierFiles` writes generated settings; `ApplyWait`), a new
  `internal/cli/apply_hook.go`, `internal/cli/main.go` (dispatch of the hidden `apply hook`),
  `internal/apply/report.go` (`Decide` and `Outcome`), `assets/claude-applier-settings.json`
  (stays the deny-rule base).
- `worker.json` of a claude apply run records that the run has turn hooks.
- `skills/agk-apply/SKILL.md`, `README.md`, `BACKLOG.md`.
- Codex applier and the reviewer are unchanged.
