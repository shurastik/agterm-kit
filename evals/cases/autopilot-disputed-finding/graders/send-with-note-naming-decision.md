---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review send [^\n]*--note [^\n]*\n(  [^\n]*\n)*?  note: [^\n]*\bD\d+\b
