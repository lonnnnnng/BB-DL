package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lonnnnnng/BB-DL/internal/bbdown"
)

const (
	serverTransferEventPrefix = "__BBDOWN_GO_SERVER_TRANSFER__ "
	serverChildEnv            = "BBDOWN_GO_SERVER_CHILD=1"
	forcePlainTextEnv         = "NO_COLOR=1"
	desktopTaskHistoryFile    = "tasks.json"
	desktopPreferencesFile    = "preferences.json"
	desktopTaskHistoryLimit   = 200
	desktopTaskHistoryLogSize = 256 * 1024
	desktopTaskHistorySaveGap = 5 * time.Second
	desktopRuntimeDirEnv      = "BBDOWN_GO_RUNTIME_DIR"
	desktopToolDirsEnv        = "BBDOWN_GO_TOOL_DIRS"
	desktopLoginShellPath     = "/bin/zsh"
)

const (
	progressStageVideoDownloaded = 0.12
	progressStageAudioDownloaded = 0.35
	progressStageExtraAudio      = 0.55
	progressStagePageDownloaded  = 0.75
	progressStageMuxStarted      = 0.90
)

var (
	pageStartPattern           = regexp.MustCompile(`^开始解析P(\d+): .* \((\d+) of (\d+)\)$`)
	pageVideoStartPattern      = regexp.MustCompile(`^开始下载P(\d+)视频\.\.\.$`)
	pageAudioStartPattern      = regexp.MustCompile(`^开始下载P(\d+)音频\.\.\.$`)
	pageExtraAudioStartPattern = regexp.MustCompile(`^开始下载P(\d+)(背景配音|配音\[.*\])\.\.\.$`)
	clipStartPattern           = regexp.MustCompile(`^开始下载P(\d+)视频, 片段\((\d+)/(\d+)\)\.\.\.$`)
	pageDonePattern            = regexp.MustCompile(`^下载P(\d+)完毕$`)
	muxStartPattern            = regexp.MustCompile(`^开始(合并音视频.*|合并分段|混流视频.*)\.\.\.$`)
)

type taskStatus string

const (
	statusPending  taskStatus = "等待中"
	statusRunning  taskStatus = "下载中"
	statusStopping taskStatus = "停止中"
	statusSuccess  taskStatus = "已完成"
	statusFailed   taskStatus = "失败"
	statusStopped  taskStatus = "已停止"
)

type Preferences struct {
	WorkDir          string `json:"workDir"`
	SelectPage       string `json:"selectPage"`
	VideoIndex       string `json:"videoIndex"`
	AudioIndex       string `json:"audioIndex"`
	DfnPriority      string `json:"dfnPriority"`
	EncodingPriority string `json:"encodingPriority"`
	ExtraArgs        string `json:"extraArgs"`
	FFmpegPath       string `json:"ffmpegPath"`
	MP4BoxPath       string `json:"mp4boxPath"`
	Aria2cPath       string `json:"aria2cPath"`
	FileExistsAction string `json:"fileExistsAction"`
	Mode             string `json:"mode"`
	Channel          string `json:"channel"`
	DownloadDanmaku  bool   `json:"downloadDanmaku"`
	SkipSubtitle     bool   `json:"skipSubtitle"`
	SkipCover        bool   `json:"skipCover"`
	SkipMux          bool   `json:"skipMux"`
	UseAria2c        bool   `json:"useAria2c"`
	AutoQueue        bool   `json:"autoQueue"`
	Theme            string `json:"theme"`
}

type TaskInput struct {
	URL string `json:"url"`
	Preferences
}

type ManagedFile struct {
	Path    string    `json:"path"`
	RelPath string    `json:"relPath"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

type TaskActions struct {
	CanStart  bool `json:"canStart"`
	CanStop   bool `json:"canStop"`
	CanRetry  bool `json:"canRetry"`
	CanDelete bool `json:"canDelete"`
}

type TaskDTO struct {
	ID               int           `json:"id"`
	URL              string        `json:"url"`
	WorkDir          string        `json:"workDir"`
	Mode             string        `json:"mode"`
	Channel          string        `json:"channel"`
	SelectPage       string        `json:"selectPage"`
	VideoIndex       string        `json:"videoIndex"`
	AudioIndex       string        `json:"audioIndex"`
	DfnPriority      string        `json:"dfnPriority"`
	EncodingPriority string        `json:"encodingPriority"`
	ExtraArgs        string        `json:"extraArgs"`
	FFmpegPath       string        `json:"ffmpegPath"`
	MP4BoxPath       string        `json:"mp4boxPath"`
	Aria2cPath       string        `json:"aria2cPath"`
	FileExistsAction string        `json:"fileExistsAction"`
	Args             []string      `json:"args"`
	Status           string        `json:"status"`
	Bytes            int64         `json:"bytes"`
	Progress         float64       `json:"progress"`
	Files            []ManagedFile `json:"files"`
	CreatedAt        time.Time     `json:"createdAt"`
	StartedAt        time.Time     `json:"startedAt"`
	EndedAt          time.Time     `json:"endedAt"`
	CurrentSpeed     float64       `json:"currentSpeed"`
	AverageSpeed     float64       `json:"averageSpeed"`
	ElapsedSeconds   int64         `json:"elapsedSeconds"`
	VideoOptions     []string      `json:"videoOptions"`
	AudioOptions     []string      `json:"audioOptions"`
	Command          string        `json:"command"`
	Actions          TaskActions   `json:"actions"`
}

type QueueSummary struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Running int `json:"running"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
	Stopped int `json:"stopped"`
}

