---
type: regex
target: {source: file, path: agk-calls.log}
flags: m
---
^  apply round 1: blocked\n[\s\S]*^agk review send [^\n]*\n[\s\S]*?  changed since round 1: [^\n]*tasks\.md[\s\S]*^  round 2: 0 finding\(s\), verdict approve\n[\s\S]*^agk apply send 
