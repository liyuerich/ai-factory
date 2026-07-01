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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	taskpkg "github.com/ai-on-gke/ai-factory/factory/pkg/task"
	"github.com/spf13/cobra"
)

func TestShouldReconcile(t *testing.T) {
	tests := []struct {
		name        string
		phase       string
		retryFailed bool
		want        bool
	}{
		{name: "empty phase", phase: "", want: true},
		{name: "pending phase", phase: taskpkg.PhasePending, want: true},
		{name: "claim created phase", phase: taskpkg.PhaseClaimCreated, want: true},
		{name: "sandbox ready phase", phase: taskpkg.PhaseSandboxReady, want: true},
		{name: "running phase", phase: taskpkg.PhaseRunning, want: false},
		{name: "succeeded phase", phase: taskpkg.PhaseSucceeded, want: false},
		{name: "failed without retry", phase: taskpkg.PhaseFailed, want: false},
		{name: "failed with retry", phase: taskpkg.PhaseFailed, retryFailed: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldReconcile(taskpkg.FactoryTask{
				Status: taskpkg.FactoryTaskStatus{Phase: tt.phase},
			}, tt.retryFailed)
			if got != tt.want {
				t.Fatalf("shouldReconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFactoryTaskListJSON(t *testing.T) {
	data := []byte(`{
	  "apiVersion": "factory.ai.gke.io/v1alpha1",
	  "kind": "FactoryTaskList",
	  "items": [
	    {
	      "apiVersion": "factory.ai.gke.io/v1alpha1",
	      "kind": "FactoryTask",
	      "metadata": {
	        "name": "validate-ai-factory",
	        "namespace": "factory-system"
	      },
	      "spec": {
	        "source": {
	          "provider": "github",
	          "repository": "liyuerich/ai-factory",
	          "baseRef": "main"
	        },
	        "agent": {
	          "name": "builder"
	        },
	        "sandbox": {
	          "templateRef": "go-dev"
	        },
	        "work": {
	          "commands": ["go test ./..."]
	        }
	      },
	      "status": {
	        "phase": "Pending"
	      }
	    }
	  ]
	}`)

	var list factoryTaskList
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("len(items) = %d", len(list.Items))
	}
	item := list.Items[0]
	if item.Metadata.Name != "validate-ai-factory" {
		t.Fatalf("metadata.name = %q", item.Metadata.Name)
	}
	if item.Spec.Source.BaseRef != "main" {
		t.Fatalf("spec.source.baseRef = %q", item.Spec.Source.BaseRef)
	}
	if item.Status.Phase != taskpkg.PhasePending {
		t.Fatalf("status.phase = %q", item.Status.Phase)
	}
}

func TestReportingProviderFallsBackToSource(t *testing.T) {
	task := &taskpkg.FactoryTask{
		Spec: taskpkg.FactoryTaskSpec{
			Source: taskpkg.SourceSpec{Provider: taskpkg.ProviderGitHub},
		},
	}
	if got := reportingProvider(task); got != taskpkg.ProviderGitHub {
		t.Fatalf("reportingProvider() = %q, want %q", got, taskpkg.ProviderGitHub)
	}

	task.Spec.Reporting.Provider = taskpkg.ProviderGitLab
	if got := reportingProvider(task); got != taskpkg.ProviderGitLab {
		t.Fatalf("reportingProvider() = %q, want %q", got, taskpkg.ProviderGitLab)
	}
}

func TestBuildReportMessage(t *testing.T) {
	task := &taskpkg.FactoryTask{
		Metadata: taskpkg.ObjectMeta{
			Name:      "validate-ai-factory",
			Namespace: "factory-system",
		},
	}
	got := buildReportMessage(task, taskpkg.PhaseSucceeded, "done")
	want := "FactoryTask `factory-system/validate-ai-factory` Succeeded\n\ndone"
	if got != want {
		t.Fatalf("buildReportMessage() = %q, want %q", got, want)
	}
}

func TestIssueWebhookHandlerIgnoresMissingRequiredLabel(t *testing.T) {
	previousOptions := webhookOptions
	previousServeOptions := webhookServeOptions
	defer func() {
		webhookOptions = previousOptions
		webhookServeOptions = previousServeOptions
	}()

	webhookOptions = struct {
		provider           string
		namespace          string
		agent              string
		promptRef          string
		sandboxTemplateRef string
		containerName      string
		reportingMode      string
		command            []string
		triggerAction      []string
		requireLabel       []string
		repository         []string
	}{
		namespace:          "factory-system",
		agent:              "builder",
		sandboxTemplateRef: "go-dev",
		requireLabel:       []string{"ai-factory"},
	}
	webhookServeOptions.apply = false

	payload := []byte(`{
	  "action": "opened",
	  "issue": {
	    "number": 42,
	    "title": "Missing label",
	    "body": "Do not run.",
	    "html_url": "https://github.com/liyuerich/ai-factory/issues/42",
	    "user": {"login": "yueli"}
	  },
	  "repository": {
	    "full_name": "liyuerich/ai-factory",
	    "html_url": "https://github.com/liyuerich/ai-factory",
	    "clone_url": "https://github.com/liyuerich/ai-factory.git",
	    "default_branch": "main"
	  },
	  "sender": {"login": "liyuerich"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(payload))
	resp := httptest.NewRecorder()

	issueWebhookHandler(&cobra.Command{}, taskpkg.ProviderGitHub)(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusAccepted, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"ignored":true`) {
		t.Fatalf("body = %s, want ignored response", resp.Body.String())
	}
}
