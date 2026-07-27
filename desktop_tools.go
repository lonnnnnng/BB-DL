package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lonnnnnng/BB-DL/internal/bbdown"
)

type processHandle struct {
	process *os.Process
}

func (handle processHandle) Kill() error {
	if handle.process == nil {
		return nil
	}
	return handle.process.Kill()
}

func ensureWorkDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("请先填写保存目录")
	}
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("保存目录不是文件夹：%s", path)
		}
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("检查保存目录失败：%w", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("创建保存目录失败：%w", err)
	}
	return path, nil
}

func runtimeDir() (string, error) {
	if value := strings.TrimSpace(os.Getenv(desktopRuntimeDirEnv)); value != "" {
		return value, nil
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return "", errors.New("无法定位用户配置目录")
	}
	return filepath.Join(base, "BBDown Go"), nil
}

func helperName() string {
	if runtime.GOOS == "windows" {
		return "BB-DL-cli.exe"
	}
	return "BB-DL-cli"
}

func locateHelper() (string, error) {
	if value := strings.TrimSpace(os.Getenv("BBDOWN_GO_HELPER")); value != "" {
		if fileExists(value) {
			return value, nil
		}
		return "", fmt.Errorf("BBDOWN_GO_HELPER 指向的文件不存在：%s", value)
	}
	executable, _ := os.Executable()
	executableDir := filepath.Dir(executable)
	for _, candidate := range []string{filepath.Join(executableDir, helperName()), filepath.Join(executableDir, "..", "Resources", helperName()), filepath.Join(".", helperName())} {
		if fileExists(candidate) {
			absolute, _ := filepath.Abs(candidate)
			return absolute, nil
		}
	}
	return "", errors.New("找不到 BB-DL-cli helper；请重新安装完整桌面包，或设置 BBDOWN_GO_HELPER")
}

func prepareRuntime() (string, string, error) {
	dir, err := runtimeDir()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	source, err := locateHelper()
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(dir, helperName())
	if err := copyHelperIfNeeded(source, target); err != nil {
		return "", "", err
	}
	return dir, target, nil
}

func copyHelperIfNeeded(source, target string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	if targetInfo, err := os.Stat(target); err == nil && sourceInfo.Size() == targetInfo.Size() && sourceInfo.ModTime().Equal(targetInfo.ModTime()) {
		same, err := sameFileContent(source, target)
		if err != nil {
			return err
		}
		if same {
			return nil
		}
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		return err
	}
	return os.Chtimes(target, time.Now(), sourceInfo.ModTime())
}

func sameFileContent(left, right string) (bool, error) {
	leftData, err := os.ReadFile(left)
	if err != nil {
		return false, err
	}
	rightData, err := os.ReadFile(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftData, rightData), nil
}
func fileExists(path string) bool { info, err := os.Stat(path); return err == nil && !info.IsDir() }

func desktopCommandEnv() []string {
	toolDirs := desktopExtraToolDirs()
	env := envWithValue(os.Environ(), "PATH", mergeToolPath(os.Getenv("PATH"), toolDirs...))
	if len(toolDirs) > 0 {
		env = envWithValue(env, desktopToolDirsEnv, strings.Join(toolDirs, string(os.PathListSeparator)))
	}
	return env
}

func envWithValue(env []string, key, value string) []string {
	result := make([]string, 0, len(env)+1)
	replaced := false
	for _, item := range env {
		itemKey := item
		if index := strings.IndexByte(item, '='); index >= 0 {
			itemKey = item[:index]
		}
		if strings.EqualFold(itemKey, key) {
			if !replaced {
				result = append(result, key+"="+value)
				replaced = true
			}
			continue
		}
		result = append(result, item)
	}
	if !replaced {
		result = append(result, key+"="+value)
	}
	return result
}

