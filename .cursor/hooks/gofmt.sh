#!/usr/bin/env bash
# afterFileEdit: gofmt agent-edited .go files. Fail open.
set -euo pipefail

input=$(cat || true)
[[ -z "${input}" ]] && exit 0

file_path=$(printf '%s' "${input}" | python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    raise SystemExit(0)
print(data.get("file_path") or "")
')

[[ -z "${file_path}" ]] && exit 0
[[ "${file_path}" == *.go ]] || exit 0
[[ -f "${file_path}" ]] || exit 0

if ! command -v gofmt >/dev/null 2>&1; then
  exit 0
fi

gofmt -w "${file_path}" >/dev/null 2>&1 || true
exit 0
