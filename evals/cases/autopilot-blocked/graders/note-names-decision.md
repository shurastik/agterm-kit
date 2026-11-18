---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk apply send [^\n]*\n(  [^\n]*\n)*?  note: [^\n]*\bD\d+\b
