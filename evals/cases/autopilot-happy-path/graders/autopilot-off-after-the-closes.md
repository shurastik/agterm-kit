---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review close [^\n]*\n[\s\S]*^agk review close [^\n]*\n[\s\S]*^agk autopilot off$
