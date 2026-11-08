---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review start --base [^\n]*\n[\s\S]*^agk review wait [^\n]*\n[\s\S]*^verify add-widget$[\s\S]*^archive add-widget$
