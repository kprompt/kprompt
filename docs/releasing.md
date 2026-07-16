# Releasing

kprompt ships binaries via [GoReleaser](https://goreleaser.com) and GitHub Releases.

## Tag a release

```bash
# on main, clean working tree
git pull origin main
VERSION=v0.3.0
git tag -a "$VERSION" -m "$VERSION"
git push origin "$VERSION"
```

The [release workflow](../.github/workflows/release.yml) builds:

| OS | Arch |
|----|------|
| darwin | amd64, arm64 |
| linux | amd64, arm64 |

Archive name: `kprompt_<version>_<os>_<arch>.tar.gz` (version without leading `v`).

## Install

```bash
curl -fsSL https://kprompt.ai/install | bash
# or
curl -fsSL https://raw.githubusercontent.com/kprompt/kprompt/main/install/install.sh | bash
```

Override version or install dir:

```bash
KPROMPT_VERSION=v0.3.0 KPROMPT_INSTALL_DIR="$HOME/bin" bash install/install.sh
```

## Local snapshot (no publish)

```bash
goreleaser release --snapshot --clean
```
