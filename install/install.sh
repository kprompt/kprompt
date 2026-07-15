#!/usr/bin/env bash
# Install kprompt from GitHub Releases.
# Usage: curl -fsSL https://raw.githubusercontent.com/kprompt/kprompt/main/install/install.sh | bash
set -euo pipefail

REPO="kprompt/kprompt"
BIN="kprompt"
PREFIX="${KPROMPT_INSTALL_DIR:-/usr/local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  *) echo "unsupported os: $os" >&2; exit 1 ;;
esac

tag="${KPROMPT_VERSION:-$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)}"
if [[ -z "$tag" ]]; then
  echo "No GitHub release found yet. Build from source:" >&2
  echo "  go install github.com/kprompt/kprompt/cmd/kprompt@latest" >&2
  exit 1
fi

asset="${BIN}_${tag#v}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${tag}/${asset}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${url}"
curl -fsSL "$url" -o "${tmp}/${asset}"
tar -xzf "${tmp}/${asset}" -C "$tmp"
install -m 755 "${tmp}/${BIN}" "${PREFIX}/${BIN}"
echo "Installed ${PREFIX}/${BIN}"
"${PREFIX}/${BIN}" version
