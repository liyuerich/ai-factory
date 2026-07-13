#!/usr/bin/env bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "required command not found: $1" >&2
    exit 2
  }
}

for command in curl docker git go jq kind kubectl; do
  require_command "$command"
done

: "${CI_PROJECT_DIR:=$(pwd)}"
: "${CI_PROJECT_ID:?CI_PROJECT_ID is required}"
: "${CI_API_V4_URL:?CI_API_V4_URL is required}"
: "${GITLAB_TOKEN:?GITLAB_TOKEN is required}"

FACTORY_SOURCE_DIR="${AI_FACTORY_SOURCE_DIR:-${CI_PROJECT_DIR}/.ai-factory-src}"
FACTORY_SOURCE_URL="${AI_FACTORY_SOURCE_URL:-https://github.com/liyuerich/ai-factory.git}"
FACTORY_SOURCE_REF="${AI_FACTORY_SOURCE_REF:-main}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-ai-factory-${CI_PIPELINE_ID:-ci}}"
AGENT_SANDBOX_SRC="${AGENT_SANDBOX_SRC:-${CI_PROJECT_DIR}/.agent-sandbox-src}"
IMAGE_PREFIX="${IMAGE_PREFIX:-ai-factory}"
IMAGE_TAG="${IMAGE_TAG:-ci}"
CODING_AGENT_IMAGE="${CODING_AGENT_IMAGE:-ai-factory/coding-agent-sandbox:${IMAGE_TAG}}"
AGENT_COMMAND="${AI_FACTORY_RUN_AGENT_COMMAND:-ai-factory-agent openai-compatible}"
VALIDATION_COMMAND="${AI_FACTORY_RUN_VALIDATION_COMMAND:-go test ./...}"
ISSUE_IID="${AI_FACTORY_ISSUE_IID:?AI_FACTORY_ISSUE_IID is required}"
WEBHOOK_SECRET="${GITLAB_WEBHOOK_SECRET:-${WEBHOOK_SECRET:-}}"
ISSUE_ACTION="${AI_FACTORY_ISSUE_ACTION:-opened}"
: "${WEBHOOK_SECRET:?GITLAB_WEBHOOK_SECRET or WEBHOOK_SECRET is required}"
if [[ "${AGENT_COMMAND}" == *openai-compatible* ]]; then
  : "${OPENAI_API_KEY:?OPENAI_API_KEY is required for openai-compatible}"
fi

cleanup() {
  kind delete cluster --name "${KIND_CLUSTER_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "${FACTORY_SOURCE_DIR}/components/factory-task/install" ]]; then
  rm -rf "${FACTORY_SOURCE_DIR}"
  if [[ -n "${AI_FACTORY_SOURCE_TOKEN:-}" ]]; then
    git -c "http.extraHeader=JOB-TOKEN: ${AI_FACTORY_SOURCE_TOKEN}" \
      clone --depth=1 --branch "${FACTORY_SOURCE_REF}" \
      "${FACTORY_SOURCE_URL}" "${FACTORY_SOURCE_DIR}"
  else
    git clone --depth=1 --branch "${FACTORY_SOURCE_REF}" \
      "${FACTORY_SOURCE_URL}" "${FACTORY_SOURCE_DIR}"
  fi
fi

if [[ ! -x "${AGENT_SANDBOX_SRC}/dev/tools/deploy-to-kube" ]]; then
  rm -rf "${AGENT_SANDBOX_SRC}"
  git clone --depth=1 https://github.com/kubernetes-sigs/agent-sandbox.git "${AGENT_SANDBOX_SRC}"
fi

kind create cluster --name "${KIND_CLUSTER_NAME}"
kind export kubeconfig --name "${KIND_CLUSTER_NAME}"

(
  cd "${FACTORY_SOURCE_DIR}"
  components/factory-task/install
)

(
  cd "${FACTORY_SOURCE_DIR}"
  IMAGE_PREFIX="${IMAGE_PREFIX}" \
  IMAGE_TAG="${IMAGE_TAG}" \
  KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME}" \
  AGENT_SANDBOX_SRC="${AGENT_SANDBOX_SRC}" \
  components/agent-sandbox/install
)