type Bootstrap struct {
	Preferences Preferences  `json:"preferences"`
	Tasks       []TaskDTO    `json:"tasks"`
	Summary     QueueSummary `json:"summary"`
	Version     string       `json:"version"`
	BuildTime   string       `json:"buildTime"`
	Status      string       `json:"status"`
}

type ToolPaths struct {
	FFmpegPath string `json:"ffmpegPath"`
	MP4BoxPath string `json:"mp4boxPath"`
	Aria2cPath string `json:"aria2cPath"`
}

type ToolDiagnostic struct {
	Name        string `json:"name"`
	Found       bool   `json:"found"`
	Path        string `json:"path"`
	Version     string `json:"version"`
	Message     string `json:"message"`
	InstallHint string `json:"installHint"`
}

type VersionInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
}

type transferEvent struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Delta bool   `json:"delta"`
}

type fileSnapshot struct {
	Size    int64
	ModTime time.Time
}

type progressState struct {
	CurrentPage int
	TotalPages  int
	Progress    float64
}

type desktopTask struct {
	ID                int
	URL               string
	WorkDir           string
	Mode              string
	Channel           string
	SelectPage        string
	VideoIndex        string
	AudioIndex        string
	DfnPriority       string
	EncodingPriority  string
	ExtraArgs         string
	FFmpegPath        string
	MP4BoxPath        string
	Aria2cPath        string
	FileExistsAction  string
	Args              []string
	Status            taskStatus
	Bytes             int64
	EventBytes        int64
	FileGrowthBytes   int64
	Progress          float64
	ProgressState     progressState
	Log               strings.Builder
	Files             []ManagedFile
	Baseline          map[string]fileSnapshot
	CreatedAt         time.Time
	StartedAt         time.Time
	EndedAt           time.Time
	RuntimeDir        string
	StopRequested     bool
	TransferPathBytes map[string]int64
	LastTransferAt    time.Time
	LastSpeedBytes    float64
	command           commandHandle
}

type commandHandle interface {
	Kill() error
}

type taskHistory struct {
	Version int          `json:"version"`
	Tasks   []taskRecord `json:"tasks"`
}

type taskRecord struct {
	ID               int           `json:"id"`
	URL              string        `json:"url"`
	WorkDir          string        `json:"workDir"`
	Mode             string        `json:"mode"`
	Channel          string        `json:"channel"`
	SelectPage       string        `json:"selectPage"`
	VideoIndex       string        `json:"videoIndex"`
	AudioIndex       string        `json:"audioIndex"`
	DfnPriority      string        `json:"dfnPriority"`
	EncodingPriority string        `json:"encodingPriority"`
	ExtraArgs        string        `json:"extraArgs"`
	FFmpegPath       string        `json:"ffmpegPath"`
	MP4BoxPath       string        `json:"mp4boxPath"`
	Aria2cPath       string        `json:"aria2cPath"`
	FileExistsAction string        `json:"fileExistsAction"`
	Args             []string      `json:"args"`
	Status           taskStatus    `json:"status"`
	Bytes            int64         `json:"bytes"`
	Progress         float64       `json:"progress"`
	Log              string        `json:"log"`
	Files            []ManagedFile `json:"files"`
	CreatedAt        time.Time     `json:"createdAt"`
	StartedAt        time.Time     `json:"startedAt"`
	EndedAt          time.Time     `json:"endedAt"`
}

type taskArgSwitches struct {
	DownloadDanmaku bool
	SkipSubtitle    bool
	SkipCover       bool
	SkipMux         bool
	UseAria2c       bool
}

func defaultPreferences() Preferences {
	return Preferences{
		WorkDir:          filepath.Join(userHomeDir(), "Downloads", "bilibili"),
		DfnPriority:      "1080P 高清,720P 高清",
		EncodingPriority: "hevc,avc,av1",
		Mode:             "下载",
		Channel:          "WEB",
		FileExistsAction: bbdown.FileExistsActionSkip,
		AutoQueue:        true,
		Theme:            "system",
		FFmpegPath:       detectToolPath("ffmpeg"),
		MP4BoxPath:       detectToolPath("mp4box", "MP4Box", "MP4box"),
		Aria2cPath:       detectToolPath("aria2c"),
	}
}

