---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk review start --change add-widget[^\n]*\n  files: [^\n]*\n  cwd: [^\n]*\.eval/worktree\n[\s\S]*^  round 1: 0 finding\(s\), verdict approve\n[\s\S]*^agk apply send 
