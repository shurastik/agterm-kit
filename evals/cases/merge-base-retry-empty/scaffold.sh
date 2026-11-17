#!/bin/bash
# Stages the case workspace; refuses to run without the stand-ins first on PATH.
# The agent commits on main itself, so the workspace carries a commit identity.
fix="$(dirname "$0")/../../_fixtures"
bash "$fix/scaffold-common.sh" && bash "$fix/scaffold-archive.sh" --change --code \
  && git config user.name eval && git config user.email eval@example.invalid \
  && git config commit.gpgsign false && git config core.hooksPath /dev/null