func taskFromInput(id int, input TaskInput) (*desktopTask, error) {
	input.URL = strings.TrimSpace(input.URL)
	if input.URL == "" {
		return nil, errors.New("请输入视频地址")
	}
	extraArgs, err := splitArgs(input.ExtraArgs)
	if err != nil {
		return nil, err
	}
	fileExistsAction := bbdown.NormalizeFileExistsAction(input.FileExistsAction)
	if err := bbdown.ValidateFileExistsAction(fileExistsAction); err != nil {
		return nil, err
	}
	task := &desktopTask{
		ID: id, URL: input.URL, WorkDir: strings.TrimSpace(input.WorkDir),
		Mode: defaultText(input.Mode, "下载"), Channel: defaultText(input.Channel, "WEB"),
		SelectPage: strings.TrimSpace(input.SelectPage), VideoIndex: streamIndexValue(input.VideoIndex), AudioIndex: streamIndexValue(input.AudioIndex),
		DfnPriority: strings.TrimSpace(input.DfnPriority), EncodingPriority: strings.TrimSpace(input.EncodingPriority),
		ExtraArgs: input.ExtraArgs, FFmpegPath: toolPathValue(input.FFmpegPath), MP4BoxPath: toolPathValue(input.MP4BoxPath), Aria2cPath: toolPathValue(input.Aria2cPath),
		FileExistsAction: fileExistsAction,
		Status:           statusPending, CreatedAt: time.Now(),
	}
	opt := bbdown.MyOption{VideoIndex: task.VideoIndex, AudioIndex: task.AudioIndex, AudioOnly: task.Mode == "仅音频", VideoOnly: task.Mode == "仅视频", OnlyShowInfo: task.Mode == "仅查看"}
	bbdown.NormalizeTrackIndexOptions(&opt)
	if err := bbdown.ValidateTrackIndexSyntax(&opt); err != nil {
		return nil, err
	}
	task.VideoIndex, task.AudioIndex = opt.VideoIndex, opt.AudioIndex
	task.Args = buildTaskArgs(task, extraArgs, taskArgSwitches{
		DownloadDanmaku: input.DownloadDanmaku, SkipSubtitle: input.SkipSubtitle, SkipCover: input.SkipCover,
		SkipMux: input.SkipMux, UseAria2c: input.UseAria2c,
	})
	return task, nil
}

func buildTaskArgs(task *desktopTask, extraArgs []string, switches taskArgSwitches) []string {
	if task == nil {
		return nil
	}
	args := []string{}
	if task.Mode == "仅查看" {
		args = append(args, "info")
	}
	appendPair := func(flag, value string) {
		if value = strings.TrimSpace(value); value != "" {
			args = append(args, flag, value)
		}
	}
	appendPair("--work-dir", task.WorkDir)
	appendPair("--select-page", task.SelectPage)
	appendPair("--dfn-priority", task.DfnPriority)
	appendPair("--encoding-priority", task.EncodingPriority)
	if task.Mode != "仅查看" {
		appendPair("--video-index", task.VideoIndex)
		appendPair("--audio-index", task.AudioIndex)
		appendPair("--ffmpeg-path", toolPathValue(task.FFmpegPath))
		appendPair("--mp4box-path", toolPathValue(task.MP4BoxPath))
		appendPair("--aria2c-path", toolPathValue(task.Aria2cPath))
		appendPair("--file-exists-action", task.FileExistsAction)
	}
	switch task.Channel {
	case "TV":
		args = append(args, "--use-tv-api")
	case "APP":
		args = append(args, "--use-app-api")
	case "国际版":
		args = append(args, "--use-intl-api")
	}
	switch task.Mode {
	case "仅视频":
		args = append(args, "--video-only")
	case "仅音频":
		args = append(args, "--audio-only")
	case "仅封面":
		args = append(args, "--cover-only")
	case "仅字幕":
		args = append(args, "--sub-only")
	case "仅弹幕":
		args = append(args, "--danmaku-only")
	}
	if switches.DownloadDanmaku || task.Mode == "仅弹幕" {
		args = append(args, "--download-danmaku", "--download-danmaku-formats", "xml,ass")
	}
	if switches.SkipSubtitle {
		args = append(args, "--skip-subtitle")
	}
	if switches.SkipCover {
		args = append(args, "--skip-cover")
	}
	if switches.SkipMux {
		args = append(args, "--skip-mux")
	}
	if switches.UseAria2c {
		args = append(args, "--use-aria2c")
	}
	args = append(args, extraArgs...)
	return append(args, task.URL)
}

