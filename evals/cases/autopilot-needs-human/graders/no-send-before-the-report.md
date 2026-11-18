---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
match: not_contains
---
^  apply round 1: needs-human\n(agk apply send)
