---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^agk apply start add-widget\n[\s\S]*^agk review close [^\n]*\.agk/reviews/1\n[\s\S]*^agk review start --change add-widget\n  files: [^\n]*\n  cwd: [^\n]*\.eval/worktree\n
