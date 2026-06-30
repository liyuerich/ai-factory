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

// ExecutionPlan is the normalized controller input produced from a FactoryTask.
type ExecutionPlan struct {
	TaskName        string
	Provider        string
	Repository      string
	CloneURL        string
	BaseRef         string
	AgentName       string
	SandboxTemplate string
	SandboxClaim    string
	ContainerName   string
	WorkDir         string
	Steps           []ExecutionStep
}

// ExecutionStep describes one high-level action for a future controller.
type ExecutionStep struct {
	Name    string
	Command []string
}

// BuildExecutionPlan normalizes provider-specific source details into the
// provider-neutral steps a controller needs to create a sandbox and run work.
func BuildExecutionPlan(task *FactoryTask) (*ExecutionPlan, error) {
	if err := task.Validate(); err != nil {
		return nil, err
	}

	cloneURL, err := task.Spec.Source.CloneURLOrDefault()
	if err != nil {
		return nil, err
	}

	claimName := task.Spec.Sandbox.ClaimName
	if claimName == "" {
		claimName = fmt.Sprintf("%s-claim", task.Metadata.Name)
	}
	containerName := task.Spec.Sandbox.ContainerName
	if containerName == "" {
		containerName = "dev"
	}
	workDir := "/workspace/repo"

	plan := &ExecutionPlan{
		TaskName:        task.Metadata.Name,
		Provider:        task.Spec.Source.Provider,
		Repository:      task.Spec.Source.Repository,
		CloneURL:        cloneURL,
		BaseRef:         task.Spec.Source.BaseRef,
		AgentName:       task.Spec.Agent.Name,
		SandboxTemplate: task.Spec.Sandbox.TemplateRef,
		SandboxClaim:    claimName,
		ContainerName:   containerName,
		WorkDir:         workDir,
		Steps: []ExecutionStep{
			{
				Name:    "clone repository",
				Command: []string{"/bin/sh", "-lc", fmt.Sprintf("mkdir -p %s && git clone %s %s", shellQuote("/workspace"), shellQuote(cloneURL), shellQuote(workDir))},
			},
			{
				Name:    "checkout base ref",
				Command: []string{"git", "-C", workDir, "checkout", task.Spec.Source.BaseRef},
			},
		},
	}

	for i, command := range task.Spec.Work.Commands {
		plan.Steps = append(plan.Steps, ExecutionStep{
			Name:    fmt.Sprintf("run command %d", i+1),
			Command: []string{"/bin/sh", "-lc", fmt.Sprintf("cd %s && %s", shellQuote(workDir), command)},
		})
	}

	return plan, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
