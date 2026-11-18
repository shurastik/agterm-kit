#!/bin/bash
# Check results for the eval stub openspec-verify-change skill; the scaffold
# stages it as .eval/verify.sh in the case workspace. Like the real skill's
# checks it only prints results: the agent writes the verification report.
# A case can play a sequence: the n-th call of a workspace prints verify-<n>.txt
# when the case holds it (else verify.txt) and plays the edit-during-verify-<n>
# marker; the plain edit-during-verify marker edits on every call.
set -euo pipefail
change="${1:-}"
[ -n "$change" ] || { echo "usage: verify.sh <change>" >&2; exit 1; }
root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
# In a change worktree made by the stand-in `agk apply start`, the log, the
# outcomes and the call counter are the main workspace's (git config eval.log).
logroot="$(git config --get eval.log 2>/dev/null || true)"
if [ -z "$logroot" ] || [ ! -d "$logroot" ]; then logroot="$root"; fi
echo "verify $change" >> "$logroot/agk-calls.log"
outcomes="$logroot/.eval/outcomes/${EVAL_AGK_CASE:-none}"
n=$(( $(cat "$logroot/.eval/verify-calls" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$logroot/.eval/verify-calls"
# The case's marker plays a file changing while verify runs.
if [ -f "$outcomes/edit-during-verify" ] || [ -f "$outcomes/edit-during-verify-$n" ]; then
  echo "// edited while verify ran" >> "$root/widget.go"
fi
results="$outcomes/verify.txt"
[ -f "$outcomes/verify-$n.txt" ] && results="$outcomes/verify-$n.txt"
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
