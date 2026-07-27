package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/lonnnnnng/BB-DL/internal/bbdown"
)

func TestNormalizeBoolFlagValues(t *testing.T) {
	args := normalizeBoolFlagValues([]string{
		"--multi-thread", "false",
		"-mt", "true",
		"--file-pattern", "false",
		"BV1xx",
	})

	for _, want := range []string{"--multi-thread=false", "-mt=true", "--file-pattern", "false", "BV1xx"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args %v missing %q", args, want)
		}
	}
	if slices.Contains(args, "--multi-thread") || slices.Contains(args, "-mt") {
		t.Fatalf("bool flags should be folded with explicit values: %v", args)
	}
}

func TestMetaFlags(t *testing.T) {
	var opt bbdown.MyOption
	cfg := bbdown.NewConfig()
	fs := newDownloadFlagSet("bbdown", &opt, cfg)
	if err := fs.Parse([]string{"--version"}); err != nil {
		t.Fatal(err)
	}
	if !opt.Version {
		t.Fatal("--version should set Version")
	}

	opt = bbdown.MyOption{}
	fs = newDownloadFlagSet("bbdown", &opt, cfg)
	if err := fs.Parse([]string{"-?"}); err != nil {
		t.Fatal(err)
	}
	if !opt.Help {
		t.Fatal("-? should set Help")
	}

	for _, helpArg := range []string{"-h", "--help"} {
		opt = bbdown.MyOption{}
		fs = newDownloadFlagSet("bbdown", &opt, cfg)
		if err := fs.Parse([]string{helpArg}); err != nil {
			t.Fatal(err)
		}
		if !opt.Help {
			t.Fatalf("%s should set Help", helpArg)
		}
	}
}

func TestSingleURLArgReportsChineseErrors(t *testing.T) {
	var opt bbdown.MyOption
	cfg := bbdown.NewConfig()
	fs := newDownloadFlagSet("bbdown", &opt, cfg)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := singleURLArg(fs); err == nil || !strings.Contains(err.Error(), "缺少视频地址") {
		t.Fatalf("missing URL error = %v", err)
	}

	fs = newDownloadFlagSet("bbdown", &opt, cfg)
	if err := fs.Parse([]string{"BV1xx", "EXTRA"}); err != nil {
		t.Fatal(err)
	}
	if _, err := singleURLArg(fs); err == nil || !strings.Contains(err.Error(), "无法识别多余参数：EXTRA") {
		t.Fatalf("extra arg error = %v", err)
	}
}

func TestHelpCommandForFlagSetUsesCurrentCommandName(t *testing.T) {
	if got := helpCommandForFlagSet(newDownloadFlagSet("bbdown", &bbdown.MyOption{}, bbdown.NewConfig())); got != "BB-DL --help" {
		t.Fatalf("download help command = %q", got)
	}
	if got := helpCommandForFlagSet(newDownloadFlagSet("download", &bbdown.MyOption{}, bbdown.NewConfig())); got != "BB-DL download --help" {
		t.Fatalf("explicit download help command = %q", got)
	}
	if got := helpCommandForFlagSet(newInfoFlagSet("info", &bbdown.MyOption{}, bbdown.NewConfig())); got != "BB-DL info --help" {
		t.Fatalf("info help command = %q", got)
	}
}

func TestDownloadUsageLineMatchesCommandName(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "bbdown", want: "BB-DL [选项] <url>"},
		{name: "download", want: "BB-DL download [选项] <url>"},
		{name: "DOWN", want: "BB-DL down [选项] <url>"},
	} {
		if got := downloadUsageLine(tc.name); got != tc.want {
			t.Fatalf("downloadUsageLine(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDownloadFlagSetCoversOriginalBBDownFlags(t *testing.T) {
	// long: 这份清单固定原版 CommandLineInvoker.cs 暴露给用户的参数，避免后续整理 CLI 时不小心丢掉旧版兼容入口。
	originalFlags := []string{
		"F",
		"M",
		"access-token",
		"add-dfn-subfix",
		"allow-pcdn",
		"app",
		"area",
		"aria2",
		"aria2c-args",
		"aria2c-path",
		"aria2c-proxy",
		"audio-ascending",
		"audio-only",
		"av1",
		"avc",
		"bandwith-ascending",
		"c",
		"config-file",
		"cookie",
		"cover-only",
		"danmaku-only",
		"dd",
		"ddf",
		"debug",
		"delay-per-page",
		"dfn-priority",
		"download-danmaku",
		"download-danmaku-formats",
		"e",
		"encoding-priority",
		"ep-host",
		"ffmpeg-path",
		"file-pattern",
		"force-http",
		"force-replace-host",
		"hevc",
		"hide-streams",
		"host",
		"hs",
		"ia",
		"info",
		"interactive",
		"intl",
		"language",
		"mp4box-path",
		"mt",
		"multi-file-pattern",
		"multi-thread",
		"no-padding-page-num",
		"only-av1",
		"only-avc",
		"only-hevc",
		"only-show-info",
		"p",
		"q",
		"save-archives-to-file",
		"select-page",
		"show-all",
		"simply-mux",
		"skip-ai",
		"skip-cover",
		"skip-mux",
		"skip-subtitle",
		"sub-only",
		"token",
		"tv",
		"tv-host",
		"ua",
		"upos-host",
		"use-app-api",
		"use-aria2c",
		"use-intl-api",
		"use-mp4box",
		"use-tv-api",
		"user-agent",
		"video-ascending",
		"video-only",
		"work-dir",
	}

	var opt bbdown.MyOption
	fs := newDownloadFlagSet("bbdown", &opt, bbdown.NewConfig())
	registered := flagSetNames(fs)
	var missing []string
	for _, name := range originalFlags {
		if !registered[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing original BBDown flags: %v", missing)
	}

	extras := flagSetExtras(registered, originalFlags)
	wantExtras := []string{"?", "audio-index", "file-exists-action", "h", "help", "version", "video-index"}
	if !slices.Equal(extras, wantExtras) {
		t.Fatalf("Go-only download flags = %v, want %v", extras, wantExtras)
	}
}

func TestHelpTextUsesChineseDescriptions(t *testing.T) {
	var opt bbdown.MyOption
	fs := newDownloadFlagSet("bbdown", &opt, bbdown.NewConfig())
	var out strings.Builder
	fs.SetOutput(&out)
	fs.Usage()
	body := out.String()
	for _, want := range []string{
		"用法: BB-DL [选项] <url>",
		"交互式按序号选择视频流和音频流",
		"同名文件处理方式：rename 追加流水号、skip 跳过、overwrite 覆盖",
		"选择分 P，支持 ALL",
		"显示当前命令帮助",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("help text missing %q: %q", want, body)
		}
	}
}

func TestFileExistsActionFlagAndExistingFinalBehavior(t *testing.T) {
	var opt bbdown.MyOption
	fs := newDownloadFlagSet("bbdown", &opt, bbdown.NewConfig())
	if err := fs.Parse([]string{"--file-exists-action", "rename", "BV1xx"}); err != nil {
		t.Fatal(err)
	}
	normalizeOptions(&opt)
	if opt.FileExistsAction != bbdown.FileExistsActionRename {
		t.Fatalf("file exists action = %q", opt.FileExistsAction)
	}
	if err := validateDownloadStartupOptions(&opt); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "existing.mp4")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !shouldSkipExistingFinal(&bbdown.MyOption{FileExistsAction: bbdown.FileExistsActionSkip}, path) {
		t.Fatal("skip action should preserve the existing final file")
	}
	for _, action := range []string{bbdown.FileExistsActionRename, bbdown.FileExistsActionOverwrite} {
		if shouldSkipExistingFinal(&bbdown.MyOption{FileExistsAction: action}, path) {
			t.Fatalf("%s action should not use existing-final skip", action)
		}
	}
	if err := validateDownloadStartupOptions(&bbdown.MyOption{FileExistsAction: "invalid"}); err == nil {
		t.Fatal("invalid file exists action should be rejected")
	}
}

func TestExplicitDownloadHelpTextUsesSubcommandUsage(t *testing.T) {
	var opt bbdown.MyOption
	fs := newDownloadFlagSet("download", &opt, bbdown.NewConfig())
	var out strings.Builder
	fs.SetOutput(&out)
	fs.Usage()

	if !strings.Contains(out.String(), "用法: BB-DL download [选项] <url>") {
		t.Fatalf("download subcommand help should use explicit usage: %q", out.String())
	}
}

func TestServeHelpTextUsesChineseDescriptions(t *testing.T) {
	var listen string
	var help bool
	fs := newFlagSetWithUsage("serve", "BB-DL serve [选项]")
	fs.StringVar(&listen, "listen", "http://0.0.0.0:23333", "服务监听地址，支持 http://host:port 或 host:port")
	registerHelpFlags(fs, &help)
	var out strings.Builder
	fs.SetOutput(&out)
	fs.Usage()
	body := out.String()
	for _, want := range []string{
		"用法: BB-DL serve [选项]",
		"服务监听地址",
		"显示当前命令帮助",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("serve help text missing %q: %q", want, body)
		}
	}
}

