#!/usr/bin/env sh
# Download a prebuilt agent-gh-repo-token binary from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/td72/agent-gh-repo-token/main/scripts/install.sh | sh
#
# Env overrides:
#   VERSION  release tag to install (default: latest)
#   PREFIX   install directory      (default: /usr/local/bin)
set -eu

REPO="td72/agent-gh-repo-token"
BIN="agent-gh-repo-token"
PREFIX="${PREFIX:-/usr/local/bin}"
VERSION="${VERSION:-latest}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux) ;;
  darwin)
    if [ "$arch" = "amd64" ]; then
      echo "Intel macOS (darwin/amd64) is not supported; use Apple Silicon or build from source (go install)." >&2
      exit 1
    fi
    ;;
  *) echo "unsupported os: $os" >&2; exit 1 ;;
esac

asset="${BIN}-${os}-${arch}"
if [ "$VERSION" = "latest" ]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
fi

tmp="$(mktemp "${TMPDIR:-/tmp}/${BIN}.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
echo "downloading $url" >&2
curl -fsSL -o "$tmp" "$url"
chmod +x "$tmp"

if [ -w "$PREFIX" ]; then
  mv "$tmp" "$PREFIX/$BIN"
else
  echo "elevating to write $PREFIX (set PREFIX= to install elsewhere)" >&2
  sudo mv "$tmp" "$PREFIX/$BIN"
fi
trap - EXIT
echo "installed $PREFIX/$BIN" >&2
