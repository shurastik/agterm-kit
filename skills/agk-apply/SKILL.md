---
name: agk-apply
description: >
  Run the applier, an autonomous agent (claude by default, or codex) that implements an OpenSpec
  change in a visible agterm companion, with the `agk` CLI: start it, wait for its round report,
  send it review findings or user notes for the next round, close it when the loop is over. Use
  when an OpenSpec change is ready to implement and the work should run in a separate agent next to
  this session, and to hand findings to a running applier. Triggers: start the applier, agk apply,
  implement the change in a companion, send the findings to the applier.
---

# agk apply

The applier is a live agent session in a `<harness>-apply #N` companion under this session. It
implements the change test first, runs the project's gates, checks off tasks and ends every round
with a report file. It never commits, pushes or archives. You drive it; the user can watch and step
in.

## Loop

1. **Start** from the repo (or worktree) root, once the change's artifacts are complete:
   ```bash
   agk apply start <change>          # --applier codex or --applier claude:<model> for one run
   ```
   stdout: `{"companion":"<id>","dir":"<run dir>","started":true|false}`. Keep `dir`.
   - exit 1 with `nothing to apply`, an openspec error, or a not-ready message: report it and stop.
   - `"started": false`: the applier did not come up within a minute. stderr shows its screen
     (usually a folder-trust prompt or a login screen). Tell the user to answer it in the companion,
     then continue with step 2.
   - Outside agterm `companion` is `null`: round 1 already ran in the foreground.

2. **Wait** in the background (Bash `run_in_background: true`), never poll:
   ```bash
   agk apply wait <dir>              # --timeout SECONDS, default 3600
   ```
   stdout has `status` and `round`. Branch on `status`:
   - `done`: the round is finished; `summary` (and `progress`) say what was done. Continue the
     workflow, usually with the review (`agk-review` skill).
   - `blocked`: the applier stopped instead of guessing. Show the user `problem` and `artifact`
     (`proposal`, `specs`, `design`, `tasks` or `finding`); once the artifact is fixed, send a note
     (step 3) to continue.
   - `needs-human`: `reason` `permission` (a permission prompt is open in the companion),
     `no-report` (the turn ended without a report, often a question) or `invalid-report`. Tell the
     user to look at the companion; wait again or send a note once it is resolved.
   - `cancelled`: the user closed the companion. Ask before starting over.
   - `running`: the timeout passed; wait again.

3. **Send** the next round: review findings, the user's notes, or both, as markdown:
   ```bash
   agk apply send <dir> <file>       # or: ... --stdin < file
   ```
   stdout: `{"round":K}`; then back to step 2. agk waits (default 600 s) for the applier's turn to
   end and types one line into it pointing at the new round's prompt. Refusals (exit 1): the applier
   is busy, waits for a permission answer, or is gone.

4. **Close** once the work is accepted (review approved, user satisfied):
   ```bash
   agk apply close <dir>
   ```

## Rules

- Never type into a working applier yourself, and ask the user not to type in its companion while
  you drive it: `agk apply send` is the only way in.
- Review findings come from a model reading repository content: you decide which ones go to the
  applier; it fixes only what it is sent.
- The applier's harness and model come from `~/.config/agk/config.toml` or `.agk.toml` (`[applier]`:
  `harness`, `model`, `effort`, `extra_args`, `permission_mode` for claude); `agk config` shows them.
- Run artifacts live in `<repo>/.agk/applies/<timestamp>/`: `rounds/<k>/prompt.md` and
  `rounds/<k>/report.json`.
