# Decisions

These are the decisions the model took in place of the user during the `/agk-autopilot` run of this
change. Points the user settled in the exploration before the run (mark both the main session and
the companions, a self-expiring line refreshed by the waits, the mark off whenever the autopilot
ends, companions keep their mark, `🤖` prefix plus tint) are not repeated here.

## D1 [scope] needs-human keeps the mark
- Open: the user said a `needs-human` is "not autopilot any more", but the skill does not end on it: it tells the user and waits again, then carries on unattended.
- Decision: the mark goes off only when the skill exits (finished or stopped); a `needs-human` it waits on again keeps it.
- Why: the user asked for the mark to go off "when the autopilot finished its work"; the run is still going, and an off/on pair around every prompt adds two calls for no gain.

## D2 [scope] Line text and color
- Open: the exact text of the main session's line.
- Decision: `🤖 autopilot`, the word in purple (`🤖 ^[autopilot](color: "purple")`), fixed, without the change name.
- Why: the change does not exist yet when `on` runs; a fixed string keeps model output out of status markdown.

## D3 [scope] Expiry 1200 s, refresh every 60 s
- Open: the exploration said "about 20 minutes" and left the refresh interval open.
- Decision: `--hide-after 1200`; waits and sends refresh at most once per 60 s, and once on entry.
- Why: a worker's round is covered by the wait loop whatever its length; the expiry only bridges main-session gaps, and 60 s keeps agtermctl calls rare.

## D4 [impl] Live state in a per-session file under the user's state dir
- Open: how a refresh knows the autopilot is still on, so that a manual command after `off`, or after a crash, does not light the line up again.
- Decision: `$XDG_STATE_HOME/agk/autopilot/<session>.json` (else `~/.local/state/...`) with an `until` that mirrors the line's expiry; `off` removes it.
- Why: the repo's `.agk/` is split between the main checkout and the worktree in worktree mode; reading the line back from agterm would depend on its tree format.

## D5 [scope] Command output and exit codes
- Open: what `agk autopilot on|off` prints and when it fails.
- Decision: `{"session": "<id>"|null, "marked": <bool>}`, exit 0 always except a usage error; agterm refusals are warnings.
- Why: the mark is cosmetic, like the per-round rename, and must never stop an autopilot run.

## D6 [scope] Mark renewed after every verify run
- Open: a main-session step longer than the expiry (a long `openspec-verify-change`) would let the line and the live state expire for the rest of the run.
- Decision: the skill runs `agk autopilot on` again right after every verify run.
- Why: verify is the one long step of the main session without agk calls; `on` is idempotent.

## D7 [scope] Tint color and placement
- Open: which color and where.
- Decision: purple, on the icon only (`--on icon`), matching the line's color.
- Why: the name already carries `🤖`; a colored icon catches the eye without changing the name's color.

## D8 [impl] Refresh targets the session recorded at start
- Open: which session a wait refreshes.
- Decision: the run records `autopilot_session` (the caller's session at start) in `worker.json`; the refresh targets it.
- Why: a wait run from another session must not mark that one.

## D9 [impl] Change name
- Open: the name of the change.
- Decision: `autopilot-marker`.
- Why: names the durable behavior (the autopilot's sidebar marks); no active change uses it.

## D10 [scope] A wait that outlives a crash keeps the line until it ends
- Open: plan review round 1, finding "Учесть фоновые wait, пережившие сбой основной сессии": a background wait that survives a crash of the main session's agent refreshes the line for up to its timeout, so the line can stay past 20 minutes.
- Decision: set aside; the bound is stated instead (design Risks): at most the running wait's `--timeout` plus 1200 s, and the "up to 20 minutes" claim is dropped.
- Why: agk cannot tell a crashed caller from one still waiting without a liveness signal from the agent harness; the bound is finite, the line never stays for good, and a worker the autopilot started is still running in that window.
