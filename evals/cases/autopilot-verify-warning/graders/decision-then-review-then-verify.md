---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^verify add-widget$[\s\S]*^agk review send [^\n]*\n(  [^\n]*\n)*?  note: [^\n]*\bD\d+\b[\s\S]*^verify add-widget$