func mergeToolPath(pathValue string, extraDirs ...string) string {
	seen := map[string]bool{}
	parts := []string{}
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" || seen[dir] {
			return
		}
		seen[dir] = true
		parts = append(parts, dir)
	}
	for _, dir := range strings.Split(pathValue, string(os.PathListSeparator)) {
		add(dir)
	}
	for _, dir := range extraDirs {
		add(dir)
	}
	for _, dir := range commonToolDirs() {
		add(dir)
	}
	for _, dir := range loginShellToolDirs("ffmpeg", "MP4Box", "mp4box", "aria2c") {
		add(dir)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

func desktopExtraToolDirs() []string {
	seen := map[string]bool{}
	dirs := []string{}
	add := func(dir string) {
		dir = strings.TrimSpace(filepath.Clean(dir))
		if dir == "" || dir == "." || seen[dir] {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	if dir, err := os.Getwd(); err == nil {
		add(dir)
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		add(dir)
		add(filepath.Join(dir, "bin"))
		add(filepath.Join(dir, "..", "Resources"))
		add(filepath.Join(dir, "..", "Resources", "bin"))
		add(filepath.Join(dir, "..", "Resources", "ffmpeg"))
	}
	if helper, err := locateHelper(); err == nil {
		dir := filepath.Dir(helper)
		add(dir)
		add(filepath.Join(dir, "bin"))
		add(filepath.Join(dir, "ffmpeg"))
	}
	if dir, err := runtimeDir(); err == nil {
		add(dir)
		add(filepath.Join(dir, "bin"))
	}
	return dirs
}

func commonToolDirs() []string {
	dirs := []string{}
	if home := userHomeDir(); home != "." {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin"))
	}
	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin")
	case "linux":
		dirs = append(dirs, "/usr/local/bin", "/usr/bin", "/bin", "/snap/bin")
	case "windows":
		dirs = append(dirs, `C:\ffmpeg\bin`, `C:\Program Files\ffmpeg\bin`)
	}
	return dirs
}

func detectToolPath(names ...string) string {
	if path := findDesktopToolPath("", names...); path != "" {
		return path
	}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil && fileExists(path) {
			return path
		}
	}
	for _, dir := range commonToolDirs() {
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if fileExists(candidate) {
				return candidate
			}
		}
	}
	return detectToolPathFromLoginShell(names...)
}

func detectToolPathFromLoginShell(names ...string) string {
	if runtime.GOOS != "darwin" || !fileExists(desktopLoginShellPath) {
		return ""
	}
	for _, name := range names {
		if strings.TrimSpace(name) == "" || !isBareToolName(name) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		out, err := exec.CommandContext(ctx, desktopLoginShellPath, "-lc", "command -v -- "+unixShellQuote(name)).Output()
		cancel()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			path := strings.TrimSpace(line)
			if path != "" && fileExists(path) {
				return path
			}
		}
	}
	return ""
}

