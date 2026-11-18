---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk apply wait [^\n]*\n  apply round 1: needs-human\nagk apply wait [^\n]*\n  apply round 1: done
