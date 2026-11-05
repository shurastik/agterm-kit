# Spec Delta

## Purpose

Lets the user decide, finding by finding, what the agent should do with a code review's results (fix, skip or discuss, with an optional note) and hands those decisions back to the agent as machine-readable JSON.

## ADDED Requirements

### Requirement: Triage input
`agk triage <review.json>` SHALL accept a findings file in the review output format and SHALL exit 1 with an error on stderr, without opening any UI, when the file is missing or does not match that format.

#### Scenario: Invalid findings file
- **WHEN** the user runs `agk triage` on a file that is not valid review output
- **THEN** the command exits 1, prints the validation error to stderr, and no overlay opens

### Requirement: No findings, no UI
When the findings file contains no findings, `agk triage` SHALL print an empty decision list `[]` and exit 0 without opening any UI.

#### Scenario: Approved review
- **WHEN** the findings file has verdict `approve` and an empty findings list
- **THEN** stdout is `[]`, the exit code is 0, and no overlay opens

### Requirement: Triage overlay inside agterm
When `AGTERM_ENABLED` and `AGTERM_SESSION_ID` are set, `agk triage` SHALL open a blocking HTML overlay on the current session, switching to it, that shows the review summary and every finding with its number, severity, title, file and line range, confidence, body and recommendation.

#### Scenario: Findings shown to the user
- **GIVEN** a findings file with 2 findings
- **WHEN** the agent runs `agk triage` from agterm session S
- **THEN** S is selected, an overlay on S shows both findings numbered 1 and 2 with all their fields, and the command blocks until the user submits or closes it

### Requirement: Per-finding decision
The triage UI SHALL let the user choose exactly one of `fix`, `skip` or `discuss` for every finding and add an optional free-text note, with `fix` preselected for `critical`, `high` and `medium` findings and `skip` preselected for `low` findings.

#### Scenario: Defaults by severity
- **WHEN** the overlay opens for findings of severity `high` and `low`
- **THEN** the `high` finding has `fix` selected and the `low` finding has `skip` selected

#### Scenario: Submit with defaults
- **WHEN** the user submits without changing anything
- **THEN** every finding receives its preselected decision and an empty note

### Requirement: Decisions output
On submit, `agk triage` SHALL print to stdout a JSON array with one object per finding, in findings order, of the form `{"id": <1-based number>, "title": "...", "file": "...", "decision": "fix|skip|discuss", "note": "..."}`, SHALL write the same array to `decisions.json` next to the findings file, and SHALL exit 0.

#### Scenario: Mixed decisions
- **GIVEN** 3 findings
- **WHEN** the user marks #1 `fix`, #2 `skip`, #3 `discuss` with note `why not a mutex?` and submits
- **THEN** stdout is a 3-element JSON array with those decisions, #3 carries the note, `decisions.json` holds the same array, and the exit code is 0

### Requirement: Dismissed triage
When the user closes the overlay without submitting, `agk triage` SHALL exit 2, print nothing to stdout and SHALL NOT write `decisions.json`.

#### Scenario: User closes the overlay
- **WHEN** the user closes the triage overlay with ⌘W
- **THEN** the command exits 2 and no decisions are emitted

### Requirement: Untrusted submitted value
`agk triage` SHALL validate the submitted value and SHALL exit 1 without emitting decisions when it is not valid JSON, references a finding that does not exist, omits a finding, or carries a decision other than `fix`, `skip` or `discuss`.

#### Scenario: Garbage result
- **WHEN** the overlay returns a submitted value that is not a valid decision list for the findings
- **THEN** the command exits 1 with an error on stderr and stdout is empty

### Requirement: Findings text cannot inject into the page
Text from the findings file SHALL be rendered as inert text: it SHALL NOT be able to add markup, run script, load external resources, or trigger any control-socket command. The page SHALL send nothing to agterm except its own submit.

#### Scenario: Hostile finding text
- **GIVEN** a finding whose title is `<script>agterm.request("session.close")</script>` and whose body contains `" onmouseover="x` and `<img src=https://example.com/x>`
- **WHEN** the triage overlay is shown
- **THEN** that text is displayed literally, no script from it runs, no network request is made, and no control-socket command other than the submit is issued

### Requirement: Fallback outside agterm
When `AGTERM_ENABLED` is not set, `agk triage` SHALL print the findings as a markdown table (number, severity, title, location, recommendation) to stdout and exit 3, so the agent can collect the decisions in chat instead.

#### Scenario: Triage outside agterm
- **GIVEN** `AGTERM_ENABLED` is unset and the findings file has findings
- **WHEN** the agent runs `agk triage`
- **THEN** no agtermctl command is executed, stdout is a markdown table with one row per finding, and the exit code is 3

#### Scenario: Table cells stay intact
- **WHEN** a finding's title or recommendation contains `|` or a newline
- **THEN** the table still has exactly one row per finding with the same number of columns

#### Scenario: Terminal control sequences are neutralized
- **WHEN** a finding's text contains terminal control characters (for example an ANSI CSI sequence or an OSC 52 clipboard write) or bidi override characters
- **THEN** stdout contains them only as visible `\uXXXX` escapes and no raw control byte reaches the terminal
