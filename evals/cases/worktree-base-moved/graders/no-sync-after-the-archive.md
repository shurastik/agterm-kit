---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
match: not_contains
---
^archive add-widget$[\s\S]*^agk sync 
