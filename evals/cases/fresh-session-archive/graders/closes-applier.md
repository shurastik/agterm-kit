---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk apply close [^\n]*applies/1$
