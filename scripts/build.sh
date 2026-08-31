#!/usr/bin/env bash
# Cross-platform build for nudge. Produces static binaries for
# linux/darwin/windows × amd64/arm64 plus SHA-256 checksums under build/.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo v1.0.0)}"
OUT="${OUT:-build}"
mkdir -p "$OUT"

TARGETS=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

LDFLAGS="-s -w"
export CGO_ENABLED=0

for t in "${TARGETS[@]}"; do
  set -- $t
  GOOS=$1; GOARCH=$2
  name="nudge_${VERSION}_${GOOS}_${GOARCH}"
  echo ">> building $name"
  if [ "$GOOS" = "windows" ]; then
    GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/$name/nudge.exe" .
    (cd "$OUT/$name" && zip -qr "../$name.zip" .)
  else
    GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/$name/nudge" .
    tar -C "$OUT" -czf "$OUT/$name.tar.gz" "$name"
  fi
  rm -rf "$OUT/$name"
done

( cd "$OUT" && sha256sum nudge_*.tar.gz nudge_*.zip > checksums.txt )
echo ">> artifacts:"
ls -la "$OUT"