GO_VERSION="$(awk '/^go / {print $2; exit}' "${FACTORY_SOURCE_DIR}/go.mod")"
docker build \
  --build-arg GO_VERSION="${GO_VERSION}" \
  -t "${CODING_AGENT_IMAGE}" \
  "${FACTORY_SOURCE_DIR}/components/agent-sandbox-images/coding-agent"
kind load docker-image "${CODING_AGENT_IMAGE}" --name "${KIND_CLUSTER_NAME}"

cat > "${CI_PROJECT_DIR}/.ai-factory-go-dev-sandbox.yaml" <<EOF
apiVersion: extensions.agents.x-k8s.io/v1beta1
kind: SandboxTemplate
metadata:
  name: go-dev
  namespace: default
spec:
  envVarsInjectionPolicy: Allowed
  networkPolicyManagement: Unmanaged
  podTemplate:
    spec:
      containers:
      - name: dev
        image: ${CODING_AGENT_IMAGE}
        imagePullPolicy: IfNotPresent
        command:
        - /bin/bash
        - -lc
        - mkdir -p /workspace && sleep 3600
        workingDir: /workspace
---
apiVersion: extensions.agents.x-k8s.io/v1beta1
kind: SandboxWarmPool
metadata:
  name: go-dev
  namespace: default
spec:
  replicas: 1
  sandboxTemplateRef:
    name: go-dev
EOF
kubectl apply -f "${CI_PROJECT_DIR}/.ai-factory-go-dev-sandbox.yaml"
kubectl wait sandboxwarmpool go-dev -n default \
  --for=jsonpath='{.status.readyReplicas}'=1 --timeout=180s

PROJECT_JSON="${CI_PROJECT_DIR}/.ai-factory-project.json"
ISSUE_JSON="${CI_PROJECT_DIR}/.ai-factory-issue.json"
PAYLOAD_JSON="${CI_PROJECT_DIR}/.ai-factory-issue-payload.json"
PROJECT_ID_ENCODED="$(printf '%s' "${CI_PROJECT_ID}" | jq -sRr @uri)"
curl --fail --silent --show-error \
  --header "PRIVATE-TOKEN: ${GITLAB_TOKEN}" \
  "${CI_API_V4_URL}/projects/${PROJECT_ID_ENCODED}" > "${PROJECT_JSON}"
curl --fail --silent --show-error \
  --header "PRIVATE-TOKEN: ${GITLAB_TOKEN}" \
  "${CI_API_V4_URL}/projects/${PROJECT_ID_ENCODED}/issues/${ISSUE_IID}" > "${ISSUE_JSON}"

if ! jq -e --arg label "${AI_FACTORY_TRIGGER_LABEL:-ai-factory-run}" \
  '.labels | index($label) != null' "${ISSUE_JSON}" >/dev/null; then
  echo "Issue ${ISSUE_IID} does not have the trigger label; refusing to run." >&2
  exit 3
fi

