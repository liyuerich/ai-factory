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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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
		fmt.Fprintf(out, "sandboxTemplate=%s sandboxClaim=%s container=%s agent=%s\n", plan.SandboxTemplate, plan.SandboxClaim, plan.ContainerName, plan.AgentName)
		for _, step := range plan.Steps {
			fmt.Fprintf(out, "- %s: %s\n", step.Name, strings.Join(step.Command, " "))
		}
		return nil
	},
}

var controllerCmd = &cobra.Command{
	Use:   "controller",
	Short: "Run FactoryTask controller operations",
	Long:  `Controller commands reconcile FactoryTask resources into sandbox work.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var manifestCmd = &cobra.Command{
	Use:   "manifest [file]",
	Short: "Render the SandboxClaim manifest a FactoryTask would create",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task, err := readTask(args[0])
		if err != nil {
			return err
		}
		output, err := taskpkg.Reconcile(task)
		if err != nil {
			return err
		}
		data, err := output.SandboxClaimYAML()
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	},
}

var runOnceOptions struct {
	timeout time.Duration
}

var runOnceCmd = &cobra.Command{
	Use:   "run-once [file]",
	Short: "Create a SandboxClaim and execute a FactoryTask once with kubectl",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task, err := readTask(args[0])
		if err != nil {
			return err
		}
		output, err := taskpkg.Reconcile(task)
		if err != nil {
			return err
		}
		manifest, err := output.SandboxClaimYAML()
		if err != nil {
			return err
		}

		namespace := output.SandboxClaim.Metadata.Namespace
		claim := output.SandboxClaim.Metadata.Name
		if err := runKubectlWithInput(manifest, "apply", "-f", "-"); err != nil {
			return err
		}
		if err := runKubectl(nil, "wait", "sandboxclaim", claim, "-n", namespace, "--for=condition=Ready", "--timeout="+runOnceOptions.timeout.String()); err != nil {
			return err
		}

		sandboxName, err := kubectlOutput("get", "sandboxclaim", claim, "-n", namespace, "-o", "jsonpath={.status.sandboxName}")
		if err != nil {
			return err
		}
		sandboxName = strings.TrimSpace(sandboxName)
		if sandboxName == "" {
			return fmt.Errorf("sandboxclaim %s/%s is ready but status.sandboxName is empty", namespace, claim)
		}

		for _, step := range output.Plan.Steps {
			fmt.Fprintf(cmd.OutOrStdout(), "--- RUN: %s\n", step.Name)
			if err := runKubectl(nil, append([]string{"exec", "-n", namespace, sandboxName, "-c", output.Plan.ContainerName, "--"}, step.Command...)...); err != nil {
				return fmt.Errorf("%s: %w", step.Name, err)
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), "PASS")
		return nil
	},
}

func init() {
	Cmd.AddCommand(validateCmd)
	Cmd.AddCommand(planCmd)
	Cmd.AddCommand(controllerCmd)
	controllerCmd.AddCommand(manifestCmd)
	controllerCmd.AddCommand(runOnceCmd)
	runOnceCmd.Flags().DurationVar(&runOnceOptions.timeout, "timeout", 5*time.Minute, "time to wait for the SandboxClaim to become Ready")
}

func readTask(path string) (*taskpkg.FactoryTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read task file: %w", err)
	}
	return taskpkg.Parse(data)
}

func runKubectl(stdin []byte, args ...string) error {
	cmd := exec.Command("kubectl", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runKubectlWithInput(stdin []byte, args ...string) error {
	return runKubectl(stdin, args...)
}

func kubectlOutput(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("kubectl %s: %s: %w", strings.Join(args, " "), stderr.String(), err)
	}
	return string(out), nil
}
