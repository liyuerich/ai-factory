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
	"context"
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

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Report FactoryTask results to external systems",
	Long:  `Report commands write FactoryTask execution results back to provider-neutral targets.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var reportCommentOptions struct {
	message string
	token   string
	apiBase string
	dryRun  bool
}

var reportCommentCmd = &cobra.Command{
	Use:   "comment [file]",
	Short: "Write a FactoryTask result comment to GitHub or GitLab",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task, err := readTask(args[0])
		if err != nil {
			return err
		}
		message := reportCommentOptions.message
		if strings.TrimSpace(message) == "" {
			message = buildReportMessage(task, "Manual", "FactoryTask report requested manually")
		}
		opts := taskpkg.CommentReportOptions{
			Provider:  reportingProvider(task),
			TargetURL: task.Spec.Reporting.TargetURL,
			Body:      message,
			Token:     reportCommentOptions.token,
			APIBase:   reportCommentOptions.apiBase,
		}
		if reportCommentOptions.dryRun {
			if opts.Token == "" {
				opts.Token = "dry-run-token"
			}
			req, err := taskpkg.BuildCommentRequest(opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "provider=%s method=%s url=%s\n", req.Provider, req.Method, req.URL)
			fmt.Fprintf(cmd.OutOrStdout(), "body=%s\n", string(req.Body))
			return nil
		}
		if err := taskpkg.PostIssueComment(context.Background(), opts); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "--- PASS: comment %s\n", task.Spec.Reporting.TargetURL)
		return nil
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
	report  bool
}

var watchOptions struct {
	namespace   string
	interval    time.Duration
	timeout     time.Duration
	once        bool
	retryFailed bool
	report      bool
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
	Cmd.AddCommand(reportCmd)
	Cmd.AddCommand(controllerCmd)
	reportCmd.AddCommand(reportCommentCmd)
	controllerCmd.AddCommand(manifestCmd)
	controllerCmd.AddCommand(runOnceCmd)
	controllerCmd.AddCommand(watchCmd)
	controllerCmd.AddCommand(patchStatusCmd)
	runOnceCmd.Flags().DurationVar(&runOnceOptions.timeout, "timeout", 5*time.Minute, "time to wait for the SandboxClaim to become Ready")
	runOnceCmd.Flags().BoolVar(&runOnceOptions.report, "report", true, "write reporting.comment results when spec.reporting is configured")
	reportCommentCmd.Flags().StringVar(&reportCommentOptions.message, "message", "", "comment body to write")
	reportCommentCmd.Flags().StringVar(&reportCommentOptions.token, "token", "", "provider API token; defaults to GITHUB_TOKEN or GITLAB_TOKEN")
	reportCommentCmd.Flags().StringVar(&reportCommentOptions.apiBase, "api-base", "", "override provider API base URL")
	reportCommentCmd.Flags().BoolVar(&reportCommentOptions.dryRun, "dry-run", false, "print the comment request without sending it")
	watchCmd.Flags().StringVarP(&watchOptions.namespace, "namespace", "n", "default", "FactoryTask namespace")
	watchCmd.Flags().DurationVar(&watchOptions.interval, "interval", 15*time.Second, "time between FactoryTask list polls")
	watchCmd.Flags().DurationVar(&watchOptions.timeout, "timeout", 5*time.Minute, "time to wait for each SandboxClaim to become Ready")
	watchCmd.Flags().BoolVar(&watchOptions.once, "once", false, "list and reconcile once, then exit")
	watchCmd.Flags().BoolVar(&watchOptions.retryFailed, "retry-failed", false, "reconcile FactoryTasks whose phase is Failed")
	watchCmd.Flags().BoolVar(&watchOptions.report, "report", true, "write reporting.comment results when spec.reporting is configured")
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
		reportTaskResult(out, task, taskpkg.PhaseFailed, fmt.Sprintf("SandboxClaim apply failed: %v", err), reportingEnabled(controllerName))
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
		reportTaskResult(out, task, taskpkg.PhaseFailed, fmt.Sprintf("SandboxClaim ready wait failed: %v", err), reportingEnabled(controllerName))
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
			reportTaskResult(out, task, taskpkg.PhaseFailed, fmt.Sprintf("%s failed: %v", step.Name, err), reportingEnabled(controllerName))
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
	reportTaskResult(out, task, taskpkg.PhaseSucceeded, "FactoryTask completed successfully", reportingEnabled(controllerName))
	fmt.Fprintln(out, "PASS")
	return nil
}

func reportingEnabled(controllerName string) bool {
	switch controllerName {
	case "run-once":
		return runOnceOptions.report
	case "watch-controller":
		return watchOptions.report
	default:
		return true
	}
}

func reportTaskResult(out io.Writer, task *taskpkg.FactoryTask, phase, message string, enabled bool) {
	if !enabled || task.Spec.Reporting.Mode != "comment" || task.Spec.Reporting.TargetURL == "" {
		return
	}
	body := buildReportMessage(task, phase, message)
	if err := taskpkg.PostIssueComment(context.Background(), taskpkg.CommentReportOptions{
		Provider:  reportingProvider(task),
		TargetURL: task.Spec.Reporting.TargetURL,
		Body:      body,
	}); err != nil {
		fmt.Fprintf(out, "--- REPORT FAILED: %v\n", err)
		return
	}
	fmt.Fprintf(out, "--- REPORT: comment %s\n", task.Spec.Reporting.TargetURL)
}

func buildReportMessage(task *taskpkg.FactoryTask, phase, message string) string {
	name := task.Metadata.Name
	if ns := namespaceForTask(task); ns != "" {
		name = ns + "/" + name
	}
	return fmt.Sprintf("FactoryTask `%s` %s\n\n%s", name, phase, message)
}

func reportingProvider(task *taskpkg.FactoryTask) string {
	if task.Spec.Reporting.Provider != "" {
		return task.Spec.Reporting.Provider
	}
	return task.Spec.Source.Provider
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
