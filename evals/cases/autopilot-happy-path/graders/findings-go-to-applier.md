---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^  round 1: 1 finding\(s\), verdict needs-attention\n[\s\S]*^agk apply send [^\n]*\n[\s\S]*^agk review send [^\n]*\n[\s\S]*^  round 2: 0 finding\(s\), verdict approve
