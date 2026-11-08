---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review wait [^\n]*\n[\s\S]*^agk review fresh 
