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

// Package task contains FactoryTask CLI commands.
package task

import (
	"fmt"
	"os"
	"strings"

	taskpkg "github.com/ai-on-gke/ai-factory/factory/pkg/task"
	"github.com/spf13/cobra"
)

// Cmd represents the task command.
var Cmd = &cobra.Command{
	Use:   "task",
	Short: "Manage factory tasks",
	Long:  `Subcommands for validating and inspecting FactoryTask resources.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate a FactoryTask YAML file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read task file: %w", err)
		}
		task, err := taskpkg.Parse(data)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "--- PASS: validate %s\n", args[0])
		fmt.Fprintf(cmd.OutOrStdout(), "provider=%s repository=%s baseRef=%s agent=%s\n",
			task.Spec.Source.Provider,
			task.Spec.Source.Repository,
			task.Spec.Source.BaseRef,
			task.Spec.Agent.Name)
		return nil
	},
}

var planCmd = &cobra.Command{
	Use:   "plan [file]",
	Short: "Render the normalized execution plan for a FactoryTask YAML file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read task file: %w", err)
		}
		task, err := taskpkg.Parse(data)
		if err != nil {
			return err
		}
		plan, err := taskpkg.BuildExecutionPlan(task)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "task=%s provider=%s repository=%s\n", plan.TaskName, plan.Provider, plan.Repository)
		fmt.Fprintf(out, "cloneURL=%s baseRef=%s\n", plan.CloneURL, plan.BaseRef)
		fmt.Fprintf(out, "sandboxTemplate=%s sandboxClaim=%s agent=%s\n", plan.SandboxTemplate, plan.SandboxClaim, plan.AgentName)
		for _, step := range plan.Steps {
			fmt.Fprintf(out, "- %s: %s\n", step.Name, strings.Join(step.Command, " "))
		}
		return nil
	},
}

func init() {
	Cmd.AddCommand(validateCmd)
	Cmd.AddCommand(planCmd)
}
