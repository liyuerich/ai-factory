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
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusMergePatchPreservesRawMessageForDebugging(t *testing.T) {
	opts := StatusPatchOptions{
		Phase:   PhaseFailed,
		Reason:  "StepFailed",
		Message: "run coding agent: agent failed",
		FailureReason: ClassifyFailure(
			"OpenAI-compatible model reached max tool rounds (40)",
		),
	}
	data, err := StatusMergePatch(opts)
	if err != nil {
		t.Fatalf("StatusMergePatch() error = %v", err)
	}
	var patch map[string]FactoryTaskStatus
	if err := json.Unmarshal(data, &patch); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	status := patch["status"]
	if len(status.Conditions) != 1 {
		t.Fatalf("len(conditions) = %d, want 1", len(status.Conditions))
	}
	cond := status.Conditions[0]
	if cond.Reason != string(ToolRoundsExhausted) {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, ToolRoundsExhausted)
	}
	if !strings.Contains(cond.Message, "OpenAI-compatible model reached max tool rounds (40)") {
		t.Fatalf("condition message should preserve raw message, got %q", cond.Message)
	}
	if !strings.Contains(cond.Message, "Likely cause") {
		t.Fatalf("condition message should include friendly explanation, got %q", cond.Message)
	}
	if status.LastMessage != opts.Message {
		t.Fatalf("status.lastMessage should keep original message, got %q", status.LastMessage)
	}
}

func TestStatusMergePatchFallsBackToCallerReason(t *testing.T) {
	opts := StatusPatchOptions{
		Phase:   PhaseFailed,
		Reason:  "SandboxClaimApplyFailed",
		Message: "kubectl apply failed: connection refused",
	}
	data, err := StatusMergePatch(opts)
	if err != nil {
		t.Fatalf("StatusMergePatch() error = %v", err)
	}
	var patch map[string]FactoryTaskStatus
	if err := json.Unmarshal(data, &patch); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	cond := patch["status"].Conditions[0]
	if cond.Reason != "SandboxClaimApplyFailed" {
		t.Fatalf("condition reason = %q, want SandboxClaimApplyFailed", cond.Reason)
	}
	if cond.Message != opts.Message {
		t.Fatalf("condition message = %q, want %q", cond.Message, opts.Message)
	}
}

func TestStatusMergePatchNonFailureUnchanged(t *testing.T) {
	opts := StatusPatchOptions{
		Phase:   PhaseSucceeded,
		Reason:  "PlanSucceeded",
		Message: "FactoryTask completed successfully",
		FailureReason: ClassifyFailure(
			"OpenAI-compatible model reached max tool rounds (40)",
		),
	}
	data, err := StatusMergePatch(opts)
	if err != nil {
		t.Fatalf("StatusMergePatch() error = %v", err)
	}
	var patch map[string]FactoryTaskStatus
	if err := json.Unmarshal(data, &patch); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	cond := patch["status"].Conditions[0]
	if cond.Reason != "PlanSucceeded" {
		t.Fatalf("condition reason = %q, want PlanSucceeded", cond.Reason)
	}
	if cond.Message != opts.Message {
		t.Fatalf("condition message = %q, want %q", cond.Message, opts.Message)
	}
}
