---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk sync add-widget$[\s\S]*^verify add-widget$
