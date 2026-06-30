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
	"strings"
	"testing"
)

func TestParseValidGitHubTask(t *testing.T) {
	data := []byte(`
apiVersion: factory.ai.gke.io/v1alpha1
kind: FactoryTask
metadata:
  name: validate-ai-factory
spec:
  source:
    provider: github
    repository: liyuerich/ai-factory
    baseRef: main
  agent:
    name: builder
  sandbox:
    templateRef: go-dev
  work:
    instructions: validate the factory-runtime-proxy spec
`)

	task, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if task.Spec.Source.Provider != ProviderGitHub {
		t.Fatalf("provider = %q, want %q", task.Spec.Source.Provider, ProviderGitHub)
	}
}

func TestParseValidGitLabTask(t *testing.T) {
	data := []byte(`
apiVersion: factory.ai.gke.io/v1alpha1
kind: FactoryTask
metadata:
  name: validate-gitlab-project
spec:
  source:
    provider: gitlab
    host: gitlab.example.com
    repository: platform/ai/ai-factory
    baseRef: main
  agent:
    name: builder
  sandbox:
    templateRef: go-dev
  work:
    commands:
    - go test ./...
`)

	task, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if task.Spec.Source.Repository != "platform/ai/ai-factory" {
		t.Fatalf("repository = %q", task.Spec.Source.Repository)
	}
}

func TestParseRejectsUnsupportedProvider(t *testing.T) {
	data := []byte(`
apiVersion: factory.ai.gke.io/v1alpha1
kind: FactoryTask
metadata:
  name: bad-provider
spec:
  source:
    provider: bitbucket
    repository: team/repo
    baseRef: main
  agent:
    name: builder
  sandbox:
    templateRef: go-dev
  work:
    instructions: do something
`)

	_, err := Parse(data)
	if err == nil {
		t.Fatal("Parse() error = nil, want unsupported provider error")
	}
	if !strings.Contains(err.Error(), "spec.source.provider") {
		t.Fatalf("error = %v, want provider validation error", err)
	}
}

func TestCloneURLOrDefault(t *testing.T) {
	tests := []struct {
		name string
		src  SourceSpec
		want string
	}{
		{
			name: "github default host",
			src: SourceSpec{
				Provider:   ProviderGitHub,
				Repository: "liyuerich/ai-factory",
			},
			want: "https://github.com/liyuerich/ai-factory.git",
		},
		{
			name: "gitlab custom host",
			src: SourceSpec{
				Provider:   ProviderGitLab,
				Host:       "gitlab.example.com",
				Repository: "platform/ai/ai-factory",
			},
			want: "https://gitlab.example.com/platform/ai/ai-factory.git",
		},
		{
			name: "explicit clone url",
			src: SourceSpec{
				Provider:   ProviderGitLab,
				Repository: "platform/ai/ai-factory",
				CloneURL:   "https://mirror.example.com/ai-factory.git",
			},
			want: "https://mirror.example.com/ai-factory.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.src.CloneURLOrDefault()
			if err != nil {
				t.Fatalf("CloneURLOrDefault() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("CloneURLOrDefault() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildExecutionPlan(t *testing.T) {
	task, err := Parse([]byte(`
apiVersion: factory.ai.gke.io/v1alpha1
kind: FactoryTask
metadata:
  name: validate-gitlab-project
spec:
  source:
    provider: gitlab
    host: gitlab.example.com
    repository: platform/ai/ai-factory
    baseRef: main
  agent:
    name: builder
  sandbox:
    templateRef: go-dev
  work:
    commands:
    - go test ./...
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	plan, err := BuildExecutionPlan(task)
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	if plan.CloneURL != "https://gitlab.example.com/platform/ai/ai-factory.git" {
		t.Fatalf("CloneURL = %q", plan.CloneURL)
	}
	if plan.SandboxClaim != "validate-gitlab-project-claim" {
		t.Fatalf("SandboxClaim = %q", plan.SandboxClaim)
	}
	if got, want := len(plan.Steps), 3; got != want {
		t.Fatalf("len(Steps) = %d, want %d", got, want)
	}
}
