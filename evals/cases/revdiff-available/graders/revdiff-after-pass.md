---
type: regex
target: {source: file, path: agk-calls.log}
flags: s
---
verdict approve.*\nrevdiff --only=
