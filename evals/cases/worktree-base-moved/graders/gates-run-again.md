---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk sync add-widget$[\s\S]*^verify add-widget$[\s\S]*^agk sync add-widget$[\s\S]*^agk review start --base main --for add-widget$[\s\S]*^verify add-widget$[\s\S]*^agk sync add-widget$[\s\S]*^archive add-widget$[\s\S]*^agk finish add-widget$
