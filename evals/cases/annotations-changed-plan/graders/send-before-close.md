---
type: regex
target: {source: file, path: agk-calls.log}
flags: s
---
revdiff: launch 1 returned annotations.*agk review send .*agk review close
