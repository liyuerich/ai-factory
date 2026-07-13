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

mkdir -p "${WORKSPACE:-/workspace}"

EXPECTED_GO=/usr/local/go/bin/go

preflight_fail() {
  echo "coding-agent sandbox preflight failed: $1" >&2
  echo "  PATH=${PATH}" >&2
  echo "  expected_go=${EXPECTED_GO}" >&2
  command -v go >&2 || true
  exit 1
}

if [ ! -x "${EXPECTED_GO}" ]; then
  preflight_fail "Go binary not found at ${EXPECTED_GO}"
fi

if ! GO_BIN="$(command -v go)"; then
  preflight_fail "Go is not available in PATH"
fi

if [ "${GO_BIN}" != "${EXPECTED_GO}" ]; then
  echo "coding-agent sandbox preflight warning: Go resolved to ${GO_BIN}, expected ${EXPECTED_GO}" >&2
fi

echo "coding-agent sandbox preflight: command -v go=${GO_BIN}"
echo "coding-agent sandbox preflight: PATH=${PATH}"
echo "coding-agent sandbox preflight: $(${GO_BIN} version)"

exec "$@"
