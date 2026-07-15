#!/usr/bin/env bash
# Install kprompt from GitHub Releases.
# Usage:
#   curl -fsSL https://kprompt.ai/install | bash
#   curl -fsSL https://raw.githubusercontent.com/kprompt/kprompt/main/install/install.sh | bash
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

if [[ -n "${KPROMPT_VERSION:-}" ]]; then
  tag="$KPROMPT_VERSION"
else
  tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1 || true)"
fi

if [[ -z "$tag" ]]; then
  echo "No GitHub release found yet. Build from source:" >&2
  echo "  go install github.com/kprompt/kprompt/cmd/kprompt@latest" >&2
  exit 1
fi

# GoReleaser Version strips leading v from the tag for archive names.
ver="${tag#v}"
asset="${BIN}_${ver}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${tag}/${asset}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${url}"
if ! curl -fL "$url" -o "${tmp}/${asset}"; then
  echo "Failed to download ${url}" >&2
  echo "Check releases: https://github.com/${REPO}/releases" >&2
  exit 1
fi

tar -xzf "${tmp}/${asset}" -C "$tmp"
bin_path="$(find "$tmp" -type f -name "$BIN" | head -1)"
if [[ -z "$bin_path" ]]; then
  echo "binary $BIN not found in archive" >&2
  exit 1
fi

mkdir -p "$PREFIX"
install -m 755 "$bin_path" "${PREFIX}/${BIN}"
echo "Installed ${PREFIX}/${BIN} (${tag})"
"${PREFIX}/${BIN}" version
