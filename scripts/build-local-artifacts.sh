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

OUT_DIR="${OUT_DIR:-dist/local}"
CLI_DIR="$OUT_DIR/release"
DESKTOP_DIR="$OUT_DIR/desktop"

echo "building local artifacts for $VERSION"
VERSION="$VERSION" OUT_DIR="$CLI_DIR" ./scripts/build-release.sh
VERSION="$VERSION" OUT_DIR="$DESKTOP_DIR" ./scripts/build-desktop.sh

./scripts/verify-artifacts.sh cli "$CLI_DIR"/*
./scripts/verify-artifacts.sh desktop "$DESKTOP_DIR"/*

cat <<EOF
local artifacts written to:
  $CLI_DIR
  $DESKTOP_DIR

Manual GitHub upload example:
  gh release upload "$VERSION" "$CLI_DIR"/* "$DESKTOP_DIR"/* --repo lonnnnnng/BB-DL --clobber
EOF
