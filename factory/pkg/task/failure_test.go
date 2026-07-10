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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := ClassifyFailure(tt.message)
			if fc.Reason != tt.want {
				t.Fatalf("ClassifyFailure() reason = %q, want %q", fc.Reason, tt.want)
			}
			msg := FriendlyFailureMessage(fc)
			if !strings.Contains(strings.ToLower(msg), tt.wantMsg) {
				t.Fatalf("FriendlyFailureMessage() = %q, want substring %q", msg, tt.wantMsg)
			}
			if !strings.Contains(msg, tt.message) {
				t.Fatalf("FriendlyFailureMessage() should preserve raw message, got %q", msg)
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

func TestFailureClassificationStderrTail(t *testing.T) {
	fc := ClassifyFailure("empty repair script").WithStderrTail("patch: no valid hunks")
	msg := FriendlyFailureMessage(fc)
	if !strings.Contains(msg, "stderr tail") {
		t.Fatalf("missing stderr tail in %q", msg)
	}
	if !strings.Contains(msg, "patch: no valid hunks") {
		t.Fatalf("missing tail content in %q", msg)
	}
}

func TestFailureReasonList(t *testing.T) {
	reasons := FailureReasonList()
	want := []FailureReason{
		ModelOutputTruncated,
		ToolRoundsExhausted,
		EmptyRepairScript,
		ModelTimeout,
		ValidationFailed,
	}
	if len(reasons) != len(want) {
		t.Fatalf("len(reasons) = %d, want %d", len(reasons), len(want))
	}
	for i, r := range reasons {
		if r != want[i] {
			t.Fatalf("reasons[%d] = %q, want %q", i, r, want[i])
		}
	}
}
