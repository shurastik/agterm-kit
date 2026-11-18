#!/bin/bash
# Stages the case workspace; refuses to run without the stand-ins first on PATH.
fix="$(dirname "$0")/../../_fixtures"
bash "$fix/scaffold-common.sh" && bash "$fix/scaffold-archive.sh" --change || exit 1
printf '\n<!-- ORIGINAL-AUTHOR-NOTE: keep this change as it is -->\n' >> openspec/changes/add-widget/proposal.md
git -c user.name=eval -c user.email=eval@example.invalid -c commit.gpgsign=false -c core.hooksPath=/dev/null commit -qam "note in the existing change"
