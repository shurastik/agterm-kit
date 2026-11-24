# Decisions

These are the decisions the model took in place of the user during the unattended (`/agk-autopilot`) run.
The scope itself (round instead of run number, no run number anywhere, the applier included) was settled
with the user in `/agk-explore` before the run.

## D1 [impl] Change name
- Open: the idea gives no change name.
- Decision: `companion-name-round`.
- Why: names the subject (the companion name) and what it now carries (the round); no active change uses it.

## D2 [scope] A failed rename does not fail the send
- Open: what `agk review send` / `agk apply send` do when `agtermctl session rename` fails.
- Decision: a warning on stderr; stdout and the exit code stay as without the rename.
- Why: the round has already reached the worker; failing the command would make the caller think it did not.

## D3 [scope] When the companion is renamed
- Open: rename before typing the round, after typing it, or only after the start is confirmed.
- Decision: after the line is typed, also when the start is not confirmed (`"started": false`); never when the
  send is refused (busy, permission prompt, gone).
- Why: the name follows the recorded round: an unconfirmed round stays recorded, a refused one is rolled back.

## D4 [scope] Info lines and inline log without the number
- Open: what replaces `codex #8 r1:` and `agk: review #8 round 1` once the run number is gone.
- Decision: `codex r1: ...` and `agk: review round 1 ...`.
- Why: the user asked for no run number anywhere; the round alone is what these lines need.

## D5 [impl] Name rebuilt from the recorded state
- Open: store the companion name in the run directory, or rebuild it on every send.
- Decision: rebuild it from `worker.json` (change or `for`, harness, plan vs diff) with the new round.
- Why: everything the name needs is already recorded; no extra file to keep in sync.