jq -n \
  --slurpfile issue "${ISSUE_JSON}" \
  --slurpfile project "${PROJECT_JSON}" \
  --arg action "${ISSUE_ACTION}" \
  --arg actor "${GITLAB_USER_LOGIN:-ai-factory-runner}" \
  '{
    object_kind: "issue",
    event_type: "issue",
    user: {username: $actor},
    project: {
      path_with_namespace: $project[0].path_with_namespace,
      web_url: $project[0].web_url,
      default_branch: ($project[0].default_branch // "main"),
      git_http_url: $project[0].http_url_to_repo
    },
    object_attributes: {
      iid: $issue[0].iid,
      title: $issue[0].title,
      description: ($issue[0].description // ""),
      url: $issue[0].web_url,
      action: $action
    },
    labels: [$issue[0].labels[]? | {title: .}]
  }' > "${PAYLOAD_JSON}"

(
  cd "${FACTORY_SOURCE_DIR}"
  export GITLAB_TOKEN
  if [[ -n "${OPENAI_API_KEY:-}" ]]; then
    export OPENAI_API_KEY
  fi
  export OPENAI_BASE_URL="${OPENAI_BASE_URL:-https://api.openai.com/v1}"
  export OPENAI_MODEL="${OPENAI_MODEL:-gpt-4.1}"
  export OPENAI_TEMPERATURE="${OPENAI_TEMPERATURE:-1}"
  export OPENAI_MAX_TOKENS="${OPENAI_MAX_TOKENS:-48000}"
  export OPENAI_MAX_TOOL_ROUNDS="${OPENAI_MAX_TOOL_ROUNDS:-40}"
  export OPENAI_MAX_FINAL_SCRIPT_ROUNDS="${OPENAI_MAX_FINAL_SCRIPT_ROUNDS:-5}"
  export OPENAI_MAX_REPAIR_ROUNDS="${OPENAI_MAX_REPAIR_ROUNDS:-3}"
  go run ./factory/cmd/factory task webhook serve \
    --addr 127.0.0.1:8080 \
    --apply \
    --namespace default \
    --require-label "${AI_FACTORY_TRIGGER_LABEL:-ai-factory-run}" \
    --sandbox-template go-dev \
    --agent-command "${AGENT_COMMAND}" \
    --agent-env OPENAI_API_KEY \
    --agent-env OPENAI_BASE_URL \
    --agent-env OPENAI_MODEL \
    --agent-env OPENAI_TEMPERATURE \
    --agent-env OPENAI_MAX_TOKENS \
    --agent-env OPENAI_MAX_TOOL_ROUNDS \
    --agent-env OPENAI_MAX_FINAL_SCRIPT_ROUNDS \
    --agent-env OPENAI_MAX_REPAIR_ROUNDS \
    --command "${VALIDATION_COMMAND}" \
    --change-request=true \
    --change-request-auth-token-env GITLAB_TOKEN \
    --reporting-mode comment \
    --secret "${WEBHOOK_SECRET}" \
    > "${CI_PROJECT_DIR}/.ai-factory-webhook.log" 2>&1 &
  WEBHOOK_PID=$!
  trap 'kill "${WEBHOOK_PID}" 2>/dev/null || true' EXIT

  for _ in $(seq 1 30); do
    if curl --fail --silent --show-error \
      -X POST http://127.0.0.1:8080/webhook/gitlab \
      -H "Content-Type: application/json" \
      -H "X-Gitlab-Token: ${WEBHOOK_SECRET}" \
      --data-binary @"${PAYLOAD_JSON}" \
      > "${CI_PROJECT_DIR}/.ai-factory-webhook-response.json"; then
      break
    fi
    sleep 1
  done
  if [[ ! -s "${CI_PROJECT_DIR}/.ai-factory-webhook-response.json" ]]; then
    cat "${CI_PROJECT_DIR}/.ai-factory-webhook.log"
    exit 1
  fi

  TASK_NAME="$(jq -r '.task // empty' "${CI_PROJECT_DIR}/.ai-factory-webhook-response.json")"
  if [[ -z "${TASK_NAME}" ]]; then
    cat "${CI_PROJECT_DIR}/.ai-factory-webhook-response.json"
    cat "${CI_PROJECT_DIR}/.ai-factory-webhook.log"
    exit 1
  fi
  kubectl get factorytask "${TASK_NAME}" -n default -o yaml
  go run ./factory/cmd/factory task controller watch \
    --namespace default \
    --once \
    --interval=1s \
    --timeout=180s \
    --create-change-request=true \
    --report=true

  for _ in $(seq 1 90); do
    phase="$(kubectl get factorytask "${TASK_NAME}" -n default -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    if [[ "${phase}" == "Succeeded" || "${phase}" == "Failed" ]]; then
      break
    fi
    sleep 2
  done
  kubectl get factorytask "${TASK_NAME}" -n default -o yaml
  [[ "${phase:-}" == "Succeeded" ]] || exit 1
)
