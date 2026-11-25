---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk autopilot on$[\s\S]*^agk review start --change add-widget
