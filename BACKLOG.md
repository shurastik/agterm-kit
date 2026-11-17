# Backlog

Known bugs and follow-ups, newest first. Each entry becomes an OpenSpec change (`/agk-explore`)
when it is picked up.

## Codex reviewer stuck `active` after the hooks-trust prompt, `agk review send` times out

**Type:** bug · **Priority:** P2

A codex reviewer that started on codex's "Hooks need review" screen (`agk review start` printed `"started":false`) and was then trusted by the user ran round 1 and wrote `out/1.json` (`agk review wait` completed), but its agterm session status stayed `active` from the start on (`statusChangedAt` never moved). `agk review send` then waited for the turn to end and failed after 600 s with `the reviewer … is busy: its turn did not end within 600 s`. Closing it and starting a fresh run (hooks already trusted) worked. Find out whether codex skips the hooks of the session in which they were trusted, and make `send` cope (e.g. accept a validated `out/<k>.json` of the current round as "turn over", or tell the user to restart the reviewer).

## Review companion name: plan/code kind and round number (was agk-7t2)

**Type:** feature · **Priority:** P2

The reviewer companion is named <harness>-review #N where N is the repo-wide run number (#15 today), which reads like a pass number and does not say what is reviewed. Wanted: the name shows the kind (plan review via --change vs code review) and the number counted inside one agk review run, i.e. the current round, updated on every agk review send. Modifies the review-companion 'Review number' requirement and the companion name. Separate OpenSpec change after agk-propose.
