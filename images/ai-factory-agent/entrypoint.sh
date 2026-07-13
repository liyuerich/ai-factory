#!/bin/sh
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

set -eu

: "${AGENT_NAME:?AGENT_NAME environment variable is required}"

REPOSITORY_URL="${REPOSITORY_URL:-https://github.com/liyuerich/ai-factory.git}"
REPOSITORY_REF="${REPOSITORY_REF:-main}"
AGENT_COMMAND="${AGENT_COMMAND:-ai-factory-agent openai-compatible}"

echo "Cloning ${REPOSITORY_URL}..."
git clone --depth=1 --branch "${REPOSITORY_REF}" "${REPOSITORY_URL}" ai-factory
cd ai-factory

PROMPT_FILE=".agents/${AGENT_NAME}/agent.md"
if [ ! -f "${PROMPT_FILE}" ]; then
  echo "Prompt file ${PROMPT_FILE} not found." >&2
  exit 1
fi

echo "Running configured agent for ${AGENT_NAME}..."
cat "${PROMPT_FILE}" | /bin/sh -lc "${AGENT_COMMAND}"
