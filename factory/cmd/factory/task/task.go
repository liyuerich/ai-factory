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
	"encoding/json"
	"fmt"
	"io"
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

var watchOptions struct {
	namespace   string
	interval    time.Duration
	timeout     time.Duration
	once        bool
	retryFailed bool
}

var patchStatusOptions struct {
	namespace        string
	phase            string
	message          string
	reason           string
	sandboxClaimName string
	sandboxName      string
	lastResultURL    string
	observedCommit   string
}

var runOnceCmd = &cobra.Command{
	Use:   "run-once [file]",
	Short: "Create a SandboxClaim and execute a FactoryTask once with kubectl",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskData, task, err := readTaskWithData(args[0])
		if err != nil {
			return err
		}
		return executeTask(cmd.OutOrStdout(), task, taskData, true, runOnceOptions.timeout, "run-once")
	},
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Continuously reconcile pending FactoryTasks with kubectl",
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace := watchOptions.namespace
		if namespace == "" {
			namespace = "default"
		}
		for {
			tasks, err := listFactoryTasks(namespace)
			if err != nil {
				return err
			}
			for i := range tasks {
				task := tasks[i]
				if !shouldReconcile(task, watchOptions.retryFailed) {
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "--- RECONCILE: %s/%s\n", namespaceForTask(&task), task.Metadata.Name)
				if err := executeTask(cmd.OutOrStdout(), &task, nil, false, watchOptions.timeout, "watch-controller"); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "FactoryTask %s/%s failed: %v\n", namespaceForTask(&task), task.Metadata.Name, err)
				}
			}
			if watchOptions.once {
				return nil
			}
			time.Sleep(watchOptions.interval)
		}
	},
}

var patchStatusCmd = &cobra.Command{
	Use:   "patch-status [task-name]",
	Short: "Patch the status subresource of a FactoryTask",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace := patchStatusOptions.namespace
		if namespace == "" {
			namespace = "default"
		}
		return patchTaskStatus(namespace, args[0], taskpkg.StatusPatchOptions{
			Phase:            patchStatusOptions.phase,
			Reason:           patchStatusOptions.reason,
			Message:          patchStatusOptions.message,
			SandboxClaimName: patchStatusOptions.sandboxClaimName,
			SandboxName:      patchStatusOptions.sandboxName,
			LastResultURL:    patchStatusOptions.lastResultURL,
			ObservedCommit:   patchStatusOptions.observedCommit,
		})
	},
}

func init() {
	Cmd.AddCommand(validateCmd)
	Cmd.AddCommand(planCmd)
	Cmd.AddCommand(controllerCmd)
	controllerCmd.AddCommand(manifestCmd)
	controllerCmd.AddCommand(runOnceCmd)
	controllerCmd.AddCommand(watchCmd)
	controllerCmd.AddCommand(patchStatusCmd)
	runOnceCmd.Flags().DurationVar(&runOnceOptions.timeout, "timeout", 5*time.Minute, "time to wait for the SandboxClaim to become Ready")
	watchCmd.Flags().StringVarP(&watchOptions.namespace, "namespace", "n", "default", "FactoryTask namespace")
	watchCmd.Flags().DurationVar(&watchOptions.interval, "interval", 15*time.Second, "time between FactoryTask list polls")
	watchCmd.Flags().DurationVar(&watchOptions.timeout, "timeout", 5*time.Minute, "time to wait for each SandboxClaim to become Ready")
	watchCmd.Flags().BoolVar(&watchOptions.once, "once", false, "list and reconcile once, then exit")
	watchCmd.Flags().BoolVar(&watchOptions.retryFailed, "retry-failed", false, "reconcile FactoryTasks whose phase is Failed")
	patchStatusCmd.Flags().StringVarP(&patchStatusOptions.namespace, "namespace", "n", "default", "FactoryTask namespace")
	patchStatusCmd.Flags().StringVar(&patchStatusOptions.phase, "phase", taskpkg.PhasePending, "FactoryTask phase")
	patchStatusCmd.Flags().StringVar(&patchStatusOptions.message, "message", "", "status message")
	patchStatusCmd.Flags().StringVar(&patchStatusOptions.reason, "reason", "ManualPatch", "condition reason")
	patchStatusCmd.Flags().StringVar(&patchStatusOptions.sandboxClaimName, "sandbox-claim-name", "", "SandboxClaim name")
	patchStatusCmd.Flags().StringVar(&patchStatusOptions.sandboxName, "sandbox-name", "", "sandbox name")
	patchStatusCmd.Flags().StringVar(&patchStatusOptions.lastResultURL, "last-result-url", "", "URL for the latest result")
	patchStatusCmd.Flags().StringVar(&patchStatusOptions.observedCommit, "observed-commit", "", "observed source commit")
}

func readTask(path string) (*taskpkg.FactoryTask, error) {
	_, task, err := readTaskWithData(path)
	return task, err
}

func readTaskWithData(path string) ([]byte, *taskpkg.FactoryTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read task file: %w", err)
	}
	task, err := taskpkg.Parse(data)
	if err != nil {
		return nil, nil, err
	}
	return data, task, nil
}

