---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk apply send [^\n]*\n[\s\S]*^agk apply wait [^\n]*\n  apply round 2: done
