package bbdown

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveRequiredBinariesUsesExplicitFFmpegPath(t *testing.T) {
	ffmpegPath := writeTestExecutable(t, "custom-ffmpeg")
	opt := MyOption{FFmpegPath: ffmpegPath}

	if err := ResolveRequiredBinaries(&opt); err != nil {
		t.Fatal(err)
	}
	if opt.FFmpegPath != ffmpegPath {
		t.Fatalf("FFmpegPath = %q, want %q", opt.FFmpegPath, ffmpegPath)
	}
}

func TestResolveRequiredBinariesAcceptsExplicitCommandName(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := writeTestExecutableInDir(t, dir, "ffmpeg")
	chdirForBinaryTest(t, t.TempDir())
	t.Setenv("PATH", dir)
	opt := MyOption{FFmpegPath: "ffmpeg"}

	if err := ResolveRequiredBinaries(&opt); err != nil {
		t.Fatal(err)
	}
	if opt.FFmpegPath != ffmpegPath {
		t.Fatalf("FFmpegPath = %q, want %q", opt.FFmpegPath, ffmpegPath)
	}
}

func TestResolveRequiredBinariesReportsMissingFFmpeg(t *testing.T) {
	chdirForBinaryTest(t, t.TempDir())
	t.Setenv("PATH", t.TempDir())
	missingPath := filepath.Join(t.TempDir(), "missing-ffmpeg")
	opt := MyOption{FFmpegPath: missingPath}

	err := ResolveRequiredBinaries(&opt)
	if err == nil || !strings.Contains(err.Error(), "找不到可执行的ffmpeg文件") {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"--ffmpeg-path", "已检查当前目录、程序目录、PATH、常见目录", missingPath} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, missing %q", err.Error(), want)
		}
	}
}

func TestResolveRequiredBinariesReportsMissingMP4Box(t *testing.T) {
	chdirForBinaryTest(t, t.TempDir())
	t.Setenv("PATH", t.TempDir())
	opt := MyOption{UseMP4box: true, Mp4boxPath: filepath.Join(t.TempDir(), "missing-mp4box")}

	err := ResolveRequiredBinaries(&opt)
	if err == nil || !strings.Contains(err.Error(), "找不到可执行的mp4box文件") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "--mp4box-path") {
		t.Fatalf("err = %q, should mention --mp4box-path", err.Error())
	}
}

func TestResolveRequiredBinariesReportsMissingAria2c(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	opt := MyOption{SkipMux: true, UseAria2c: true, Aria2cPath: filepath.Join(t.TempDir(), "missing-aria2c")}

	err := ResolveRequiredBinaries(&opt)
	if err == nil || !strings.Contains(err.Error(), "找不到可执行的aria2c文件") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "--aria2c-path") {
		t.Fatalf("err = %q, should mention --aria2c-path", err.Error())
	}
}

func TestResolveRequiredBinariesSkipsMuxToolForResourceOnlyModes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*MyOption)
	}{
		{name: "only show info", apply: func(opt *MyOption) { opt.OnlyShowInfo = true }},
		{name: "cover only", apply: func(opt *MyOption) { opt.CoverOnly = true }},
		{name: "danmaku only", apply: func(opt *MyOption) { opt.DanmakuOnly = true }},
		{name: "subtitle only", apply: func(opt *MyOption) { opt.SubOnly = true }},
		{name: "audio only", apply: func(opt *MyOption) { opt.AudioOnly = true }},
		{name: "video only", apply: func(opt *MyOption) { opt.VideoOnly = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opt := MyOption{FFmpegPath: filepath.Join(t.TempDir(), "missing-ffmpeg")}
			tc.apply(&opt)

			if err := ResolveRequiredBinaries(&opt); err != nil {
				t.Fatalf("task without mux output should not require mux tool: %v", err)
			}
		})
	}
}

