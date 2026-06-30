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
	SandboxClaimName string
	SandboxName      string
	LastResultURL    string
	ObservedCommit   string
	Now              time.Time
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
				Reason:             opts.Reason,
				Message:            opts.Message,
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
