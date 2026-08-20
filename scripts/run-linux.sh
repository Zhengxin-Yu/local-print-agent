#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${LOCAL_PRINT_AGENT_PRINTER_MODE:-demo}"
browser_path="${LOCAL_PRINT_AGENT_BROWSER_PATH:-}"
go_cache="${GOCACHE:-}"

usage() {
  cat <<'EOF'
Usage: ./scripts/run-linux.sh [--mode demo|platform] [--browser-path PATH] [--go-cache DIR]

demo      Generate real PDF previews and use a non-printing Mock Printer.
platform  Submit generated PDFs to an explicitly selected CUPS queue.

The LOCAL_PRINT_AGENT_PRINTER_MODE, LOCAL_PRINT_AGENT_BROWSER_PATH, and GOCACHE
environment variables provide defaults; command-line options take precedence.
EOF
}

while (($#)); do
  case "$1" in
    --mode)
      [[ $# -ge 2 ]] || { echo 'Missing value for --mode.' >&2; exit 2; }
      mode="$2"
      shift 2
      ;;
    --browser-path)
      [[ $# -ge 2 ]] || { echo 'Missing value for --browser-path.' >&2; exit 2; }
      browser_path="$2"
      shift 2
      ;;
    --go-cache)
      [[ $# -ge 2 ]] || { echo 'Missing value for --go-cache.' >&2; exit 2; }
      go_cache="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "${mode,,}" in
  demo|platform) mode="${mode,,}" ;;
  *) echo "Invalid mode '$mode'; use demo or platform." >&2; exit 2 ;;
esac

[[ -f "$repo_root/go.mod" ]] || { echo "Repository root is invalid: $repo_root" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo 'Go was not found on PATH. Install Go 1.23 or newer.' >&2; exit 1; }
if [[ -n "$browser_path" && ! -f "$browser_path" ]]; then
  echo "Chrome/Chromium executable does not exist or is not a file: $browser_path" >&2
  exit 1
fi
if [[ -n "$browser_path" ]]; then
  browser_path="$(cd "$(dirname "$browser_path")" && pwd)/$(basename "$browser_path")"
fi
if [[ -n "$go_cache" ]]; then
  mkdir -p "$go_cache"
  [[ -d "$go_cache" && -w "$go_cache" ]] || { echo "Go cache is not a writable directory: $go_cache" >&2; exit 1; }
  export GOCACHE="$(cd "$go_cache" && pwd)"
fi
if [[ "$mode" == platform ]]; then
  command -v lp >/dev/null 2>&1 || { echo "Platform mode requires the CUPS 'lp' command." >&2; exit 1; }
  command -v lpstat >/dev/null 2>&1 || { echo "Platform mode requires the CUPS 'lpstat' command." >&2; exit 1; }
fi

cd "$repo_root"
if [[ "$mode" == demo ]]; then
  echo 'Starting in demo mode: PDF previews are real; Mock Printer never submits to an OS queue.'
else
  echo 'Starting in platform mode: jobs may be submitted to the selected CUPS queue.'
fi
if [[ -z "$browser_path" ]]; then
  echo 'No browser path was supplied; the agent will use PATH and common install locations.'
  unset LOCAL_PRINT_AGENT_BROWSER_PATH
else
  export LOCAL_PRINT_AGENT_BROWSER_PATH="$browser_path"
fi
export LOCAL_PRINT_AGENT_PRINTER_MODE="$mode"
echo 'Open the loopback URL printed below in your browser.'
exec go run ./cmd/local-print-agent
