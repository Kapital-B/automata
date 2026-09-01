#!/usr/bin/env bash
# Builds Lambda bootstrap binaries into .dist/ for archive_file (plan + apply).
# Invoked as a Terraform data.external program: reads JSON on stdin, writes JSON on stdout.
set -euo pipefail

MODULE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${MODULE_DIR}/../../.." && pwd)"
OUT="${MODULE_DIR}/.dist"

QUERY="$(cat)"
GOARCH="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("arch") or "amd64")' <<<"$QUERY")"
FINGERPRINT="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("fingerprint") or "")' <<<"$QUERY")"

MARKER="${OUT}/.fingerprint"
need_build=0
if [[ ! -f "${MARKER}" ]] || [[ "$(cat "${MARKER}")" != "${FINGERPRINT}" ]]; then
  need_build=1
fi
for d in api scheduler worker migrate; do
  if [[ ! -f "${OUT}/${d}/bootstrap" ]]; then
    need_build=1
  fi
done

if [[ "${need_build}" -eq 1 ]]; then
  rm -rf "${OUT}"
  mkdir -p "${OUT}/api" "${OUT}/scheduler" "${OUT}/worker" "${OUT}/migrate"
  docker run --rm -v "${ROOT}:/workspace" -w /workspace "golang:1.24" bash -c "
    set -euo pipefail
    export CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH}
    go build -trimpath -o /workspace/terraform/modules/automata/.dist/api/bootstrap ./cmd/api
    go build -trimpath -o /workspace/terraform/modules/automata/.dist/scheduler/bootstrap ./cmd/scheduler
    go build -trimpath -o /workspace/terraform/modules/automata/.dist/worker/bootstrap ./cmd/worker
    go build -trimpath -o /workspace/terraform/modules/automata/.dist/migrate/bootstrap ./cmd/migrate
  " 1>&2
  printf '%s\n' "${FINGERPRINT}" >"${MARKER}"
fi

# Only JSON on stdout — required by terraform data.external.
python3 -c 'import json; print(json.dumps({"ok":"true","api":"api","scheduler":"scheduler","worker":"worker","migrate":"migrate"}))'
