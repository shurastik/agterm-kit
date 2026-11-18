---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review start --change [a-z0-9]+(-[a-z0-9]+)*$
