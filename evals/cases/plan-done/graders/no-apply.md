---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
match: not_contains
---
^agk apply
