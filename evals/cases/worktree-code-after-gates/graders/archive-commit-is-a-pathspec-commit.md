---
type: regex
target: trace
match: not_contains
---
git add -A(?! -- openspec)[^\n]{0,40}&&[^\n]{0,40}git commit[^\n]{0,300}chore\(openspec\): archive add-widget
