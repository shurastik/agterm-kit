---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^archive add-widget$[\s\S]*^agk apply close [^\n]*applies/1$
