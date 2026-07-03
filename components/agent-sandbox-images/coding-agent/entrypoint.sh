#!/usr/bin/env bash
set -euo pipefail

mkdir -p "${WORKSPACE:-/workspace}"
exec "$@"
