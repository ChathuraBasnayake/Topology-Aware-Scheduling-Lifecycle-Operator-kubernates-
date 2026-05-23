#!/bin/bash
# What this does:
# Deletes the entire topology-test Kind cluster.

set -euo pipefail

KIND_CLUSTER="topology-test"

# Detect kind binary path
KIND_BIN="kind"
if ! command -v kind &> /dev/null; then
  if [ -f "$HOME/go/bin/kind" ]; then
    KIND_BIN="$HOME/go/bin/kind"
  elif [ -f "$HOME/go/bin/kind.exe" ]; then
    KIND_BIN="$HOME/go/bin/kind.exe"
  else
    # Fallback/guess for Windows environment in git bash / WSL
    USERPROFILE_WIN=$(cmd.exe /c "echo %USERPROFILE%" 2>/dev/null | tr -d '\r')
    if [ -n "${USERPROFILE_WIN}" ]; then
      WSL_USERPROFILE=$(echo "${USERPROFILE_WIN}" | sed -e 's/\\/\//g' -e 's/^C:/\/mnt\/c/')
      GITBASH_USERPROFILE=$(echo "${USERPROFILE_WIN}" | sed -e 's/\\/\//g' -e 's/^C:/\/c/')
      if [ -f "${WSL_USERPROFILE}/go/bin/kind.exe" ]; then
        KIND_BIN="${WSL_USERPROFILE}/go/bin/kind.exe"
      elif [ -f "${GITBASH_USERPROFILE}/go/bin/kind.exe" ]; then
        KIND_BIN="${GITBASH_USERPROFILE}/go/bin/kind.exe"
      fi
    fi
  fi
fi

echo "Deleting Kind cluster '${KIND_CLUSTER}' using ${KIND_BIN}..."
"${KIND_BIN}" delete cluster --name "${KIND_CLUSTER}"
