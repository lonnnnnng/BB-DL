package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2/widget"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "BB-DL-desktop-test-runtime-*")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "create desktop test runtime dir: %v\n", err)
		os.Exit(1)
	}
	// long 2026-06-27 16:19:00：桌面 helper 会复制到运行目录；测试里的假 helper 只能落在临时目录，不能污染用户真实的 BBDown Go 配置目录。
	_ = os.Setenv(desktopRuntimeDirEnv, dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestSplitArgsSupportsQuotedValues(t *testing.T) {
	got, err := splitArgs(`--file-pattern '<videoTitle>[<dfn>]' --skip-ai=false --user-agent "BBDown Go"`)
	if err != nil {
		t.Fatalf("splitArgs returned error: %v", err)
	}
	want := []string{"--file-pattern", "<videoTitle>[<dfn>]", "--skip-ai=false", "--user-agent", "BBDown Go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitArgs = %#v, want %#v", got, want)
	}
}

func TestSplitArgsKeepsQuotedEmptyValues(t *testing.T) {
	got, err := splitArgs(`--download-danmaku-formats "" --file-pattern '' BV1J9EB6xEAB`)
	if err != nil {
		t.Fatalf("splitArgs returned error: %v", err)
	}
	want := []string{"--download-danmaku-formats", "", "--file-pattern", "", "BV1J9EB6xEAB"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitArgs = %#v, want %#v", got, want)
	}
}

func TestSplitArgsPreservesWindowsBackslashPaths(t *testing.T) {
	got, err := splitArgs(`--ffmpeg-path C:\ffmpeg\bin\ffmpeg.exe --work-dir "D:\Bili Downloads"`)
	if err != nil {
		t.Fatalf("splitArgs returned error: %v", err)
	}
	want := []string{"--ffmpeg-path", `C:\ffmpeg\bin\ffmpeg.exe`, "--work-dir", `D:\Bili Downloads`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitArgs = %#v, want %#v", got, want)
	}
}

func TestSplitArgsUsesBackslashForShellEscapes(t *testing.T) {
	got, err := splitArgs(`--file-pattern title\ with\ spaces --user-agent \"BBDown\ Go\" --literal C:\\tools`)
	if err != nil {
		t.Fatalf("splitArgs returned error: %v", err)
	}
	want := []string{"--file-pattern", "title with spaces", "--user-agent", `"BBDown Go"`, "--literal", `C:\tools`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitArgs = %#v, want %#v", got, want)
	}
}

func TestSplitArgsRejectsUnclosedQuote(t *testing.T) {
	if _, err := splitArgs(`--file-pattern '<videoTitle>`); err == nil {
		t.Fatal("splitArgs should reject unclosed quote")
	}
}

func TestEnsureWorkDirCreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads", "bilibili")
	got, err := ensureWorkDir("  " + path + "  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("ensureWorkDir path = %q, want %q", got, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("created path is not directory: %s", path)
	}
}

func TestEnsureWorkDirRejectsEmptyPath(t *testing.T) {
	if _, err := ensureWorkDir("  "); err == nil || !strings.Contains(err.Error(), "请先填写保存目录") {
		t.Fatalf("ensureWorkDir empty error = %v", err)
	}
}

func TestEnsureWorkDirRejectsFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureWorkDir(path); err == nil || !strings.Contains(err.Error(), "保存目录不是文件夹") {
		t.Fatalf("ensureWorkDir file path error = %v", err)
	}
}

