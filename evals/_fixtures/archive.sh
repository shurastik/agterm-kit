#!/bin/bash
# Archive step for the eval stub openspec-archive-change skill; the scaffold
# stages it as .eval/archive.sh in the case workspace. It moves the change under
# openspec/changes/archive/, or fails when the case holds an archive-fail marker.
set -euo pipefail
change="${1:-}"
[ -n "$change" ] || { echo "usage: archive.sh <change>" >&2; exit 1; }
root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
echo "archive $change" >> "$root/agk-calls.log"
if [ -f "$root/.eval/outcomes/${EVAL_AGK_CASE:-none}/archive-fail" ]; then
  echo "archive: cannot move openspec/changes/$change: target is locked" >&2
  exit 1
fi
[ -d "$root/openspec/changes/$change" ] || { echo "archive: change \"$change\" not found" >&2; exit 1; }
mkdir -p "$root/openspec/changes/archive"
mv "$root/openspec/changes/$change" "$root/openspec/changes/archive/2026-11-08-$change"
echo "archived $change as openspec/changes/archive/2026-11-08-$change"
