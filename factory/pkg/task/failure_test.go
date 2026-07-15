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
	"strings"
	"testing"
)

func TestClassifyFailureRecognizesKnownReasons(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    FailureReason
		wantMsg string
		wantCmd string
	}{
		{
			name:    "ModelOutputTruncated",
			message: "OpenAI error: finish_reason=length, output truncated",
			want:    ModelOutputTruncated,
			wantMsg: "truncated",
		},
		{
			name:    "ToolRoundsExhausted",
			message: "OpenAI-compatible model reached max tool rounds (40)",
			want:    ToolRoundsExhausted,
			wantMsg: "shell tool rounds",
		},
		{
			name:    "EmptyRepairScript",
			message: "agent failed: empty repair script after apply",
			want:    EmptyRepairScript,
			wantMsg: "empty repair script",
		},
		{
			name:    "EmptyRepairResponseToolCalls",
			message: "OpenAI-compatible model returned an empty repair response with tool calls",
			want:    EmptyRepairScript,
			wantMsg: "empty repair response",
		},
		{
			name:    "RepeatedInvalidRepairScript",
			message: "RepeatedInvalidRepairScript: candidate matches a previously failed script",
			want:    RepeatedInvalidRepairScript,
			wantMsg: "stopped before executing it again",
		},
		{
			name:    "RepeatedInvalidRepairResponseFormat",
			message: "RepeatedInvalidRepairResponseFormat: the model returned the same invalid format again",
			want:    RepeatedInvalidRepairResponseFormat,
			wantMsg: "same invalid repair response format",
		},
		{
			name:    "RepairRoundsExhausted",
			message: "RepairRoundsExhausted: no repair succeeded within 3 rounds",
			want:    RepairRoundsExhausted,
			wantMsg: "repair-round limit",
		},
		{
			name:    "InvalidGeneratedScript",
			message: "OpenAI-compatible generated script validation failed: response contained explanatory prose instead of a shell script",
			want:    InvalidGeneratedScript,
			wantMsg: "non-executable or invalid shell content",
		},
		{
			name:    "InvalidGeneratedScriptTimeoutPrecedence",
			message: "OpenAI-compatible generated script validation failed: shell syntax validation timed out",
			want:    InvalidGeneratedScript,
			wantMsg: "non-executable or invalid shell content",
		},
		{
			name:    "UnterminatedPythonTripleQuotedString",
			message: "embedded Python syntax validation failed: SyntaxError: unterminated triple-quoted string literal",
			want:    InvalidGeneratedScript,
			wantMsg: "non-executable or invalid shell content",
		},
		{
			name:    "MalformedShellHeredoc",
			message: "shell syntax validation failed: here-document delimited by end-of-file",
			want:    InvalidGeneratedScript,
			wantMsg: "non-executable or invalid shell content",
		},
		{
			name:    "MarkdownFencedScript",
			message: "generated script validation failed: Markdown code fences are not allowed",
			want:    InvalidGeneratedScript,
			wantMsg: "non-executable or invalid shell content",
		},
		{
			name:    "ModelTimeout",
			message: "api request failed: unexpected EOF while waiting for response",
			want:    ModelTimeout,
			wantMsg: "network request",
		},
		{
			name:    "ValidationFailedGoTest",
			message: "go test ./... failed: error exit status 1",
			want:    ValidationFailed,
			wantMsg: "running go tests",
		},
		{
			name:    "ValidationFailedSyntaxError",
			message: "SyntaxError: unexpected token '}' in generated script",
			want:    ValidationFailed,
			wantMsg: "syntax error",
		},
		{
			name:    "CommandUnavailableBash",
			message: "/tmp/ai-factory-agent.sh: line 16: go: command not found",
			want:    CommandUnavailable,
			wantMsg: "missing the \"go\" command",
			wantCmd: "go",
		},
		{
			name:    "CommandUnavailableGoExec",
			message: `exec: "terraform": executable file not found in $PATH`,
			want:    CommandUnavailable,
			wantMsg: "missing the \"terraform\" command",
			wantCmd: "terraform",
		},
		{
			name:    "CommandUnavailableYAML",
			message: "/tmp/ai-factory-agent.sh: line 3: yaml: command not found",
			want:    CommandUnavailable,
			wantMsg: "missing the \"yaml\" command",
			wantCmd: "yaml",
		},
		{
			name:    "NoChangeRequest",
			message: "no change request created: source branch missing",
			want:    NoChangeRequest,
			wantMsg: "without creating a change request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := ClassifyFailure(tt.message)
			if fc.Reason != tt.want {
				t.Fatalf("ClassifyFailure() reason = %q, want %q", fc.Reason, tt.want)
			}
			msg := FriendlyFailureMessage(fc)
			if !strings.Contains(strings.ToLower(msg), strings.ToLower(tt.wantMsg)) {
				t.Fatalf("FriendlyFailureMessage() = %q, want substring %q", msg, tt.wantMsg)
			}
			if !strings.Contains(msg, tt.message) {
				t.Fatalf("FriendlyFailureMessage() should preserve raw message, got %q", msg)
			}
			if tt.wantCmd != "" && fc.MissingCommand != tt.wantCmd {
				t.Fatalf("MissingCommand = %q, want %q", fc.MissingCommand, tt.wantCmd)
			}
		})
	}
}

