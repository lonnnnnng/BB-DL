#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
export LANG=C

usage() {
  echo "usage: $0 cli|desktop <artifact>..." >&2
}

if [[ $# -lt 2 ]]; then
  usage
  exit 2
fi

mode="$1"
shift
case "$mode" in
  cli|desktop) ;;
  *)
    usage
    exit 2
    ;;
esac

TMP_DIR="$(mktemp -d)"
# long 2026-06-27 13:28:00：校验只解包到本次临时目录，避免检查 release 资产时碰到用户本地下载目录或已有构建缓存。
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
  echo "artifact verification failed: $*" >&2
  exit 1
}

archive_entries() {
  local artifact="$1"
  case "$artifact" in
    *.zip)
      unzip -Z1 "$artifact"
      ;;
    *.tar.gz|*.tgz)
      tar -tzf "$artifact"
      ;;
    *)
      fail "$artifact: unsupported artifact format"
      ;;
  esac
}

assert_safe_archive_entry() {
  local artifact="$1"
  local entry="$2"
  if [[ -z "$entry" ]]; then
    fail "$artifact: archive contains empty entry name"
  fi
  if [[ "$entry" == /* || "$entry" == *\\* || "$entry" == *//* ]]; then
    fail "$artifact: unsafe archive entry path: $entry"
  fi
  if [[ "$entry" == "." || "$entry" == "./"* || "$entry" == *"/./"* ]]; then
    fail "$artifact: unsafe archive entry path: $entry"
  fi
  if [[ "$entry" == ".." || "$entry" == "../"* || "$entry" == *"/../"* ]]; then
    fail "$artifact: unsafe archive entry path: $entry"
  fi
}

assert_safe_archive_entries() {
  local artifact="$1"
  local entries_file
  local entry
  entries_file="$(mktemp "$TMP_DIR/archive-entries.XXXXXX")"
  # long 2026-06-27 14:06:00：先把条目列表落到临时文件并检查命令退出码；进程替换里的 tar/unzip 失败不一定会可靠触发主脚本退出，损坏包必须在解包前被明确拒绝。
  if ! archive_entries "$artifact" >"$entries_file"; then
    fail "$artifact: cannot list archive entries"
  fi
  if [[ ! -s "$entries_file" ]]; then
    fail "$artifact: archive contains no entries"
  fi
  while IFS= read -r entry || [[ -n "$entry" ]]; do
    assert_safe_archive_entry "$artifact" "$entry"
  done < "$entries_file"
}

extract_artifact() {
  local artifact="$1"
  local target="$2"
  assert_safe_archive_entries "$artifact"
  mkdir -p "$target"
  case "$artifact" in
    *.zip)
      unzip -q "$artifact" -d "$target"
      ;;
    *.tar.gz|*.tgz)
      tar -xzf "$artifact" -C "$target"
      ;;
    *)
      fail "$artifact: unsupported artifact format"
      ;;
  esac
  assert_no_symlinks "$target" "$artifact"
}

assert_no_symlinks() {
  local root="$1"
  local artifact="$2"
  local match
  while IFS= read -r match; do
    fail "$artifact: unexpected symlink in artifact: ${match#$root/}"
  done < <(find "$root" -type l -print)
}

assert_no_docs_or_readme() {
  local root="$1"
  local artifact="$2"
  local match
  while IFS= read -r match; do
    fail "$artifact: unexpected documentation file in artifact: ${match#$root/}"
  done < <(find "$root" \( -iname 'README*' -o -iname '*.md' -o -path '*/docs' -o -path '*/docs/*' \) -print)
}

regular_files() {
  local root="$1"
  find "$root" -type f -print | sort
}

assert_single_top_level() {
  local root="$1"
  local artifact="$2"
  local count
  count="$(find "$root" -mindepth 1 -maxdepth 1 -print | wc -l | tr -d ' ')"
  [[ "$count" == "1" ]] || fail "$artifact: expected one top-level item, got $count"
}

assert_cli_artifact() {
  local artifact="$1"
  local root="$2"
  local file_count
  local file
  file_count="$(regular_files "$root" | wc -l | tr -d ' ')"
  [[ "$file_count" == "1" ]] || fail "$artifact: CLI package must contain exactly one file, got $file_count"
  file="$(regular_files "$root")"
  case "$(basename "$file")" in
    BB-DL|BB-DL.exe) ;;
    *) fail "$artifact: unexpected CLI file: ${file#$root/}" ;;
  esac
}

assert_desktop_macos_artifact() {
  local artifact="$1"
  local root="$2"
  local app="$root/BB-DL.app"
  [[ -d "$app" ]] || fail "$artifact: expected top-level BB-DL.app"
  [[ -f "$app/Contents/MacOS/BB-DL" ]] || fail "$artifact: missing desktop executable inside app bundle"
  [[ -f "$app/Contents/Resources/BB-DL-cli" ]] || fail "$artifact: missing CLI helper inside app bundle"
}

assert_desktop_flat_artifact() {
  local artifact="$1"
  local root="$2"
  local package_dir
  local file_count
  assert_single_top_level "$root" "$artifact"
  package_dir="$(find "$root" -mindepth 1 -maxdepth 1 -type d -print)"
  [[ -n "$package_dir" ]] || fail "$artifact: expected package directory"
  file_count="$(regular_files "$package_dir" | wc -l | tr -d ' ')"
  [[ "$file_count" == "2" ]] || fail "$artifact: desktop package must contain desktop executable and helper only, got $file_count files"
  if [[ "$artifact" == *windows* ]]; then
    [[ -f "$package_dir/BB-DL.exe" ]] || fail "$artifact: missing Windows desktop executable"
    [[ -f "$package_dir/BB-DL-cli.exe" ]] || fail "$artifact: missing Windows CLI helper"
  else
    [[ -f "$package_dir/BB-DL" ]] || fail "$artifact: missing desktop executable"
    [[ -f "$package_dir/BB-DL-cli" ]] || fail "$artifact: missing CLI helper"
  fi
}

assert_desktop_artifact() {
  local artifact="$1"
  local root="$2"
  if [[ "$artifact" == *darwin* ]]; then
    assert_single_top_level "$root" "$artifact"
    assert_desktop_macos_artifact "$artifact" "$root"
    return
  fi
  assert_desktop_flat_artifact "$artifact" "$root"
}

for artifact in "$@"; do
  [[ -f "$artifact" ]] || fail "$artifact: file not found"
  work_dir="$TMP_DIR/$(basename "$artifact").d"
  extract_artifact "$artifact" "$work_dir"
  assert_no_docs_or_readme "$work_dir" "$artifact"
  case "$mode" in
    cli) assert_cli_artifact "$artifact" "$work_dir" ;;
    desktop) assert_desktop_artifact "$artifact" "$work_dir" ;;
  esac
  echo "verified $mode artifact: $artifact"
done
