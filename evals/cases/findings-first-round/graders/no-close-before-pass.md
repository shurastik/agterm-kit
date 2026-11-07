---
type: regex
target: {source: file, path: agk-calls.log}
flags: s
match: not_contains
---
verdict needs-attention\nagk review close
