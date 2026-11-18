---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
match: not_contains
---
^  round 1: wait played cancelled\n[\s\S]*^(agk review send|agk review start|verify |agk review close|agk apply close|agk apply send)
