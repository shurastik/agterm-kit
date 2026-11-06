// Package assets holds files embedded into the agk binary.
package assets

import _ "embed"

// ReviewSchema is the JSON schema codex output must follow (copied from the
// codex Claude Code plugin's review-output.schema.json).
//
//go:embed review-output.schema.json
var ReviewSchema []byte

// ReviewPrompt is the text/template for the codex review prompt.
//
//go:embed review-prompt.md
var ReviewPrompt string

// ReviewerTools is the allowlist of tools for a claude reviewer, one per line;
// blank lines and # comments are ignored.
//
//go:embed reviewer-tools.txt
var ReviewerTools string

// RereviewPrompt is the text/template wrapped around the review prompt for
// a later round that resumes the reviewer's session.
//
//go:embed rereview-prompt.md
var RereviewPrompt string
