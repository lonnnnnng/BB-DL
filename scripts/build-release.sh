#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
export LANG=C

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VERSION="${VERSION:-}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(git describe --tags --exact-match 2>/dev/null || true)"
fi
if [[ -z "$VERSION" ]]; then
  VERSION="v$(awk -F'"' '/const Version/ { print $2; exit }' internal/bbdown/models.go)"
fi

OUT_DIR="${OUT_DIR:-dist/release}"
mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"
BUILD_TIME="${BUILD_TIME:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"

TMP_DIR="$(mktemp -d)"
# long: 打包过程只清理本次创建的临时目录，避免误删用户已有产物或下载文件。
trap 'rm -rf "$TMP_DIR"' EXIT

TARGETS="${TARGETS:-darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64}"

for target in $TARGETS; do
  goos="${target%/*}"
  goarch="${target#*/}"
  package_name="BB-DL_${VERSION}_cli_${goos}_${goarch}"
  package_dir="$TMP_DIR/$package_name"
  mkdir -p "$package_dir"

  binary="$package_dir/BB-DL"
  if [[ "$goos" == "windows" ]]; then
    binary="$binary.exe"
  fi

  echo "building $package_name"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w -X github.com/lonnnnnng/BB-DL/internal/bbdown.BuildTime=$BUILD_TIME" -o "$binary" ./cmd/bbdown

  if [[ "$goos" == "windows" ]]; then
    (cd "$TMP_DIR" && zip -qr "$OUT_DIR/$package_name.zip" "$package_name")
  else
    tar -czf "$OUT_DIR/$package_name.tar.gz" -C "$TMP_DIR" "$package_name"
  fi
done

echo "release artifacts written to $OUT_DIR"
