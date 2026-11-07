#!/bin/bash
# Shared scaffold of every eval case; runs in the case workspace. It refuses
# to stage the case unless agk and revdiff resolve to the stand-ins, so a run
# outside scripts/eval-skills.sh never reaches the real tools.
set -euo pipefail
fix="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
for tool in agk revdiff; do
  got="$(command -v "$tool" || true)"
  real=""
  if [ -n "$got" ]; then real="$(cd "$(dirname "$got")" && pwd -P)/$(basename "$got")"; fi
  if [ "$real" != "$fix/bin/$tool" ]; then
    echo "eval scaffold: $tool resolves to '${got:-nothing}', not the stand-in $fix/bin/$tool; run the suite with scripts/eval-skills.sh" >&2
    exit 1
  fi
done
g() { git -c user.name=eval -c user.email=eval@example.invalid -c commit.gpgsign=false -c core.hooksPath=/dev/null "$@"; }
# The agent's Bash sandbox cannot read the repo's eval dirs: stage what the
# stub skills and the stand-ins read into the workspace.
mkdir -p .eval
cp "$fix/write-change.sh" .eval/write-change.sh
cp -R "$fix/outcomes" .eval/outcomes
g init -q -b main
mkdir -p openspec/specs openspec/changes
printf '# Demo project\n\nA small CLI that the eval cases plan changes for.\n' > README.md
printf '.agk/\n.eval/\nagk-calls.log\n' > .gitignore
touch openspec/specs/.keep openspec/changes/.keep
g add -A
g commit -qm init
