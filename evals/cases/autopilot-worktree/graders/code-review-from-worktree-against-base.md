---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review start --base main --for add-widget[^\n]*\n  cwd: [^\n]*\.eval/worktree