func TestClassifyFailureCommandNotFoundNegativeCases(t *testing.T) {
	cases := []struct {
		name    string
		message string
	}{
		{
			name:    "permission denied",
			message: "exec: \"./tool\": permission denied",
		},
		{
			name:    "path syntax error",
			message: "bash: syntax error near unexpected token `go'",
		},
		{
			name:    "runtime error",
			message: "panic: runtime error: invalid memory address or nil pointer dereference",
		},
		{
			name:    "command not found in user log",
			message: "Please install go: command not found is not an error I can fix",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fc := ClassifyFailure(tt.message)
			if fc.Reason == CommandUnavailable {
				t.Fatalf("classified as CommandUnavailable, message: %q", tt.message)
			}
		})
	}
}

func TestClassifyFailureReturnsEmptyForUnknown(t *testing.T) {
	fc := ClassifyFailure("some unexpected failure")
	if fc.Reason != "" {
		t.Fatalf("reason = %q, want empty", fc.Reason)
	}
	if FriendlyFailureMessage(fc) != fc.RawMessage {
		t.Fatalf("FriendlyFailureMessage() should return raw message unchanged")
	}
}

func TestShouldRetryFailure(t *testing.T) {
	tests := []struct {
		reason FailureReason
		want   bool
	}{
		{reason: ModelOutputTruncated, want: true},
		{reason: ToolRoundsExhausted, want: true},
		{reason: EmptyRepairScript, want: true},
		{reason: RepeatedInvalidRepairScript, want: false},
		{reason: RepeatedInvalidRepairResponseFormat, want: false},
		{reason: RepairRoundsExhausted, want: false},
		{reason: InvalidGeneratedScript, want: false},
		{reason: ModelTimeout, want: true},
		{reason: ValidationFailed, want: false},
		{reason: CommandUnavailable, want: false},
		{reason: NoChangeRequest, want: false},
		{reason: "", want: false},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			got := ShouldRetryFailure(FailureClassification{Reason: tt.reason})
			if got != tt.want {
				t.Fatalf("ShouldRetryFailure(%q) = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

func TestFailureClassificationStderrTail(t *testing.T) {
	fc := ClassifyFailure("empty repair script").WithStderrTail("patch: no valid hunks")
	msg := FriendlyFailureMessage(fc)
	if !strings.Contains(msg, "stderr tail") {
		t.Fatalf("FriendlyFailureMessage() should include stderr tail, got %q", msg)
	}
	if !strings.Contains(msg, fc.RawStderrTail) {
		t.Fatalf("FriendlyFailureMessage() should include tail content, got %q", msg)
	}
}

func TestFailureReasonList(t *testing.T) {
	reasons := FailureReasonList()
	if len(reasons) == 0 {
		t.Fatal("expected non-empty failure reason list")
	}
	seen := make(map[FailureReason]bool)
	for _, r := range reasons {
		if seen[r] {
			t.Fatalf("duplicate reason %q", r)
		}
		seen[r] = true
	}
	for _, want := range []FailureReason{ModelOutputTruncated, ToolRoundsExhausted, EmptyRepairScript, InvalidGeneratedScript, ModelTimeout, ValidationFailed, CommandUnavailable, NoChangeRequest} {
		if !seen[want] {
			t.Fatalf("expected reason %q in FailureReasonList", want)
		}
	}
}
