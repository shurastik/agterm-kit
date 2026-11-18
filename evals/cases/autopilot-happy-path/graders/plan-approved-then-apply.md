---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review wait [^\n]*\n  round 1: 0 finding\(s\), verdict approve\n[\s\S]*^agk apply start add-widget
