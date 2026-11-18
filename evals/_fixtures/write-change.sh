#!/bin/bash
# Writes the fixed change (add-widget, or the name given as the one argument)
# for the eval stub openspec-propose skill; the scaffold stages it as
# .eval/write-change.sh in the case workspace. A given name that is already an
# active change is refused, like `openspec new change` does; without an argument
# the fixed change is (re)written as before.
set -euo pipefail
root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
name="${1:-add-widget}"
[[ "$name" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]] || { echo "write-change: invalid change name \"$name\": want kebab-case" >&2; exit 1; }
if [ $# -ge 1 ] && [ -d "$root/openspec/changes/$name" ]; then
  echo "write-change: change \"$name\" already exists; pick another name" >&2
  exit 1
fi
d="$root/openspec/changes/$name"
mkdir -p "$d/specs/widget"
cat > "$d/.openspec.yaml" <<'X'
schema: spec-driven
X
cat > "$d/proposal.md" <<'X'
# Proposal

## Why

Users want to list the widgets of a project from the command line.

## What Changes

- New `widget list` command that prints the project's widgets, one per line.

## Capabilities

### New Capabilities

- `widget`: listing widgets.
X
cat > "$d/design.md" <<'X'
# Design

## Decisions

**Widgets come from the `widgets/` directory.** `widget list` prints the name of every file in
`widgets/`, sorted. No configuration file is read.
X
cat > "$d/specs/widget/spec.md" <<'X'
# Spec Delta

## ADDED Requirements

### Requirement: List widgets
`widget list` SHALL print the name of every file in `widgets/`, sorted, one per line.

#### Scenario: Two widgets
- **GIVEN** `widgets/` holds `b` and `a`
- **WHEN** the user runs `widget list`
- **THEN** it prints `a` and then `b`
X
cat > "$d/tasks.md" <<'X'
# Tasks

## 1. Widget list

- [ ] 1.1 Add the `widget list` command printing the sorted file names of `widgets/`, with a test for two widgets.
X
find "$d" -type f | sed "s#^$root/##" | sort
