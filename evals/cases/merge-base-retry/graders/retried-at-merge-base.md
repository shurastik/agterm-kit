---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review start --base main --for add-widget$[\s\S]*^agk review start --base [0-9a-f]{40} --for add-widget$[\s\S]*^verify add-widget$[\s\S]*^archive add-widget$