func rebuildTaskArgs(task *desktopTask) ([]string, error) {
	if task == nil {
		return nil, errors.New("任务不存在")
	}
	if task.Channel == "账号" {
		if strings.Contains(task.Mode+task.URL, "TV") {
			return []string{"logintv"}, nil
		}
		return []string{"login"}, nil
	}
	if strings.TrimSpace(task.URL) == "" {
		return nil, errors.New("任务缺少视频地址，无法重建命令参数")
	}
	extra, err := splitArgs(task.ExtraArgs)
	if err != nil {
		return nil, fmt.Errorf("重建任务参数失败：%w", err)
	}
	return buildTaskArgs(task, extra, taskArgSwitches{
		DownloadDanmaku: boolArgEnabled(task.Args, "--download-danmaku"),
		SkipSubtitle:    boolArgEnabled(task.Args, "--skip-subtitle"), SkipCover: boolArgEnabled(task.Args, "--skip-cover"),
		SkipMux: boolArgEnabled(task.Args, "--skip-mux"), UseAria2c: boolAnyArgEnabled(task.Args, "--use-aria2c", "--aria2"),
	}), nil
}

func taskInputFromTask(task *desktopTask) TaskInput {
	if task == nil {
		return TaskInput{}
	}
	mode := task.Mode
	if mode == "仅查看" {
		mode = "下载"
	}
	return TaskInput{URL: task.URL, Preferences: Preferences{
		WorkDir: task.WorkDir, SelectPage: task.SelectPage, VideoIndex: task.VideoIndex, AudioIndex: task.AudioIndex,
		DfnPriority: task.DfnPriority, EncodingPriority: task.EncodingPriority, ExtraArgs: task.ExtraArgs,
		FFmpegPath: task.FFmpegPath, MP4BoxPath: task.MP4BoxPath, Aria2cPath: task.Aria2cPath, FileExistsAction: task.FileExistsAction,
		Mode: mode, Channel: task.Channel, DownloadDanmaku: task.Mode == "仅弹幕" || boolArgEnabled(task.Args, "--download-danmaku"),
		SkipSubtitle: boolArgEnabled(task.Args, "--skip-subtitle"), SkipCover: boolArgEnabled(task.Args, "--skip-cover"),
		SkipMux: boolArgEnabled(task.Args, "--skip-mux"), UseAria2c: boolAnyArgEnabled(task.Args, "--use-aria2c", "--aria2"), AutoQueue: true,
	}}
}

func (task *desktopTask) cloneForRetry(id int) *desktopTask {
	return &desktopTask{ID: id, URL: task.URL, WorkDir: task.WorkDir, Mode: task.Mode, Channel: task.Channel,
		SelectPage: task.SelectPage, VideoIndex: task.VideoIndex, AudioIndex: task.AudioIndex, DfnPriority: task.DfnPriority,
		EncodingPriority: task.EncodingPriority, ExtraArgs: task.ExtraArgs, FFmpegPath: task.FFmpegPath, MP4BoxPath: task.MP4BoxPath,
		Aria2cPath: task.Aria2cPath, FileExistsAction: task.FileExistsAction, Args: append([]string(nil), task.Args...), Status: statusPending, CreatedAt: time.Now()}
}

func taskHistoryPath() (string, error) {
	dir, err := runtimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, desktopTaskHistoryFile), nil
}

func preferencesPath() (string, error) {
	dir, err := runtimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, desktopPreferencesFile), nil
}

func saveJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func saveTaskHistoryFile(path string, tasks []*desktopTask) error {
	if len(tasks) > desktopTaskHistoryLimit {
		tasks = tasks[len(tasks)-desktopTaskHistoryLimit:]
	}
	history := taskHistory{Version: 1, Tasks: make([]taskRecord, 0, len(tasks))}
	for _, task := range tasks {
		if task != nil {
			history.Tasks = append(history.Tasks, persistTask(task))
		}
	}
	return saveJSONAtomic(path, history)
}

func loadTaskHistoryFile(path string) ([]*desktopTask, int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 1, nil
	}
	if err != nil {
		return nil, 1, err
	}
	var history taskHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, 1, err
	}
	tasks := make([]*desktopTask, 0, len(history.Tasks))
	nextID := 1
	for _, record := range history.Tasks {
		task := restoreTask(record)
		if task.ID <= 0 {
			task.ID = nextID
		}
		if task.ID >= nextID {
			nextID = task.ID + 1
		}
		tasks = append(tasks, task)
	}
	return tasks, nextID, nil
}

