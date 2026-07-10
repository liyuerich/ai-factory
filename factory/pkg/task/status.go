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
	"time"
)

const (
	PhasePending      = "Pending"
	PhaseClaimCreated = "ClaimCreated"
	PhaseSandboxReady = "SandboxReady"
	PhaseRunning      = "Running"
	PhaseSucceeded    = "Succeeded"
	PhaseFailed       = "Failed"
)

// StatusPatchOptions contains one FactoryTask status transition.
type StatusPatchOptions struct {
	Phase            string
	Message          string
	Reason           string
	FailureReason    FailureClassification
	SandboxClaimName string
	SandboxName      string
	LastResultURL    string
	ObservedCommit   string
	Now              time.Time
}

// failureReason returns a stable reason for failures, falling back to the caller reason.
func failureReason(opts StatusPatchOptions) string {
	if opts.Phase == PhaseFailed && opts.FailureReason.Reason != "" {
		return string(opts.FailureReason.Reason)
	}
	return opts.Reason
}

// failureMessage includes the raw message with a friendly explanation for failures.
func failureMessage(opts StatusPatchOptions) string {
	if opts.Phase == PhaseFailed && opts.FailureReason.Reason != "" {
		return FriendlyFailureMessage(opts.FailureReason)
	}
	return opts.Message
}

// StatusMergePatch renders a Kubernetes merge patch for the status subresource.
func StatusMergePatch(opts StatusPatchOptions) ([]byte, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	timestamp := now.UTC().Format(time.RFC3339)

	status := FactoryTaskStatus{
		Phase:            opts.Phase,
		SandboxClaimName: opts.SandboxClaimName,
		SandboxName:      opts.SandboxName,
		LastResultURL:    opts.LastResultURL,
		LastMessage:      opts.Message,
		ObservedCommit:   opts.ObservedCommit,
		Conditions: []Condition{
			{
				Type:               opts.Phase,
				Status:             "True",
				Reason:             failureReason(opts),
				Message:            failureMessage(opts),
				LastTransitionTime: timestamp,
			},
		},
	}
	if opts.Phase == PhaseRunning {
		status.StartedAt = timestamp
	}
	if opts.Phase == PhaseSucceeded || opts.Phase == PhaseFailed {
		status.CompletedAt = timestamp
	}

	return json.Marshal(map[string]FactoryTaskStatus{
		"status": status,
	})
}
