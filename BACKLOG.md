# Backlog

Known bugs and follow-ups, newest first. Each entry becomes an OpenSpec change (`/agk-explore`)
when it is picked up.

## Codex reviewer stuck `active` after the hooks-trust prompt, `agk review send` times out

**Type:** bug · **Priority:** P2

A codex reviewer that started on codex's "Hooks need review" screen (`agk review start` printed `"started":false`) and was then trusted by the user ran round 1 and wrote `out/1.json` (`agk review wait` completed), but its agterm session status stayed `active` from the start on (`statusChangedAt` never moved). `agk review send` then waited for the turn to end and failed after 600 s with `the reviewer … is busy: its turn did not end within 600 s`. Closing it and starting a fresh run (hooks already trusted) worked. Find out whether codex skips the hooks of the session in which they were trusted, and make `send` cope (e.g. accept a validated `out/<k>.json` of the current round as "turn over", or tell the user to restart the reviewer).