func persistTask(task *desktopTask) taskRecord {
	status := task.Status
	if status == statusRunning || status == statusStopping {
		status = statusStopped
	}
	return taskRecord{ID: task.ID, URL: task.URL, WorkDir: task.WorkDir, Mode: task.Mode, Channel: task.Channel,
		SelectPage: task.SelectPage, VideoIndex: task.VideoIndex, AudioIndex: task.AudioIndex, DfnPriority: task.DfnPriority,
		EncodingPriority: task.EncodingPriority, ExtraArgs: task.ExtraArgs, FFmpegPath: task.FFmpegPath, MP4BoxPath: task.MP4BoxPath,
		Aria2cPath: task.Aria2cPath, FileExistsAction: task.FileExistsAction, Args: append([]string(nil), task.Args...), Status: status, Bytes: task.Bytes, Progress: task.Progress,
		Log: trimTaskLog(task.Log.String()), Files: append([]ManagedFile(nil), task.Files...), CreatedAt: task.CreatedAt, StartedAt: task.StartedAt, EndedAt: task.EndedAt}
}

func restoreTask(record taskRecord) *desktopTask {
	status := record.Status
	if status == "" {
		status = statusPending
	}
	if status == statusRunning || status == statusStopping {
		status = statusStopped
	}
	task := &desktopTask{ID: record.ID, URL: record.URL, WorkDir: record.WorkDir, Mode: record.Mode, Channel: record.Channel,
		SelectPage: record.SelectPage, VideoIndex: record.VideoIndex, AudioIndex: record.AudioIndex, DfnPriority: record.DfnPriority,
		EncodingPriority: record.EncodingPriority, ExtraArgs: record.ExtraArgs, FFmpegPath: record.FFmpegPath, MP4BoxPath: record.MP4BoxPath,
		Aria2cPath: record.Aria2cPath, FileExistsAction: bbdown.NormalizeFileExistsAction(record.FileExistsAction), Args: append([]string(nil), record.Args...), Status: status, Bytes: record.Bytes, Progress: record.Progress,
		ProgressState: progressState{Progress: record.Progress}, Files: append([]ManagedFile(nil), record.Files...), CreatedAt: record.CreatedAt,
		StartedAt: record.StartedAt, EndedAt: record.EndedAt}
	task.Log.WriteString(record.Log)
	return task
}

func trimTaskLog(text string) string {
	if len(text) <= desktopTaskHistoryLogSize {
		return text
	}
	marker := fmt.Sprintf("... 日志过长，仅保留最后 %s ...\n", formatBytes(desktopTaskHistoryLogSize))
	start := len(text) - (desktopTaskHistoryLogSize - len(marker))
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return marker + text[start:]
}

func loadPreferencesFile(path string) (Preferences, error) {
	prefs := defaultPreferences()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return prefs, nil
	}
	if err != nil {
		return prefs, err
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return prefs, err
	}
	prefs = normalizePreferences(prefs)
	return prefs, nil
}

func normalizePreferences(prefs Preferences) Preferences {
	defaults := defaultPreferences()
	if strings.TrimSpace(prefs.WorkDir) == "" {
		prefs.WorkDir = defaults.WorkDir
	}
	if strings.TrimSpace(prefs.Mode) == "" {
		prefs.Mode = defaults.Mode
	}
	if strings.TrimSpace(prefs.Channel) == "" {
		prefs.Channel = defaults.Channel
	}
	if strings.TrimSpace(prefs.Theme) == "" {
		prefs.Theme = defaults.Theme
	}
	if strings.TrimSpace(prefs.DfnPriority) == "" {
		prefs.DfnPriority = defaults.DfnPriority
	}
	if strings.TrimSpace(prefs.EncodingPriority) == "" {
		prefs.EncodingPriority = defaults.EncodingPriority
	}
	prefs.FileExistsAction = bbdown.NormalizeFileExistsAction(prefs.FileExistsAction)
	if bbdown.ValidateFileExistsAction(prefs.FileExistsAction) != nil {
		prefs.FileExistsAction = defaults.FileExistsAction
	}
	prefs.FFmpegPath = normalizeToolPath(prefs.FFmpegPath, "ffmpeg")
	prefs.MP4BoxPath = normalizeToolPath(prefs.MP4BoxPath, "mp4box", "MP4Box", "MP4box")
	prefs.Aria2cPath = normalizeToolPath(prefs.Aria2cPath, "aria2c")
	return prefs
}

