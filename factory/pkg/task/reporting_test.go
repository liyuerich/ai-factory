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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildGitHubCommentRequest(t *testing.T) {
	req, err := BuildCommentRequest(CommentReportOptions{
		Provider:  ProviderGitHub,
		TargetURL: "https://github.com/liyuerich/ai-factory/issues/12",
		Body:      "FactoryTask succeeded",
		Token:     "test-token",
		APIBase:   "https://api.github.test",
	})
	if err != nil {
		t.Fatalf("BuildCommentRequest() error = %v", err)
	}
	if req.Method != http.MethodPost {
		t.Fatalf("method = %q", req.Method)
	}
	if req.URL != "https://api.github.test/repos/liyuerich/ai-factory/issues/12/comments" {
		t.Fatalf("url = %q", req.URL)
	}
	if req.Headers["Authorization"] != "Bearer test-token" {
		t.Fatalf("authorization = %q", req.Headers["Authorization"])
	}
	var payload map[string]string
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["body"] != "FactoryTask succeeded" {
		t.Fatalf("body = %q", payload["body"])
	}
}

func TestBuildGitLabCommentRequest(t *testing.T) {
	req, err := BuildCommentRequest(CommentReportOptions{
		Provider:  ProviderGitLab,
		TargetURL: "https://gitlab.example.com/platform/ai/ai-factory/-/issues/34",
		Body:      "FactoryTask failed",
		Token:     "test-token",
	})
	if err != nil {
		t.Fatalf("BuildCommentRequest() error = %v", err)
	}
	if req.URL != "https://gitlab.example.com/api/v4/projects/platform%2Fai%2Fai-factory/issues/34/notes" {
		t.Fatalf("url = %q", req.URL)
	}
	if req.Headers["PRIVATE-TOKEN"] != "test-token" {
		t.Fatalf("private token = %q", req.Headers["PRIVATE-TOKEN"])
	}
}

func TestInferReportingProvider(t *testing.T) {
	tests := []struct {
		name      string
		targetURL string
		want      string
	}{
		{
			name:      "github",
			targetURL: "https://github.com/liyuerich/ai-factory/issues/1",
			want:      ProviderGitHub,
		},
		{
			name:      "gitlab",
			targetURL: "https://gitlab.example.com/group/project/-/issues/2",
			want:      ProviderGitLab,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InferReportingProvider(tt.targetURL)
			if err != nil {
				t.Fatalf("InferReportingProvider() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("InferReportingProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPostIssueComment(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	err := PostIssueComment(context.Background(), CommentReportOptions{
		Provider:  ProviderGitHub,
		TargetURL: "https://github.com/liyuerich/ai-factory/issues/9",
		Body:      "done",
		Token:     "token",
		APIBase:   server.URL,
	})
	if err != nil {
		t.Fatalf("PostIssueComment() error = %v", err)
	}
	if gotPath != "/repos/liyuerich/ai-factory/issues/9/comments" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer token" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotBody["body"] != "done" {
		t.Fatalf("body = %q", gotBody["body"])
	}
}
