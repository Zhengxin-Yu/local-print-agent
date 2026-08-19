#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
echo "Starting local-print-agent. Open the URL printed below in your browser."
exec go run ./cmd/local-print-agent