func applyProgressLine(task *desktopTask, line string) bool {
	if task == nil {
		return false
	}
	state := &task.ProgressState
	line = cleanProgressLine(line)
	if match := pageStartPattern.FindStringSubmatch(line); match != nil {
		current, total := parseInt(match[1]), parseInt(match[3])
		if total > 0 {
			state.TotalPages = total
		}
		if current > 0 {
			state.CurrentPage = current
		}
		return setProgress(task, progressForPageStage(current, state.TotalPages, 0))
	}
	if match := pageVideoStartPattern.FindStringSubmatch(line); match != nil {
		return setProgress(task, progressForPageStage(parseInt(match[1]), state.TotalPages, progressStageVideoDownloaded))
	}
	if match := pageAudioStartPattern.FindStringSubmatch(line); match != nil {
		return setProgress(task, progressForPageStage(parseInt(match[1]), state.TotalPages, progressStageAudioDownloaded))
	}
	if match := pageExtraAudioStartPattern.FindStringSubmatch(line); match != nil {
		return setProgress(task, progressForPageStage(parseInt(match[1]), state.TotalPages, progressStageExtraAudio))
	}
	if match := clipStartPattern.FindStringSubmatch(line); match != nil {
		page, clip, total := parseInt(match[1]), parseInt(match[2]), parseInt(match[3])
		if state.TotalPages <= 0 || total <= 0 {
			return false
		}
		stage := float64(clip) / float64(total+1) * progressStagePageDownloaded
		return setProgress(task, progressForPageStage(page, state.TotalPages, stage))
	}
	if match := pageDonePattern.FindStringSubmatch(line); match != nil {
		page := parseInt(match[1])
		if state.TotalPages == 0 {
			state.TotalPages = page
		}
		return setProgress(task, progressForPageStage(page, state.TotalPages, progressStagePageDownloaded))
	}
	if muxStartPattern.MatchString(line) {
		return setProgress(task, progressForPageStage(state.CurrentPage, state.TotalPages, progressStageMuxStarted))
	}
	if strings.Contains(line, "任务完成") {
		return setProgress(task, 1)
	}
	return false
}

func cleanProgressLine(line string) string {
	line = strings.TrimSpace(line)
	if close := strings.Index(line, "] - "); strings.HasPrefix(line, "[") && close >= 0 {
		return strings.TrimSpace(line[close+4:])
	}
	return line
}

func setProgress(task *desktopTask, value float64) bool {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	if value <= task.ProgressState.Progress {
		return false
	}
	task.ProgressState.Progress, task.Progress = value, value
	return true
}

func progressForPageStage(page, total int, stage float64) float64 {
	if total <= 0 {
		return 0
	}
	if page <= 0 {
		page = 1
	}
	if stage < 0 {
		stage = 0
	}
	if stage > 1 {
		stage = 1
	}
	return (float64(page-1) + stage) / float64(total)
}

func parseTransferEvent(line string) (transferEvent, bool) {
	payload := strings.TrimPrefix(line, serverTransferEventPrefix)
	if payload == line {
		return transferEvent{}, false
	}
	var event transferEvent
	if json.Unmarshal([]byte(payload), &event) != nil || strings.TrimSpace(event.Path) == "" || event.Bytes <= 0 {
		return transferEvent{}, false
	}
	return event, true
}

func applyTransferEvent(task *desktopTask, event transferEvent, observedAt time.Time) bool {
	if task == nil || event.Bytes <= 0 || strings.TrimSpace(event.Path) == "" {
		return false
	}
	if task.TransferPathBytes == nil {
		task.TransferPathBytes = map[string]int64{}
	}
	key := transferPathKey(task, event.Path)
	delta := event.Bytes
	if event.Delta {
		task.TransferPathBytes[key] += delta
	} else {
		growth := event.Bytes - transferBaselineBytes(task, key)
		seen := task.TransferPathBytes[key]
		if growth <= seen || growth <= 0 {
			return false
		}
		delta, task.TransferPathBytes[key] = growth-seen, growth
	}
	task.EventBytes += delta
	task.Bytes = task.EventBytes
	last := task.LastTransferAt
	if last.IsZero() {
		last = task.StartedAt
	}
	if !last.IsZero() && observedAt.After(last) {
		task.LastSpeedBytes = float64(delta) / observedAt.Sub(last).Seconds()
	}
	task.LastTransferAt = observedAt
	return true
}

func applyFileGrowthFallback(task *desktopTask, observedAt time.Time) bool {
	if task == nil || task.WorkDir == "" || task.EventBytes > 0 || len(task.TransferPathBytes) > 0 {
		return false
	}
	growth := totalFileGrowthBytes(task.WorkDir, task.Baseline)
	if growth <= task.FileGrowthBytes {
		return false
	}
	delta := growth - task.FileGrowthBytes
	task.FileGrowthBytes, task.Bytes = growth, growth
	last := task.LastTransferAt
	if last.IsZero() {
		last = task.StartedAt
	}
	if !last.IsZero() && observedAt.After(last) {
		task.LastSpeedBytes = float64(delta) / observedAt.Sub(last).Seconds()
	}
	task.LastTransferAt = observedAt
	return true
}

func totalFileGrowthBytes(root string, baseline map[string]fileSnapshot) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isDownloadMonitorFile(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		abs, _ := filepath.Abs(path)
		if info.Size() > baseline[abs].Size {
			total += info.Size() - baseline[abs].Size
		}
		return nil
	})
	return total
}