func TestDoctorHelpTextUsesChineseDescriptions(t *testing.T) {
	var out strings.Builder
	if err := runDoctor(&out, []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{
		"用法: BB-DL doctor [选项]",
		"指定 ffmpeg 可执行文件路径或命令名",
		"以 JSON 格式输出诊断结果",
		"显示当前命令帮助",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("doctor help text missing %q: %q", want, body)
		}
	}
}

func TestRootMetaVersionCommand(t *testing.T) {
	var out strings.Builder
	handled, err := handleRootMeta(&out, []string{"version"})
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("version command should be handled before download parsing")
	}
	for _, want := range []string{
		"版本: " + bbdown.Version,
		"构建时间: " + bbdown.BuildTime,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("version output missing %q: %q", want, out.String())
		}
	}
}

func TestRootMetaCommandsAreCaseInsensitive(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"HELP"}, want: "BBDown 风格复刻版"},
		{args: []string{"Help", "LOGIN"}, want: "WEB 二维码登录"},
		{args: []string{"VERSION"}, want: "版本: " + bbdown.Version},
		{args: []string{"Version", "--help"}, want: "用法: BB-DL version"},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			var out strings.Builder
			handled, err := handleRootMeta(&out, tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if !handled {
				t.Fatalf("%v should be handled before download parsing", tc.args)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("%v output missing %q: %q", tc.args, tc.want, out.String())
			}
		})
	}
}

