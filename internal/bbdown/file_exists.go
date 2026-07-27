package bbdown

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	FileExistsActionSkip      = "skip"
	FileExistsActionRename    = "rename"
	FileExistsActionOverwrite = "overwrite"
)

func NormalizeFileExistsAction(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", FileExistsActionSkip:
		return FileExistsActionSkip
	case FileExistsActionRename, "sequence", "serial", "number":
		return FileExistsActionRename
	case FileExistsActionOverwrite, "replace":
		return FileExistsActionOverwrite
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func ValidateFileExistsAction(value string) error {
	switch NormalizeFileExistsAction(value) {
	case FileExistsActionSkip, FileExistsActionRename, FileExistsActionOverwrite:
		return nil
	default:
		return fmt.Errorf("同名文件处理方式无效: %s；可选值为 rename、skip、overwrite", value)
	}
}

func ResolveFileExistsPath(path, action string) (string, error) {
	return resolveFileExistsPath(path, action, nil)
}

func resolveFileExistsPath(path, action string, reserved map[string]struct{}) (string, error) {
	if NormalizeFileExistsAction(action) != FileExistsActionRename {
		return path, nil
	}
	collides, err := outputGroupExists(path)
	if !collides && reserved != nil {
		_, collides = reserved[outputGroupKey(path)]
	}
	if err != nil || !collides {
		reserveOutputGroup(path, reserved)
		return path, err
	}

	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	// long 2026-07-27 00:03:56：流水号必须作用于整组输出基名，主媒体、字幕、封面、弹幕和临时分轨才能落到同一组新文件中。
	for serial := 1; serial < 1_000_000; serial++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, serial, ext)
		collides, err = outputGroupExists(candidate)
		if err != nil {
			return "", err
		}
		if !collides && reserved != nil {
			_, collides = reserved[outputGroupKey(candidate)]
		}
		if !collides {
			reserveOutputGroup(candidate, reserved)
			return candidate, nil
		}
	}
	return "", fmt.Errorf("无法为同名文件分配可用流水号: %s", path)
}

func ResolveFileExistsPathInDir(path, action, workDir string) (string, error) {
	return resolveFileExistsPathInDir(path, action, workDir, nil)
}

func resolveFileExistsPathInDir(path, action, workDir string, reserved map[string]struct{}) (string, error) {
	if filepath.IsAbs(path) || strings.TrimSpace(workDir) == "" {
		return resolveFileExistsPath(path, action, reserved)
	}
	resolved, err := resolveFileExistsPath(filepath.Join(workDir, path), action, reserved)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(workDir, resolved)
	if err != nil {
		return "", err
	}
	return relative, nil
}

func outputGroupKey(path string) string {
	cleaned := filepath.Clean(path)
	return strings.TrimSuffix(cleaned, filepath.Ext(cleaned))
}

func reserveOutputGroup(path string, reserved map[string]struct{}) {
	if reserved != nil {
		reserved[outputGroupKey(path)] = struct{}{}
	}
}

func outputGroupExists(path string) (bool, error) {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	filename := filepath.Base(path)
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	for _, entry := range entries {
		name := entry.Name()
		if name == filename || name == stem || strings.HasPrefix(name, stem+".") || isDownloadPartForStem(name, stem) {
			return true, nil
		}
	}
	return false, nil
}

func isDownloadPartForStem(name, stem string) bool {
	if len(name) <= 6 || name[5] != '_' {
		return false
	}
	for _, ch := range name[:5] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	remainder := name[6:]
	return strings.HasPrefix(remainder, stem+".") && (strings.HasSuffix(remainder, ".vclip") || strings.HasSuffix(remainder, ".aclip"))
}

func IsFileOverwriteEnabled(opt *MyOption) bool {
	return opt != nil && NormalizeFileExistsAction(opt.FileExistsAction) == FileExistsActionOverwrite
}

func ShouldSkipExistingFile(path string, opt *MyOption) bool {
	if opt == nil || NormalizeFileExistsAction(opt.FileExistsAction) != FileExistsActionSkip {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func PrepareDownloadDestination(path string, opt *MyOption) error {
	if !IsFileOverwriteEnabled(opt) || strings.TrimSpace(path) == "" {
		return nil
	}
	// long 2026-07-27 00:03:56：覆盖模式必须同时清掉目标文件和断点续传状态，否则同尺寸文件、.tmp、aria2 控制文件或旧分片仍会让下载器复用旧内容。
	for _, candidate := range []string{path, path + ".aria2", singleThreadTempPath(path)} {
		if err := removeExistingFile(candidate); err != nil {
			return err
		}
	}
	return removeMultiThreadParts(path)
}

func singleThreadTempPath(path string) string {
	return filepath.Join(filepath.Dir(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".tmp")
}

func removeExistingFile(path string) error {
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func removeMultiThreadParts(path string) error {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	suffix := ".aclip"
	if ext == ".mp4" {
		suffix = ".vclip"
	}
	for _, entry := range entries {
		name := entry.Name()
		if !isExactDownloadPart(name, base, suffix) {
			continue
		}
		if err := removeExistingFile(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func isExactDownloadPart(name, base, suffix string) bool {
	if len(name) <= 6 || name[5] != '_' {
		return false
	}
	remainder := name[6:]
	if len(remainder) != len(base)+len(suffix) || remainder[:len(base)] != base || !strings.EqualFold(remainder[len(base):], suffix) {
		return false
	}
	for _, ch := range name[:5] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