func loginShellToolDirs(names ...string) []string {
	seen := map[string]bool{}
	dirs := []string{}
	for _, name := range names {
		path := detectToolPathFromLoginShell(name)
		if path == "" {
			continue
		}
		dir := filepath.Dir(path)
		if dir != "." && !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func normalizeToolPath(value string, names ...string) string {
	value = toolPathValue(value)
	if value != "" && fileExists(value) {
		return value
	}
	if value != "" && isBareToolName(value) {
		if path := detectToolPath(append([]string{value}, names...)...); path != "" {
			return path
		}
	}
	if path := detectToolPath(names...); path != "" {
		return path
	}
	return value
}

func toolPathValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "自动" {
		return ""
	}
	return value
}
func isBareToolName(name string) bool { return !strings.ContainsAny(name, `/\`) }

func findDesktopToolPath(explicit string, names ...string) string {
	if path := bbdown.FindBinaryPath(explicit, names...); path != "" {
		return path
	}
	candidates := []string{}
	if explicit != "" && isBareToolName(explicit) {
		candidates = append(candidates, explicit)
	}
	candidates = append(candidates, names...)
	for _, dir := range desktopExtraToolDirs() {
		for _, name := range candidates {
			if name = strings.TrimSpace(name); name != "" && isBareToolName(name) {
				candidate := filepath.Join(dir, name)
				if fileExists(candidate) {
					return candidate
				}
			}
		}
	}
	return ""
}

func resolveDesktopToolPath(explicit string, names ...string) (string, error) {
	explicit = toolPathValue(explicit)
	if path := findDesktopToolPath(explicit, names...); path != "" {
		return path, nil
	}
	if explicit != "" && !isBareToolName(explicit) {
		if path := findDesktopToolPath("", names...); path != "" {
			return path, nil
		}
	}
	name := "工具"
	if len(names) > 0 {
		name = names[0]
	}
	message := fmt.Sprintf("找不到可执行的%s文件。请填写完整路径或安装后重试；已检查程序目录、PATH 和常见目录", name)
	if runtime.GOOS == "darwin" && strings.EqualFold(name, "ffmpeg") {
		message += "；可执行 brew install ffmpeg"
	}
	if explicit != "" && !isBareToolName(explicit) {
		message += "；显式路径不存在：" + explicit
	}
	return "", errors.New(message)
}

func resolveTaskToolPaths(task *desktopTask) error {
	if task == nil || task.Channel == "账号" || task.Mode == "仅查看" {
		return nil
	}
	if taskNeedsMuxTool(task.Args) {
		if boolArgEnabled(task.Args, "--use-mp4box") {
			explicit := effectiveValueFlag(task.Args, "--mp4box-path", task.MP4BoxPath)
			path, err := resolveDesktopToolPath(explicit, "mp4box", "MP4Box", "MP4box")
			if err != nil {
				return err
			}
			task.MP4BoxPath = path
			task.Args = upsertValueFlag(task.Args, "--mp4box-path", path)
		} else {
			explicit := effectiveValueFlag(task.Args, "--ffmpeg-path", task.FFmpegPath)
			path, err := resolveDesktopToolPath(explicit, "ffmpeg")
			if err != nil {
				if explicit == "" || isBareToolName(explicit) {
					path = "ffmpeg"
				} else {
					return err
				}
			}
			task.FFmpegPath = path
			task.Args = upsertValueFlag(task.Args, "--ffmpeg-path", path)
		}
	}
	if taskNeedsAria2cTool(task.Args) {
		explicit := effectiveValueFlag(task.Args, "--aria2c-path", task.Aria2cPath)
		path, err := resolveDesktopToolPath(explicit, "aria2c")
		if err != nil {
			return err
		}
		task.Aria2cPath = path
		task.Args = upsertValueFlag(task.Args, "--aria2c-path", path)
	}
	return nil
}

func desktopToolVersion(label, path string) string {
	arg := "-version"
	if strings.EqualFold(label, "aria2c") {
		arg = "--version"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, arg).CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

func checkTools(paths ToolPaths) []ToolDiagnostic {
	type check struct {
		name, input, flag, brew string
		names                   []string
	}
	checks := []check{{"FFmpeg", paths.FFmpegPath, "--ffmpeg-path", "ffmpeg", []string{"ffmpeg"}}, {"MP4Box", paths.MP4BoxPath, "--mp4box-path", "gpac", []string{"mp4box", "MP4Box", "MP4box"}}, {"aria2c", paths.Aria2cPath, "--aria2c-path", "aria2", []string{"aria2c"}}}
	result := make([]ToolDiagnostic, 0, len(checks))
	for _, item := range checks {
		path, err := resolveDesktopToolPath(item.input, item.names...)
		diagnostic := ToolDiagnostic{Name: item.name, Path: path}
		if err == nil {
			diagnostic.Found = true
			diagnostic.Version = desktopToolVersion(item.name, path)
			diagnostic.Message = "已找到"
		} else {
			diagnostic.Message = err.Error()
			if runtime.GOOS == "darwin" {
				diagnostic.InstallHint = "brew install " + item.brew
			} else {
				diagnostic.InstallHint = "请安装 " + item.name + "，或使用 " + item.flag + " 指定路径"
			}
		}
		result = append(result, diagnostic)
	}
	return result
}

func desktopDoctorJSON(ctx context.Context, paths ToolPaths) (string, error) {
	dir, helper, err := prepareRuntime()
	if err != nil {
		return "", err
	}
	args := []string{"doctor", "--json"}
	appendPath := func(flag, value string) {
		if value = toolPathValue(value); value != "" {
			args = append(args, flag, value)
		}
	}
	appendPath("--ffmpeg-path", paths.FFmpegPath)
	appendPath("--mp4box-path", paths.MP4BoxPath)
	appendPath("--aria2c-path", paths.Aria2cPath)
	command := exec.CommandContext(ctx, helper, args...)
	command.Dir = dir
	command.Env = append(desktopCommandEnv(), forcePlainTextEnv)
	out, err := command.CombinedOutput()
	if err != nil {
		if text := strings.TrimSpace(string(out)); text != "" {
			return "", fmt.Errorf("%w: %s", err, text)
		}
		return "", err
	}
	return string(out), nil
}

func openPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("路径不能为空")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", path)
	case "windows":
		command = exec.Command("cmd", "/C", "start", "", path)
	default:
		command = exec.Command("xdg-open", path)
	}
	return command.Start()
}

func revealPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("路径不能为空")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", "-R", path)
	case "windows":
		command = exec.Command("explorer", "/select,", path)
	default:
		command = exec.Command("xdg-open", filepath.Dir(path))
	}
	return command.Start()
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}