func snapshotFiles(root string) map[string]fileSnapshot {
	result := map[string]fileSnapshot{}
	if strings.TrimSpace(root) == "" {
		return result
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		abs, _ := filepath.Abs(path)
		result[abs] = fileSnapshot{Size: info.Size(), ModTime: info.ModTime()}
		return nil
	})
	return result
}

func changedFiles(root string, baseline map[string]fileSnapshot) []ManagedFile {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	files := []ManagedFile{}
	_ = filepath.WalkDir(rootAbs, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		abs, _ := filepath.Abs(path)
		before, existed := baseline[abs]
		if existed && before.Size == info.Size() && before.ModTime.Equal(info.ModTime()) {
			return nil
		}
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			rel = filepath.Base(abs)
		}
		if !isManagedOutputFile(rel) {
			return nil
		}
		files = append(files, ManagedFile{Path: abs, RelPath: rel, Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime.After(files[j].ModTime) })
	if len(files) > 200 {
		files = files[:200]
	}
	return files
}

func isDownloadMonitorFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4a", ".flv", ".jpg", ".jpeg", ".png", ".webp", ".xml", ".ass", ".srt", ".tmp", ".vclip", ".aclip":
		return true
	default:
		return false
	}
}

func isManagedOutputFile(path string) bool {
	switch strings.ToLower(filepath.Base(path)) {
	case "qrcode.png", "bbdown.data", "bbdowntv.data", "bbdownapp.data", "bbdown.config", "bbdown.archives":
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".tmp", ".vclip", ".aclip", ".aria2":
		return false
	}
	return isDownloadMonitorFile(path)
}

func transferPathKey(task *desktopTask, path string) string {
	if task != nil && task.WorkDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(task.WorkDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func transferBaselineBytes(task *desktopTask, key string) int64 {
	if task == nil || task.Baseline == nil {
		return 0
	}
	return task.Baseline[key].Size
}

func streamIndexOptionsFromLog(logText string) ([]string, []string) {
	video, audio, section := []string{}, []string{}, ""
	for _, line := range strings.Split(logText, "\n") {
		switch {
		case strings.Contains(line, "条视频流."):
			section = "video"
			continue
		case strings.Contains(line, "条音频流."):
			section = "audio"
			continue
		case strings.Contains(line, "共计"):
			section = ""
			continue
		}
		option, ok := streamOptionLine(line)
		if !ok {
			continue
		}
		if section == "video" {
			video = append(video, option)
		}
		if section == "audio" {
			audio = append(audio, option)
		}
	}
	return video, audio
}

func streamOptionLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	digits := leadingDigits(trimmed)
	if digits == "" {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, digits))
	if !strings.HasPrefix(rest, ".") {
		return "", false
	}
	return digits + ". " + strings.TrimSpace(strings.TrimPrefix(rest, ".")), true
}

func leadingDigits(value string) string {
	for index, r := range value {
		if r < '0' || r > '9' {
			return value[:index]
		}
	}
	return value
}

func streamIndexValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "自动" {
		return ""
	}
	digits := leadingDigits(value)
	rest := strings.TrimSpace(strings.TrimPrefix(value, digits))
	if digits != "" && (rest == "" || strings.HasPrefix(rest, ".")) {
		return digits
	}
	return value
}

func boolArgEnabled(args []string, flag string) bool { return boolAnyArgEnabled(args, flag) }

func boolAnyArgEnabled(args []string, flags ...string) bool {
	enabled := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		for _, flag := range flags {
			if arg == flag {
				if index+1 < len(args) {
					if value, ok := parseBool(args[index+1]); ok {
						enabled = value
						index++
						break
					}
				}
				enabled = true
			}
			if strings.HasPrefix(arg, flag+"=") {
				enabled, _ = parseBool(strings.TrimPrefix(arg, flag+"="))
			}
		}
	}
	return enabled
}

func parseBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

func taskNeedsMuxTool(args []string) bool {
	return !(boolArgEnabled(args, "--skip-mux") || taskOnlyShowsInfo(args) || boolArgEnabled(args, "--cover-only") || boolArgEnabled(args, "--danmaku-only") || boolArgEnabled(args, "--sub-only") || boolArgEnabled(args, "--audio-only") || boolArgEnabled(args, "--video-only"))
}

func taskNeedsAria2cTool(args []string) bool {
	return boolAnyArgEnabled(args, "--use-aria2c", "--aria2") && !taskOnlyShowsInfo(args)
}
func taskOnlyShowsInfo(args []string) bool {
	return len(args) > 0 && args[0] == "info" || boolAnyArgEnabled(args, "--only-show-info", "--info")
}

