---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review start --change add-widget\n  files: [^\n]*decisions\.md
