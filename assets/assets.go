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

// ApplierRules are the applier's standing rules: the claude applier gets them
// as an appended system prompt, the codex applier at the top of each prompt.
//
//go:embed applier-rules.md
var ApplierRules string

// ApplyPrompt is the text/template of the applier's first round.
//
//go:embed apply-prompt.md
var ApplyPrompt string

// FixPrompt is the text/template of a later applier round.
//
//go:embed fix-prompt.md
var FixPrompt string

// ClaudeApplierSettings is the claude applier's settings layer: deny rules
// for committing, pushing and rewriting history.
//
//go:embed claude-applier-settings.json
var ClaudeApplierSettings []byte

// PlanReviewPrompt is the text/template for the review of an OpenSpec
// change's planning artifacts (`agk review start --change`).
//
//go:embed plan-review-prompt.md
var PlanReviewPrompt string
