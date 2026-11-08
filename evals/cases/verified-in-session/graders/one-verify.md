---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
match: not_contains
---
^verify add-widget$[\s\S]*^verify add-widget$
