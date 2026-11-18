#!/bin/bash
# Stages the case workspace; refuses to run without the stand-ins first on PATH.
exec bash "$(dirname "$0")/../../_fixtures/scaffold-common.sh"
