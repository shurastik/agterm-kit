#!/bin/bash
# Stages the case workspace; refuses to run without the stand-ins first on PATH.
fix="$(dirname "$0")/../../_fixtures"
bash "$fix/scaffold-common.sh" && bash "$fix/scaffold-archive.sh" --change --main-branch --code --run applies:1:add-widget
