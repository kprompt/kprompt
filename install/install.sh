#!/usr/bin/env bash
# Install kprompt from GitHub Releases.
# Usage:
#   curl -fsSL https://kprompt-website.vercel.app/install | bash
#   curl -fsSL https://raw.githubusercontent.com/kprompt/kprompt/main/install/install.sh | bash
# Later (when DNS is live): curl -fsSL https://kprompt.ai/install | bash
#
# Optional:
#   KPROMPT_VERSION=v0.1.0
#   KPROMPT_INSTALL_DIR=/usr/local/bin   # use sudo if this dir is not writable
set -euo pipefail

REPO="kprompt/kprompt"
BIN="kprompt"

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

# Prefer an explicitly set dir; otherwise /usr/local/bin if writable; else ~/.local/bin.
resolve_prefix() {
  if [[ -n "${KPROMPT_INSTALL_DIR:-}" ]]; then
    echo "$KPROMPT_INSTALL_DIR"
    return
  fi
  local candidate="/usr/local/bin"
  if [[ -d "$candidate" && -w "$candidate" ]]; then
    echo "$candidate"
    return
  fi
  # Directory missing but parent writable (rare).
  if [[ ! -e "$candidate" && -w "$(dirname "$candidate")" ]]; then
    echo "$candidate"
    return
  fi
  echo "${HOME}/.local/bin"
}

PREFIX="$(resolve_prefix)"

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
if [[ ! -w "$PREFIX" ]]; then
  echo "Cannot write to ${PREFIX} (permission denied)." >&2
  echo "Retry with a user-writable dir, e.g.:" >&2
  echo "  KPROMPT_INSTALL_DIR=\"\$HOME/.local/bin\" curl -fsSL https://kprompt-website.vercel.app/install | bash" >&2
  echo "Or install system-wide:" >&2
  echo "  curl -fsSL https://kprompt-website.vercel.app/install | sudo bash" >&2
  exit 1
fi

install -m 755 "$bin_path" "${PREFIX}/${BIN}"
echo "Installed ${PREFIX}/${BIN} (${tag})"

case ":${PATH}:" in
  *":${PREFIX}:"*) ;;
  *)
    echo "Note: ${PREFIX} is not on your PATH." >&2
    echo "Add this to your shell profile:" >&2
    echo "  export PATH=\"${PREFIX}:\$PATH\"" >&2
    ;;
esac

"${PREFIX}/${BIN}" version
