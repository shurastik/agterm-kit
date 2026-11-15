---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^verify add-widget$[\s\S]*^agk sync add-widget$[\s\S]*^verify add-widget$[\s\S]*^archive add-widget$
