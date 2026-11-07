#!/bin/bash
# Writes the fixed change add-widget for the eval stub openspec-propose skill;
# the scaffold stages it as .eval/write-change.sh in the case workspace.
set -euo pipefail
root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
d="$root/openspec/changes/add-widget"
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
