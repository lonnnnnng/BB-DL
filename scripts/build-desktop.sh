#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
export LANG=C
export COPYFILE_DISABLE=1

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VERSION="${VERSION:-}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(git describe --tags --exact-match 2>/dev/null || true)"
fi
if [[ -z "$VERSION" ]]; then
  VERSION="v$(awk -F'"' '/const Version/ { print $2; exit }' internal/bbdown/models.go)"
fi

BUILD_TIME="${BUILD_TIME:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"
GOOS_VALUE="${GOOS:-$(go env GOOS)}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"
OUT_DIR="${OUT_DIR:-dist/desktop}"
PACKAGE_NAME="BB-DL_${VERSION}_desktop_${GOOS_VALUE}_${GOARCH_VALUE}"
TMP_DIR="$(mktemp -d)"

# long 2026-07-26 19:20:40：Wails 桌面端依赖各系统原生 WebView，跨平台发布继续由对应系统的 CI runner 构建，避免把交叉编译产物误当成可运行应用。
trap 'rm -rf "$TMP_DIR"' EXIT
mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"

helper_name="BB-DL-cli"
desktop_name="BB-DL"
if [[ "$GOOS_VALUE" == "windows" ]]; then
  helper_name+=".exe"
  desktop_name+=".exe"
fi

if [[ "$GOOS_VALUE" == "darwin" ]]; then
  # long 2026-06-25 02:36:00：GitHub macos-latest 可能使用更新的 macOS SDK；显式写入部署目标，避免 Mach-O 被标成只能在构建机系统版本上运行。
  export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-11.0}"
  export CGO_CFLAGS="${CGO_CFLAGS:-} -mmacosx-version-min=${MACOSX_DEPLOYMENT_TARGET}"
  export CGO_LDFLAGS="${CGO_LDFLAGS:-} -mmacosx-version-min=${MACOSX_DEPLOYMENT_TARGET}"
fi

build_helper() {
  local output="$1"
  local ldflags="-s -w -X github.com/lonnnnnng/BB-DL/internal/bbdown.BuildTime=$BUILD_TIME"
  if [[ "$GOOS_VALUE" == "darwin" ]]; then
    ldflags+=" -linkmode=external -extldflags=-mmacosx-version-min=${MACOSX_DEPLOYMENT_TARGET}"
  fi
  go build -trimpath -ldflags="$ldflags" -o "$output" ./cmd/bbdown
}

resolve_wails() {
  if [[ -n "${WAILS_BIN:-}" && -x "${WAILS_BIN}" ]]; then
    printf '%s\n' "$WAILS_BIN"
    return
  fi
  if command -v wails >/dev/null 2>&1; then
    command -v wails
    return
  fi
  local go_wails
  go_wails="$(go env GOPATH)/bin/wails"
  if [[ -x "$go_wails" ]]; then
    printf '%s\n' "$go_wails"
    return
  fi
  echo "未找到 Wails CLI，请先执行 go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0" >&2
  return 1
}

build_desktop() {
  local output="$1"
  local wails_bin
  local ldflags="-s -w -X github.com/lonnnnnng/BB-DL/internal/bbdown.BuildTime=$BUILD_TIME"
  local -a args
  wails_bin="$(resolve_wails)"
  args=(build -platform "${GOOS_VALUE}/${GOARCH_VALUE}" -o "$output" -ldflags "$ldflags")
  if [[ "$GOOS_VALUE" == "linux" ]]; then
    # long 2026-07-26 19:20:40：Ubuntu 24.04 只提供 WebKit2GTK 4.1；显式使用 Wails 官方 webkit2_41 tag，保证 Linux runner 与发布二进制链接同一套库。
    args+=(-tags webkit2_41)
  fi
  "$wails_bin" "${args[@]}"
}

if [[ "$GOOS_VALUE" == "darwin" ]]; then
  app_dir="$TMP_DIR/BB-DL.app"
  contents_dir="$app_dir/Contents"
  macos_dir="$contents_dir/MacOS"
  resources_dir="$contents_dir/Resources"
  build_desktop "BB-DL"
  source_app="build/bin/BB-DL.app"
  [[ -d "$source_app" ]] || { echo "Wails 未生成预期应用包：$source_app" >&2; exit 1; }
  ditto "$source_app" "$app_dir"
  mkdir -p "$macos_dir" "$resources_dir"
  build_helper "$resources_dir/$helper_name"
  /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString ${VERSION#v}" "$contents_dir/Info.plist"
  /usr/libexec/PlistBuddy -c "Set :CFBundleVersion ${VERSION#v}" "$contents_dir/Info.plist"
  /usr/libexec/PlistBuddy -c "Set :CFBundleDisplayName BB-DL" "$contents_dir/Info.plist"
  /usr/libexec/PlistBuddy -c "Set :LSMinimumSystemVersion ${MACOSX_DEPLOYMENT_TARGET}" "$contents_dir/Info.plist"
  chmod +x "$macos_dir/BB-DL" "$resources_dir/$helper_name"
  if command -v codesign >/dev/null 2>&1; then
    codesign --force --deep --sign - "$app_dir" >/dev/null
  fi
  ditto -c -k --norsrc --keepParent "$app_dir" "$OUT_DIR/$PACKAGE_NAME.zip"
else
  package_dir="$TMP_DIR/$PACKAGE_NAME"
  mkdir -p "$package_dir"
  build_desktop "$desktop_name"
  desktop_output="build/bin/$desktop_name"
  [[ -f "$desktop_output" ]] || { echo "Wails 未生成预期桌面程序：$desktop_output" >&2; exit 1; }
  cp "$desktop_output" "$package_dir/$desktop_name"
  build_helper "$package_dir/$helper_name"
  chmod +x "$package_dir/$desktop_name" "$package_dir/$helper_name" 2>/dev/null || true
  if [[ "$GOOS_VALUE" == "windows" ]]; then
    (cd "$TMP_DIR" && zip -qr "$OUT_DIR/$PACKAGE_NAME.zip" "$PACKAGE_NAME")
  else
    tar -czf "$OUT_DIR/$PACKAGE_NAME.tar.gz" -C "$TMP_DIR" "$PACKAGE_NAME"
  fi
fi

echo "desktop artifact written to $OUT_DIR"
