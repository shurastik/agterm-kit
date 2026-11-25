# Design

## Context

See proposal.md for the motivation and the specs for the behavior. What shapes the approach:

- agk binary invocations are short-lived; the only long-lived agk processes of an autopilot run
  are `agk apply wait` / `agk review wait` (up to 3600 s each, run in the background by the main
  session) and `agk apply send` / `agk review send` (up to 600 s while they wait for the turn to
  end). Every wait, the claude applier's hooked wait included, goes through
  `Env.waitPolling` in `internal/cli/worker.go`, which loops in 5-second slices.
- `agtermctl session info` takes `--hide-after SECONDS` (0..86400, each set restarts it) and
  `--clear`; `agtermctl session tint <color> [--on icon|name]` persists until `tint clear`, but a
  companion's tint dies with the companion. `agterm.CLI.Info` today always passes `--hide-after 0`.
- agk never sets anything on the main session today; the info line is used only on the reviewer
  companion (`reporter.set`).
- A worker companion's name comes from `companionName(change, harness, kind, round)`, used at start
  and by the per-round rename in both sends.
- Worktree mode: the applier, the code reviewer and their run dirs live under
  `.worktrees/<change>/.agk/`, while the plan reviewer and `agk autopilot on` run from the main
  checkout. Anything shared between them cannot live under one checkout's `.agk/`.

## Goals / Non-Goals

**Goals:**
- The main session's line is live while an autopilot runs; after `off` nothing lights it again,
  and after a crash it goes away by itself (see Risks for the bound), never kept for good.
- One refresh point that covers every wait, instead of one per wait loop.

**Non-Goals:**
- Distinct states on the line (needs-human, done): the line means "autopilot running", nothing more.
- Marking the main session by rename or tint; changing the reviewer's own info line on its
  companion.
- A config key for the expiry, the refresh interval or the tint color.

## Decisions

### D1. Live state in a per-session file outside the repository

`agk autopilot on` writes `<state>/agk/autopilot/<session-id>.json` holding `{"until": <RFC 3339>}`
(now + 1200 s), where `<state>` is `$XDG_STATE_HOME` when it is absolute, else `~/.local/state`.
`off` removes it. A refresh sets the line only when the file exists and `until` is in the future,
and then rewrites `until` to now + 1200 s, in the same step as the line. The file mirrors the line's
expiry, so a crashed autopilot (no `off`) can never light up again once its line expired, and a
manual `agk apply send` after `off` finds no file.

Alternatives: a file under the repo's `.agk/` (split between the main checkout and the worktree in
worktree mode); reading the line back from `agtermctl tree` (agk would depend on the tree's info
field and could not tell an autopilot line from anyone else's); no live state at all (a manual
`send` on a kept applier would re-mark the main session after `off`).

The session id becomes a file name: an id that is empty or holds anything but letters, digits,
`-` and `_` is treated as "no live autopilot" and `on` reports it as a warning with
`"marked": false`.

### D1b. `on`, `off` and a beat of one session are serialized

Each of them holds an exclusive `flock` on `<state>/agk/autopilot/<session>.lock` for its whole
sequence: `on` (write `until`, set the line), a beat (check live, set the line, write `until`) and
`off` (remove the file, clear the line). Background waits are separate processes; without the lock
a beat that checked "live" just before `off` would set the line and recreate the file after `off`
returned. With it, `off` either runs before the beat's check (the beat finds nothing live) or after
the beat (its clear is the last call). The lock is held across one agtermctl call, which is bounded
by the usual 5 s status timeout. `syscall.Flock` is in the standard library on darwin and linux;
the lock file is never removed, so it cannot race its own deletion.

Alternative: a token written by `on` and compared by the beat (still a window between the compare
and the line); an agterm-side lock (none exists).

### D2. The run records its session; the refresh targets that session

