package bbdown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const binaryToolDirsEnv = "BBDOWN_GO_TOOL_DIRS"

var binaryLoginShellPath = "/bin/zsh"

func ResolveRequiredBinaries(opt *MyOption) error {
	if requiresMuxBinary(opt) {
		if opt.UseMP4box {
			mp4boxPath := resolveBinaryPath(opt.Mp4boxPath, "mp4box", "MP4box", "MP4Box")
			if mp4boxPath == "" {
				return missingBinaryError("mp4box", "--mp4box-path", opt.Mp4boxPath)
			}
			opt.Mp4boxPath = mp4boxPath
		} else {
			if err := ResolveFFmpegBinary(opt); err != nil {
				return err
			}
		}
	}
	if requiresAria2cBinary(opt) {
		aria2cPath := resolveBinaryPath(opt.Aria2cPath, "aria2c")
		if aria2cPath == "" {
			return missingBinaryError("aria2c", "--aria2c-path", opt.Aria2cPath)
		}
		opt.Aria2cPath = aria2cPath
	}
	return nil
}

func ResolveFFmpegBinary(opt *MyOption) error {
	if opt == nil {
		opt = &MyOption{}
	}
	ffmpegPath := resolveBinaryPath(opt.FFmpegPath, "ffmpeg")
	if ffmpegPath == "" {
		return missingBinaryError("ffmpeg", "--ffmpeg-path", opt.FFmpegPath)
	}
	opt.FFmpegPath = ffmpegPath
	return nil
}

func requiresMuxBinary(opt *MyOption) bool {
	if opt == nil || opt.SkipMux {
		return false
	}
	// long 2026-06-27 22:06:00：单轨下载最终保留原始媒体文件，不进入音视频混流；缺失 ffmpeg 不应阻止用户先下载音频或视频分轨。
	return !opt.OnlyShowInfo && !opt.CoverOnly && !opt.DanmakuOnly && !opt.SubOnly && !opt.AudioOnly && !opt.VideoOnly
}

func requiresAria2cBinary(opt *MyOption) bool {
	return opt != nil && opt.UseAria2c && !opt.OnlyShowInfo
}

func FindBinaryPath(explicit string, names ...string) string {
	return resolveBinaryPath(explicit, names...)
}

func resolveBinaryPath(explicit string, names ...string) string {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		// long 2026-06-27 10:18:00：桌面端和命令行都可能把 "ffmpeg" 这类命令名传进来；裸命令名应按 PATH/常见工具目录解析，只有真正的路径写错时才直接报错。
		if fileExistsForBinary(explicit) {
			return explicit
		}
		if isBareBinaryName(explicit) {
			if path := searchBinaryNames(append([]string{explicit}, names...)...); path != "" {
				return path
			}
		}
		return ""
	}
	return searchBinaryNames(names...)
}

func searchBinaryNames(names ...string) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	configuredDirs := configuredBinaryDirs()
	searchDirs := append([]string{wd, AppDir()}, configuredDirs...)
	for _, dir := range searchDirs {
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if fileExistsForBinary(candidate) {
				return candidate
			}
		}
	}
	pathDirs := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	pathDirs = append(pathDirs, configuredDirs...)
	pathDirs = append(pathDirs, commonBinaryDirs()...)
	for _, name := range names {
		for _, dir := range pathDirs {
			if strings.TrimSpace(dir) == "" {
				continue
			}
			candidate := filepath.Join(dir, name)
			if fileExistsForBinary(candidate) {
				return candidate
			}
		}
	}
	if path := searchLoginShellBinaryNames(names...); path != "" {
		return path
	}
	return ""
}

func configuredBinaryDirs() []string {
	raw := strings.TrimSpace(os.Getenv(binaryToolDirsEnv))
	if raw == "" {
		return nil
	}
	dirs := make([]string, 0)
	seen := map[string]bool{}
	for _, dir := range strings.Split(raw, string(os.PathListSeparator)) {
		dir = strings.TrimSpace(dir)
		if dir == "" || seen[dir] {
			continue
		}
		// long 2026-06-27 23:10:00：桌面 helper 会被复制到用户配置目录；额外工具目录保留原 app 资源目录，避免 helper 换了 AppDir 后看不到随 app 放置的 ffmpeg。
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	return dirs
}

func isBareBinaryName(name string) bool {
	return !strings.ContainsAny(name, `/\`)
}

func searchLoginShellBinaryNames(names ...string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	shell := strings.TrimSpace(binaryLoginShellPath)
	if shell == "" || !fileExistsForBinary(shell) {
		return ""
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || !isBareBinaryName(name) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		out, err := exec.CommandContext(ctx, shell, "-lc", "command -v -- "+shellQuoteBinaryName(name)).Output()
		cancel()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			path := strings.TrimSpace(line)
			if path != "" && fileExistsForBinary(path) {
				return path
			}
		}
	}
	return ""
}

func shellQuoteBinaryName(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func missingBinaryError(name, flag, explicit string) error {
	parts := []string{
		fmt.Sprintf("找不到可执行的%s文件", name),
		fmt.Sprintf("请安装后重试，或使用 %s 指定完整路径", flag),
		"已检查当前目录、程序目录、PATH、常见目录：" + strings.Join(commonBinaryDirs(), "、"),
	}
	if runtime.GOOS == "darwin" {
		// long 2026-06-27 04:45:00：macOS 桌面 helper 报错时用户看不到 GUI 环境和终端 PATH 的差异，错误里直接说明 zsh 登录环境兜底是否参与排查。
		parts = append(parts, "macOS 下还会读取 zsh 登录环境中的 command -v 结果")
	}
	if value := strings.TrimSpace(explicit); value != "" && !isBareBinaryName(value) {
		parts = append(parts, "显式路径不存在："+value)
	}
	return errors.New(strings.Join(parts, "；"))
}

func commonBinaryDirs() []string {
	dirs := []string{}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin"))
	}
	switch runtime.GOOS {
	case "darwin":
		// long 2026-06-26 00:21:48：从 Finder 启动桌面 app 时不会继承 zsh 的 Homebrew PATH，混流依赖仍要能找到用户常规安装位置。
		dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin")
	case "linux":
		dirs = append(dirs, "/usr/local/bin", "/usr/bin", "/bin", "/snap/bin")
	case "windows":
		dirs = append(dirs, `C:\ffmpeg\bin`, `C:\Program Files\ffmpeg\bin`)
	}
	return dirs
}

func fileExistsForBinary(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
