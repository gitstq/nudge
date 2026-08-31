#!/usr/bin/env sh
# Install the latest nudge release binary (Linux/macOS).
# Usage: curl -fsSL https://raw.githubusercontent.com/gitstq/nudge/main/scripts/install.sh | sh
set -eu

REPO="gitstq/nudge"
PREFIX="${INSTALL_PREFIX:-/usr/local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux) os=linux ;;
  darwin) os=darwin ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
[ -n "$tag" ] || { echo "could not resolve latest release" >&2; exit 1; }
ver="${tag#v}"
url="https://github.com/$REPO/releases/download/$tag/nudge_${tag}_${os}_${arch}.tar.gz"
echo "downloading $url"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp/nudge.tar.gz"
tar -xzf "$tmp/nudge.tar.gz" -C "$tmp"
mkdir -p "$PREFIX"
if [ -w "$PREFIX" ]; then
  cp "$tmp/nudge_${tag}_${os}_${arch}/nudge" "$PREFIX/nudge"
else
  sudo cp "$tmp/nudge_${tag}_${os}_${arch}/nudge" "$PREFIX/nudge"
fi
chmod +x "$PREFIX/nudge"
echo "installed: $PREFIX/nudge ($tag)"
"$PREFIX/nudge" version
