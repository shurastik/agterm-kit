---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^verify add-widget$[\s\S]*^verify add-widget$\n[\s\S]*^agk review fresh [^\n]*\n  fresh: true