func patchTaskStatus(namespace, name string, opts taskpkg.StatusPatchOptions) error {
	patch, err := taskpkg.StatusMergePatch(opts)
	if err != nil {
		return err
	}
	return runKubectl(nil, "patch", "factorytask", name, "-n", namespace, "--type=merge", "--subresource=status", "-p", string(patch))
}

func executeTask(out io.Writer, task *taskpkg.FactoryTask, taskData []byte, applyTask bool, timeout time.Duration, controllerName string) error {
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
	if applyTask {
		if err := runKubectlWithInput(taskData, "apply", "-f", "-"); err != nil {
			return err
		}
	}
	if err := patchTaskStatus(namespace, task.Metadata.Name, taskpkg.StatusPatchOptions{
		Phase:            taskpkg.PhasePending,
		Reason:           "TaskAccepted",
		Message:          fmt.Sprintf("FactoryTask accepted by %s", controllerName),
		SandboxClaimName: claim,
	}); err != nil {
		return err
	}
	if err := runKubectlWithInput(manifest, "apply", "-f", "-"); err != nil {
		_ = patchTaskStatus(namespace, task.Metadata.Name, taskpkg.StatusPatchOptions{
			Phase:   taskpkg.PhaseFailed,
			Reason:  "SandboxClaimApplyFailed",
			Message: err.Error(),
		})
		return err
	}
	if err := patchTaskStatus(namespace, task.Metadata.Name, taskpkg.StatusPatchOptions{
		Phase:            taskpkg.PhaseClaimCreated,
		Reason:           "SandboxClaimCreated",
		Message:          "SandboxClaim created",
		SandboxClaimName: claim,
	}); err != nil {
		return err
	}
	if err := runKubectl(nil, "wait", "sandboxclaim", claim, "-n", namespace, "--for=condition=Ready", "--timeout="+timeout.String()); err != nil {
		_ = patchTaskStatus(namespace, task.Metadata.Name, taskpkg.StatusPatchOptions{
			Phase:            taskpkg.PhaseFailed,
			Reason:           "SandboxClaimReadyTimeout",
			Message:          err.Error(),
			SandboxClaimName: claim,
		})
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
	if err := patchTaskStatus(namespace, task.Metadata.Name, taskpkg.StatusPatchOptions{
		Phase:            taskpkg.PhaseSandboxReady,
		Reason:           "SandboxReady",
		Message:          "SandboxClaim is ready",
		SandboxClaimName: claim,
		SandboxName:      sandboxName,
	}); err != nil {
		return err
	}
	if err := patchTaskStatus(namespace, task.Metadata.Name, taskpkg.StatusPatchOptions{
		Phase:            taskpkg.PhaseRunning,
		Reason:           "PlanStarted",
		Message:          "Executing generated plan",
		SandboxClaimName: claim,
		SandboxName:      sandboxName,
	}); err != nil {
		return err
	}

	for _, step := range output.Plan.Steps {
		fmt.Fprintf(out, "--- RUN: %s\n", step.Name)
		if err := runKubectl(nil, append([]string{"exec", "-n", namespace, sandboxName, "-c", output.Plan.ContainerName, "--"}, step.Command...)...); err != nil {
			_ = patchTaskStatus(namespace, task.Metadata.Name, taskpkg.StatusPatchOptions{
				Phase:            taskpkg.PhaseFailed,
				Reason:           "StepFailed",
				Message:          fmt.Sprintf("%s: %v", step.Name, err),
				SandboxClaimName: claim,
				SandboxName:      sandboxName,
			})
			return fmt.Errorf("%s: %w", step.Name, err)
		}
	}
	if err := patchTaskStatus(namespace, task.Metadata.Name, taskpkg.StatusPatchOptions{
		Phase:            taskpkg.PhaseSucceeded,
		Reason:           "PlanSucceeded",
		Message:          "FactoryTask completed successfully",
		SandboxClaimName: claim,
		SandboxName:      sandboxName,
	}); err != nil {
		return err
	}
	fmt.Fprintln(out, "PASS")
	return nil
}

type factoryTaskList struct {
	Items []taskpkg.FactoryTask `json:"items"`
}

func listFactoryTasks(namespace string) ([]taskpkg.FactoryTask, error) {
	out, err := kubectlOutput("get", "factorytasks", "-n", namespace, "-o", "json")
	if err != nil {
		return nil, err
	}
	var list factoryTaskList
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return nil, fmt.Errorf("decode FactoryTask list: %w", err)
	}
	return list.Items, nil
}

func shouldReconcile(task taskpkg.FactoryTask, retryFailed bool) bool {
	switch task.Status.Phase {
	case "", taskpkg.PhasePending, taskpkg.PhaseClaimCreated, taskpkg.PhaseSandboxReady:
		return true
	case taskpkg.PhaseFailed:
		return retryFailed
	default:
		return false
	}
}

func namespaceForTask(task *taskpkg.FactoryTask) string {
	if task.Metadata.Namespace == "" {
		return "default"
	}
	return task.Metadata.Namespace
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
