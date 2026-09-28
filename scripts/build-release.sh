#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-dev}"
DIST="dist"
rm -rf "$DIST"
mkdir -p "$DIST"

build() {
  local goos=$1 goarch=$2 ext=$3
  echo "building $goos/$goarch..."
  GOOS=$goos GOARCH=$goarch go build -ldflags "-X erdtool/internal/buildinfo.Version=$VERSION" -o "$DIST/erdtool-$goos-$goarch$ext" ./cmd/erdtool
}

build windows amd64 .exe
build darwin amd64 ""
build darwin arm64 ""
build linux amd64 ""

echo "done -> $DIST/ (version=$VERSION)"
