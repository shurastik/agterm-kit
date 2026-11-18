---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^verify add-widget$[\s\S]*?^  fresh: false\n[\s\S]*^agk review send [^\n]*\n[\s\S]*^verify add-widget$