func effectiveValueFlag(args []string, flag, fallback string) string {
	value := strings.TrimSpace(fallback)
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == flag && index+1 < len(args) {
			value = strings.TrimSpace(args[index+1])
			index++
		}
		if strings.HasPrefix(arg, flag+"=") {
			value = strings.TrimSpace(strings.TrimPrefix(arg, flag+"="))
		}
	}
	return value
}

func upsertValueFlag(args []string, flag, value string) []string {
	cleaned := make([]string, 0, len(args)+2)
	for index := 0; index < len(args); index++ {
		if args[index] == flag {
			index++
			continue
		}
		if strings.HasPrefix(args[index], flag+"=") {
			continue
		}
		cleaned = append(cleaned, args[index])
	}
	insert := len(cleaned)
	if insert > 0 {
		insert--
	}
	cleaned = append(cleaned, "", "")
	copy(cleaned[insert+2:], cleaned[insert:])
	cleaned[insert], cleaned[insert+1] = flag, value
	return cleaned
}

func splitArgs(input string) ([]string, error) {
	args := []string{}
	var current strings.Builder
	var quote rune
	started := false
	runes := []rune(input)
	for index := 0; index < len(runes); index++ {
		r := runes[index]
		switch {
		case r == '\\':
			if index+1 < len(runes) && splitArgBackslashEscapes(runes[index+1], quote) {
				index++
				current.WriteRune(runes[index])
			} else {
				current.WriteRune(r)
			}
			started = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
				started = true
			}
		case r == '\'' || r == '"':
			quote = r
			started = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if started {
				args = append(args, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, errors.New("额外参数引号未闭合")
	}
	if started {
		args = append(args, current.String())
	}
	return args, nil
}

func splitArgBackslashEscapes(next, quote rune) bool {
	if quote != 0 {
		return next == quote || next == '\\'
	}
	return next == '\\' || next == '\'' || next == '"' || next == ' ' || next == '\t' || next == '\n' || next == '\r'
}

func shellJoin(args []string) string {
	items := make([]string, 0, len(args))
	for _, arg := range args {
		items = append(items, shellQuoteForOS(runtime.GOOS, arg))
	}
	return strings.Join(items, " ")
}
func shellQuoteForOS(goos, value string) string {
	if goos == "windows" {
		return windowsShellQuote(value)
	}
	return unixShellQuote(value)
}
func unixShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if shellSafeArg(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
func windowsShellQuote(value string) string {
	if value == "" {
		return `""`
	}
	if shellSafeArg(value) {
		return value
	}
	var b strings.Builder
	b.WriteByte('"')
	slash := 0
	for _, r := range value {
		if r == '\\' {
			slash++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat(`\`, slash*2+1))
			b.WriteRune(r)
			slash = 0
			continue
		}
		if slash > 0 {
			b.WriteString(strings.Repeat(`\`, slash))
			slash = 0
		}
		b.WriteRune(r)
	}
	if slash > 0 {
		b.WriteString(strings.Repeat(`\`, slash*2))
	}
	b.WriteByte('"')
	return b.String()
}
func shellSafeArg(value string) bool {
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_@%+=:,./-", r) {
			continue
		}
		return false
	}
	return true
}
func defaultText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func parseInt(value string) int { result, _ := strconv.Atoi(strings.TrimSpace(value)); return result }
func formatBytes(value int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	size := float64(value)
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", value, units[unit])
	}
	return fmt.Sprintf("%.2f %s", size, units[unit])
}
func currentTaskSpeed(task *desktopTask, now time.Time) float64 {
	if task == nil || task.Status != statusRunning && task.Status != statusStopping || task.LastTransferAt.IsZero() || now.Sub(task.LastTransferAt) > 3*time.Second {
		return 0
	}
	return task.LastSpeedBytes
}
func averageTaskSpeed(task *desktopTask, now time.Time) float64 {
	if task == nil || task.Bytes <= 0 || task.StartedAt.IsZero() {
		return 0
	}
	end := now
	if !task.EndedAt.IsZero() {
		end = task.EndedAt
	}
	elapsed := end.Sub(task.StartedAt).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(task.Bytes) / elapsed
}
func taskElapsed(task *desktopTask, now time.Time) time.Duration {
	if task == nil || task.StartedAt.IsZero() {
		return 0
	}
	if !task.EndedAt.IsZero() {
		return task.EndedAt.Sub(task.StartedAt)
	}
	return now.Sub(task.StartedAt)
}
func isActiveStatus(status taskStatus) bool {
	return status == statusPending || status == statusRunning || status == statusStopping
}
func isRetryableStatus(status taskStatus) bool {
	return status == statusFailed || status == statusStopped
}
func shouldSaveTaskHistory(last, now time.Time) bool {
	return last.IsZero() || now.Before(last) || now.Sub(last) >= desktopTaskHistorySaveGap
}
