#!/usr/bin/env bash
set -euo pipefail
echo "PATH=$PATH"
echo "expected_go=/usr/local/go/bin/go"
command -v go >/dev/null || { echo "ERROR: go not found in PATH"; exit 1; }
go version