func TestRootCommandNameNormalizesSubcommands(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: " INFO ", want: "info"},
		{in: "LOGIN", want: "login"},
		{in: "LogInTV", want: "logintv"},
		{in: "DOCTOR", want: "doctor"},
		{in: "Serve", want: "serve"},
	} {
		if got := rootCommandName(tc.in); got != tc.want {
			t.Fatalf("rootCommandName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRootDownloadArgsSupportsExplicitSubcommand(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "bare url", in: []string{"BV1J9EB6xEAB"}, want: []string{"BV1J9EB6xEAB"}},
		{name: "download command", in: []string{"download", "--work-dir", "/tmp/out", "BV1J9EB6xEAB"}, want: []string{"--work-dir", "/tmp/out", "BV1J9EB6xEAB"}},
		{name: "down command", in: []string{"DOWN", "BV1J9EB6xEAB"}, want: []string{"BV1J9EB6xEAB"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rootDownloadArgs(tc.in)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("rootDownloadArgs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRootVersionCommandHandlesHelpFlags(t *testing.T) {
	for _, flag := range []string{"--help", "-h", "-?"} {
		t.Run(flag, func(t *testing.T) {
			var out strings.Builder
			handled, err := handleRootMeta(&out, []string{"version", flag})
			if err != nil {
				t.Fatal(err)
			}
			if !handled {
				t.Fatalf("version %s should be handled before download parsing", flag)
			}
			body := out.String()
			for _, want := range []string{"用法: BB-DL version", "显示当前版本和构建时间", "BB-DL --version"} {
				if !strings.Contains(body, want) {
					t.Fatalf("version %s missing %q: %q", flag, want, body)
				}
			}
		})
	}
}

func TestRootHelpCommandUsesChineseDescriptions(t *testing.T) {
	var out strings.Builder
	handled, err := handleRootMeta(&out, []string{"help"})
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("help command should be handled before download parsing")
	}
	body := out.String()
	for _, want := range []string{
		"BBDown 风格复刻版",
		"可用命令",
		"help [命令]",
		"BB-DL help login",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("root help missing %q: %q", want, body)
		}
	}
}

func TestRootHelpTopicUsesSubcommandUsage(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{topic: "download", want: "用法: BB-DL [选项] <url>"},
		{topic: "info", want: "用法: BB-DL info [选项] <url>"},
		{topic: "login", want: "WEB 二维码登录"},
		{topic: "logintv", want: "TV 二维码登录"},
		{topic: "doctor", want: "用法: BB-DL doctor [选项]"},
		{topic: "serve", want: "用法: BB-DL serve [选项]"},
		{topic: "version", want: "用法: BB-DL version"},
		{topic: "help", want: "用法: BB-DL help [命令]"},
		{topic: "bbdown", want: "BBDown 风格复刻版"},
	}
	for _, tc := range tests {
		t.Run(tc.topic, func(t *testing.T) {
			var out strings.Builder
			handled, err := handleRootMeta(&out, []string{"help", tc.topic})
			if err != nil {
				t.Fatal(err)
			}
			if !handled {
				t.Fatal("help topic should be handled before download parsing")
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("help %s missing %q: %q", tc.topic, tc.want, out.String())
			}
		})
	}
}

func TestRootHelpCommandHandlesHelpFlags(t *testing.T) {
	for _, flag := range []string{"--help", "-h", "-?"} {
		t.Run(flag, func(t *testing.T) {
			var out strings.Builder
			handled, err := handleRootMeta(&out, []string{"help", flag})
			if err != nil {
				t.Fatal(err)
			}
			if !handled {
				t.Fatalf("help %s should be handled before download parsing", flag)
			}
			body := out.String()
			for _, want := range []string{"用法: BB-DL help [命令]", "可用主题", "BB-DL help download"} {
				if !strings.Contains(body, want) {
					t.Fatalf("help %s missing %q: %q", flag, want, body)
				}
			}
		})
	}
}

func TestRootMetaRejectsUnexpectedArgs(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"version", "extra"}, want: "version 不支持额外参数"},
		{args: []string{"help", "login", "extra"}, want: "help 只支持一个命令名"},
		{args: []string{"help", "missing"}, want: "未知帮助主题"},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			var out strings.Builder
			handled, err := handleRootMeta(&out, tc.args)
			if !handled {
				t.Fatalf("%v should be handled before download parsing", tc.args)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoginHelpTextUsesChineseDescriptions(t *testing.T) {
	tests := []struct {
		command     string
		description string
		file        string
	}{
		{command: "login", description: "WEB 二维码登录", file: "BBDown.data"},
		{command: "logintv", description: "TV 二维码登录", file: "BBDownTV.data"},
	}
	for _, tc := range tests {
		t.Run(tc.command, func(t *testing.T) {
			var out strings.Builder
			handled, err := handleLoginMeta(&out, []string{"--help"}, tc.command, tc.description+"，扫码成功后保存 "+tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if !handled {
				t.Fatal("login help should be handled before starting QR login")
			}
			body := out.String()
			for _, want := range []string{
				"用法: BB-DL " + tc.command,
				tc.description,
				"保存 " + tc.file,
				"显示当前命令帮助",
			} {
				if !strings.Contains(body, want) {
					t.Fatalf("login help text missing %q: %q", want, body)
				}
			}
		})
	}
}

func TestLoginMetaRejectsUnexpectedArgs(t *testing.T) {
	var out strings.Builder
	handled, err := handleLoginMeta(&out, []string{"--cookie", "x"}, "login", "WEB 二维码登录，扫码成功后保存 BBDown.data")
	if !handled {
		t.Fatal("unexpected login args should be handled before starting QR login")
	}
	if err == nil || !strings.Contains(err.Error(), "login 不支持额外参数") {
		t.Fatalf("unexpected login arg error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected args should not print help by default: %q", out.String())
	}
}

func TestRunDoctorReportsToolsAndCredentialsWithoutSecrets(t *testing.T) {
	dir := t.TempDir()
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
	if err := os.WriteFile(filepath.Join(dir, "BBDown.data"), []byte("SESSDATA=secret-cookie"), 0o644); err != nil {
		t.Fatal(err)
	}
	credentialPath, err := filepath.Abs("BBDown.data")
	if err != nil {
		t.Fatal(err)
	}
	ffmpegPath := writeMainTestExecutable(t, dir, "ffmpeg")
	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nprintf '\\nffmpeg version text-test\\nextra\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	aria2cPath := writeMainTestExecutable(t, dir, "aria2c")

	var out strings.Builder
	err = runDoctor(&out, []string{
		"--ffmpeg-path", ffmpegPath,
		"--mp4box-path", filepath.Join(dir, "missing-MP4Box"),
		"--aria2c-path", aria2cPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{
		"BB-DL 诊断",
		"可执行文件: ",
		"平台: " + runtime.GOOS + "/" + runtime.GOARCH,
		"Go运行时: " + runtime.Version(),
		"WEB Cookie: 已找到",
		credentialPath,
		"ffmpeg: 已找到 " + ffmpegPath,
		"版本: ffmpeg version text-test",
		"MP4Box: 未找到",
		"当前指定值不可用: " + filepath.Join(dir, "missing-MP4Box"),
		"可使用 --mp4box-path 指定完整路径",
		"aria2c: 已找到 " + aria2cPath,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("doctor output missing %q: %q", want, body)
		}
	}
	if runtime.GOOS == "darwin" && !strings.Contains(body, "brew install gpac") {
		t.Fatalf("doctor output should mention gpac install on macOS: %q", body)
	}
	if strings.Contains(body, "secret-cookie") || strings.Contains(body, "SESSDATA=") {
		t.Fatalf("doctor output leaked credential content: %q", body)
	}
}

func TestRunDoctorJSONReportsDiagnosticsWithoutSecrets(t *testing.T) {
	dir := t.TempDir()
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
	if err := os.WriteFile(filepath.Join(dir, "BBDown.data"), []byte("SESSDATA=json-secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectedWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nprintf '\\nffmpeg version 8.1 test-build\\nextra\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := runDoctor(&out, []string{"--json", "--ffmpeg-path", ffmpegPath, "--mp4box-path", filepath.Join(dir, "missing-MP4Box")}); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	var report doctorReport
	if err := json.Unmarshal([]byte(body), &report); err != nil {
		t.Fatalf("doctor json should parse: %v\n%s", err, body)
	}
	if report.Version != bbdown.Version || report.WorkingDir != expectedWD {
		t.Fatalf("unexpected report metadata: %+v", report)
	}
	if report.Executable == "" || !filepath.IsAbs(report.Executable) {
		t.Fatalf("executable should be absolute: %+v", report)
	}
	if report.Runtime.GOOS != runtime.GOOS || report.Runtime.GOARCH != runtime.GOARCH || report.Runtime.GoVersion != runtime.Version() {
		t.Fatalf("runtime report = %+v", report.Runtime)
	}
	if len(report.Credentials) != 3 || !report.Credentials[0].Found || report.Credentials[0].Path == "" {
		t.Fatalf("credential report = %+v", report.Credentials)
	}
	if !filepath.IsAbs(report.Credentials[0].Path) {
		t.Fatalf("credential path should be absolute: %+v", report.Credentials[0])
	}
	if len(report.Tools) != 3 || !report.Tools[0].Found || report.Tools[0].Path != ffmpegPath || report.Tools[1].Found {
		t.Fatalf("tool report = %+v", report.Tools)
	}
	if report.Tools[0].Version != "ffmpeg version 8.1 test-build" {
		t.Fatalf("ffmpeg version = %q", report.Tools[0].Version)
	}
	if report.Tools[1].Flag != "--mp4box-path" || report.Tools[1].InstallHint == "" || report.Tools[1].Input != filepath.Join(dir, "missing-MP4Box") {
		t.Fatalf("missing MP4Box should report actionable hint: %+v", report.Tools[1])
	}
	if strings.Contains(body, "json-secret") || strings.Contains(body, "SESSDATA=") {
		t.Fatalf("doctor json leaked credential content: %q", body)
	}
}

func TestFirstNonEmptyLine(t *testing.T) {
	if got := firstNonEmptyLine("\n\n  tool version 1.2.3  \nnext"); got != "tool version 1.2.3" {
		t.Fatalf("firstNonEmptyLine = %q", got)
	}
	if got := firstNonEmptyLine("\n\t\n"); got != "" {
		t.Fatalf("empty firstNonEmptyLine = %q", got)
	}
}

func TestStartupBanner(t *testing.T) {
	out := captureStdout(t, printStartupBanner)
	for _, want := range []string{
		"版本: " + bbdown.Version,
		"构建时间: " + bbdown.BuildTime,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("startup banner missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "nilaoda/"+"BBDown/issues") || strings.Contains(out, "遇到"+"问题") {
		t.Fatalf("startup banner should not point users to upstream issue tracker: %q", out)
	}
}

func TestPrintVersionInfoIncludesInjectedBuildTime(t *testing.T) {
	oldBuildTime := bbdown.BuildTime
	bbdown.BuildTime = "2026-06-20T09:00:00Z"
	defer func() { bbdown.BuildTime = oldBuildTime }()

	out := captureStdout(t, printVersionInfo)
	for _, want := range []string{
		"版本: " + bbdown.Version,
		"构建时间: 2026-06-20T09:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("version info missing %q: %q", want, out)
		}
	}
}

func writeMainTestExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func flagSetNames(fs *flag.FlagSet) map[string]bool {
	names := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		names[f.Name] = true
	})
	return names
}

func flagSetExtras(registered map[string]bool, baseline []string) []string {
	base := map[string]bool{}
	for _, name := range baseline {
		base[name] = true
	}
	var extras []string
	for name := range registered {
		if !base[name] {
			extras = append(extras, name)
		}
	}
	slices.Sort(extras)
	return extras
}

func TestNormalizeFlagOrderMovesOptionsAfterURL(t *testing.T) {
	args := normalizeFlagOrder(normalizeBoolFlagValues([]string{
		"BV1xx",
		"-p", "1",
		"--multi-thread", "false",
		"--file-pattern", "<videoTitle>",
		"--file-exists-action", "skip",
	}))

	var opt bbdown.MyOption
	fs := newDownloadFlagSet("bbdown", &opt, bbdown.NewConfig())
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "BV1xx" {
		t.Fatalf("positional args = %v", fs.Args())
	}
	if opt.SelectPage != "1" || opt.MultiThread || opt.FilePattern != "<videoTitle>" || opt.FileExistsAction != bbdown.FileExistsActionSkip {
		t.Fatalf("options not parsed after reordering: %+v", opt)
	}
}

func TestNormalizeFlagOrderKeepsExtraPositionals(t *testing.T) {
	args := normalizeFlagOrder([]string{"BV1xx", "EXTRA", "-p", "1"})
	var opt bbdown.MyOption
	fs := newDownloadFlagSet("bbdown", &opt, bbdown.NewConfig())
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	if got := fs.Args(); !slices.Equal(got, []string{"BV1xx", "EXTRA"}) {
		t.Fatalf("positionals = %v", got)
	}
}

func TestPrefersEncodingPriorityFirst(t *testing.T) {
	if !prefersEncodingPriorityFirst([]string{"--encoding-priority", "hevc", "--dfn-priority", "1080P"}) {
		t.Fatal("long encoding-priority before dfn-priority should prefer encoding")
	}
	if prefersEncodingPriorityFirst([]string{"-e", "hevc", "-q", "1080P"}) {
		t.Fatal("short aliases should match original Environment.CommandLine long-name check and keep dfn first")
	}
	if prefersEncodingPriorityFirst([]string{"--dfn-priority", "1080P", "--encoding-priority", "hevc"}) {
		t.Fatal("dfn-priority before encoding-priority should keep dfn first")
	}
	if prefersEncodingPriorityFirst([]string{"--encoding-priority", "hevc"}) {
		t.Fatal("single priority option should keep default dfn first")
	}
}

func TestNormalizeOptionsAria2cProxy(t *testing.T) {
	opt := bbdown.MyOption{Aria2cArgs: "-x 4", Aria2cProxy: "http://127.0.0.1:7890"}
	normalizeOptions(&opt)
	if !strings.Contains(opt.Aria2cArgs, "-x 4") || !strings.Contains(opt.Aria2cArgs, `--all-proxy="http://127.0.0.1:7890"`) {
		t.Fatalf("Aria2cArgs = %q", opt.Aria2cArgs)
	}
}

func TestNormalizeOptionsAddDfnSubfix(t *testing.T) {
	opt := bbdown.MyOption{AddDfnSubfix: true}
	normalizeOptions(&opt)
	if opt.FilePattern != "<videoTitle>[<dfn>]" {
		t.Fatalf("FilePattern = %q", opt.FilePattern)
	}
	if opt.MultiFilePattern != "<videoTitle>/[P<pageNumberWithZero>]<pageTitle>[<dfn>]" {
		t.Fatalf("MultiFilePattern = %q", opt.MultiFilePattern)
	}

	video := &bbdown.Video{Dfn: "1080P 高码率"}
	page := bbdown.Page{Index: 1, Title: "P1", Aid: "80433022", Cid: "137649199"}
	got := bbdown.FormatSavePath(savePathPattern(&opt, 1), "标题", video, nil, page, 1, "WEB", 0)
	if got != "标题[1080P 高码率].mp4" {
		t.Fatalf("single page save path = %q", got)
	}
}

func TestNormalizeOptionsAddDfnSubfixWithNoPadding(t *testing.T) {
	opt := bbdown.MyOption{AddDfnSubfix: true, NoPaddingPageNum: true}
	normalizeOptions(&opt)
	if opt.MultiFilePattern != "<videoTitle>/[P<pageNumber>]<pageTitle>[<dfn>]" {
		t.Fatalf("MultiFilePattern = %q", opt.MultiFilePattern)
	}

	video := &bbdown.Video{Dfn: "720P 高清"}
	page := bbdown.Page{Index: 2, Title: "第二集", Aid: "80433022", Cid: "137649199"}
	got := bbdown.FormatSavePath(savePathPattern(&opt, 12), "标题", video, nil, page, 12, "WEB", 0)
	if got != "标题/[P2]第二集[720P 高清].mp4" {
		t.Fatalf("multi page save path = %q", got)
	}
}

func TestNormalizeOptionsAddDfnSubfixDoesNotOverrideCustomPattern(t *testing.T) {
	opt := bbdown.MyOption{AddDfnSubfix: true, FilePattern: "custom"}
	normalizeOptions(&opt)
	if opt.FilePattern != "custom" || opt.MultiFilePattern != "" {
		t.Fatalf("patterns = %q %q", opt.FilePattern, opt.MultiFilePattern)
	}
}

func TestSavePathPatternKeepsWhitespaceMultiFilePatternLikeOriginal(t *testing.T) {
	opt := bbdown.MyOption{FilePattern: "<videoTitle>", MultiFilePattern: "   "}
	if got := savePathPattern(&opt, 2); got != "   " {
		t.Fatalf("savePathPattern = %q, want explicit whitespace multi-file pattern", got)
	}
}

func TestSavePathPatternForVideoKeepsMultiDefaultAfterSinglePageSelection(t *testing.T) {
	opt := bbdown.MyOption{}
	vInfo := &bbdown.VInfo{Title: "标题"}
	page := bbdown.Page{Index: 2, Title: "第二集", Aid: "80433022", Cid: "137649199"}

	pattern := savePathPatternForVideo(&opt, vInfo, 12)
	got := bbdown.FormatSavePath(pattern, vInfo.Title, nil, nil, page, 1, "WEB", 0)
	if got != "标题/[P2]第二集.mp4" {
		t.Fatalf("multi-page selected path = %q", got)
	}
}

func TestSavePathPatternForVideoUsesMultiDefaultForUnfinishedBangumi(t *testing.T) {
	opt := bbdown.MyOption{}
	vInfo := &bbdown.VInfo{Title: "番剧", IsBangumi: true, IsBangumiEnd: false}
	page := bbdown.Page{Index: 1, Title: "第一集", Aid: "1", Cid: "2"}

	pattern := savePathPatternForVideo(&opt, vInfo, 1)
	got := bbdown.FormatSavePath(pattern, vInfo.Title, nil, nil, page, 1, "WEB", 0)
	if got != "番剧/[P1]第一集.mp4" {
		t.Fatalf("unfinished bangumi path = %q", got)
	}
}

func TestSelectedPagesText(t *testing.T) {
	if got := selectedPagesText(nil); got != "ALL" {
		t.Fatalf("nil selected text = %q", got)
	}
	if got := selectedPagesText([]string{"1", "3", "12"}); got != "1,3,12" {
		t.Fatalf("selected text = %q", got)
	}
}

func TestSubtitleMuxSuffix(t *testing.T) {
	if got := subtitleMuxSuffix(nil); got != "" {
		t.Fatalf("empty subtitle suffix = %q", got)
	}
	if got := subtitleMuxSuffix([]bbdown.Subtitle{{Path: "a.srt"}}); got != "和字幕" {
		t.Fatalf("subtitle suffix = %q", got)
	}
}

func TestBVIDFromAidRejectsNonIntegerAidLikeOriginal(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("bvidFromAid should panic when aid is not a strict integer")
		}
	}()

	_ = bvidFromAid("80433022abc")
}

func TestOriginalChapterPointsPrefersFetchedPoints(t *testing.T) {
	fetched := []bbdown.ViewPoint{{Title: "接口章节", Start: 1, End: 2}}
	extra := []bbdown.ViewPoint{{Title: "播放解析章节", Start: 3, End: 4}}

	got := originalChapterPoints(fetched, extra)
	if len(got) != 1 || got[0].Title != "接口章节" {
		t.Fatalf("chapter points = %+v, want fetched points first", got)
	}

	got = originalChapterPoints(nil, extra)
	if len(got) != 1 || got[0].Title != "播放解析章节" {
		t.Fatalf("chapter points fallback = %+v, want extra points", got)
	}
}

func TestSidecarWorkBeforeExistingMediaSkipMatchesOriginalOrder(t *testing.T) {
	cases := []struct {
		name string
		opt  bbdown.MyOption
		want bool
	}{
		{name: "default subtitle sidecar", opt: bbdown.MyOption{}, want: true},
		{name: "skip subtitle no sidecar", opt: bbdown.MyOption{SkipSubtitle: true}, want: false},
		{name: "download danmaku", opt: bbdown.MyOption{SkipSubtitle: true, DownloadDanmaku: true}, want: true},
		{name: "danmaku only", opt: bbdown.MyOption{SkipSubtitle: true, DanmakuOnly: true}, want: true},
		{name: "cover only", opt: bbdown.MyOption{SkipSubtitle: true, CoverOnly: true}, want: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := sidecarWorkBeforeExistingMediaSkip(&tt.opt); got != tt.want {
				t.Fatalf("sidecarWorkBeforeExistingMediaSkip(%+v) = %v, want %v", tt.opt, got, tt.want)
			}
		})
	}
}

func TestFileExistsNonEmptyMatchesOriginalMuxOutputCheck(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.mp4")
	if fileExistsNonEmpty(missing) {
		t.Fatal("missing mux output should be treated as merge failure")
	}

	empty := filepath.Join(dir, "empty.mp4")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if fileExistsNonEmpty(empty) {
		t.Fatal("empty mux output should be treated as merge failure")
	}

	full := filepath.Join(dir, "full.mp4")
	if err := os.WriteFile(full, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileExistsNonEmpty(full) {
		t.Fatal("non-empty mux output should be accepted")
	}
}

func TestDownloadCoverForMuxKeepsEmptyExistingCoverLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "video.mp4")
	coverPath := strings.TrimSuffix(savePath, ".mp4") + ".jpg"
	if err := os.WriteFile(coverPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got := downloadCoverForMux(
		context.Background(),
		nil,
		nil,
		&bbdown.MyOption{},
		&bbdown.VInfo{Pic: "https://example.test/cover.jpg"},
		bbdown.Page{},
		savePath,
	)
	if got != coverPath {
		t.Fatalf("existing empty cover path = %q, want %q", got, coverPath)
	}
}

func TestLogDebugSavePath(t *testing.T) {
	quiet := captureStdout(t, func() {
		logDebugSavePath(&bbdown.Config{}, "<videoTitle>", "title.mp4")
	})
	if quiet != "" {
		t.Fatalf("debug disabled output = %q", quiet)
	}

	out := captureStdout(t, func() {
		logDebugSavePath(&bbdown.Config{Debug: true}, "<videoTitle>", "title.mp4")
	})
	if !strings.Contains(out, "Format Before: <videoTitle>") || !strings.Contains(out, "Format After: title.mp4") {
		t.Fatalf("debug output = %q", out)
	}
}

func TestApplyWorkDirDebugOutput(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	quietDir := filepath.Join(t.TempDir(), "quiet")
	quiet := captureStdout(t, func() {
		if err := applyWorkDir(&bbdown.MyOption{WorkDir: quietDir}); err != nil {
			t.Fatal(err)
		}
	})
	if quiet != "" {
		t.Fatalf("debug disabled workdir output = %q", quiet)
	}

	debugDir := filepath.Join(t.TempDir(), "debug")
	out := captureStdout(t, func() {
		if err := applyWorkDir(&bbdown.MyOption{WorkDir: debugDir, Debug: true}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "切换工作目录至："+debugDir) {
		t.Fatalf("debug workdir output = %q", out)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	realWd, err := filepath.EvalSymlinks(wd)
	if err != nil {
		t.Fatal(err)
	}
	realWant, err := filepath.EvalSymlinks(debugDir)
	if err != nil {
		t.Fatal(err)
	}
	if realWd != realWant {
		t.Fatalf("working dir = %q, want %q", realWd, realWant)
	}
}

func TestLogDebugSetup(t *testing.T) {
	quiet := captureStdout(t, func() {
		logDebugSetup(&bbdown.Config{}, &bbdown.MyOption{Url: "BV1xx"})
	})
	if quiet != "" {
		t.Fatalf("debug disabled setup output = %q", quiet)
	}

	out := captureStdout(t, func() {
		logDebugSetup(&bbdown.Config{Debug: true}, &bbdown.MyOption{Url: "BV1xx"})
	})
	if !strings.Contains(out, "AppDirectory: ") || !strings.Contains(out, "运行参数：") || !strings.Contains(out, `"Url":"BV1xx"`) {
		t.Fatalf("debug setup output = %q", out)
	}
}

func TestLogDebugSubtitleFetch(t *testing.T) {
	quiet := captureStdout(t, func() {
		logDebugSubtitleFetch(&bbdown.Config{})
	})
	if quiet != "" {
		t.Fatalf("debug disabled subtitle output = %q", quiet)
	}

	out := captureStdout(t, func() {
		logDebugSubtitleFetch(&bbdown.Config{Debug: true})
	})
	if !strings.Contains(out, "获取字幕...") {
		t.Fatalf("debug subtitle output = %q", out)
	}
}

func TestLogDebugSubtitleDownload(t *testing.T) {
	quiet := captureStdout(t, func() {
		logDebugSubtitleDownload(&bbdown.Config{}, "https://example.test/subtitle.json")
	})
	if quiet != "" {
		t.Fatalf("debug disabled subtitle download output = %q", quiet)
	}

	out := captureStdout(t, func() {
		logDebugSubtitleDownload(&bbdown.Config{Debug: true}, "https://example.test/subtitle.json")
	})
	if !strings.Contains(out, "下载：https://example.test/subtitle.json") {
		t.Fatalf("debug subtitle download output = %q", out)
	}
}

func TestCoverExtKeepsActualURLExtension(t *testing.T) {
	cases := map[string]string{
		"https://i0.hdslb.com/bfs/archive/a.jpg@672w_378h_1c.webp": ".webp",
		"https://i0.hdslb.com/bfs/archive/a.PNG?x=1":               ".png",
		"https://i0.hdslb.com/bfs/archive/no-extension":            ".jpg",
	}
	for input, want := range cases {
		if got := bbdown.CoverExt(input); got != want {
			t.Fatalf("CoverExt(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFinalizeDanmakuFilesRemovesInvalidXML(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "bad.xml")
	assPath := filepath.Join(dir, "bad.ass")
	if err := os.WriteFile(xmlPath, []byte("<i>"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := finalizeDanmakuFiles(xmlPath, assPath, map[bbdown.DanmakuFormat]bool{bbdown.DanmakuXML: true, bbdown.DanmakuASS: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(xmlPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid xml should be removed, stat err = %v", err)
	}
}

func TestFinalizeDanmakuFilesRemovesEmptyXML(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "empty.xml")
	assPath := filepath.Join(dir, "empty.ass")
	if err := os.WriteFile(xmlPath, []byte("<i></i>"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := finalizeDanmakuFiles(xmlPath, assPath, map[bbdown.DanmakuFormat]bool{bbdown.DanmakuXML: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(xmlPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty xml should be removed, stat err = %v", err)
	}
}

func TestFinalizeDanmakuFilesCreatesASSAndKeepsXML(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "danmaku.xml")
	assPath := filepath.Join(dir, "danmaku.ass")
	body := `<i><d p="1.5,1,25,16777215,0,0,0,0">hello</d></i>`
	if err := os.WriteFile(xmlPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := finalizeDanmakuFiles(xmlPath, assPath, map[bbdown.DanmakuFormat]bool{bbdown.DanmakuXML: true, bbdown.DanmakuASS: true}, nil); err != nil {
		t.Fatal(err)
	}
	if !fileExistsNonEmpty(xmlPath) {
		t.Fatal("xml should be kept when xml format is requested")
	}
	if !fileExistsNonEmpty(assPath) {
		t.Fatal("ass should be generated when ass format is requested")
	}
}

func TestFinalizeDanmakuFilesSkipKeepsExistingASS(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "danmaku.xml")
	assPath := filepath.Join(dir, "danmaku.ass")
	if err := os.WriteFile(xmlPath, []byte(`<i><d p="1.5,1,25,16777215,0,0,0,0">new</d></i>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assPath, []byte("existing-ass"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := finalizeDanmakuFiles(xmlPath, assPath, map[bbdown.DanmakuFormat]bool{bbdown.DanmakuXML: true, bbdown.DanmakuASS: true}, &bbdown.MyOption{FileExistsAction: bbdown.FileExistsActionSkip}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(assPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "existing-ass" {
		t.Fatalf("skip action overwrote existing ASS: %q", body)
	}
}

func TestFinalizeDanmakuFilesDeletesXMLForExplicitEmptyFormatList(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "danmaku.xml")
	assPath := filepath.Join(dir, "danmaku.ass")
	body := `<i><d p="1.5,1,25,16777215,0,0,0,0">hello</d></i>`
	if err := os.WriteFile(xmlPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := finalizeDanmakuFiles(xmlPath, assPath, bbdown.ParseDanmakuFormats(" ，,, "), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(xmlPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("xml should be removed when the explicit format list is empty like original, stat err = %v", err)
	}
	if _, err := os.Stat(assPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ass should not be generated when the explicit format list is empty, stat err = %v", err)
	}
}

func TestApplyOnlyModeToTracksAudioOnlyClearsVideoTracks(t *testing.T) {
	opt := bbdown.MyOption{AudioOnly: true}
	tracks := &bbdown.ParsedTracks{
		VideoTracks: []bbdown.Video{{Dfn: "1080P"}},
		AudioTracks: []bbdown.Audio{{Codecs: "M4A"}},
	}
	applyOnlyModeToTracks(&opt, tracks)
	if len(tracks.VideoTracks) != 0 {
		t.Fatalf("audio-only should clear video tracks: %+v", tracks.VideoTracks)
	}
	if len(tracks.AudioTracks) != 1 {
		t.Fatalf("audio-only should keep audio tracks: %+v", tracks.AudioTracks)
	}
}

func TestApplyOnlyModeToTracksVideoOnlyClearsAudioTracks(t *testing.T) {
	opt := bbdown.MyOption{VideoOnly: true}
	tracks := &bbdown.ParsedTracks{
		VideoTracks:      []bbdown.Video{{Dfn: "1080P"}},
		AudioTracks:      []bbdown.Audio{{Codecs: "M4A"}},
		BackgroundAudios: []bbdown.Audio{{Codecs: "EAC3"}},
		RoleAudioLists:   []bbdown.AudioMaterialInfo{{Title: "角色音频", Audio: []bbdown.Audio{{Codecs: "M4A"}}}},
	}
	applyOnlyModeToTracks(&opt, tracks)
	if len(tracks.VideoTracks) != 1 {
		t.Fatalf("video-only should keep video tracks: %+v", tracks.VideoTracks)
	}
	if len(tracks.AudioTracks) != 0 {
		t.Fatalf("video-only should clear audio tracks: %+v", tracks.AudioTracks)
	}
	if len(tracks.BackgroundAudios) != 0 || len(tracks.RoleAudioLists) != 0 {
		t.Fatalf("video-only should clear extra audio tracks: %+v", tracks)
	}
}

func TestApplyOnlyModeToTracksKeepsFLVClips(t *testing.T) {
	opt := bbdown.MyOption{AudioOnly: true}
	tracks := &bbdown.ParsedTracks{
		VideoTracks: []bbdown.Video{{Dfn: "720P"}},
		Clips:       []string{"https://example.test/clip.flv"},
	}
	applyOnlyModeToTracks(&opt, tracks)
	if len(tracks.VideoTracks) != 1 || len(tracks.Clips) != 1 {
		t.Fatalf("FLV clips should not be changed by DASH-only mode filter: %+v", tracks)
	}
}

func TestShouldSkipMissingOnlyMode(t *testing.T) {
	if !shouldSkipMissingOnlyMode(&bbdown.MyOption{VideoOnly: true}, &bbdown.ParsedTracks{AudioTracks: []bbdown.Audio{{Codecs: "M4A"}}}) {
		t.Fatal("video-only with no video tracks should skip page")
	}
	if !shouldSkipMissingOnlyMode(&bbdown.MyOption{AudioOnly: true}, &bbdown.ParsedTracks{VideoTracks: []bbdown.Video{{Dfn: "720P"}}}) {
		t.Fatal("audio-only with no audio tracks should skip page")
	}
	if shouldSkipMissingOnlyMode(&bbdown.MyOption{}, &bbdown.ParsedTracks{}) {
		t.Fatal("normal mode should report parse failure instead of missing-only skip")
	}
	if shouldSkipMissingOnlyMode(&bbdown.MyOption{AudioOnly: true}, &bbdown.ParsedTracks{Clips: []string{"clip"}}) {
		t.Fatal("FLV clips should keep their own handling")
	}
}

func TestEnsureFLVMergeBinarySkipsSingleClip(t *testing.T) {
	opt := bbdown.MyOption{FFmpegPath: filepath.Join(t.TempDir(), "missing-ffmpeg")}
	if err := ensureFLVMergeBinary(&opt, 1); err != nil {
		t.Fatalf("single FLV clip should not require ffmpeg: %v", err)
	}
}

func TestEnsureFLVMergeBinaryRequiresFFmpegForMultipleClips(t *testing.T) {
	opt := bbdown.MyOption{FFmpegPath: filepath.Join(t.TempDir(), "missing-ffmpeg")}
	err := ensureFLVMergeBinary(&opt, 2)
	if err == nil || !strings.Contains(err.Error(), "找不到可执行的ffmpeg文件") {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalizeOptionsInteractiveShowsStreams(t *testing.T) {
	opt := bbdown.MyOption{Interactive: true, HideStreams: true}
	normalizeOptions(&opt)
	if opt.HideStreams {
		t.Fatal("interactive mode should force HideStreams=false")
	}
}

func TestPrintManualSelectionHintForMultipleTracks(t *testing.T) {
	out := captureStdout(t, func() {
		printManualSelectionHint(&bbdown.MyOption{}, &bbdown.ParsedTracks{
			VideoTracks: []bbdown.Video{{Dfn: "480P"}, {Dfn: "360P"}},
			AudioTracks: []bbdown.Audio{{ID: "0"}},
		})
	})
	for _, want := range []string{"--video-index", "--audio-index", "--interactive", "-ia"} {
		if !strings.Contains(out, want) {
			t.Fatalf("manual selection hint should mention %s, got %q", want, out)
		}
	}

	suppressed := captureStdout(t, func() {
		printManualSelectionHint(&bbdown.MyOption{Interactive: true}, &bbdown.ParsedTracks{
			VideoTracks: []bbdown.Video{{Dfn: "480P"}, {Dfn: "360P"}},
		})
	})
	if suppressed != "" {
		t.Fatalf("interactive mode should not print hint, got %q", suppressed)
	}
}

func TestExplicitTrackIndexes(t *testing.T) {
	opt := &bbdown.MyOption{VideoIndex: "1", AudioIndex: "2"}
	tracks := &bbdown.ParsedTracks{
		VideoTracks: []bbdown.Video{{Dfn: "360P"}, {Dfn: "480P"}},
		AudioTracks: []bbdown.Audio{{ID: "0"}, {ID: "1"}, {ID: "2"}},
	}
	videoIndex := 0
	audioIndex := 0
	videoIndex, audioIndex, err := bbdown.SelectedTrackIndexes(opt, tracks)
	if err != nil {
		t.Fatalf("SelectedTrackIndexes returned error: %v", err)
	}
	if videoIndex != 1 || audioIndex != 2 {
		t.Fatalf("indexes = video %d audio %d, want 1 and 2", videoIndex, audioIndex)
	}

	if _, _, err := bbdown.SelectedTrackIndexes(&bbdown.MyOption{VideoIndex: "9"}, tracks); err == nil || !strings.Contains(err.Error(), "视频流序号超出范围") {
		t.Fatalf("out-of-range video index should fail clearly, got %v", err)
	}
	if _, _, err := bbdown.SelectedTrackIndexes(&bbdown.MyOption{AudioIndex: "abc"}, tracks); err == nil || !strings.Contains(err.Error(), "音频流序号无效") {
		t.Fatalf("invalid audio index should fail clearly, got %v", err)
	}
}

func TestValidateDownloadStartupOptionsRejectsInvalidTrackIndex(t *testing.T) {
	if err := validateDownloadStartupOptions(&bbdown.MyOption{VideoIndex: "bad"}); err == nil || !strings.Contains(err.Error(), "视频流序号无效") {
		t.Fatalf("invalid video index should fail before network work, got %v", err)
	}
	if err := validateDownloadStartupOptions(&bbdown.MyOption{AudioOnly: true, VideoIndex: "bad", AudioIndex: "0"}); err != nil {
		t.Fatalf("audio-only should ignore video index at startup: %v", err)
	}
	opt := bbdown.MyOption{AudioOnly: true, VideoOnly: true, VideoIndex: "bad", AudioIndex: "0"}
	normalizeOptions(&opt)
	bbdown.NormalizeTrackIndexOptions(&opt)
	if err := validateDownloadStartupOptions(&opt); err == nil || !strings.Contains(err.Error(), "视频流序号无效") {
		t.Fatalf("audio-only + video-only normalize to normal download, bad video index should fail: %v", err)
	}
}

func TestNormalizeInteractiveIndexMatchesOriginalBoundary(t *testing.T) {
	if got := normalizeInteractiveIndexLikeOriginal(-1, 3); got != 0 {
		t.Fatalf("negative index = %d, want 0", got)
	}
	if got := normalizeInteractiveIndexLikeOriginal(4, 3); got != 0 {
		t.Fatalf("index greater than count = %d, want 0", got)
	}
	if got := normalizeInteractiveIndexLikeOriginal(3, 3); got != 3 {
		t.Fatalf("index equal count should be preserved like original, got %d", got)
	}
}

func TestTrackAtOriginalIndexReturnsNilForEqualCount(t *testing.T) {
	videos := []bbdown.Video{{Dfn: "720P"}, {Dfn: "480P"}}
	if got := videoTrackAtOriginalIndex(videos, 2); got != nil {
		t.Fatalf("video index equal count should mirror ElementAtOrDefault nil, got %+v", got)
	}
	if got := videoTrackAtOriginalIndex(videos, 1); got == nil || got.Dfn != "480P" {
		t.Fatalf("video index 1 = %+v", got)
	}

	audios := []bbdown.Audio{{ID: "0"}, {ID: "1"}}
	if got := audioTrackAtOriginalIndex(audios, 2); got != nil {
		t.Fatalf("audio index equal count should mirror ElementAtOrDefault nil, got %+v", got)
	}
	if got := audioTrackAtOriginalIndex(audios, 0); got == nil || got.ID != "0" {
		t.Fatalf("audio index 0 = %+v", got)
	}
}

func TestFLVQualityByIndex(t *testing.T) {
	dfns := []string{"80", "64", "32"}
	if got := flvQualityByIndex(dfns, 1); got != "64" {
		t.Fatalf("flvQualityByIndex = %q, want 64", got)
	}
	if got := flvQualityByIndex(dfns, len(dfns)); got != "" {
		t.Fatalf("index equal count should not fall back to first qn, got %q", got)
	}
	if got := flvQualityByIndex(dfns, -1); got != "80" {
		t.Fatalf("negative index should fall back to first qn, got %q", got)
	}
	if got := flvQualityByIndex(dfns, 99); got != "80" {
		t.Fatalf("out-of-range index should fall back to first qn, got %q", got)
	}
	if got := flvQualityByIndex(nil, 0); got != "" {
		t.Fatalf("empty dfns should return empty qn, got %q", got)
	}
}

func TestNormalizeOptionsConflictModes(t *testing.T) {
	opt := bbdown.MyOption{AudioOnly: true, VideoOnly: true, SkipSubtitle: true, SubOnly: true}
	normalizeOptions(&opt)
	if opt.AudioOnly || opt.VideoOnly || opt.SubOnly {
		t.Fatalf("conflict options not normalized: %+v", opt)
	}
}

func TestShouldRetryPageDownloadRetriesTwice(t *testing.T) {
	oldDelay := pageRetryDelay
	pageRetryDelay = 0
	defer func() { pageRetryDelay = oldDelay }()

	var retryCount int
	err := errors.New("temporary download failure")
	if !shouldRetryPageDownload(err, &retryCount) {
		t.Fatal("first page failure should retry")
	}
	if !shouldRetryPageDownload(err, &retryCount) {
		t.Fatal("second page failure should retry")
	}
	if shouldRetryPageDownload(err, &retryCount) {
		t.Fatal("third page failure should stop retrying")
	}
	if retryCount != 3 {
		t.Fatalf("retryCount = %d, want 3", retryCount)
	}
}

func TestRecordArchiveIfNeeded(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(filepath.Dir(exe), "BBDown.archives")
	_ = os.Remove(archivePath)
	t.Cleanup(func() { _ = os.Remove(archivePath) })

	recordArchiveIfNeeded(&bbdown.MyOption{}, "100")
	if bbdown.CheckAidFromArchive("100") {
		t.Fatal("archive should not be written when SaveArchivesToFile is false")
	}

	recordArchiveIfNeeded(&bbdown.MyOption{SaveArchivesToFile: true}, "100")
	if !bbdown.CheckAidFromArchive("100") {
		t.Fatal("archive should be written when SaveArchivesToFile is true")
	}
}

func TestFinishPageLikeOriginalRecordsArchiveOnNormalReturn(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(filepath.Dir(exe), "BBDown.archives")
	_ = os.Remove(archivePath)
	t.Cleanup(func() { _ = os.Remove(archivePath) })

	finishPageLikeOriginal(&bbdown.MyOption{SaveArchivesToFile: true}, "200")
	if !bbdown.CheckAidFromArchive("200") {
		t.Fatal("normal DownloadPageAsync return should be archived like original")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	return string(body)
}
