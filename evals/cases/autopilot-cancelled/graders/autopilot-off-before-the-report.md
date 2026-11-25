---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^  round 1: wait played cancelled\n[\s\S]*^agk autopilot off$
