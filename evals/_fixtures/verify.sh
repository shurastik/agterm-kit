#!/bin/bash
# Check results for the eval stub openspec-verify-change skill; the scaffold
# stages it as .eval/verify.sh in the case workspace. Like the real skill's
# checks it only prints results: the agent writes the verification report.
set -euo pipefail
change="${1:-}"
[ -n "$change" ] || { echo "usage: verify.sh <change>" >&2; exit 1; }
root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
echo "verify $change" >> "$root/agk-calls.log"
# The case's marker plays a file changing while verify runs.
if [ -f "$root/.eval/outcomes/${EVAL_AGK_CASE:-none}/edit-during-verify" ]; then
  echo "// edited while verify ran" >> "$root/widget.go"
fi
results="$root/.eval/outcomes/${EVAL_AGK_CASE:-none}/verify.txt"
if [ -f "$results" ]; then
  cat "$results"
else
  cat <<X
Check results for $change:
- completeness: all tasks in tasks.md are done: pass
- correctness: every requirement of the change has an implementation: pass
- coherence: the code follows design.md: pass
X
fi
