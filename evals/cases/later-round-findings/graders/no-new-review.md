---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
match: not_contains
---
^agk review start --base [^\n]*\n[\s\S]*^agk review start --base 
