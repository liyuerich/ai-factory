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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestFactoryTaskFromGitHubIssueWebhook(t *testing.T) {
	task, err := FactoryTaskFromIssueWebhook([]byte(githubIssuePayload), IssueWebhookOptions{
		Provider:           ProviderGitHub,
		Namespace:          "factory-system",
		AgentName:          "builder",
		SandboxTemplateRef: "go-dev",
		Commands:           []string{"go test ./..."},
	})
	if err != nil {
		t.Fatalf("FactoryTaskFromIssueWebhook() error = %v", err)
	}
	if task.Metadata.Name != "github-liyuerich-ai-factory-42" {
		t.Fatalf("metadata.name = %q", task.Metadata.Name)
	}
	if task.Metadata.Namespace != "factory-system" {
		t.Fatalf("metadata.namespace = %q", task.Metadata.Namespace)
	}
	if task.Spec.Source.Provider != ProviderGitHub {
		t.Fatalf("source.provider = %q", task.Spec.Source.Provider)
	}
	if task.Spec.Source.Repository != "liyuerich/ai-factory" {
		t.Fatalf("source.repository = %q", task.Spec.Source.Repository)
	}
	if task.Spec.Source.BaseRef != "main" {
		t.Fatalf("source.baseRef = %q", task.Spec.Source.BaseRef)
	}
	if task.Spec.Trigger.URL != "https://github.com/liyuerich/ai-factory/issues/42" {
		t.Fatalf("trigger.url = %q", task.Spec.Trigger.URL)
	}
	if task.Spec.Reporting.TargetURL != task.Spec.Trigger.URL {
		t.Fatalf("reporting.targetURL = %q", task.Spec.Reporting.TargetURL)
	}
	if !strings.Contains(task.Spec.Work.Instructions, "Add webhook support") {
		t.Fatalf("instructions = %q", task.Spec.Work.Instructions)
	}
	if len(task.Spec.Work.Commands) != 1 || task.Spec.Work.Commands[0] != "go test ./..." {
		t.Fatalf("commands = %#v", task.Spec.Work.Commands)
	}
}

func TestFactoryTaskFromGitLabIssueWebhook(t *testing.T) {
	task, err := FactoryTaskFromIssueWebhook([]byte(gitlabIssuePayload), IssueWebhookOptions{
		Provider:           ProviderGitLab,
		AgentName:          "builder",
		SandboxTemplateRef: "go-dev",
	})
	if err != nil {
		t.Fatalf("FactoryTaskFromIssueWebhook() error = %v", err)
	}
	if task.Metadata.Name != "gitlab-platform-ai-ai-factory-7" {
		t.Fatalf("metadata.name = %q", task.Metadata.Name)
	}
	if task.Spec.Source.Host != "gitlab.example.com" {
		t.Fatalf("source.host = %q", task.Spec.Source.Host)
	}
	if task.Spec.Source.Repository != "platform/ai/ai-factory" {
		t.Fatalf("source.repository = %q", task.Spec.Source.Repository)
	}
	if task.Spec.Trigger.Actor != "yueli" {
		t.Fatalf("trigger.actor = %q", task.Spec.Trigger.Actor)
	}
	if task.Spec.Reporting.Provider != ProviderGitLab {
		t.Fatalf("reporting.provider = %q", task.Spec.Reporting.Provider)
	}
}

func TestFactoryTaskYAMLRoundTrip(t *testing.T) {
	task, err := FactoryTaskFromIssueWebhook([]byte(githubIssuePayload), IssueWebhookOptions{
		Provider:           ProviderGitHub,
		AgentName:          "builder",
		SandboxTemplateRef: "go-dev",
	})
	if err != nil {
		t.Fatalf("FactoryTaskFromIssueWebhook() error = %v", err)
	}
	data, err := FactoryTaskYAML(task)
	if err != nil {
		t.Fatalf("FactoryTaskYAML() error = %v", err)
	}
	if _, err := Parse(data); err != nil {
		t.Fatalf("Parse(FactoryTaskYAML()) error = %v\n%s", err, data)
	}
}

func TestVerifyGitHubWebhookSignature(t *testing.T) {
	body := []byte(githubIssuePayload)
	secret := "top-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if err := VerifyGitHubWebhookSignature(secret, body, signature); err != nil {
		t.Fatalf("VerifyGitHubWebhookSignature() error = %v", err)
	}
	if err := VerifyGitHubWebhookSignature(secret, body, "sha256=bad"); err == nil {
		t.Fatal("VerifyGitHubWebhookSignature() error = nil, want mismatch")
	}
}

func TestVerifyGitLabWebhookToken(t *testing.T) {
	if err := VerifyGitLabWebhookToken("top-secret", "top-secret"); err != nil {
		t.Fatalf("VerifyGitLabWebhookToken() error = %v", err)
	}
	if err := VerifyGitLabWebhookToken("top-secret", "wrong"); err == nil {
		t.Fatal("VerifyGitLabWebhookToken() error = nil, want mismatch")
	}
}

const githubIssuePayload = `{
  "action": "opened",
  "issue": {
    "number": 42,
    "title": "Add webhook support",
    "body": "Convert issues into FactoryTasks.",
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
}`

const gitlabIssuePayload = `{
  "object_kind": "issue",
  "event_type": "issue",
  "user": {"username": "yueli", "name": "Yue Li"},
  "project": {
    "path_with_namespace": "platform/ai/ai-factory",
    "web_url": "https://gitlab.example.com/platform/ai/ai-factory",
    "default_branch": "main",
    "git_http_url": "https://gitlab.example.com/platform/ai/ai-factory.git"
  },
  "object_attributes": {
    "iid": 7,
    "title": "Run agent task",
    "description": "Please validate this project.",
    "url": "https://gitlab.example.com/platform/ai/ai-factory/-/issues/7",
    "action": "open"
  }
}`
