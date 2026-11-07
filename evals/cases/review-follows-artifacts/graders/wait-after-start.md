---
type: regex
target: {source: file, path: agk-calls.log}
flags: s
---
agk review start --change add-widget.*\nagk review wait \S*\.agk/reviews/1\n
