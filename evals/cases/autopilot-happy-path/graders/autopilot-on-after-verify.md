---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^verify add-widget$\n(?:  [^\n]*\n)*agk autopilot on$