`worker.json` of both workers gains `"autopilot": true` and `"autopilot_session": "<id>"` (the
caller's `AGTERM_SESSION_ID` at start, empty outside agterm). The refresh of a run targets
`autopilot_session`, not the session the later command runs in, so a wait run from somewhere else
still refreshes the right line. Runs without the fields (older agk, no flag) refresh nothing.

### D3. One heartbeat hook in `waitPolling`, plus one at command entry

`Env` gets a heartbeat that a command arms for an autopilot run (`e.beat = heartbeat{session,
last}` or a func field). It is called once when `wait` or `send` loads the run, and from
`waitPolling` between slices; it does work only when 60 s passed since its last beat. The send's
wait for the turn (`typeWhenIdle`) takes any positive `--timeout`. On the committed base it is a
single `Ctl.Wait` for the whole budget; the in-flight change `codex-turn-over-fallback` makes it
`waitPolling` with a codex `settle` check (turn over when the output is on disk and the screen is
stable) for codex workers, keeping the single `Ctl.Wait` for claude. Whichever is in the code when
this lands, the rule is: every path of `typeWhenIdle` that waits in one `Ctl.Wait` runs through
`waitPolling` instead when the heartbeat is armed (a done check that never fires, the same statuses
and budget), and a path that already polls keeps its own done check (the codex settle stays as it
is). The beat comes from `waitPolling` either way, so a send waiting an hour on a busy worker beats
every 60 s like a wait. Runs without the heartbeat keep their calls unchanged, and existing tests
(`TestApplySendCodexStatusStuckActiveSettledScreen`, `TestReviewSendCodexStatusStuckActiveSettledScreen`
once present) keep passing.

Alternative: refresh in each wait loop (`ApplyWait`, `waitHooked`, `waitInteractive`): three call
sites that would drift; `waitPolling` already is the one place every slice passes through.

### D4. `Ctl` changes

- `Info(ctx, target, markdown string, hideAfter time.Duration)`: the existing reviewer line passes
  0 (no change in behavior); the autopilot line passes 1200 s.
- `ClearInfo(ctx, target)`: `session info --clear --target <id>`.
- `Tint(ctx, target, color string)`: `session tint <color> --on icon --target <id>`.
- The fake records the calls like the others, so tests assert argv.

### D5. Marks

- Line text: `🤖 autopilot`, as inline markdown with a fixed color (`🤖 ^[autopilot](color:
  "purple")`), a fixed string per the "no model output in status markdown" convention.
- Companion: `companionName` gets the autopilot flag and prepends `🤖 `; both starts and both sends
  pass the run's flag, so the rename keeps the prefix. The tint is `purple` on the icon, set right
  after the companion is created and its id recorded; a failure is a warning.
- Expiry 1200 s, beat interval 60 s: the expiry only has to bridge the gaps between agk commands
  in the main session (reading findings, running verify), never a worker's round, which a wait
  covers.

### D6. Command shape

`agk autopilot on|off` is a new top-level command in `internal/cli/main.go` dispatch, its own file
`internal/cli/autopilot.go`. Output `{"session": "<id>"|null, "marked": <bool>}`; exit 0 except for
a usage error. Every agterm failure is a warning (best effort, like the rename).

### D7. Skill

`skills/agk-autopilot/SKILL.md`: `agk autopilot on` right after the preconditions (step 0);
`--autopilot` on the plan review start (step 2), the applier start (step 3), the worktree plan
review start (Applier blocked) and the code review start (step 4); `agk autopilot off` as the last
command before the final report, on the finished path (after closing the reviewers) and on every
stop; `agk autopilot on` again right after every `openspec-verify-change` run. The stand-in `agk` of `evals/` accepts `autopilot on|off` and `--autopilot`, and the
happy-path, cancelled and needs-human cases grade the calls.

## Risks / Trade-offs

- [The main session's info line is shared with anything else that writes it] → agk is the only
  writer today; `off` clears the line whoever wrote it last, which is acceptable for a line the
  autopilot owned.
- [A crash of the main session's agent leaves the line] → with no wait running, it goes away within
  1200 s. A background `agk apply wait` / `agk review wait` (or a `send`) that outlives the crash
  keeps beating until it ends: at most its `--timeout` (3600 s by default for a wait) plus 1200 s.
  It cannot tell a crashed caller from one that is still waiting. Accepted (decisions.md D10): the
  bound is finite, and the line still says the truth about a worker an autopilot started still
  running.
- [A main-session gap without agk commands longer than 1200 s (a very long verify)] → the line and
  the live file would expire and no later beat could bring them back. The skill runs `agk autopilot
  on` again right after every verify run, the one long step of the main session; a gap elsewhere
  past 20 minutes still drops the line for the rest of the run, a known limit stated in README.
- [Emoji in names] → names go to agtermctl as their own argv element, no shell quoting; the fish
  `--command` restriction applies only to the companion command, not the name.
- [Concurrent change `codex-turn-over-fallback` also edits `internal/agterm/agtermtest/fake.go` and
  README] → expected merge conflicts at `agk sync`, resolved there.