func TestChangedFilesReturnsManagedOutputFilesOnly(t *testing.T) {
	dir := t.TempDir()
	unchanged := filepath.Join(dir, "old.mp4")
	modified := filepath.Join(dir, "mod.mp4")
	if err := os.WriteFile(unchanged, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modified, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseline := snapshotFiles(dir)
	time.Sleep(time.Millisecond)
	if err := os.WriteFile(modified, []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"notes.txt", "part.tmp", "00000_demo.vclip", "demo.aria2", "qrcode.png", "BBDown.config"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("ignored"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files := changedFiles(dir, baseline)
	names := map[string]bool{}
	for _, file := range files {
		names[file.RelPath] = true
	}
	if !names["mod.mp4"] || !names["new.mp4"] || !names["cover.jpg"] {
		t.Fatalf("changed files = %#v, want managed output files", files)
	}
	for _, unwanted := range []string{"old.mp4", "notes.txt", "part.tmp", "00000_demo.vclip", "demo.aria2", "qrcode.png", "BBDown.config"} {
		if names[unwanted] {
			t.Fatalf("changed files should not include %s: %#v", unwanted, files)
		}
	}
}

func TestIsManagedOutputFileFiltersRuntimeAndTempFiles(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "demo.mp4", want: true},
		{path: "demo.video.mp4", want: true},
		{path: "demo.audio.m4a", want: true},
		{path: "demo.ass", want: true},
		{path: "demo.tmp", want: false},
		{path: "00000_demo.vclip", want: false},
		{path: "00000_demo.aclip", want: false},
		{path: "demo.aria2", want: false},
		{path: "qrcode.png", want: false},
		{path: "BBDown.config", want: false},
	}
	for _, tc := range tests {
		if got := isManagedOutputFile(tc.path); got != tc.want {
			t.Fatalf("isManagedOutputFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestChangedFilesSkipsUnchangedOutput(t *testing.T) {
	dir := t.TempDir()
	unchanged := filepath.Join(dir, "old.mp4")
	if err := os.WriteFile(unchanged, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseline := snapshotFiles(dir)
	files := changedFiles(dir, baseline)
	names := map[string]bool{}
	for _, file := range files {
		names[file.RelPath] = true
	}
	if names["old.mp4"] {
		t.Fatalf("unchanged file should not be returned: %#v", files)
	}
}

func TestParseTransferEventAcceptsServerJSON(t *testing.T) {
	event, ok := parseTransferEvent(serverTransferEventPrefix + `{"Path":"demo.video.mp4","Bytes":123,"Delta":true}`)
	if !ok {
		t.Fatal("server transfer event should parse")
	}
	if event.Path != "demo.video.mp4" || event.Bytes != 123 || !event.Delta {
		t.Fatalf("event = %#v", event)
	}
	if _, ok := parseTransferEvent("普通日志"); ok {
		t.Fatal("normal log line should not parse as transfer event")
	}
}

func TestApplyTransferEventToTaskTracksPathGrowth(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "old.video.mp4")
	if err := os.WriteFile(existing, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := &desktopTask{
		WorkDir:   dir,
		Baseline:  snapshotFiles(dir),
		StartedAt: time.Date(2026, 6, 25, 13, 20, 0, 0, time.Local),
	}
	if !applyTransferEventToTask(task, transferEvent{Path: "new.video.mp4", Bytes: 100, Delta: true}, task.StartedAt.Add(time.Second)) {
		t.Fatal("delta event should update task")
	}
	if task.Bytes != 100 || int64(task.LastSpeedBytes) != 100 {
		t.Fatalf("after delta bytes=%d speed=%v", task.Bytes, task.LastSpeedBytes)
	}
	if applyTransferEventToTask(task, transferEvent{Path: "new.video.mp4", Bytes: 100}, task.StartedAt.Add(2*time.Second)) {
		t.Fatal("final event for already counted path should not double count")
	}
	if !applyTransferEventToTask(task, transferEvent{Path: existing, Bytes: 15}, task.StartedAt.Add(3*time.Second)) {
		t.Fatal("absolute final event should count growth over baseline")
	}
	if task.Bytes != 105 {
		t.Fatalf("task bytes = %d, want 105", task.Bytes)
	}
}

func TestApplyFileGrowthFallbackTracksDirectoryGrowth(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "old.mp4")
	notes := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(existing, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, 6, 27, 12, 0, 0, 0, time.Local)
	task := &desktopTask{
		WorkDir:   dir,
		Baseline:  snapshotFiles(dir),
		StartedAt: startedAt,
	}
	if err := os.WriteFile(existing, []byte("0123456789abcde"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes, bytesOfLen(200), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.video.mp4"), bytesOfLen(100), 0o644); err != nil {
		t.Fatal(err)
	}

	if !applyFileGrowthFallbackToTask(task, startedAt.Add(2*time.Second)) {
		t.Fatal("file growth fallback should update task bytes")
	}
	if task.Bytes != 105 || task.FileGrowthBytes != 105 {
		t.Fatalf("bytes=%d fallback=%d, want 105", task.Bytes, task.FileGrowthBytes)
	}
	if task.LastSpeedBytes != 52.5 {
		t.Fatalf("speed=%v, want 52.5", task.LastSpeedBytes)
	}
}

func TestIsDownloadMonitorFileKeepsDownloadArtifactsOnly(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "demo.mp4", want: true},
		{path: "demo.m4a", want: true},
		{path: "demo.jpg", want: true},
		{path: "demo.ass", want: true},
		{path: "00000_demo.video.vclip", want: true},
		{path: "demo.tmp", want: true},
		{path: "notes.txt", want: false},
		{path: "BBDown.config", want: false},
		{path: "demo.aria2", want: false},
	}
	for _, tc := range tests {
		if got := isDownloadMonitorFile(tc.path); got != tc.want {
			t.Fatalf("isDownloadMonitorFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestApplyFileGrowthFallbackSkipsAfterTransferEvents(t *testing.T) {
	dir := t.TempDir()
	task := &desktopTask{
		WorkDir:   dir,
		Baseline:  snapshotFiles(dir),
		StartedAt: time.Date(2026, 6, 27, 12, 0, 0, 0, time.Local),
	}
	if !applyTransferEventToTask(task, transferEvent{Path: "event.video.mp4", Bytes: 50, Delta: true}, task.StartedAt.Add(time.Second)) {
		t.Fatal("transfer event should update task")
	}
	if err := os.WriteFile(filepath.Join(dir, "new.video.mp4"), bytesOfLen(100), 0o644); err != nil {
		t.Fatal(err)
	}

	if applyFileGrowthFallbackToTask(task, task.StartedAt.Add(2*time.Second)) {
		t.Fatal("fallback should not update after hidden transfer events")
	}
	if task.Bytes != 50 {
		t.Fatalf("bytes = %d, want hidden event bytes 50", task.Bytes)
	}
}

func TestApplyDesktopProgressLineTracksTimestampedDownloadStages(t *testing.T) {
	task := &desktopTask{}

	if applyDesktopProgressLine(task, "[2026-06-27 11:02:03.004] - 开始解析P1: 1... (1 of 2)") {
		t.Fatal("first page start should keep progress at zero")
	}
	if task.ProgressState.TotalPages != 2 {
		t.Fatalf("total pages = %d, want 2", task.ProgressState.TotalPages)
	}
	if !applyDesktopProgressLine(task, "[2026-06-27 11:02:04.004] - 开始下载P1视频...") {
		t.Fatal("video stage should update progress")
	}
	videoProgress := task.Progress
	if videoProgress <= 0 || videoProgress >= 0.10 {
		t.Fatalf("video progress = %v, want early first-page progress", videoProgress)
	}
	if !applyDesktopProgressLine(task, "[2026-06-27 11:02:05.004] - 开始下载P1音频...") {
		t.Fatal("audio stage should update progress")
	}
	if task.Progress <= videoProgress {
		t.Fatalf("audio progress = %v, video progress = %v", task.Progress, videoProgress)
	}
	if !applyDesktopProgressLine(task, "[2026-06-27 11:02:06.004] - 下载P1完毕") {
		t.Fatal("page completion should update progress")
	}
	pageDoneProgress := task.Progress
	if pageDoneProgress < 0.37 || pageDoneProgress > 0.38 {
		t.Fatalf("page done progress = %v, want about 37.5%%", pageDoneProgress)
	}
	if applyDesktopProgressLine(task, "开始下载P1视频...") {
		t.Fatal("older repeated stage should not move progress backwards")
	}
	if !applyDesktopProgressLine(task, "开始解析P2: 2... (2 of 2)") {
		t.Fatal("second page start should advance progress")
	}
	if task.Progress < 0.50 || task.Progress > 0.51 {
		t.Fatalf("second page progress = %v, want about 50%%", task.Progress)
	}
	if !applyDesktopProgressLine(task, "任务完成") {
		t.Fatal("completion should update progress")
	}
	if task.Progress != 1 {
		t.Fatalf("final progress = %v, want 1", task.Progress)
	}
}

func TestApplyDesktopProgressLineTracksClipAndMuxStages(t *testing.T) {
	task := &desktopTask{}
	_ = applyDesktopProgressLine(task, "开始解析P1: 1... (1 of 1)")

	if !applyDesktopProgressLine(task, "开始下载P1视频, 片段(1/3)...") {
		t.Fatal("clip stage should update progress")
	}
	if task.Progress <= 0 || task.Progress >= 0.30 {
		t.Fatalf("clip progress = %v, want early in page", task.Progress)
	}
	if !applyDesktopProgressLine(task, "开始合并音视频和字幕...") {
		t.Fatal("mux stage should update progress")
	}
	if task.Progress < 0.89 || task.Progress > 0.91 {
		t.Fatalf("mux progress = %v, want about 90%%", task.Progress)
	}
}

func TestProgressPercentTextClampsAndFormats(t *testing.T) {
	tests := []struct {
		progress float64
		want     string
	}{
		{progress: -0.1, want: "0%"},
		{progress: 0.375, want: "38%"},
		{progress: 1.2, want: "100%"},
	}
	for _, tc := range tests {
		if got := progressPercentText(tc.progress); got != tc.want {
			t.Fatalf("progressPercentText(%v) = %q, want %q", tc.progress, got, tc.want)
		}
	}
}

func TestBuildArgsIncludesToolPathsForDownloadOnly(t *testing.T) {
	state := &desktopState{
		danmakuCheck:      widget.NewCheck("", nil),
		skipSubtitleCheck: widget.NewCheck("", nil),
		skipCoverCheck:    widget.NewCheck("", nil),
		skipMuxCheck:      widget.NewCheck("", nil),
		aria2cCheck:       widget.NewCheck("", nil),
	}
	task := &desktopTask{
		URL:              "BV1J9EB6xEAB",
		Mode:             "下载",
		Channel:          "WEB",
		FFmpegPath:       "/opt/homebrew/bin/ffmpeg",
		MP4BoxPath:       "/opt/homebrew/bin/MP4Box",
		Aria2cPath:       "/opt/homebrew/bin/aria2c",
		VideoIndex:       "1",
		AudioIndex:       "2",
		EncodingPriority: "hevc,avc,av1",
	}
	got := state.buildArgs(task, nil)
	for _, want := range []string{"--ffmpeg-path", "/opt/homebrew/bin/ffmpeg", "--mp4box-path", "/opt/homebrew/bin/MP4Box", "--aria2c-path", "/opt/homebrew/bin/aria2c", "--video-index", "1", "--audio-index", "2"} {
		if !containsArg(got, want) {
			t.Fatalf("download args %v missing %q", got, want)
		}
	}
	task.FFmpegPath = "自动"
	task.MP4BoxPath = " "
	task.Aria2cPath = "自动"
	got = state.buildArgs(task, nil)
	for _, unwanted := range []string{"--ffmpeg-path", "--mp4box-path", "--aria2c-path", "自动"} {
		if containsArg(got, unwanted) {
			t.Fatalf("automatic tool path value should not be emitted in args: %v", got)
		}
	}

	task.Mode = "仅查看"
	got = state.buildArgs(task, nil)
	for _, unwanted := range []string{"--ffmpeg-path", "--mp4box-path", "--aria2c-path", "--video-index", "--audio-index"} {
		if containsArg(got, unwanted) {
			t.Fatalf("info args should not include %s: %v", unwanted, got)
		}
	}
}

func TestResolveTaskToolPathsRepairsBareFFmpegCommand(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := writeDesktopTestExecutable(t, dir, "ffmpeg")
	t.Setenv("PATH", dir)
	state := &desktopState{}
	task := &desktopTask{
		URL:        "BV1J9EB6xEAB",
		Mode:       "下载",
		Channel:    "WEB",
		FFmpegPath: "ffmpeg",
		Args:       []string{"--work-dir", t.TempDir(), "--ffmpeg-path", "ffmpeg", "BV1J9EB6xEAB"},
	}

	if err := state.resolveTaskToolPaths(task); err != nil {
		t.Fatal(err)
	}
	if task.FFmpegPath != ffmpegPath {
		t.Fatalf("FFmpegPath = %q, want %q", task.FFmpegPath, ffmpegPath)
	}
	if !containsAdjacentArgs(task.Args, "--ffmpeg-path", ffmpegPath) {
		t.Fatalf("args should contain repaired ffmpeg path: %v", task.Args)
	}
	if containsAdjacentArgs(task.Args, "--ffmpeg-path", "ffmpeg") {
		t.Fatalf("args should not keep stale ffmpeg command: %v", task.Args)
	}
}

func TestResolveTaskToolPathsTreatsAutomaticToolPathAsEmpty(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := writeDesktopTestExecutable(t, dir, "ffmpeg")
	t.Setenv("PATH", dir)
	state := &desktopState{}
	task := &desktopTask{
		URL:        "BV1J9EB6xEAB",
		Mode:       "下载",
		Channel:    "WEB",
		FFmpegPath: "自动",
		Args:       []string{"--work-dir", t.TempDir(), "--ffmpeg-path", "自动", "BV1J9EB6xEAB"},
	}

	if err := state.resolveTaskToolPaths(task); err != nil {
		t.Fatal(err)
	}
	if task.FFmpegPath != ffmpegPath {
		t.Fatalf("FFmpegPath = %q, want %q", task.FFmpegPath, ffmpegPath)
	}
	if containsArg(task.Args, "自动") {
		t.Fatalf("args should not keep automatic tool path marker: %v", task.Args)
	}
}

func TestResolveTaskToolPathsRepairsStaleExplicitFFmpegPath(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := writeDesktopTestExecutable(t, dir, "ffmpeg")
	t.Setenv("PATH", dir)
	state := &desktopState{}
	stalePath := filepath.Join(t.TempDir(), "old", "ffmpeg")
	task := &desktopTask{
		URL:        "BV1J9EB6xEAB",
		Mode:       "下载",
		Channel:    "WEB",
		FFmpegPath: stalePath,
		Args:       []string{"--work-dir", t.TempDir(), "--ffmpeg-path", stalePath, "BV1J9EB6xEAB"},
	}

	if err := state.resolveTaskToolPaths(task); err != nil {
		t.Fatal(err)
	}
	if task.FFmpegPath != ffmpegPath {
		t.Fatalf("FFmpegPath = %q, want %q", task.FFmpegPath, ffmpegPath)
	}
	if !containsAdjacentArgs(task.Args, "--ffmpeg-path", ffmpegPath) {
		t.Fatalf("args should contain repaired ffmpeg path: %v", task.Args)
	}
	if containsAdjacentArgs(task.Args, "--ffmpeg-path", stalePath) {
		t.Fatalf("args should not keep stale ffmpeg path: %v", task.Args)
	}
}

func TestResolveTaskToolPathsUsesEffectiveBoolValues(t *testing.T) {
	ffmpegPath := writeDesktopTestExecutable(t, t.TempDir(), "ffmpeg")
	state := &desktopState{}
	task := &desktopTask{
		URL:        "BV1J9EB6xEAB",
		Mode:       "下载",
		Channel:    "WEB",
		FFmpegPath: ffmpegPath,
		MP4BoxPath: filepath.Join(t.TempDir(), "missing-mp4box"),
		Aria2cPath: filepath.Join(t.TempDir(), "missing-aria2c"),
		Args: []string{
			"--skip-mux=false",
			"--use-mp4box=false",
			"--use-aria2c=false",
			"BV1J9EB6xEAB",
		},
	}

	if err := state.resolveTaskToolPaths(task); err != nil {
		t.Fatal(err)
	}
	if !containsAdjacentArgs(task.Args, "--ffmpeg-path", ffmpegPath) {
		t.Fatalf("args should contain ffmpeg path when --skip-mux=false and --use-mp4box=false: %v", task.Args)
	}
	if containsArg(task.Args, "--mp4box-path") || containsArg(task.Args, "--aria2c-path") {
		t.Fatalf("disabled mp4box/aria2c should not add tool paths: %v", task.Args)
	}
}

func TestResolveTaskToolPathsUsesLastExplicitToolPathValue(t *testing.T) {
	formFFmpeg := writeDesktopTestExecutable(t, t.TempDir(), "ffmpeg")
	extraFFmpeg := writeDesktopTestExecutable(t, t.TempDir(), "custom-ffmpeg")
	state := &desktopState{}
	task := &desktopTask{
		URL:        "BV1J9EB6xEAB",
		Mode:       "下载",
		Channel:    "WEB",
		FFmpegPath: formFFmpeg,
		Args: []string{
			"--ffmpeg-path", formFFmpeg,
			"--ffmpeg-path=" + extraFFmpeg,
			"BV1J9EB6xEAB",
		},
	}

	if err := state.resolveTaskToolPaths(task); err != nil {
		t.Fatal(err)
	}
	if task.FFmpegPath != extraFFmpeg {
		t.Fatalf("FFmpegPath = %q, want extra args path %q", task.FFmpegPath, extraFFmpeg)
	}
	if !containsAdjacentArgs(task.Args, "--ffmpeg-path", extraFFmpeg) {
		t.Fatalf("args should keep effective extra ffmpeg path: %v", task.Args)
	}
	if containsAdjacentArgs(task.Args, "--ffmpeg-path", formFFmpeg) {
		t.Fatalf("args should not keep earlier form ffmpeg path: %v", task.Args)
	}
}

func TestDeferredDesktopToolPathUsesHelperCommandName(t *testing.T) {
	if got := deferredDesktopToolPath("", "ffmpeg"); got != "ffmpeg" {
		t.Fatalf("deferredDesktopToolPath empty = %q, want ffmpeg", got)
	}
	if got := deferredDesktopToolPath("ffmpeg", "ffmpeg"); got != "ffmpeg" {
		t.Fatalf("deferredDesktopToolPath bare = %q, want ffmpeg", got)
	}
	if got := deferredDesktopToolPath("/old/homebrew/bin/ffmpeg", "ffmpeg"); got != "ffmpeg" {
		t.Fatalf("deferredDesktopToolPath stale = %q, want ffmpeg", got)
	}
	if got := deferredDesktopToolPath("", ""); got != "" {
		t.Fatalf("deferredDesktopToolPath nameless = %q, want empty", got)
	}
}

func TestEffectiveValueFlagUsesLastCLIValue(t *testing.T) {
	args := []string{"--ffmpeg-path", "form", "--ffmpeg-path=inline", "--ffmpeg-path", "extra", "BV1"}
	if got := effectiveValueFlag(args, "--ffmpeg-path", "fallback"); got != "extra" {
		t.Fatalf("effectiveValueFlag = %q, want extra", got)
	}
	if got := effectiveValueFlag([]string{"BV1"}, "--ffmpeg-path", " fallback "); got != "fallback" {
		t.Fatalf("effectiveValueFlag fallback = %q", got)
	}
}

func TestResolveTaskToolPathsSkipsMuxToolForResourceOnlyTask(t *testing.T) {
	missingFFmpeg := filepath.Join(t.TempDir(), "missing-ffmpeg")
	state := &desktopState{}
	task := &desktopTask{
		URL:        "BV1J9EB6xEAB",
		Mode:       "仅封面",
		Channel:    "WEB",
		FFmpegPath: missingFFmpeg,
		Args:       []string{"--cover-only", "--ffmpeg-path", missingFFmpeg, "BV1J9EB6xEAB"},
	}

	if err := state.resolveTaskToolPaths(task); err != nil {
		t.Fatalf("cover-only task should not require ffmpeg: %v", err)
	}
	if !containsAdjacentArgs(task.Args, "--ffmpeg-path", missingFFmpeg) {
		t.Fatalf("resource-only task should leave user args untouched: %v", task.Args)
	}
}

func TestTaskNeedsMuxToolUsesLastBoolValue(t *testing.T) {
	if taskNeedsMuxTool([]string{"--cover-only"}) {
		t.Fatal("cover-only task should not need mux tool")
	}
	if taskNeedsMuxTool([]string{"--audio-only"}) {
		t.Fatal("audio-only task should not need mux tool")
	}
	if taskNeedsMuxTool([]string{"--video-only"}) {
		t.Fatal("video-only task should not need mux tool")
	}
	if !taskNeedsMuxTool([]string{"--cover-only", "--cover-only=false"}) {
		t.Fatal("explicit false should re-enable mux tool requirement")
	}
	if !taskNeedsMuxTool([]string{"--audio-only", "--audio-only=false"}) {
		t.Fatal("explicit false should re-enable audio-only mux tool requirement")
	}
	if taskNeedsMuxTool([]string{"--skip-mux=false", "--sub-only=true"}) {
		t.Fatal("sub-only task should not need mux tool even when skip-mux is false")
	}
	if !taskNeedsMuxTool([]string{"--audio-only", "false", "BV1J9EB6xEAB"}) {
		t.Fatal("separate false should re-enable mux tool requirement")
	}
	if taskNeedsMuxTool([]string{"--skip-mux", "false", "--sub-only", "true", "BV1J9EB6xEAB"}) {
		t.Fatal("separate bool values should follow CLI semantics for mux checks")
	}
}

func TestTaskNeedsAria2cToolSkipsInfoTasks(t *testing.T) {
	for _, args := range [][]string{
		{"info", "--use-aria2c", "BV1J9EB6xEAB"},
		{"--only-show-info", "--use-aria2c", "BV1J9EB6xEAB"},
		{"--info", "--use-aria2c", "BV1J9EB6xEAB"},
	} {
		if taskNeedsAria2cTool(args) {
			t.Fatalf("info args should not need aria2c: %v", args)
		}
	}
	if !taskNeedsAria2cTool([]string{"--cover-only", "--use-aria2c", "BV1J9EB6xEAB"}) {
		t.Fatal("resource download with --use-aria2c should still require aria2c")
	}
}

func TestTaskNeedsAria2cToolSupportsAria2Alias(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{args: []string{"--aria2", "BV1J9EB6xEAB"}, want: true},
		{args: []string{"--aria2=false", "BV1J9EB6xEAB"}, want: false},
		{args: []string{"--aria2", "--use-aria2c=false", "BV1J9EB6xEAB"}, want: false},
		{args: []string{"--use-aria2c=false", "--aria2", "BV1J9EB6xEAB"}, want: true},
		{args: []string{"--aria2", "false", "BV1J9EB6xEAB"}, want: false},
		{args: []string{"--use-aria2c", "false", "--aria2", "true", "BV1J9EB6xEAB"}, want: true},
		{args: []string{"info", "--aria2", "BV1J9EB6xEAB"}, want: false},
	}
	for _, tc := range tests {
		if got := taskNeedsAria2cTool(tc.args); got != tc.want {
			t.Fatalf("taskNeedsAria2cTool(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestAutoQueueEnabledIsNilSafe(t *testing.T) {
	if (&desktopState{}).autoQueueEnabled() {
		t.Fatal("auto queue should be disabled when checkbox is missing")
	}
	state := &desktopState{autoQueueCheck: widget.NewCheck("自动队列", nil)}
	if state.autoQueueEnabled() {
		t.Fatal("auto queue should use unchecked checkbox state")
	}
	state.autoQueueCheck.SetChecked(true)
	if !state.autoQueueEnabled() {
		t.Fatal("auto queue should be enabled when checkbox is checked")
	}
}

func TestActiveDesktopTasksKeepsOnlyPendingAndRunningStates(t *testing.T) {
	tasks := []*desktopTask{
		{ID: 1, Status: statusPending},
		{ID: 2, Status: statusRunning},
		{ID: 3, Status: statusStopping},
		{ID: 4, Status: statusSuccess},
		{ID: 5, Status: statusFailed},
		{ID: 6, Status: statusStopped},
		nil,
	}

	got := activeDesktopTasks(tasks)
	ids := make([]int, 0, len(got))
	for _, task := range got {
		ids = append(ids, task.ID)
	}
	want := []int{1, 2, 3}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("active task IDs = %v, want %v", ids, want)
	}
	if !hasEndedDesktopTask(tasks) {
		t.Fatal("task list with success/failed/stopped should report ended tasks")
	}
	if hasEndedDesktopTask(got) {
		t.Fatal("filtered active task list should not report ended tasks")
	}
}

func TestRetryableDesktopTasksKeepsFailedAndStoppedOnly(t *testing.T) {
	tasks := []*desktopTask{
		{ID: 1, Status: statusPending},
		{ID: 2, Status: statusSuccess},
		{ID: 3, Status: statusFailed},
		{ID: 4, Status: statusStopped},
		{ID: 5, Status: statusRunning},
		nil,
	}

	got := retryableDesktopTasks(tasks)
	ids := make([]int, 0, len(got))
	for _, task := range got {
		ids = append(ids, task.ID)
	}
	want := []int{3, 4}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("retryable task IDs = %v, want %v", ids, want)
	}
	if !hasRetryableDesktopTask(tasks) {
		t.Fatal("task list with failed/stopped should report retryable tasks")
	}
	if hasRetryableDesktopTask([]*desktopTask{{ID: 1, Status: statusPending}, {ID: 2, Status: statusSuccess}}) {
		t.Fatal("pending/success tasks should not be reported as retryable")
	}
}

func TestResolveTaskToolPathsSkipsAria2cForInfoTask(t *testing.T) {
	missingAria2c := filepath.Join(t.TempDir(), "missing-aria2c")
	state := &desktopState{}
	task := &desktopTask{
		URL:        "BV1J9EB6xEAB",
		Mode:       "下载",
		Channel:    "WEB",
		Aria2cPath: missingAria2c,
		Args:       []string{"--only-show-info", "--use-aria2c", "--aria2c-path", missingAria2c, "BV1J9EB6xEAB"},
	}

	if err := state.resolveTaskToolPaths(task); err != nil {
		t.Fatalf("only-show-info task should not require aria2c: %v", err)
	}
}

func TestCheckToolPathsWritesDiagnosticsAndUpdatesEntries(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nprintf '\\nffmpeg version desktop-test\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	aria2cPath := writeDesktopTestExecutable(t, dir, "aria2c")
	t.Setenv("PATH", dir)
	state := &desktopState{
		ffmpegPathEntry: widget.NewEntry(),
		mp4boxPathEntry: widget.NewEntry(),
		aria2cPathEntry: widget.NewEntry(),
		logEntry:        widget.NewTextGrid(),
		statusLabel:     widget.NewLabel(""),
	}
	state.ffmpegPathEntry.SetText("ffmpeg")
	state.mp4boxPathEntry.SetText(filepath.Join(t.TempDir(), "missing-MP4Box"))
	state.aria2cPathEntry.SetText("aria2c")

	state.checkToolPaths()

	body := state.logEntry.Text()
	for _, want := range []string{
		"工具检测:",
		"FFmpeg: 已找到 " + ffmpegPath,
		"MP4Box: 未找到",
		"可在输入框填写完整路径或用 --mp4box-path",
		"当前填写路径不存在：" + state.mp4boxPathEntry.Text,
		"aria2c: 已找到 " + aria2cPath,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("tool diagnostics missing %q: %q", want, body)
		}
	}
	if runtime.GOOS != "windows" && !strings.Contains(body, "版本: ffmpeg version desktop-test") {
		t.Fatalf("tool diagnostics should include executable version on POSIX-like runners: %q", body)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(body, "brew install gpac") {
		t.Fatalf("macOS MP4Box hint should mention gpac install: %q", body)
	}
	if state.ffmpegPathEntry.Text != ffmpegPath || state.aria2cPathEntry.Text != aria2cPath {
		t.Fatalf("tool entries not repaired: ffmpeg=%q aria2c=%q", state.ffmpegPathEntry.Text, state.aria2cPathEntry.Text)
	}
	if state.statusLabel.Text != "工具检测完成" {
		t.Fatalf("status = %q", state.statusLabel.Text)
	}
}

func TestDesktopToolMissingCheckTextIsActionable(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing-ffmpeg")
	got := desktopToolMissingCheckText("FFmpeg", "--ffmpeg-path", "ffmpeg", missingPath)
	for _, want := range []string{"未找到", "--ffmpeg-path", missingPath} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing tool text = %q, missing %q", got, want)
		}
	}
	if runtime.GOOS == "darwin" && !strings.Contains(got, "brew install ffmpeg") {
		t.Fatalf("macOS missing tool text should mention brew: %q", got)
	}
}

func TestDesktopFirstNonEmptyLine(t *testing.T) {
	if got := desktopFirstNonEmptyLine("\n\t\n  tool version desktop  \nnext"); got != "tool version desktop" {
		t.Fatalf("desktopFirstNonEmptyLine = %q", got)
	}
	if got := desktopFirstNonEmptyLine("\n\t\n"); got != "" {
		t.Fatalf("empty desktopFirstNonEmptyLine = %q", got)
	}
}

func TestCopyHelperIfNeededReplacesSameSizeDifferentContent(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source-helper")
	target := filepath.Join(dir, "target-helper")
	if err := os.WriteFile(source, []byte("new-helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old-helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 6, 27, 15, 2, 0, 0, time.Local)
	if err := os.Chtimes(source, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	if err := copyHelperIfNeeded(source, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-helper" {
		t.Fatalf("target helper was not replaced: %q", got)
	}
}

func TestRuntimeDirCanBeIsolatedForTests(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	t.Setenv(desktopRuntimeDirEnv, dir)

	got, err := runtimeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("runtimeDir = %q, want isolated dir %q", got, dir)
	}
}

func TestFillFormFromInfoTaskKeepsToolPathAndStreamSelection(t *testing.T) {
	ffmpegPath := writeDesktopTestExecutable(t, t.TempDir(), "ffmpeg")
	state := &desktopState{
		urlEntry:          widget.NewEntry(),
		workDirEntry:      widget.NewEntry(),
		pageEntry:         widget.NewEntry(),
		videoIndexEntry:   widget.NewSelectEntry([]string{"自动"}),
		audioIndexEntry:   widget.NewSelectEntry([]string{"自动"}),
		dfnEntry:          widget.NewEntry(),
		encodingEntry:     widget.NewEntry(),
		extraArgsEntry:    widget.NewMultiLineEntry(),
		ffmpegPathEntry:   widget.NewEntry(),
		mp4boxPathEntry:   widget.NewEntry(),
		aria2cPathEntry:   widget.NewEntry(),
		modeSelect:        widget.NewSelect(desktopModeOptionsForTest(), nil),
		channelSelect:     widget.NewSelect([]string{"WEB"}, nil),
		danmakuCheck:      widget.NewCheck("", nil),
		skipSubtitleCheck: widget.NewCheck("", nil),
		skipCoverCheck:    widget.NewCheck("", nil),
		skipMuxCheck:      widget.NewCheck("", nil),
		aria2cCheck:       widget.NewCheck("", nil),
		statusLabel:       widget.NewLabel(""),
		tasks:             []*desktopTask{{URL: "BV1J9EB6xEAB", Mode: "仅查看", Channel: "WEB", SelectPage: "1"}},
		selectedTaskIndex: 0,
	}
	state.ffmpegPathEntry.SetText(ffmpegPath)
	state.videoIndexEntry.SetText("3. [480P 清晰] [AVC]")
	state.audioIndexEntry.SetText("2. [M4A] [65 kbps]")

	state.fillFormFromSelected()

	if state.modeSelect.Selected != "下载" {
		t.Fatalf("mode = %q, want 下载", state.modeSelect.Selected)
	}
	if state.ffmpegPathEntry.Text != ffmpegPath {
		t.Fatalf("ffmpeg path = %q, want %q", state.ffmpegPathEntry.Text, ffmpegPath)
	}
	if state.videoIndexEntry.Text != "3. [480P 清晰] [AVC]" {
		t.Fatalf("video index selection was cleared: %q", state.videoIndexEntry.Text)
	}
	if state.audioIndexEntry.Text != "2. [M4A] [65 kbps]" {
		t.Fatalf("audio index selection was cleared: %q", state.audioIndexEntry.Text)
	}
}

func TestFillFormFromDownloadTaskClearsStaleStreamSelection(t *testing.T) {
	state := desktopStateForFillFormSwitchTest()
	state.videoIndexEntry.SetText("3. [480P 清晰] [AVC]")
	state.audioIndexEntry.SetText("2. [M4A] [65 kbps]")
	state.tasks = []*desktopTask{{
		URL:     "BV1J9EB6xEAB",
		Mode:    "下载",
		Channel: "WEB",
		Args:    []string{"BV1J9EB6xEAB"},
	}}
	state.selectedTaskIndex = 0

	state.fillFormFromSelected()

	if state.videoIndexEntry.Text != "自动" || state.audioIndexEntry.Text != "自动" {
		t.Fatalf("empty task indexes should reset stale form selections: video=%q audio=%q", state.videoIndexEntry.Text, state.audioIndexEntry.Text)
	}
}

func TestSetStreamIndexEntryFromTask(t *testing.T) {
	entry := widget.NewSelectEntry([]string{"自动"})
	entry.SetText("3. [480P 清晰] [AVC]")
	setStreamIndexEntryFromTask(entry, "", true)
	if entry.Text != "3. [480P 清晰] [AVC]" {
		t.Fatalf("info task should keep current stream selection, got %q", entry.Text)
	}
	setStreamIndexEntryFromTask(entry, "", false)
	if entry.Text != "自动" {
		t.Fatalf("empty download task stream index should reset to 自动, got %q", entry.Text)
	}
	setStreamIndexEntryFromTask(entry, "2", false)
	if entry.Text != "2" {
		t.Fatalf("stored task stream index should be restored, got %q", entry.Text)
	}
}

func TestFillFormFromSelectedRestoresCommonSwitches(t *testing.T) {
	state := desktopStateForFillFormSwitchTest()
	state.tasks = []*desktopTask{{
		URL:     "BV1J9EB6xEAB",
		Mode:    "下载",
		Channel: "WEB",
		Args: []string{
			"--download-danmaku",
			"--skip-subtitle=true",
			"--skip-cover",
			"--skip-mux",
			"--use-aria2c",
			"BV1J9EB6xEAB",
		},
	}}
	state.selectedTaskIndex = 0

	state.fillFormFromSelected()

	if !state.danmakuCheck.Checked || !state.skipSubtitleCheck.Checked || !state.skipCoverCheck.Checked || !state.skipMuxCheck.Checked || !state.aria2cCheck.Checked {
		t.Fatalf("checks not restored: danmaku=%v sub=%v cover=%v mux=%v aria2c=%v",
			state.danmakuCheck.Checked,
			state.skipSubtitleCheck.Checked,
			state.skipCoverCheck.Checked,
			state.skipMuxCheck.Checked,
			state.aria2cCheck.Checked,
		)
	}
}

func TestFillFormFromSelectedRestoresAria2Alias(t *testing.T) {
	state := desktopStateForFillFormSwitchTest()
	state.tasks = []*desktopTask{{
		URL:     "BV1J9EB6xEAB",
		Mode:    "下载",
		Channel: "WEB",
		Args:    []string{"--aria2", "BV1J9EB6xEAB"},
	}}
	state.selectedTaskIndex = 0

	state.fillFormFromSelected()

	if !state.aria2cCheck.Checked {
		t.Fatal("aria2 alias should restore aria2c checkbox")
	}

	state.tasks[0].Args = []string{"--aria2", "--use-aria2c=false", "BV1J9EB6xEAB"}
	state.fillFormFromSelected()
	if state.aria2cCheck.Checked {
		t.Fatal("later false official flag should clear aria2c checkbox")
	}
}

func TestFillFormFromSelectedRespectsFalseBoolSwitches(t *testing.T) {
	state := desktopStateForFillFormSwitchTest()
	state.danmakuCheck.SetChecked(true)
	state.skipSubtitleCheck.SetChecked(true)
	state.skipCoverCheck.SetChecked(true)
	state.skipMuxCheck.SetChecked(true)
	state.aria2cCheck.SetChecked(true)
	state.tasks = []*desktopTask{{
		URL:     "BV1J9EB6xEAB",
		Mode:    "下载",
		Channel: "WEB",
		Args: []string{
			"--download-danmaku", "false",
			"--skip-subtitle", "false",
			"--skip-cover", "false",
			"--skip-mux", "false",
			"--use-aria2c", "false",
			"BV1J9EB6xEAB",
		},
	}}
	state.selectedTaskIndex = 0

	state.fillFormFromSelected()

	if state.danmakuCheck.Checked || state.skipSubtitleCheck.Checked || state.skipCoverCheck.Checked || state.skipMuxCheck.Checked || state.aria2cCheck.Checked {
		t.Fatalf("false switches should clear checks: danmaku=%v sub=%v cover=%v mux=%v aria2c=%v",
			state.danmakuCheck.Checked,
			state.skipSubtitleCheck.Checked,
			state.skipCoverCheck.Checked,
			state.skipMuxCheck.Checked,
			state.aria2cCheck.Checked,
		)
	}
}

func TestStreamIndexValueAcceptsDropdownAndManualInput(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: "自动", want: ""},
		{input: "2", want: "2"},
		{input: "2. [480P 清晰] [AVC]", want: "2"},
		{input: "bad", want: "bad"},
	}
	for _, tc := range tests {
		if got := streamIndexValue(tc.input); got != tc.want {
			t.Fatalf("streamIndexValue(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestTaskFromFormRejectsInvalidStreamIndexBeforeStart(t *testing.T) {
	state := desktopStateForFillFormSwitchTest()
	state.modeSelect.SetSelected("下载")
	state.channelSelect.SetSelected("WEB")
	state.videoIndexEntry.SetText("bad")

	_, err := state.taskFromForm("BV1J9EB6xEAB")
	if err == nil || !strings.Contains(err.Error(), "视频流序号无效") {
		t.Fatalf("invalid stream index should fail while creating desktop task, got %v", err)
	}
}

func TestTaskFromFormClearsUnusedSingleTrackIndexes(t *testing.T) {
	state := desktopStateForFillFormSwitchTest()
	state.modeSelect.SetSelected("仅音频")
	state.channelSelect.SetSelected("WEB")
	state.videoIndexEntry.SetText("bad")
	state.audioIndexEntry.SetText("1")

	task, err := state.taskFromForm("BV1J9EB6xEAB")
	if err != nil {
		t.Fatal(err)
	}
	if task.VideoIndex != "" || containsArg(task.Args, "--video-index") {
		t.Fatalf("audio-only task should clear video index: index=%q args=%v", task.VideoIndex, task.Args)
	}
	if state.videoIndexEntry.Text != "自动" {
		t.Fatalf("audio-only form should reset ignored video index, got %q", state.videoIndexEntry.Text)
	}
	if task.AudioIndex != "1" || !containsAdjacentArgs(task.Args, "--audio-index", "1") {
		t.Fatalf("audio-only task should keep audio index: index=%q args=%v", task.AudioIndex, task.Args)
	}
	if state.audioIndexEntry.Text != "1" {
		t.Fatalf("audio-only form should keep audio index, got %q", state.audioIndexEntry.Text)
	}

	state = desktopStateForFillFormSwitchTest()
	state.modeSelect.SetSelected("仅视频")
	state.channelSelect.SetSelected("WEB")
	state.videoIndexEntry.SetText("1")
	state.audioIndexEntry.SetText("bad")

	task, err = state.taskFromForm("BV1J9EB6xEAB")
	if err != nil {
		t.Fatal(err)
	}
	if task.AudioIndex != "" || containsArg(task.Args, "--audio-index") {
		t.Fatalf("video-only task should clear audio index: index=%q args=%v", task.AudioIndex, task.Args)
	}
	if state.audioIndexEntry.Text != "自动" {
		t.Fatalf("video-only form should reset ignored audio index, got %q", state.audioIndexEntry.Text)
	}
	if task.VideoIndex != "1" || !containsAdjacentArgs(task.Args, "--video-index", "1") {
		t.Fatalf("video-only task should keep video index: index=%q args=%v", task.VideoIndex, task.Args)
	}
	if state.videoIndexEntry.Text != "1" {
		t.Fatalf("video-only form should keep video index, got %q", state.videoIndexEntry.Text)
	}
}

func TestStreamIndexOptionsFromLog(t *testing.T) {
	logText := `[2026-06-27 02:33:21.074] - 共计6条视频流.
                            0. [360P 流畅] [640x360] [AVC]
https://example.invalid/video0.m4s
                            1. [480P 清晰] [852x480] [HEVC]
[2026-06-27 02:33:21.074] - 共计3条音频流.
                            0. [M4A] [192 kbps]
https://example.invalid/audio0.m4s
                            2. [M4A] [65 kbps]
[2026-06-27 02:33:21.280] - 已选择的流:`
	videos, audios := streamIndexOptionsFromLog(logText)
	wantVideos := []string{
		"0. [360P 流畅] [640x360] [AVC]",
		"1. [480P 清晰] [852x480] [HEVC]",
	}
	wantAudios := []string{
		"0. [M4A] [192 kbps]",
		"2. [M4A] [65 kbps]",
	}
	if !reflect.DeepEqual(videos, wantVideos) {
		t.Fatalf("video options = %#v, want %#v", videos, wantVideos)
	}
	if !reflect.DeepEqual(audios, wantAudios) {
		t.Fatalf("audio options = %#v, want %#v", audios, wantAudios)
	}
}

func TestBoolArgEnabledUsesLastEffectiveValue(t *testing.T) {
	args := []string{"--skip-cover=false", "--skip-cover", "--skip-mux", "--skip-mux=false", "--use-aria2c=false", "--use-aria2c=true", "--download-danmaku=1", "--skip-subtitle", "true", "--skip-subtitle", "false"}
	tests := []struct {
		flag string
		want bool
	}{
		{flag: "--skip-cover", want: true},
		{flag: "--skip-mux", want: false},
		{flag: "--use-aria2c", want: true},
		{flag: "--download-danmaku", want: true},
		{flag: "--skip-subtitle", want: false},
	}
	for _, tc := range tests {
		if got := boolArgEnabled(args, tc.flag); got != tc.want {
			t.Fatalf("boolArgEnabled(%q) = %v, want %v", tc.flag, got, tc.want)
		}
	}
}

func TestBoolArgEnabledSupportsSeparateBoolValues(t *testing.T) {
	tests := []struct {
		args []string
		flag string
		want bool
	}{
		{args: []string{"--skip-cover", "false"}, flag: "--skip-cover", want: false},
		{args: []string{"--skip-cover", "true"}, flag: "--skip-cover", want: true},
		{args: []string{"--skip-cover", "1"}, flag: "--skip-cover", want: true},
		{args: []string{"--skip-cover", "0"}, flag: "--skip-cover", want: false},
		{args: []string{"--skip-cover", "false", "--skip-cover"}, flag: "--skip-cover", want: true},
	}
	for _, tc := range tests {
		if got := boolArgEnabled(tc.args, tc.flag); got != tc.want {
			t.Fatalf("boolArgEnabled(%v, %q) = %v, want %v", tc.args, tc.flag, got, tc.want)
		}
	}
}

func TestBoolAnyArgEnabledUsesLastEffectiveAliasValue(t *testing.T) {
	args := []string{"--aria2", "--use-aria2c=false", "--aria2=true"}
	if !boolAnyArgEnabled(args, "--use-aria2c", "--aria2") {
		t.Fatalf("alias should re-enable after official false: %v", args)
	}
	args = []string{"--use-aria2c", "--aria2=false"}
	if boolAnyArgEnabled(args, "--use-aria2c", "--aria2") {
		t.Fatalf("alias false should disable later: %v", args)
	}
}

func TestMergeToolPathAddsCommonMacToolDirs(t *testing.T) {
	got := mergeToolPath("/usr/bin")
	if !strings.Contains(got, "/usr/bin") {
		t.Fatalf("merged path should keep existing path: %q", got)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(got, "/opt/homebrew/bin") {
		t.Fatalf("macOS merged path should include Homebrew bin: %q", got)
	}
}

func TestDesktopCommandEnvExportsHelperToolDirs(t *testing.T) {
	helperDir := t.TempDir()
	helper := writeDesktopTestExecutable(t, helperDir, helperName())
	t.Setenv("BBDOWN_GO_HELPER", helper)
	env := desktopCommandEnv()
	pathValue := envValueForTest(env, "PATH")
	toolDirsValue := envValueForTest(env, desktopToolDirsEnv)

	for _, value := range []string{pathValue, toolDirsValue} {
		if !strings.Contains(value, helperDir) {
			t.Fatalf("desktop command env should include helper dir %q, got %q", helperDir, value)
		}
	}
}

func TestFindDesktopToolPathUsesHelperToolDir(t *testing.T) {
	helperDir := t.TempDir()
	helper := writeDesktopTestExecutable(t, helperDir, helperName())
	tool := writeDesktopTestExecutable(t, helperDir, "custom-ffmpeg")
	t.Setenv("BBDOWN_GO_HELPER", helper)
	t.Setenv("PATH", t.TempDir())

	if got := findDesktopToolPath("", "custom-ffmpeg"); got != tool {
		t.Fatalf("findDesktopToolPath = %q, want helper dir tool %q", got, tool)
	}
}

func TestDetectToolPathFromLoginShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login shell probing is a zsh/POSIX fallback; Windows tool lookup is covered by PATH and helper-dir tests")
	}
	dir := t.TempDir()
	toolPath := writeDesktopTestExecutable(t, dir, "custom-ffmpeg")
	shellPath := filepath.Join(t.TempDir(), "zsh")
	if err := os.WriteFile(shellPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$BBDOWN_TEST_TOOL_PATH\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldShell := desktopLoginShellPath
	desktopLoginShellPath = shellPath
	t.Cleanup(func() { desktopLoginShellPath = oldShell })
	t.Setenv("BBDOWN_TEST_TOOL_PATH", toolPath)

	got := detectToolPathFromLoginShell("custom-ffmpeg")
	if got != toolPath {
		t.Fatalf("login shell tool path = %q, want %q", got, toolPath)
	}
}

func TestResolveDesktopToolPathReportsActionableMissingToolHint(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-tool")

	_, err := resolveDesktopToolPath(missing, "missing-tool-for-test")
	if err == nil {
		t.Fatal("missing explicit tool path should fail")
	}
	text := err.Error()
	for _, want := range []string{"找不到可执行的missing-tool-for-test文件", "missing-tool-for-test 输入框", "显式路径不存在：" + missing} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing tool error should contain %q: %s", want, text)
		}
	}
}

func TestDesktopStartupHintExplainsMissingFFmpeg(t *testing.T) {
	hint := desktopStartupHint(errors.New("找不到可执行的ffmpeg文件"))

	for _, want := range []string{"完整下载需要 FFmpeg", "检测工具"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint = %q, missing %q", hint, want)
		}
	}
	if runtime.GOOS == "darwin" && !strings.Contains(hint, "brew install ffmpeg") {
		t.Fatalf("macOS hint should mention brew install: %s", hint)
	}
}

func TestDesktopStartupHintExplainsOptionalTools(t *testing.T) {
	tests := []struct {
		errText string
		want    []string
	}{
		{errText: "找不到可执行的mp4box文件", want: []string{"需要 MP4Box", "检测工具"}},
		{errText: "找不到可执行的aria2c文件", want: []string{"启用了 aria2c", "检测工具"}},
	}
	for _, tc := range tests {
		hint := desktopStartupHint(errors.New(tc.errText))
		for _, want := range tc.want {
			if !strings.Contains(hint, want) {
				t.Fatalf("hint for %q = %q, missing %q", tc.errText, hint, want)
			}
		}
	}
}

func TestDesktopTaskFailureHintReadsHelperLog(t *testing.T) {
	logText := "[2026-06-28] - 找不到可执行的ffmpeg文件；请安装后重试，或使用 --ffmpeg-path 指定完整路径"
	hint := desktopTaskFailureHint(logText)

	for _, want := range []string{"完整下载需要 FFmpeg", "检测工具"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint = %q, missing %q", hint, want)
		}
	}
}

func TestTaskHistoryRoundTripRestoresTaskState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	modTime := time.Date(2026, 6, 27, 11, 55, 0, 0, time.FixedZone("CST", 8*3600))
	completed := &desktopTask{
		ID:               7,
		URL:              "BV1J9EB6xEAB",
		WorkDir:          "/tmp/bilibili",
		Mode:             "下载",
		Channel:          "WEB",
		SelectPage:       "1",
		VideoIndex:       "3",
		AudioIndex:       "2",
		DfnPriority:      "480P 清晰",
		EncodingPriority: "avc",
		ExtraArgs:        "--skip-ai=false",
		FFmpegPath:       "/opt/homebrew/bin/ffmpeg",
		Args:             []string{"--work-dir", "/tmp/bilibili", "--video-index", "3", "BV1J9EB6xEAB"},
		Status:           statusSuccess,
		Bytes:            123456,
		Progress:         1,
		Files:            []managedFile{{Path: "/tmp/bilibili/out.mp4", RelPath: "out.mp4", Size: 123456, ModTime: modTime}},
		CreatedAt:        modTime.Add(-2 * time.Minute),
		StartedAt:        modTime.Add(-time.Minute),
		EndedAt:          modTime,
	}
	completed.Log.WriteString("任务完成\n")
	running := &desktopTask{
		ID:        8,
		URL:       "BV-running",
		Mode:      "下载",
		Channel:   "WEB",
		Status:    statusRunning,
		Progress:  0.42,
		CreatedAt: modTime,
		StartedAt: modTime,
		Command:   &exec.Cmd{},
	}
	running.Log.WriteString("下载中\n")

	if err := saveTaskHistoryFile(path, []*desktopTask{completed, running}); err != nil {
		t.Fatal(err)
	}
	got, nextID, err := loadTaskHistoryFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if nextID != 9 {
		t.Fatalf("nextID = %d, want 9", nextID)
	}
	if len(got) != 2 {
		t.Fatalf("loaded tasks = %d, want 2", len(got))
	}
	if got[0].Status != statusSuccess || got[0].Log.String() != "任务完成\n" || got[0].Files[0].RelPath != "out.mp4" {
		t.Fatalf("completed task not restored: %#v log=%q", got[0], got[0].Log.String())
	}
	if got[0].VideoIndex != "3" || got[0].AudioIndex != "2" || got[0].FFmpegPath != "/opt/homebrew/bin/ffmpeg" {
		t.Fatalf("task form fields not restored: %#v", got[0])
	}
	if got[1].Status != statusStopped || got[1].Command != nil || got[1].Log.String() != "下载中\n" {
		t.Fatalf("running task should be persisted as stopped without command: %#v log=%q", got[1], got[1].Log.String())
	}
}

func TestEnsureDesktopTaskArgsRebuildsLegacyDownloadTask(t *testing.T) {
	task := &desktopTask{
		URL:              "BV1J9EB6xEAB",
		WorkDir:          "/tmp/bilibili",
		Mode:             "下载",
		Channel:          "TV",
		SelectPage:       "1",
		VideoIndex:       "3",
		AudioIndex:       "2",
		DfnPriority:      "480P 清晰",
		EncodingPriority: "avc",
		ExtraArgs:        `--skip-ai=false --file-pattern "legacy <videoTitle>"`,
		FFmpegPath:       "/opt/homebrew/bin/ffmpeg",
	}

	if err := ensureDesktopTaskArgs(task); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{
		{"--work-dir", "/tmp/bilibili"},
		{"--select-page", "1"},
		{"--video-index", "3"},
		{"--audio-index", "2"},
		{"--ffmpeg-path", "/opt/homebrew/bin/ffmpeg"},
		{"--file-pattern", "legacy <videoTitle>"},
	} {
		if !containsAdjacentArgs(task.Args, pair[0], pair[1]) {
			t.Fatalf("rebuilt args should contain %s %q: %v", pair[0], pair[1], task.Args)
		}
	}
	for _, want := range []string{"--use-tv-api", "--skip-ai=false", "BV1J9EB6xEAB"} {
		if !containsArg(task.Args, want) {
			t.Fatalf("rebuilt args missing %q: %v", want, task.Args)
		}
	}
}

func TestEnsureDesktopTaskArgsRebuildsLegacyLoginTask(t *testing.T) {
	tests := []struct {
		task desktopTask
		want string
	}{
		{task: desktopTask{Channel: "账号", Mode: "WEB 登录", URL: "WEB 登录"}, want: "login"},
		{task: desktopTask{Channel: "账号", Mode: "TV 登录", URL: "TV 登录"}, want: "logintv"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			task := tc.task
			if err := ensureDesktopTaskArgs(&task); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(task.Args, []string{tc.want}) {
				t.Fatalf("rebuilt login args = %v, want %s", task.Args, tc.want)
			}
		})
	}
}

func TestRefreshActionsEnablesCopyCommandForRebuildableLegacyTask(t *testing.T) {
	copyButton := widget.NewButton("复制命令", nil)
	state := &desktopState{
		tasks: []*desktopTask{{
			URL:     "BV1J9EB6xEAB",
			Mode:    "下载",
			Channel: "WEB",
			Status:  statusStopped,
		}},
		selectedTaskIndex: 0,
		selectedFileIndex: -1,
		copyCommandBtn:    copyButton,
	}

	state.refreshActions()
	if copyButton.Disabled() {
		t.Fatal("legacy task with rebuildable args should enable copy command")
	}

	state.tasks[0].URL = ""
	state.refreshActions()
	if !copyButton.Disabled() {
		t.Fatal("task without args or URL should disable copy command")
	}
}

func TestLoadTaskHistoryFileMissingStartsFresh(t *testing.T) {
	tasks, nextID, err := loadTaskHistoryFile(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 || nextID != 1 {
		t.Fatalf("missing history = tasks %d nextID %d, want empty and 1", len(tasks), nextID)
	}
}

func TestTaskHistoryKeepsRecentTasksOnly(t *testing.T) {
	tasks := make([]*desktopTask, 0, desktopTaskHistoryLimit+5)
	for i := 1; i <= desktopTaskHistoryLimit+5; i++ {
		tasks = append(tasks, &desktopTask{ID: i, URL: "BV", Mode: "下载", Channel: "WEB", Status: statusPending})
	}
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := saveTaskHistoryFile(path, tasks); err != nil {
		t.Fatal(err)
	}
	got, nextID, err := loadTaskHistoryFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != desktopTaskHistoryLimit {
		t.Fatalf("history tasks = %d, want %d", len(got), desktopTaskHistoryLimit)
	}
	if got[0].ID != 6 || got[len(got)-1].ID != desktopTaskHistoryLimit+5 {
		t.Fatalf("history should keep recent task IDs, got first=%d last=%d", got[0].ID, got[len(got)-1].ID)
	}
	if nextID != desktopTaskHistoryLimit+6 {
		t.Fatalf("nextID = %d, want %d", nextID, desktopTaskHistoryLimit+6)
	}
}

func TestTaskHistoryTrimsLargeLogsAtUTF8Boundary(t *testing.T) {
	var logText strings.Builder
	for i := 0; i < 300; i++ {
		logText.WriteString("旧日志")
	}
	logText.WriteString("最新日志-洗衣机演奏起风了")
	task := &desktopTask{ID: 1, URL: "BV", Mode: "下载", Channel: "WEB", Status: statusSuccess}
	original := logText.String()
	task.Log.WriteString(original)

	item := persistDesktopTaskWithLogLimit(task, 160)
	if len(item.Log) > 160 {
		t.Fatalf("trimmed log bytes = %d, want <= 160", len(item.Log))
	}
	if !strings.Contains(item.Log, "仅保留最后") || !strings.Contains(item.Log, "最新日志-洗衣机演奏起风了") {
		t.Fatalf("trimmed log should keep marker and tail, got %q", item.Log)
	}
	if !utf8.ValidString(item.Log) {
		t.Fatalf("trimmed log is not valid UTF-8: %q", item.Log)
	}
	if task.Log.String() != original {
		t.Fatal("persisting history should not mutate in-memory task log")
	}
}

func TestShouldSaveTaskHistoryRespectsThrottle(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 10, 0, 0, time.FixedZone("CST", 8*3600))
	tests := []struct {
		name string
		last time.Time
		now  time.Time
		want bool
	}{
		{name: "first save", last: time.Time{}, now: now, want: true},
		{name: "too soon", last: now.Add(-desktopTaskHistorySaveGap + time.Second), now: now, want: false},
		{name: "at interval", last: now.Add(-desktopTaskHistorySaveGap), now: now, want: true},
		{name: "clock moved backwards", last: now.Add(time.Second), now: now, want: true},
	}
	for _, tc := range tests {
		if got := shouldSaveTaskHistory(tc.last, tc.now, desktopTaskHistorySaveGap); got != tc.want {
			t.Fatalf("%s: shouldSaveTaskHistory = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTaskStatusImportanceUsesSemanticLevels(t *testing.T) {
	tests := []struct {
		status taskStatus
		want   widget.Importance
	}{
		{statusPending, widget.LowImportance},
		{statusRunning, widget.HighImportance},
		{statusStopping, widget.WarningImportance},
		{statusSuccess, widget.SuccessImportance},
		{statusFailed, widget.DangerImportance},
		{statusStopped, widget.LowImportance},
	}
	for _, tt := range tests {
		if got := taskStatusImportance(tt.status); got != tt.want {
			t.Errorf("taskStatusImportance(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestStatusMessageImportanceKeepsErrorsAndSuccessDistinct(t *testing.T) {
	if got := statusMessageImportance("请输入视频地址"); got != widget.DangerImportance {
		t.Fatalf("input error importance = %v", got)
	}
	if got := statusMessageImportance("任务 #3 已完成"); got != widget.SuccessImportance {
		t.Fatalf("success importance = %v", got)
	}
	if got := statusMessageImportance("就绪"); got != widget.MediumImportance {
		t.Fatalf("neutral importance = %v", got)
	}
}

func TestSyncSelectedTaskLogAppendsAndSwitchesTasks(t *testing.T) {
	state := &desktopState{
		logEntry:          widget.NewTextGrid(),
		renderedLogTaskID: -1,
	}
	first := &desktopTask{ID: 1}
	first.Log.WriteString("第一行\n")
	state.syncSelectedTaskLog(first)
	first.Log.WriteString("第二行\n")
	state.syncSelectedTaskLog(first)
	if got := state.logEntry.Text(); got != "第一行\n第二行\n" {
		t.Fatalf("appended log = %q", got)
	}

	second := &desktopTask{ID: 2}
	second.Log.WriteString("另一个任务\n")
	state.syncSelectedTaskLog(second)
	if got := state.logEntry.Text(); got != "另一个任务\n" {
		t.Fatalf("switched log = %q", got)
	}
}

func TestTaskCommandTextUsesRuntimeHelperAndShellQuoting(t *testing.T) {
	runtimeDir := t.TempDir()
	helperPath := writeDesktopTestExecutable(t, runtimeDir, helperName())
	task := &desktopTask{
		RuntimeDir: runtimeDir,
		Args:       []string{"--work-dir", "/tmp/download dir", "--file-pattern", "<videoTitle>[<dfn>]", "https://www.bilibili.com/video/BV1J9EB6xEAB"},
	}
	got := taskCommandText(task)
	quotedHelper := shellQuoteForOS(runtime.GOOS, helperPath)
	if !strings.HasPrefix(got, quotedHelper+" ") {
		t.Fatalf("command = %q, want helper prefix %q", got, helperPath)
	}
	for _, want := range []string{
		"--work-dir",
		shellQuoteForOS(runtime.GOOS, "/tmp/download dir"),
		"--file-pattern",
		shellQuoteForOS(runtime.GOOS, "<videoTitle>[<dfn>]"),
		"https://www.bilibili.com/video/BV1J9EB6xEAB",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("command = %q, missing %q", got, want)
		}
	}
}

func TestShellQuoteProtectsShellMetacharacters(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "https://www.bilibili.com/video/BV1J9EB6xEAB", want: "https://www.bilibili.com/video/BV1J9EB6xEAB"},
		{value: "<videoTitle>[<dfn>]", want: "'<videoTitle>[<dfn>]'"},
		{value: "title with spaces", want: "'title with spaces'"},
		{value: "long's pattern", want: "'long'\\''s pattern'"},
	}
	for _, tc := range tests {
		if got := shellQuoteForOS("linux", tc.value); got != tc.want {
			t.Fatalf("shellQuoteForOS(linux, %q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestShellJoinForOSUsesWindowsQuoting(t *testing.T) {
	got := shellJoinForOS("windows", []string{
		`C:\Program Files\BBDown Go\BB-DL-cli.exe`,
		"--file-pattern",
		"<videoTitle>[<dfn>]",
		"--user-agent",
		`BBDown "Go"`,
		"--work-dir",
		`C:\Bili Downloads\`,
		"BV1J9EB6xEAB",
		"",
	})
	want := `"C:\Program Files\BBDown Go\BB-DL-cli.exe" --file-pattern "<videoTitle>[<dfn>]" --user-agent "BBDown \"Go\"" --work-dir "C:\Bili Downloads\\" BV1J9EB6xEAB ""`
	if got != want {
		t.Fatalf("shellJoinForOS(windows) = %q, want %q", got, want)
	}
}

func TestTaskCommandHelperFallsBackToHelperName(t *testing.T) {
	t.Setenv("BBDOWN_GO_HELPER", filepath.Join(t.TempDir(), "missing-helper"))
	task := &desktopTask{RuntimeDir: t.TempDir()}
	if got := taskCommandHelper(task); got != helperName() {
		t.Fatalf("taskCommandHelper = %q, want fallback %q", got, helperName())
	}
}

func TestSelectedFilePathTextUsesAbsolutePath(t *testing.T) {
	file := &managedFile{Path: "/tmp/bilibili/out.mp4", RelPath: "out.mp4"}
	if got := selectedFilePathText(file); got != "/tmp/bilibili/out.mp4" {
		t.Fatalf("selectedFilePathText = %q, want absolute path", got)
	}
	if got := selectedFilePathText(nil); got != "" {
		t.Fatalf("nil selected file path = %q, want empty", got)
	}
}

func TestAllFilePathsTextJoinsNonEmptyPaths(t *testing.T) {
	files := []managedFile{
		{Path: " /tmp/bilibili/out.mp4 "},
		{Path: ""},
		{Path: "/tmp/bilibili/subtitle.srt"},
	}
	want := "/tmp/bilibili/out.mp4\n/tmp/bilibili/subtitle.srt"
	if got := allFilePathsText(files); got != want {
		t.Fatalf("allFilePathsText = %q, want %q", got, want)
	}
	text, count := allFilePathsTextWithCount(files)
	if text != want || count != 2 {
		t.Fatalf("allFilePathsTextWithCount = (%q, %d), want (%q, 2)", text, count, want)
	}
	if got := allFilePathsText(nil); got != "" {
		t.Fatalf("empty allFilePathsText = %q", got)
	}
}

func TestSelectTaskAfterMutationClearsSelectedFileAndClampsIndex(t *testing.T) {
	state := &desktopState{
		tasks:             []*desktopTask{{ID: 1}, {ID: 2}},
		selectedTaskIndex: 1,
		selectedFileIndex: 3,
	}
	state.tasks = state.tasks[:1]
	state.selectTaskAfterMutation(nil, 1)

	if state.selectedTaskIndex != 0 {
		t.Fatalf("selectedTaskIndex = %d, want 0", state.selectedTaskIndex)
	}
	if state.selectedFileIndex != -1 {
		t.Fatalf("selectedFileIndex = %d, want reset", state.selectedFileIndex)
	}
}

func TestSelectTaskAfterMutationKeepsPreferredTask(t *testing.T) {
	kept := &desktopTask{ID: 2}
	state := &desktopState{
		tasks:             []*desktopTask{{ID: 1}, kept, {ID: 3}},
		selectedTaskIndex: 1,
		selectedFileIndex: 2,
	}
	state.tasks = []*desktopTask{kept, {ID: 4}}
	state.selectTaskAfterMutation(kept, 1)

	if state.selectedTaskIndex != 0 {
		t.Fatalf("selectedTaskIndex = %d, want preferred task index 0", state.selectedTaskIndex)
	}
	if state.selectedFileIndex != -1 {
		t.Fatalf("selectedFileIndex = %d, want reset", state.selectedFileIndex)
	}
}

func TestDesktopDoctorJSONUsesHelperAndToolPathArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses POSIX shell")
	}
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	helperPath := filepath.Join(dir, helperName())
	script := "#!/bin/sh\n" +
		"# long 2026-06-27 12:28:00：桌面诊断必须调用同仓库 CLI helper，避免 GUI 自己拼一份和命令行不一致的环境快照。\n" +
		"for arg in \"$@\"; do printf '%s\\n' \"$arg\"; done > " + shellQuote(argsPath) + "\n" +
		"printf '{\"ok\":true}\\n'\n"
	if err := os.WriteFile(helperPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BBDOWN_GO_HELPER", helperPath)
	state := &desktopState{
		ffmpegPathEntry: widget.NewEntry(),
		mp4boxPathEntry: widget.NewEntry(),
		aria2cPathEntry: widget.NewEntry(),
	}
	state.ffmpegPathEntry.SetText("/opt/homebrew/bin/ffmpeg")
	state.mp4boxPathEntry.SetText("/opt/homebrew/bin/MP4Box")
	state.aria2cPathEntry.SetText("/opt/homebrew/bin/aria2c")

	out, err := state.desktopDoctorJSON(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != `{"ok":true}` {
		t.Fatalf("doctor output = %q", out)
	}
	argsBody, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"doctor", "--json", "--ffmpeg-path", "/opt/homebrew/bin/ffmpeg", "--mp4box-path", "/opt/homebrew/bin/MP4Box", "--aria2c-path", "/opt/homebrew/bin/aria2c"} {
		if !strings.Contains(string(argsBody), want) {
			t.Fatalf("helper args missing %q: %s", want, argsBody)
		}
	}
}

func TestDesktopDoctorJSONSkipsAutomaticToolPathValues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses POSIX shell")
	}
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	helperPath := filepath.Join(dir, helperName())
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do printf '%s\\n' \"$arg\"; done > " + shellQuote(argsPath) + "\n" +
		"printf '{\"ok\":true}\\n'\n"
	if err := os.WriteFile(helperPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BBDOWN_GO_HELPER", helperPath)
	state := &desktopState{
		ffmpegPathEntry: widget.NewEntry(),
		mp4boxPathEntry: widget.NewEntry(),
		aria2cPathEntry: widget.NewEntry(),
	}
	state.ffmpegPathEntry.SetText("自动")
	state.mp4boxPathEntry.SetText("/opt/homebrew/bin/MP4Box")
	state.aria2cPathEntry.SetText("   ")

	if _, err := state.desktopDoctorJSON(context.Background()); err != nil {
		t.Fatal(err)
	}
	argsBody, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(argsBody)
	for _, unwanted := range []string{"--ffmpeg-path", "自动", "--aria2c-path"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("helper args should skip automatic tool value %q: %s", unwanted, body)
		}
	}
	if !strings.Contains(body, "--mp4box-path") || !strings.Contains(body, "/opt/homebrew/bin/MP4Box") {
		t.Fatalf("helper args should keep explicit MP4Box path: %s", body)
	}
}

func TestDesktopToolPathValueSkipsAutomaticText(t *testing.T) {
	if got := desktopToolPathValue(" 自动 "); got != "" {
		t.Fatalf("automatic value should be omitted, got %q", got)
	}
	if got := desktopToolPathValue(" ffmpeg "); got != "ffmpeg" {
		t.Fatalf("explicit value = %q, want ffmpeg", got)
	}
}

func TestDoctorReportLogSummaryKeepsToolStatus(t *testing.T) {
	report := `{"tools":[{"name":"ffmpeg","found":true,"path":"/opt/homebrew/bin/ffmpeg","version":"ffmpeg version 8.1","flag":"--ffmpeg-path"},{"name":"MP4Box","found":false,"input":"/bad/MP4Box","flag":"--mp4box-path","installHint":"macOS 可执行 brew install gpac"},{"name":"aria2c","found":true,"flag":"--aria2c-path"}]}`

	got := doctorReportLogSummary(report)
	for _, want := range []string{"已复制诊断到剪贴板", "ffmpeg=/opt/homebrew/bin/ffmpeg [ffmpeg version 8.1]", "MP4Box=未找到（当前指定值不可用: /bad/MP4Box；可用 --mp4box-path；macOS 可执行 brew install gpac）", "aria2c=已找到"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, `"tools"`) {
		t.Fatalf("summary should not include full JSON: %q", got)
	}
}

func TestDoctorReportLogSummaryHandlesInvalidJSON(t *testing.T) {
	if got := doctorReportLogSummary("not-json"); got != "已复制诊断到剪贴板。\n" {
		t.Fatalf("summary = %q", got)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsAdjacentArgs(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func envValueForTest(env []string, key string) string {
	for _, item := range env {
		if strings.HasPrefix(item, key+"=") {
			return strings.TrimPrefix(item, key+"=")
		}
	}
	return ""
}

func desktopStateForFillFormSwitchTest() *desktopState {
	return &desktopState{
		urlEntry:          widget.NewEntry(),
		workDirEntry:      widget.NewEntry(),
		pageEntry:         widget.NewEntry(),
		videoIndexEntry:   widget.NewSelectEntry([]string{"自动"}),
		audioIndexEntry:   widget.NewSelectEntry([]string{"自动"}),
		dfnEntry:          widget.NewEntry(),
		encodingEntry:     widget.NewEntry(),
		extraArgsEntry:    widget.NewMultiLineEntry(),
		ffmpegPathEntry:   widget.NewEntry(),
		mp4boxPathEntry:   widget.NewEntry(),
		aria2cPathEntry:   widget.NewEntry(),
		modeSelect:        widget.NewSelect(desktopModeOptionsForTest(), nil),
		channelSelect:     widget.NewSelect([]string{"WEB"}, nil),
		danmakuCheck:      widget.NewCheck("", nil),
		skipSubtitleCheck: widget.NewCheck("", nil),
		skipCoverCheck:    widget.NewCheck("", nil),
		skipMuxCheck:      widget.NewCheck("", nil),
		aria2cCheck:       widget.NewCheck("", nil),
		statusLabel:       widget.NewLabel(""),
	}
}

func writeDesktopTestExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func desktopModeOptionsForTest() []string {
	return []string{"下载", "仅查看", "仅视频", "仅音频", "仅封面", "仅字幕", "仅弹幕"}
}

func bytesOfLen(length int) []byte {
	return make([]byte, length)
}