func TestResolveRequiredBinariesResourceOnlyStillChecksAria2c(t *testing.T) {
	opt := MyOption{
		CoverOnly:  true,
		UseAria2c:  true,
		FFmpegPath: filepath.Join(t.TempDir(), "missing-ffmpeg"),
		Aria2cPath: filepath.Join(t.TempDir(), "missing-aria2c"),
	}

	err := ResolveRequiredBinaries(&opt)
	if err == nil || !strings.Contains(err.Error(), "找不到可执行的aria2c文件") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveRequiredBinariesOnlyShowInfoSkipsAria2c(t *testing.T) {
	opt := MyOption{
		OnlyShowInfo: true,
		UseAria2c:    true,
		FFmpegPath:   filepath.Join(t.TempDir(), "missing-ffmpeg"),
		Aria2cPath:   filepath.Join(t.TempDir(), "missing-aria2c"),
	}

	if err := ResolveRequiredBinaries(&opt); err != nil {
		t.Fatalf("only-show-info should not require download tools: %v", err)
	}
}

func TestResolveRequiredBinariesSkipMuxDefersMuxTool(t *testing.T) {
	opt := MyOption{
		SkipMux:    true,
		FFmpegPath: filepath.Join(t.TempDir(), "missing-ffmpeg"),
	}

	if err := ResolveRequiredBinaries(&opt); err != nil {
		t.Fatalf("skip-mux startup check should defer mux tool until track type is known: %v", err)
	}
}

func TestResolveFFmpegBinaryReportsMissingFLVMergeTool(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing-ffmpeg")
	opt := MyOption{SkipMux: true, FFmpegPath: missingPath}

	err := ResolveFFmpegBinary(&opt)
	if err == nil || !strings.Contains(err.Error(), "找不到可执行的ffmpeg文件") || !strings.Contains(err.Error(), "--ffmpeg-path") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveRequiredBinariesFindsUppercaseMP4Box(t *testing.T) {
	dir := t.TempDir()
	mp4boxPath := writeTestExecutableInDir(t, dir, "MP4box")
	t.Setenv("PATH", dir)
	opt := MyOption{UseMP4box: true}

	if err := ResolveRequiredBinaries(&opt); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(opt.Mp4boxPath, mp4boxPath) {
		t.Fatalf("Mp4boxPath = %q, want %q", opt.Mp4boxPath, mp4boxPath)
	}
}

func TestResolveRequiredBinariesFindsCurrentDirAsAbsolutePath(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	})
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	ffmpegPath := writeTestExecutableInDir(t, wd, "ffmpeg")
	t.Setenv("PATH", t.TempDir())
	opt := MyOption{}

	if err := ResolveRequiredBinaries(&opt); err != nil {
		t.Fatal(err)
	}
	if opt.FFmpegPath != ffmpegPath {
		t.Fatalf("FFmpegPath = %q, want %q", opt.FFmpegPath, ffmpegPath)
	}
	if !filepath.IsAbs(opt.FFmpegPath) {
		t.Fatalf("FFmpegPath = %q, want absolute path", opt.FFmpegPath)
	}
}

func TestResolveRequiredBinariesFindsCommonFFmpegDirWithoutShellPath(t *testing.T) {
	commonFFmpeg := ""
	for _, dir := range commonBinaryDirs() {
		candidate := filepath.Join(dir, "ffmpeg")
		if fileExistsForBinary(candidate) {
			commonFFmpeg = candidate
			break
		}
	}
	if commonFFmpeg == "" {
		t.Skip("ffmpeg not installed in common binary dirs")
	}
	chdirForBinaryTest(t, t.TempDir())
	t.Setenv("PATH", t.TempDir())
	opt := MyOption{}

	if err := ResolveRequiredBinaries(&opt); err != nil {
		t.Fatal(err)
	}
	if opt.FFmpegPath != commonFFmpeg {
		t.Fatalf("FFmpegPath = %q, want common dir path %q", opt.FFmpegPath, commonFFmpeg)
	}
}

func TestResolveBinaryPathFindsLoginShellToolPath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("login shell binary lookup is a macOS GUI fallback")
	}
	dir := t.TempDir()
	toolPath := writeTestExecutableInDir(t, dir, "custom-ffmpeg")
	shellPath := filepath.Join(t.TempDir(), "zsh")
	script := "#!/bin/sh\n" +
		"# long 2026-06-27 11:58:00：模拟 macOS GUI 启动时 PATH 过短，但用户登录 shell 仍能解析 Homebrew 或自定义工具目录。\n" +
		"case \"$2\" in\n" +
		"*custom-ffmpeg*) printf '%s\\n' " + shellQuoteForTest(toolPath) + " ;;\n" +
		"*) exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(shellPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	oldShell := binaryLoginShellPath
	binaryLoginShellPath = shellPath
	t.Cleanup(func() {
		binaryLoginShellPath = oldShell
	})
	chdirForBinaryTest(t, t.TempDir())
	t.Setenv("PATH", t.TempDir())

	if got := resolveBinaryPath("custom-ffmpeg"); got != toolPath {
		t.Fatalf("resolveBinaryPath = %q, want %q", got, toolPath)
	}
}

func TestResolveBinaryPathFindsConfiguredToolDirs(t *testing.T) {
	dir := t.TempDir()
	toolPath := writeTestExecutableInDir(t, dir, "custom-ffmpeg")
	chdirForBinaryTest(t, t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv(binaryToolDirsEnv, dir+string(os.PathListSeparator)+dir)

	if got := resolveBinaryPath("", "custom-ffmpeg"); got != toolPath {
		t.Fatalf("resolveBinaryPath = %q, want configured tool path %q", got, toolPath)
	}
}

func TestResolveRequiredBinariesMatchesOriginalPathExistenceCheck(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	// long: 原版 FindExecutable 只判断 File.Exists；这里保留同样行为，避免预检阶段和 C# 版出现不同的接受/报错时机。
	if err := os.WriteFile(ffmpegPath, []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	opt := MyOption{}

	if err := ResolveRequiredBinaries(&opt); err != nil {
		t.Fatal(err)
	}
	if opt.FFmpegPath != ffmpegPath {
		t.Fatalf("FFmpegPath = %q, want %q", opt.FFmpegPath, ffmpegPath)
	}
}

func writeTestExecutable(t *testing.T, name string) string {
	t.Helper()
	return writeTestExecutableInDir(t, t.TempDir(), name)
}

func writeTestExecutableInDir(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuoteForTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func chdirForBinaryTest(t *testing.T, dir string) {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	})
}
