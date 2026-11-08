#!/usr/bin/env bash
# Cold-install smoke: install a published release into an empty prefix, then
# version + hard-deny jq. No cluster required.
#
# Env:
#   KPROMPT_VERSION  tag to install (default: latest via install.sh, or v0.12.5 pin in CI)
#   KPROMPT_INSTALL_DIR  install prefix (default: workdir/bin)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="${WORKDIR:-$(mktemp -d "${TMPDIR:-/tmp}/kprompt-cold-install.XXXXXX")}"
INSTALL_DIR="${KPROMPT_INSTALL_DIR:-$WORKDIR/bin}"
mkdir -p "$INSTALL_DIR" "$WORKDIR/home"

echo "==> cold install into $INSTALL_DIR (version=${KPROMPT_VERSION:-latest})"
export KPROMPT_INSTALL_DIR="$INSTALL_DIR"
# Prefer repo install.sh so CI matches source; fall back to site URL only if missing.
INSTALL_SH="$ROOT/install/install.sh"
if [[ -f "$INSTALL_SH" ]]; then
  bash "$INSTALL_SH"
else
  curl -fsSL https://kprompt.ai/install | bash
fi

KPROMPT="$INSTALL_DIR/kprompt"
if [[ ! -x "$KPROMPT" ]]; then
  echo "FAIL: binary missing at $KPROMPT" >&2
  ls -la "$INSTALL_DIR" >&2 || true
  exit 1
fi

# Prefer the just-installed binary over any kprompt already on PATH.
export PATH="${INSTALL_DIR}:$PATH"

echo "==> version"
"$KPROMPT" version
ver="$("$KPROMPT" version 2>/dev/null | tr -d '[:space:]')"
if [[ -n "${KPROMPT_VERSION:-}" ]]; then
  want="${KPROMPT_VERSION#v}"
  if [[ "$ver" != "$want" && "$ver" != "$KPROMPT_VERSION" ]]; then
    echo "FAIL: expected version $KPROMPT_VERSION, got $ver" >&2
    exit 1
  fi
fi

echo "==> demo --check (other tools may be missing — non-fatal)"
# Isolate HOME so demo does not pick up a developer kube/config state.
HOME="$WORKDIR/home" KUBECONFIG=/var/empty/kprompt-no-kube "$KPROMPT" demo --check || true

echo "==> hard-deny jq via installed binary"
KPROMPT="$KPROMPT" HOME="$WORKDIR/home" \
  KPROMPT_HOME="$WORKDIR/home/.kprompt" \
  KUBECONFIG=/var/empty/kprompt-no-kube \
  WORKDIR="$WORKDIR/jq" \
  "$ROOT/scripts/planresult-jq-gate.sh"

echo "PASS cold-install smoke"
echo "artifacts: $WORKDIR"
