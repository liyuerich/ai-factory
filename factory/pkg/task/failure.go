// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package task

import (
	"fmt"
	"strings"
)

// FailureReason is a stable classification for common agent failures.
// It is used as the FactoryTask status Reason and in issue comments.
type FailureReason string

const (
	// ModelOutputTruncated means the model response was cut off by a token limit.
	ModelOutputTruncated FailureReason = "ModelOutputTruncated"
	// ToolRoundsExhausted means the agent consumed all available tool rounds.
	ToolRoundsExhausted FailureReason = "ToolRoundsExhausted"
	// EmptyRepairScript means the model returned an empty repair script after
	// a generated script failed.
	EmptyRepairScript FailureReason = "EmptyRepairScript"
	// ModelTimeout means the model API or network request timed out.
	ModelTimeout FailureReason = "ModelTimeout"
	// ValidationFailed means validation commands (e.g. go test) failed.
	ValidationFailed FailureReason = "ValidationFailed"
	// CommandUnavailable means an expected tool was not available in the sandbox.
	CommandUnavailable FailureReason = "CommandUnavailable"
	// NoChangeRequest means the task expected a change request but no commit,
	// branch update, or pull/merge request was created.
	NoChangeRequest FailureReason = "NoChangeRequest"
)

// FailureClassification groups the stable reason and a user-friendly message
// with the original raw message preserved for debugging.
type FailureClassification struct {
	Reason        FailureReason
	Friendly      string
	RawMessage    string
	RawStderrTail string
}

// ClassifyFailure inspects a raw failure message and returns a stable reason
// and a friendly explanation. If no known pattern matches, Reason is empty.
func ClassifyFailure(message string) FailureClassification {
	lower := strings.ToLower(message)
	fc := FailureClassification{RawMessage: message}

	switch {
	case strings.Contains(lower, "finish_reason") && strings.Contains(lower, "length"):
		fc.Reason = ModelOutputTruncated
		fc.Friendly = "Model output was truncated by the token limit."
	case strings.Contains(lower, "max tool rounds") ||
		strings.Contains(lower, "shell tool limit") ||
		strings.Contains(lower, "tool calls during all final script attempts"):
		fc.Reason = ToolRoundsExhausted
		fc.Friendly = "The agent used all available shell tool rounds before producing a final script."
	case strings.Contains(lower, "empty repair script") ||
		strings.Contains(lower, "empty repair response"):
		fc.Reason = EmptyRepairScript
		fc.Friendly = "The model returned an empty repair response after the generated script failed."
	case strings.Contains(lower, "api request failed") ||
		strings.Contains(lower, "unexpected eof") ||
		strings.Contains(lower, "timed out"):
		fc.Reason = ModelTimeout
		fc.Friendly = "The model API or network request failed or timed out."
	case strings.Contains(lower, "go test") && (strings.Contains(lower, "fail") || strings.Contains(lower, "error")):
		fc.Reason = ValidationFailed
		fc.Friendly = "Validation failed while running Go tests."
	case strings.Contains(lower, "syntaxerror") || strings.Contains(lower, "syntax error"):
		fc.Reason = ValidationFailed
		fc.Friendly = "The generated script had a syntax error."
	case strings.Contains(lower, "command not found") || strings.Contains(lower, "executable file not found"):
		fc.Reason = CommandUnavailable
		fc.Friendly = "The sandbox is missing a command required by the generated script."
	case strings.Contains(lower, "no changes to commit") ||
		strings.Contains(lower, "no change branch push needed") ||
		strings.Contains(lower, "no change request created"):
		fc.Reason = NoChangeRequest
		fc.Friendly = "The task completed without creating a change request."
	}

	return fc
}

// ShouldRetryFailure returns true for failures that are often transient or can
// be recovered by asking the agent to regenerate a smaller/fixed script.
func ShouldRetryFailure(fc FailureClassification) bool {
	switch fc.Reason {
	case ModelOutputTruncated, ToolRoundsExhausted, EmptyRepairScript, ModelTimeout:
		return true
	default:
		return false
	}
}

// WithStderrTail returns a copy of fc with the given stderr tail attached.
func (fc FailureClassification) WithStderrTail(tail string) FailureClassification {
	fc.RawStderrTail = tail
	return fc
}

// FriendlyFailureMessage formats a classification as a user-friendly issue
// comment while preserving the raw message for debugging.
func FriendlyFailureMessage(fc FailureClassification) string {
	message := fc.RawMessage
	if fc.Friendly != "" {
		message = fmt.Sprintf("Likely cause: %s\n\n%s", fc.Friendly, message)
	}
	if fc.RawStderrTail != "" {
		message = fmt.Sprintf("%s\n\n**stderr tail:**\n```\n%s\n```", message, fc.RawStderrTail)
	}
	return message
}

// FailureReasonList returns all supported failure reasons for tests or validation.
func FailureReasonList() []FailureReason {
	return []FailureReason{
		ModelOutputTruncated,
		ToolRoundsExhausted,
		EmptyRepairScript,
		ModelTimeout,
		ValidationFailed,
		CommandUnavailable,
		NoChangeRequest,
	}
}
