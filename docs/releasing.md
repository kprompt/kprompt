# Releasing

kprompt ships binaries via [GoReleaser](https://goreleaser.com) and GitHub Releases.

## Tag a release

```bash
# on main, clean working tree
git pull origin main
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

The [release workflow](../.github/workflows/release.yml) builds:

| OS | Arch |
|----|------|
| darwin | amd64, arm64 |
| linux | amd64, arm64 |

Archive name: `kprompt_<version>_<os>_<arch>.tar.gz` (version without leading `v`).

## Install

Until the brand domain DNS is live, prefer the Vercel host:

```bash
curl -fsSL https://kprompt-website.vercel.app/install | bash
# or
curl -fsSL https://raw.githubusercontent.com/kprompt/kprompt/main/install/install.sh | bash
```

Override version or install dir:

```bash
KPROMPT_VERSION=v0.2.0 KPROMPT_INSTALL_DIR="$HOME/bin" bash install/install.sh
```

## Local snapshot (no publish)

```bash
goreleaser release --snapshot --clean
```
