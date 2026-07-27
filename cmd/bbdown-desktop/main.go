package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/lonnnnnng/BB-DL/internal/bbdown"
)

const (
	appID                     = "com.lonnnnnng.bbdown-go.desktop"
	serverTransferEventPrefix = "__BBDOWN_GO_SERVER_TRANSFER__ "
	serverChildEnv            = "BBDOWN_GO_SERVER_CHILD=1"
	forcePlainTextEnv         = "NO_COLOR=1"
	desktopTaskHistoryFile    = "tasks.json"
	desktopTaskHistoryLimit   = 200
	desktopTaskHistoryLogSize = 256 * 1024
	desktopTaskHistorySaveGap = 5 * time.Second
	desktopRuntimeDirEnv      = "BBDOWN_GO_RUNTIME_DIR"
	desktopToolDirsEnv        = "BBDOWN_GO_TOOL_DIRS"
)

var (
	desktopPageStartPattern           = regexp.MustCompile(`^开始解析P(\d+): .* \((\d+) of (\d+)\)$`)
	desktopPageVideoStartPattern      = regexp.MustCompile(`^开始下载P(\d+)视频\.\.\.$`)
	desktopPageAudioStartPattern      = regexp.MustCompile(`^开始下载P(\d+)音频\.\.\.$`)
	desktopPageExtraAudioStartPattern = regexp.MustCompile(`^开始下载P(\d+)(背景配音|配音\[.*\])\.\.\.$`)
	desktopClipStartPattern           = regexp.MustCompile(`^开始下载P(\d+)视频, 片段\((\d+)/(\d+)\)\.\.\.$`)
	desktopPageDonePattern            = regexp.MustCompile(`^下载P(\d+)完毕$`)
	desktopMuxStartPattern            = regexp.MustCompile(`^开始(合并音视频.*|合并分段|混流视频.*)\.\.\.$`)
	desktopLoginShellPath             = "/bin/zsh"
)

const (
	desktopProgressStageVideoDownloaded = 0.12
	desktopProgressStageAudioDownloaded = 0.35
	desktopProgressStageExtraAudio      = 0.55
	desktopProgressStagePageDownloaded  = 0.75
	desktopProgressStageMuxStarted      = 0.90
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

type transferEvent struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Delta bool   `json:"delta"`
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
	Args              []string
	Status            taskStatus
	Bytes             int64
	EventBytes        int64
	FileGrowthBytes   int64
	Progress          float64
	ProgressState     desktopProgressState
	Log               strings.Builder
	Files             []managedFile
	Baseline          map[string]fileSnapshot
	CreatedAt         time.Time
	StartedAt         time.Time
	EndedAt           time.Time
	RuntimeDir        string
	StopRequested     bool
	Command           *exec.Cmd
	TransferPathBytes map[string]int64
	LastTransferAt    time.Time
	LastSpeedBytes    float64
}

type desktopTaskHistory struct {
	Version int                    `json:"version"`
	Tasks   []persistedDesktopTask `json:"tasks"`
}

type persistedDesktopTask struct {
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
	Args             []string      `json:"args"`
	Status           taskStatus    `json:"status"`
	Bytes            int64         `json:"bytes"`
	Progress         float64       `json:"progress"`
	Log              string        `json:"log"`
	Files            []managedFile `json:"files"`
	CreatedAt        time.Time     `json:"createdAt"`
	StartedAt        time.Time     `json:"startedAt"`
	EndedAt          time.Time     `json:"endedAt"`
}

type managedFile struct {
	Path    string
	RelPath string
	Size    int64
	ModTime time.Time
}

type fileSnapshot struct {
	Size    int64
	ModTime time.Time
}

type desktopProgressState struct {
	CurrentPage int
	TotalPages  int
	Progress    float64
}

type desktopState struct {
	window fyne.Window

	urlEntry          *widget.Entry
	workDirEntry      *widget.Entry
	pageEntry         *widget.Entry
	videoIndexEntry   *widget.SelectEntry
	audioIndexEntry   *widget.SelectEntry
	dfnEntry          *widget.Entry
	encodingEntry     *widget.Entry
	extraArgsEntry    *widget.Entry
	ffmpegPathEntry   *widget.Entry
	mp4boxPathEntry   *widget.Entry
	aria2cPathEntry   *widget.Entry
	modeSelect        *widget.Select
	channelSelect     *widget.Select
	danmakuCheck      *widget.Check
	skipSubtitleCheck *widget.Check
	skipCoverCheck    *widget.Check
	skipMuxCheck      *widget.Check
	aria2cCheck       *widget.Check
	autoQueueCheck    *widget.Check

	taskList        *widget.List
	fileList        *widget.List
	summaryLabel    *widget.Label
	statusLabel     *widget.Label
	taskTitleLabel  *widget.Label
	taskMetaLabel   *widget.Label
	taskStatusLabel *widget.Label
	bytesLabel      *widget.Label
	speedLabel      *widget.Label
	elapsedLabel    *widget.Label
	logEntry        *widget.TextGrid
	logFollowCheck  *widget.Check
	progress        *widget.ProgressBar
	qrImage         *canvas.Image
	createButton    *widget.Button
	startButton     *widget.Button
	nextButton      *widget.Button
	fillFormBtn     *widget.Button
	stopButton      *widget.Button
	retryButton     *widget.Button
	retryFailedBtn  *widget.Button
	removeButton    *widget.Button
	clearEndedBtn   *widget.Button
	openFileBtn     *widget.Button
	revealFileBtn   *widget.Button
	copyFilePathBtn *widget.Button
	copyAllFilesBtn *widget.Button
	copyCommandBtn  *widget.Button
	copyLogBtn      *widget.Button
	copyDoctorBtn   *widget.Button
	checkToolsBtn   *widget.Button

	mu                sync.Mutex
	tasks             []*desktopTask
	nextID            int
	selectedTaskIndex int
	selectedFileIndex int
	runningTask       *desktopTask
	lastHistorySave   time.Time
	renderedLogTaskID int
	renderedLogBytes  int
	uiRefreshPending  bool
}

func main() {
	a := app.NewWithID(appID)
	w := a.NewWindow("BB-DL")
	w.Resize(fyne.NewSize(1280, 800))

	state := newDesktopState(w)
	w.SetContent(state.content())
	w.SetCloseIntercept(func() {
		if state.hasRunningTask() {
			dialog.ShowConfirm("任务仍在运行", "退出会停止当前任务，确定退出吗？", func(ok bool) {
				if ok {
					state.stopRunningTask()
					w.Close()
				}
			}, w)
			return
		}
		w.Close()
	})
	w.ShowAndRun()
}

func newDesktopState(w fyne.Window) *desktopState {
	state := &desktopState{
		window:            w,
		nextID:            1,
		selectedTaskIndex: -1,
		selectedFileIndex: -1,
		renderedLogTaskID: -1,
	}

	state.urlEntry = widget.NewEntry()
	state.urlEntry.SetPlaceHolder("BV / av / ep / URL")
	state.urlEntry.Validator = func(text string) error {
		if strings.TrimSpace(text) == "" {
			return errors.New("请输入视频地址")
		}
		return nil
	}
	state.urlEntry.OnSubmitted = func(string) { state.createTask(true) }
	state.workDirEntry = widget.NewEntry()
	state.workDirEntry.SetText(filepath.Join(userHomeDir(), "Downloads", "bilibili"))
	state.pageEntry = widget.NewEntry()
	state.pageEntry.SetPlaceHolder("例如 1,2,LAST")
	state.videoIndexEntry = widget.NewSelectEntry([]string{"自动"})
	state.videoIndexEntry.SetPlaceHolder("留空自动，例如 0")
	state.audioIndexEntry = widget.NewSelectEntry([]string{"自动"})
	state.audioIndexEntry.SetPlaceHolder("留空自动，例如 0")
	state.dfnEntry = widget.NewEntry()
	state.dfnEntry.SetText("1080P 高清,720P 高清")
	state.encodingEntry = widget.NewEntry()
	state.encodingEntry.SetText("hevc,avc,av1")
	state.ffmpegPathEntry = widget.NewEntry()
	state.ffmpegPathEntry.SetPlaceHolder("自动探测，或填写 /opt/homebrew/bin/ffmpeg")
	state.ffmpegPathEntry.SetText(detectToolPath("ffmpeg"))
	state.mp4boxPathEntry = widget.NewEntry()
	state.mp4boxPathEntry.SetPlaceHolder("可选，例如 /opt/homebrew/bin/MP4Box")
	state.mp4boxPathEntry.SetText(detectToolPath("mp4box", "MP4Box"))
	state.aria2cPathEntry = widget.NewEntry()
	state.aria2cPathEntry.SetPlaceHolder("可选，例如 /opt/homebrew/bin/aria2c")
	state.aria2cPathEntry.SetText(detectToolPath("aria2c"))
	state.extraArgsEntry = widget.NewMultiLineEntry()
	state.extraArgsEntry.SetPlaceHolder("额外 CLI 参数，例如 --file-pattern '<videoTitle>[<dfn>]'")
	state.extraArgsEntry.SetMinRowsVisible(3)

	state.modeSelect = widget.NewSelect([]string{"下载", "仅查看", "仅视频", "仅音频", "仅封面", "仅字幕", "仅弹幕"}, nil)
	state.modeSelect.SetSelected("下载")
	state.channelSelect = widget.NewSelect([]string{"WEB", "TV", "APP", "国际版"}, nil)
	state.channelSelect.SetSelected("WEB")
	state.danmakuCheck = widget.NewCheck("下载弹幕 XML/ASS", nil)
	state.skipSubtitleCheck = widget.NewCheck("跳过字幕", nil)
	state.skipCoverCheck = widget.NewCheck("跳过封面", nil)
	state.skipMuxCheck = widget.NewCheck("跳过混流", nil)
	state.aria2cCheck = widget.NewCheck("使用 aria2c", nil)
	state.autoQueueCheck = widget.NewCheck("任务完成后自动开始下一个", nil)
	state.autoQueueCheck.SetChecked(true)

	state.statusLabel = widget.NewLabel("就绪")
	state.statusLabel.Wrapping = fyne.TextWrapWord
	state.statusLabel.Importance = widget.LowImportance
	state.summaryLabel = widget.NewLabel("任务 0")
	state.summaryLabel.Alignment = fyne.TextAlignCenter
	state.taskTitleLabel = widget.NewLabelWithStyle("未选择任务", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	state.taskTitleLabel.Truncation = fyne.TextTruncateEllipsis
	state.taskMetaLabel = widget.NewLabel("")
	state.taskMetaLabel.Wrapping = fyne.TextWrapWord
	state.taskMetaLabel.Importance = widget.LowImportance
	state.taskStatusLabel = widget.NewLabel("状态 · 未选择")
	state.taskStatusLabel.TextStyle = fyne.TextStyle{Bold: true}
	state.taskStatusLabel.Importance = widget.LowImportance
	state.bytesLabel = widget.NewLabel("已下载 0 B")
	state.speedLabel = widget.NewLabel("速度 0 B/s")
	state.elapsedLabel = widget.NewLabel("耗时 00:00")
	state.logEntry = widget.NewTextGrid()
	state.logEntry.Scroll = fyne.ScrollBoth
	state.logFollowCheck = widget.NewCheck("跟随末尾", nil)
	state.logFollowCheck.SetChecked(true)
	state.progress = widget.NewProgressBar()
	state.progress.TextFormatter = func() string {
		task := state.selectedTask()
		if task == nil {
			return "0%"
		}
		return progressPercentText(task.Progress)
	}
	state.progress.Hide()
	state.qrImage = canvas.NewImageFromFile("")
	state.qrImage.FillMode = canvas.ImageFillContain
	state.qrImage.SetMinSize(fyne.NewSize(160, 160))
	state.qrImage.Hide()

	state.taskList = widget.NewList(
		func() int { return len(state.tasks) },
		func() fyne.CanvasObject {
			title := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			title.Truncation = fyne.TextTruncateEllipsis
			status := widget.NewLabel("")
			status.TextStyle = fyne.TextStyle{Bold: true}
			meta := widget.NewLabel("")
			meta.Alignment = fyne.TextAlignTrailing
			meta.Truncation = fyne.TextTruncateEllipsis
			progress := widget.NewProgressBar()
			return container.NewVBox(title, container.NewGridWithColumns(2, status, meta), progress)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			box := obj.(*fyne.Container)
			title := box.Objects[0].(*widget.Label)
			statusRow := box.Objects[1].(*fyne.Container)
			status := statusRow.Objects[0].(*widget.Label)
			meta := statusRow.Objects[1].(*widget.Label)
			progress := box.Objects[2].(*widget.ProgressBar)
			task := state.tasks[id]
			title.SetText(fmt.Sprintf("#%d %s", task.ID, taskDisplayTitle(task)))
			speed := "-"
			if task.Status == statusRunning || task.Status == statusStopping {
				speed = formatBytes(int64(currentTaskSpeed(task, time.Now()))) + "/s"
			}
			status.SetText(string(task.Status))
			status.Importance = taskStatusImportance(task.Status)
			status.Refresh()
			meta.SetText(fmt.Sprintf("%s · %s · %s", task.Mode, formatBytes(task.Bytes), speed))
			progress.SetValue(task.Progress)
		},
	)
	state.taskList.OnSelected = func(id widget.ListItemID) {
		state.selectedTaskIndex = id
		state.selectedFileIndex = -1
		state.refreshDetails()
	}

	state.fileList = widget.NewList(
		func() int {
			task := state.selectedTask()
			if task == nil {
				return 0
			}
			return len(task.Files)
		},
		func() fyne.CanvasObject {
			name := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			meta := widget.NewLabel("")
			return container.NewVBox(name, meta)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			task := state.selectedTask()
			if task == nil || id >= len(task.Files) {
				return
			}
			file := task.Files[id]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(file.RelPath)
			box.Objects[1].(*widget.Label).SetText(fmt.Sprintf("%s · %s", formatBytes(file.Size), file.ModTime.Format("2006-01-02 15:04:05")))
		},
	)
	state.fileList.OnSelected = func(id widget.ListItemID) {
		state.selectedFileIndex = id
		state.refreshActions()
	}

	state.createButton = widget.NewButtonWithIcon("加入队列", theme.ContentAddIcon(), func() { state.createTask(false) })
	state.createButton.Importance = widget.LowImportance
	state.startButton = widget.NewButtonWithIcon("开始任务", theme.MediaPlayIcon(), state.startSelected)
	state.startButton.Importance = widget.HighImportance
	state.nextButton = widget.NewButtonWithIcon("开始下一个", theme.NavigateNextIcon(), state.startNextPending)
	state.fillFormBtn = widget.NewButtonWithIcon("填入下载表单", theme.ContentPasteIcon(), state.fillFormFromSelected)
	state.fillFormBtn.Importance = widget.LowImportance
	state.stopButton = widget.NewButtonWithIcon("停止", theme.MediaStopIcon(), state.stopSelected)
	state.stopButton.Importance = widget.DangerImportance
	state.retryButton = widget.NewButtonWithIcon("重新执行", theme.ViewRefreshIcon(), state.retrySelected)
	state.retryFailedBtn = widget.NewButtonWithIcon("重试失败/停止", theme.ViewRefreshIcon(), state.retryFailedTasks)
	state.removeButton = widget.NewButtonWithIcon("删除任务", theme.DeleteIcon(), state.confirmRemoveSelected)
	state.removeButton.Importance = widget.DangerImportance
	state.clearEndedBtn = widget.NewButtonWithIcon("清理已结束", theme.DeleteIcon(), state.confirmClearEndedTasks)
	state.clearEndedBtn.Importance = widget.DangerImportance
	state.openFileBtn = widget.NewButtonWithIcon("打开文件", theme.FileIcon(), state.openSelectedFile)
	state.revealFileBtn = widget.NewButtonWithIcon("定位文件", theme.FolderOpenIcon(), state.revealSelectedFile)
	state.copyFilePathBtn = widget.NewButtonWithIcon("复制路径", theme.ContentCopyIcon(), state.copySelectedFilePath)
	state.copyAllFilesBtn = widget.NewButtonWithIcon("复制全部", theme.ContentCopyIcon(), state.copyAllFilePaths)
	state.copyCommandBtn = widget.NewButtonWithIcon("复制命令", theme.ContentCopyIcon(), state.copySelectedCommand)
	state.copyLogBtn = widget.NewButtonWithIcon("复制日志", theme.ContentCopyIcon(), state.copySelectedLog)
	state.copyDoctorBtn = widget.NewButtonWithIcon("复制诊断", theme.ContentCopyIcon(), state.copyDoctorReport)
	state.checkToolsBtn = widget.NewButtonWithIcon("检测工具", theme.ViewRefreshIcon(), state.checkToolPaths)
	state.loadPreferences()
	state.refreshToolPathEntries()
	if err := state.loadTaskHistory(); err != nil {
		state.setStatus("加载任务历史失败：" + err.Error())
	}
	state.refreshActions()
	state.startRuntimeTicker()
	return state
}

func (s *desktopState) content() fyne.CanvasObject {
	chooseButton := widget.NewButtonWithIcon("选择", theme.FolderOpenIcon(), s.chooseWorkDir)
	openDirButton := widget.NewButtonWithIcon("打开保存目录", theme.FolderOpenIcon(), s.openWorkDir)
	createStartButton := widget.NewButtonWithIcon("创建并开始", theme.MediaPlayIcon(), func() { s.createTask(true) })
	openDirButton.Importance = widget.LowImportance
	createStartButton.Importance = widget.HighImportance
	loginButton := widget.NewButtonWithIcon("WEB 登录", theme.LoginIcon(), func() { s.createLoginTask("WEB 登录", "login") })
	loginTVButton := widget.NewButtonWithIcon("TV 登录", theme.LoginIcon(), func() { s.createLoginTask("TV 登录", "logintv") })

	appTitle := widget.NewLabelWithStyle("BB-DL", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	appTitle.SizeName = theme.SizeNameHeadingText
	header := container.NewBorder(
		nil,
		nil,
		appTitle,
		container.NewHBox(loginButton, loginTVButton, openDirButton),
		s.summaryLabel,
	)

	basicForm := widget.NewForm(
		widget.NewFormItem("视频地址", s.urlEntry),
		widget.NewFormItem("保存目录", container.NewBorder(nil, nil, nil, chooseButton, s.workDirEntry)),
		widget.NewFormItem("模式", s.modeSelect),
		widget.NewFormItem("接口", s.channelSelect),
	)
	downloadOptions := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("分 P", s.pageEntry),
			widget.NewFormItem("视频序号", s.videoIndexEntry),
			widget.NewFormItem("音频序号", s.audioIndexEntry),
			widget.NewFormItem("清晰度", s.dfnEntry),
			widget.NewFormItem("编码", s.encodingEntry),
		),
		container.NewAdaptiveGrid(2, s.danmakuCheck, s.skipSubtitleCheck, s.skipCoverCheck, s.skipMuxCheck, s.aria2cCheck, s.autoQueueCheck),
	)
	advancedOptions := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("FFmpeg", s.ffmpegPathEntry),
			widget.NewFormItem("MP4Box", s.mp4boxPathEntry),
			widget.NewFormItem("aria2c", s.aria2cPathEntry),
			widget.NewFormItem("额外参数", s.extraArgsEntry),
		),
		s.checkToolsBtn,
	)
	settings := widget.NewAccordion(
		widget.NewAccordionItem("下载设置", downloadOptions),
		widget.NewAccordionItem("工具与高级参数", advancedOptions),
	)
	settings.Open(0)
	createPanel := container.NewVBox(
		widget.NewLabelWithStyle("新建任务", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		basicForm,
		container.NewGridWithColumns(2, s.createButton, createStartButton),
		settings,
	)
	left := container.NewPadded(container.NewVScroll(createPanel))

	taskHeader := container.NewVBox(
		widget.NewLabelWithStyle("任务队列", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2, s.startButton, s.stopButton),
		container.NewGridWithColumns(2, s.nextButton, s.retryButton),
		container.NewGridWithColumns(2, s.fillFormBtn, s.retryFailedBtn),
	)
	taskFooter := container.NewVBox(widget.NewSeparator(), container.NewGridWithColumns(2, s.removeButton, s.clearEndedBtn))
	tasks := container.NewPadded(container.NewBorder(taskHeader, taskFooter, nil, nil, s.taskList))

	overview := container.NewVBox(
		s.taskTitleLabel,
		s.taskMetaLabel,
		s.progress,
		container.NewGridWithColumns(2, s.taskStatusLabel, s.bytesLabel, s.speedLabel, s.elapsedLabel),
		s.qrImage,
	)
	filesBox := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("完成文件", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewGridWithColumns(4, s.openFileBtn, s.revealFileBtn, s.copyFilePathBtn, s.copyAllFilesBtn),
		),
		nil,
		nil,
		nil,
		s.fileList,
	)
	logBox := container.NewBorder(
		container.NewVBox(
			container.NewGridWithColumns(4, s.logFollowCheck, s.copyCommandBtn, s.copyLogBtn, s.copyDoctorBtn),
			widget.NewLabelWithStyle("运行日志", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
		nil,
		nil,
		nil,
		s.logEntry,
	)
	detailsTabs := container.NewAppTabs(
		container.NewTabItem("日志", logBox),
		container.NewTabItem("文件", filesBox),
	)
	details := container.NewPadded(container.NewBorder(overview, nil, nil, nil, detailsTabs))

	rightSplit := container.NewHSplit(tasks, details)
	rightSplit.Offset = 0.42
	mainSplit := container.NewHSplit(left, rightSplit)
	mainSplit.Offset = 0.30
	statusBar := container.NewBorder(nil, nil, widget.NewIcon(theme.InfoIcon()), nil, s.statusLabel)
	return container.NewBorder(
		container.NewVBox(container.NewPadded(header), widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), container.NewPadded(statusBar)),
		nil,
		nil,
		mainSplit,
	)
}

func (s *desktopState) chooseWorkDir() {
	dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			s.appendGlobalLog("选择目录失败：" + err.Error() + "\n")
			return
		}
		if uri != nil {
			s.workDirEntry.SetText(uri.Path())
			s.savePreferences()
		}
	}, s.window).Show()
}

func (s *desktopState) openWorkDir() {
	path, err := ensureWorkDir(s.workDirEntry.Text)
	if err != nil {
		s.setStatus(err.Error())
		return
	}
	openPath(path)
	s.setStatus("已打开保存目录")
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
	// long 2026-06-27 14:22:00：首次使用桌面版时保存目录可能还没被下载任务创建；打开目录按钮应先准备好目标目录，避免系统 open 静默失败。
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("创建保存目录失败：%w", err)
	}
	return path, nil
}

func (s *desktopState) createTask(startNow bool) {
	url := strings.TrimSpace(s.urlEntry.Text)
	if url == "" {
		s.urlEntry.SetValidationError(errors.New("请输入视频地址"))
		if s.window != nil {
			s.window.Canvas().Focus(s.urlEntry)
		}
		s.setStatus("请输入视频地址")
		return
	}
	s.urlEntry.SetValidationError(nil)
	task, err := s.taskFromForm(url)
	if err != nil {
		s.setStatus(err.Error())
		return
	}
	s.addTask(task)
	if startNow {
		s.startTask(task)
	}
}

func (s *desktopState) createLoginTask(title, command string) {
	task := &desktopTask{
		ID:        s.nextTaskID(),
		URL:       title,
		Mode:      title,
		Channel:   "账号",
		WorkDir:   strings.TrimSpace(s.workDirEntry.Text),
		Args:      []string{command},
		Status:    statusPending,
		CreatedAt: time.Now(),
	}
	s.addTask(task)
	s.startTask(task)
}

func (s *desktopState) taskFromForm(url string) (*desktopTask, error) {
	extraArgs, err := splitArgs(s.extraArgsEntry.Text)
	if err != nil {
		return nil, err
	}
	task := &desktopTask{
		ID:               s.nextTaskID(),
		URL:              url,
		WorkDir:          strings.TrimSpace(s.workDirEntry.Text),
		Mode:             defaultText(s.modeSelect.Selected, "下载"),
		Channel:          defaultText(s.channelSelect.Selected, "WEB"),
		SelectPage:       strings.TrimSpace(s.pageEntry.Text),
		VideoIndex:       streamIndexValue(s.videoIndexEntry.Text),
		AudioIndex:       streamIndexValue(s.audioIndexEntry.Text),
		DfnPriority:      strings.TrimSpace(s.dfnEntry.Text),
		EncodingPriority: strings.TrimSpace(s.encodingEntry.Text),
		ExtraArgs:        s.extraArgsEntry.Text,
		FFmpegPath:       desktopToolPathValue(s.ffmpegPathEntry.Text),
		MP4BoxPath:       desktopToolPathValue(s.mp4boxPathEntry.Text),
		Aria2cPath:       desktopToolPathValue(s.aria2cPathEntry.Text),
		Status:           statusPending,
		CreatedAt:        time.Now(),
	}
	if err := normalizeAndValidateDesktopTaskStreamIndexes(task); err != nil {
		return nil, err
	}
	s.syncStreamIndexEntriesFromTask(task)
	task.Args = s.buildArgs(task, extraArgs)
	if err := s.resolveTaskToolPaths(task); err != nil {
		return nil, err
	}
	s.savePreferences()
	return task, nil
}

func normalizeAndValidateDesktopTaskStreamIndexes(task *desktopTask) error {
	if task == nil {
		return nil
	}
	opt := bbdown.MyOption{
		VideoIndex:   task.VideoIndex,
		AudioIndex:   task.AudioIndex,
		AudioOnly:    task.Mode == "仅音频",
		VideoOnly:    task.Mode == "仅视频",
		OnlyShowInfo: task.Mode == "仅查看",
	}
	bbdown.NormalizeTrackIndexOptions(&opt)
	task.VideoIndex = opt.VideoIndex
	task.AudioIndex = opt.AudioIndex
	return bbdown.ValidateTrackIndexSyntax(&opt)
}

func (s *desktopState) syncStreamIndexEntriesFromTask(task *desktopTask) {
	if task == nil {
		return
	}
	setStreamIndexEntryFromTask(s.videoIndexEntry, task.VideoIndex, false)
	setStreamIndexEntryFromTask(s.audioIndexEntry, task.AudioIndex, false)
}

func (s *desktopState) buildArgs(task *desktopTask, extraArgs []string) []string {
	return buildDesktopTaskArgs(task, extraArgs, desktopTaskArgSwitches{
		DownloadDanmaku: s.danmakuCheck != nil && s.danmakuCheck.Checked,
		SkipSubtitle:    s.skipSubtitleCheck != nil && s.skipSubtitleCheck.Checked,
		SkipCover:       s.skipCoverCheck != nil && s.skipCoverCheck.Checked,
		SkipMux:         s.skipMuxCheck != nil && s.skipMuxCheck.Checked,
		UseAria2c:       s.aria2cCheck != nil && s.aria2cCheck.Checked,
	})
}

func (s *desktopState) nextTaskID() int {
	id := s.nextID
	s.nextID++
	return id
}

func (s *desktopState) addTask(task *desktopTask) {
	s.tasks = append(s.tasks, task)
	s.selectedTaskIndex = len(s.tasks) - 1
	s.taskList.Refresh()
	s.taskList.Select(s.selectedTaskIndex)
	s.refreshDetails()
	s.saveTaskHistory()
	s.setStatus(fmt.Sprintf("任务 #%d 已加入队列", task.ID))
}

func (s *desktopState) startSelected() {
	task := s.selectedTask()
	if task == nil {
		return
	}
	s.startTask(task)
}

func (s *desktopState) retrySelected() {
	task := s.selectedTask()
	if task == nil || s.runningTask != nil {
		return
	}
	retry := task.cloneForRetry(s.nextTaskID())
	s.addTask(retry)
	s.startTask(retry)
}

func (s *desktopState) retryFailedTasks() {
	if s.runningTask != nil {
		s.setStatus("已有任务运行中，失败任务稍后再重试")
		return
	}
	retryable := retryableDesktopTasks(s.tasks)
	if len(retryable) == 0 {
		s.setStatus("没有失败或已停止任务可重试")
		return
	}
	firstRetryIndex := len(s.tasks)
	for _, task := range retryable {
		s.tasks = append(s.tasks, task.cloneForRetry(s.nextTaskID()))
	}
	s.selectedTaskIndex = firstRetryIndex
	s.selectedFileIndex = -1
	s.taskList.Refresh()
	s.taskList.Select(s.selectedTaskIndex)
	s.refreshDetails()
	s.saveTaskHistory()
	s.setStatus(fmt.Sprintf("已重新排队 %d 个任务", len(retryable)))
	if s.autoQueueEnabled() {
		s.startNextPending()
	}
}

func (s *desktopState) fillFormFromSelected() {
	task := s.selectedTask()
	if task == nil || task.Channel == "账号" || strings.TrimSpace(task.URL) == "" {
		s.setStatus("没有可填入表单的任务")
		return
	}
	s.urlEntry.SetText(task.URL)
	if strings.TrimSpace(task.WorkDir) != "" {
		s.workDirEntry.SetText(task.WorkDir)
	}
	if task.Mode == "仅查看" {
		s.modeSelect.SetSelected("下载")
	} else {
		s.modeSelect.SetSelected(defaultText(task.Mode, "下载"))
	}
	s.channelSelect.SetSelected(defaultText(task.Channel, "WEB"))
	s.pageEntry.SetText(task.SelectPage)
	s.dfnEntry.SetText(task.DfnPriority)
	s.encodingEntry.SetText(task.EncodingPriority)
	setStreamIndexEntryFromTask(s.videoIndexEntry, task.VideoIndex, task.Mode == "仅查看")
	setStreamIndexEntryFromTask(s.audioIndexEntry, task.AudioIndex, task.Mode == "仅查看")
	s.extraArgsEntry.SetText(task.ExtraArgs)
	setToolEntryFromTask(s.ffmpegPathEntry, task.FFmpegPath, "ffmpeg")
	setToolEntryFromTask(s.mp4boxPathEntry, task.MP4BoxPath, "mp4box", "MP4Box", "MP4box")
	setToolEntryFromTask(s.aria2cPathEntry, task.Aria2cPath, "aria2c")
	s.restoreChecksFromTask(task)
	s.savePreferences()
	s.setStatus("已填入下载表单")
}

func setStreamIndexEntryFromTask(entry *widget.SelectEntry, value string, keepCurrentWhenEmpty bool) {
	if entry == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value != "" {
		entry.SetText(value)
		return
	}
	if keepCurrentWhenEmpty {
		return
	}
	entry.SetText("自动")
}

func (s *desktopState) restoreChecksFromTask(task *desktopTask) {
	if task == nil {
		return
	}
	// long: 把已创建任务当作新任务模板时，复选框必须跟随原任务 CLI 参数，否则重建下载任务会静默丢失 skip/aria2c/弹幕等行为。
	setCheckState(s.danmakuCheck, task.Mode == "仅弹幕" || boolArgEnabled(task.Args, "--download-danmaku"))
	setCheckState(s.skipSubtitleCheck, boolArgEnabled(task.Args, "--skip-subtitle"))
	setCheckState(s.skipCoverCheck, boolArgEnabled(task.Args, "--skip-cover"))
	setCheckState(s.skipMuxCheck, boolArgEnabled(task.Args, "--skip-mux"))
	setCheckState(s.aria2cCheck, boolAnyArgEnabled(task.Args, "--use-aria2c", "--aria2"))
}

func (s *desktopState) startNextPending() {
	if s.runningTask != nil {
		s.setStatus("已有任务运行中")
		return
	}
	for _, task := range s.tasks {
		if task.Status == statusPending {
			s.startTask(task)
			return
		}
	}
	s.setStatus("没有等待中的任务")
}

func (s *desktopState) startTask(task *desktopTask) {
	if task == nil {
		return
	}
	if s.runningTask != nil {
		s.setStatus("已有任务运行中，任务已保留在队列中")
		return
	}
	if err := ensureDesktopTaskArgs(task); err != nil {
		s.failBeforeStart(task, err)
		return
	}
	if err := s.resolveTaskToolPaths(task); err != nil {
		s.failBeforeStart(task, err)
		return
	}
	runtimeDir, helper, err := prepareRuntime()
	if err != nil {
		s.failBeforeStart(task, err)
		return
	}
	if task.WorkDir != "" {
		if err := os.MkdirAll(task.WorkDir, 0o755); err != nil {
			s.failBeforeStart(task, err)
			return
		}
		task.Baseline = snapshotFiles(task.WorkDir)
	}

	cmd := exec.Command(helper, task.Args...)
	cmd.Dir = runtimeDir
	cmd.Env = append(desktopCommandEnv(), serverChildEnv, forcePlainTextEnv)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.failBeforeStart(task, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.failBeforeStart(task, err)
		return
	}

	task.Status = statusRunning
	task.StartedAt = time.Now()
	task.EndedAt = time.Time{}
	task.RuntimeDir = runtimeDir
	task.StopRequested = false
	task.Command = cmd
	task.Bytes = 0
	task.EventBytes = 0
	task.FileGrowthBytes = 0
	task.Progress = 0
	task.ProgressState = desktopProgressState{}
	task.TransferPathBytes = make(map[string]int64)
	task.LastTransferAt = time.Time{}
	task.LastSpeedBytes = 0
	task.Files = nil
	task.Log.Reset()
	task.Log.WriteString("$ " + shellJoin(append([]string{helper}, task.Args...)) + "\n")
	s.runningTask = task
	s.refreshAll()
	s.saveTaskHistory()

	if err := cmd.Start(); err != nil {
		s.failBeforeStart(task, err)
		return
	}
	s.setStatus(fmt.Sprintf("任务 #%d 已开始", task.ID))
	go s.consumeOutput(task, stdout)
	go s.consumeOutput(task, stderr)
	go func() {
		err := cmd.Wait()
		fyne.Do(func() {
			s.finishTask(task, err)
		})
	}()
}

func (s *desktopState) consumeOutput(task *desktopTask, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		fyne.Do(func() {
			s.handleLine(task, line)
		})
	}
	if err := scanner.Err(); err != nil {
		fyne.Do(func() {
			task.Log.WriteString("读取输出失败：" + err.Error() + "\n")
			s.scheduleTaskRefresh(task)
		})
	}
}

func (s *desktopState) handleLine(task *desktopTask, line string) {
	if strings.HasPrefix(line, serverTransferEventPrefix) {
		s.applyTransferEvent(task, line)
		return
	}
	task.Log.WriteString(line + "\n")
	progressChanged := applyDesktopProgressLine(task, line)
	if strings.Contains(line, "生成二维码成功") {
		s.reloadQRCode(task)
	}
	if progressChanged {
		s.scheduleTaskRefresh(task)
		return
	}
	s.scheduleTaskRefresh(task)
}

func (s *desktopState) applyTransferEvent(task *desktopTask, line string) {
	event, ok := parseTransferEvent(line)
	if !ok {
		return
	}
	applyTransferEventToTask(task, event, time.Now())
	s.scheduleTaskRefresh(task)
}

func (s *desktopState) stopSelected() {
	task := s.selectedTask()
	s.stopTask(task)
}

func (s *desktopState) stopRunningTask() {
	s.stopTask(s.runningTask)
}

func (s *desktopState) stopTask(task *desktopTask) {
	if task == nil || task.Command == nil || task.Command.Process == nil {
		return
	}
	task.StopRequested = true
	task.Status = statusStopping
	_ = task.Command.Process.Kill()
	task.Log.WriteString("\n已请求停止当前任务。\n")
	s.refreshAll()
	s.saveTaskHistory()
	s.setStatus(fmt.Sprintf("正在停止任务 #%d", task.ID))
}

func (s *desktopState) finishTask(task *desktopTask, err error) {
	statusHint := ""
	task.EndedAt = time.Now()
	task.Command = nil
	task.Files = changedFiles(task.WorkDir, task.Baseline)
	task.LastSpeedBytes = 0
	if s.selectedTask() == task {
		if len(task.Files) > 0 {
			s.selectedFileIndex = 0
		} else {
			s.selectedFileIndex = -1
		}
	}
	if task.StopRequested {
		task.Status = statusStopped
		task.Log.WriteString("\n任务已停止。\n")
	} else if err != nil {
		task.Status = statusFailed
		task.Log.WriteString("\n任务失败：" + err.Error() + "\n")
		if hint := desktopTaskFailureHint(task.Log.String()); hint != "" {
			task.Log.WriteString(hint + "\n")
			statusHint = hint
		}
	} else {
		task.Status = statusSuccess
		setDesktopProgress(task, 1)
		task.Log.WriteString("\n任务完成。\n")
	}
	if s.runningTask == task {
		s.runningTask = nil
	}
	s.refreshAll()
	s.saveTaskHistory()
	s.setStatus(taskCompletionStatus(task))
	if s.autoQueueEnabled() && task.Status != statusStopped {
		s.startNextPending()
	}
	if statusHint != "" && s.runningTask == nil {
		s.setStatus(statusHint)
	}
}

func (s *desktopState) failBeforeStart(task *desktopTask, err error) {
	task.Status = statusFailed
	task.EndedAt = time.Now()
	task.Log.WriteString("启动失败：" + err.Error() + "\n")
	if hint := desktopStartupHint(err); hint != "" {
		task.Log.WriteString(hint + "\n")
		s.setStatus(hint)
	} else {
		s.setStatus("启动失败：" + err.Error())
	}
	if s.runningTask == task {
		s.runningTask = nil
	}
	s.refreshAll()
	s.saveTaskHistory()
	if s.autoQueueEnabled() {
		s.startNextPending()
	}
}

func desktopTaskFailureHint(logText string) string {
	return desktopStartupHint(errors.New(logText))
}

func desktopStartupHint(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if strings.Contains(text, "找不到可执行的ffmpeg文件") {
		return desktopMissingToolHint("完整下载需要 FFmpeg 混流", "FFmpeg", "ffmpeg")
	}
	if strings.Contains(text, "找不到可执行的mp4box文件") {
		return desktopMissingToolHint("当前任务需要 MP4Box 混流", "MP4Box", "gpac")
	}
	if strings.Contains(text, "找不到可执行的aria2c文件") {
		return desktopMissingToolHint("当前任务启用了 aria2c 下载", "aria2c", "aria2")
	}
	return ""
}

func desktopMissingToolHint(reason, label, brewPackage string) string {
	if runtime.GOOS == "darwin" && brewPackage != "" {
		return fmt.Sprintf("提示：%s；请点击“检测工具”，或执行 brew install %s 后重试。", reason, brewPackage)
	}
	return fmt.Sprintf("提示：%s；请点击“检测工具”，或安装 %s 后在输入框填写完整路径。", reason, label)
}

func (s *desktopState) autoQueueEnabled() bool {
	return s != nil && s.autoQueueCheck != nil && s.autoQueueCheck.Checked
}

func (s *desktopState) removeSelected() {
	task := s.selectedTask()
	if task == nil || task.Status == statusRunning || task.Status == statusStopping {
		return
	}
	index := s.selectedTaskIndex
	s.tasks = append(s.tasks[:index], s.tasks[index+1:]...)
	s.selectTaskAfterMutation(nil, index)
	s.taskList.Refresh()
	if s.selectedTaskIndex >= 0 {
		s.taskList.Select(s.selectedTaskIndex)
	}
	s.refreshDetails()
	s.saveTaskHistory()
	s.setStatus(fmt.Sprintf("已删除任务 #%d", task.ID))
}

func (s *desktopState) clearEndedTasks() {
	before := len(s.tasks)
	selected := s.selectedTask()
	s.tasks = activeDesktopTasks(s.tasks)
	s.selectTaskAfterMutation(selected, s.selectedTaskIndex)
	s.taskList.Refresh()
	if s.selectedTaskIndex >= 0 {
		s.taskList.Select(s.selectedTaskIndex)
	}
	s.refreshDetails()
	s.saveTaskHistory()
	s.setStatus(fmt.Sprintf("已清理 %d 个结束任务", before-len(s.tasks)))
}

func (s *desktopState) confirmRemoveSelected() {
	task := s.selectedTask()
	if task == nil || task.Status == statusRunning || task.Status == statusStopping {
		return
	}
	if s.window == nil {
		s.removeSelected()
		return
	}
	dialog.ShowConfirm("删除任务", fmt.Sprintf("确定删除任务 #%d 吗？已下载文件不会被删除。", task.ID), func(ok bool) {
		if ok {
			s.removeSelected()
		}
	}, s.window)
}

func (s *desktopState) confirmClearEndedTasks() {
	count := len(s.tasks) - len(activeDesktopTasks(s.tasks))
	if count <= 0 {
		return
	}
	if s.window == nil {
		s.clearEndedTasks()
		return
	}
	dialog.ShowConfirm("清理已结束任务", fmt.Sprintf("确定从列表移除 %d 个已结束任务吗？已下载文件不会被删除。", count), func(ok bool) {
		if ok {
			s.clearEndedTasks()
		}
	}, s.window)
}

func activeDesktopTasks(tasks []*desktopTask) []*desktopTask {
	filtered := make([]*desktopTask, 0, len(tasks))
	for _, task := range tasks {
		if task != nil && isActiveDesktopTask(task.Status) {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func hasEndedDesktopTask(tasks []*desktopTask) bool {
	for _, task := range tasks {
		if task != nil && !isActiveDesktopTask(task.Status) {
			return true
		}
	}
	return false
}

func isActiveDesktopTask(status taskStatus) bool {
	return status == statusPending || status == statusRunning || status == statusStopping
}

func retryableDesktopTasks(tasks []*desktopTask) []*desktopTask {
	retryable := make([]*desktopTask, 0, len(tasks))
	for _, task := range tasks {
		if task != nil && isRetryableDesktopTask(task.Status) {
			retryable = append(retryable, task)
		}
	}
	return retryable
}

func hasRetryableDesktopTask(tasks []*desktopTask) bool {
	for _, task := range tasks {
		if task != nil && isRetryableDesktopTask(task.Status) {
			return true
		}
	}
	return false
}

func isRetryableDesktopTask(status taskStatus) bool {
	return status == statusFailed || status == statusStopped
}

func (s *desktopState) selectTaskAfterMutation(preferred *desktopTask, fallbackIndex int) {
	s.selectedFileIndex = -1
	if len(s.tasks) == 0 {
		s.selectedTaskIndex = -1
		return
	}
	if preferred != nil {
		for index, task := range s.tasks {
			if task == preferred {
				s.selectedTaskIndex = index
				return
			}
		}
	}
	if fallbackIndex < 0 {
		fallbackIndex = 0
	}
	if fallbackIndex >= len(s.tasks) {
		fallbackIndex = len(s.tasks) - 1
	}
	s.selectedTaskIndex = fallbackIndex
}

func (s *desktopState) openSelectedFile() {
	file := s.selectedFile()
	if file != nil {
		openPath(file.Path)
	}
}

func (s *desktopState) revealSelectedFile() {
	file := s.selectedFile()
	if file != nil {
		revealPath(file.Path)
	}
}

func (s *desktopState) copySelectedFilePath() {
	path := selectedFilePathText(s.selectedFile())
	if path == "" {
		s.setStatus("没有可复制的文件路径")
		return
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(path)
		s.setStatus("已复制文件路径")
	}
}

func (s *desktopState) copyAllFilePaths() {
	task := s.selectedTask()
	text := allFilePathsText(nil)
	count := 0
	if task != nil {
		text, count = allFilePathsTextWithCount(task.Files)
	}
	if text == "" {
		s.setStatus("没有可复制的完成文件")
		return
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(text)
		s.setStatus(fmt.Sprintf("已复制 %d 个文件路径", count))
	}
}

func (s *desktopState) checkToolPaths() {
	var b strings.Builder
	b.WriteString("工具检测:\n")
	check := func(label string, entry *widget.Entry, flag, brewPackage string, names ...string) {
		value := ""
		if entry != nil {
			value = desktopToolPathValue(entry.Text)
		}
		path, err := resolveDesktopToolPath(value, names...)
		if err != nil {
			fmt.Fprintf(&b, "  %s: %s\n", label, desktopToolMissingCheckText(label, flag, brewPackage, value))
			return
		}
		if entry != nil {
			entry.SetText(path)
		}
		if version := desktopToolVersion(label, path); version != "" {
			fmt.Fprintf(&b, "  %s: 已找到 %s；版本: %s\n", label, path, version)
			return
		}
		fmt.Fprintf(&b, "  %s: 已找到 %s\n", label, path)
	}
	check("FFmpeg", s.ffmpegPathEntry, "--ffmpeg-path", "ffmpeg", "ffmpeg")
	check("MP4Box", s.mp4boxPathEntry, "--mp4box-path", "gpac", "mp4box", "MP4Box", "MP4box")
	check("aria2c", s.aria2cPathEntry, "--aria2c-path", "aria2", "aria2c")
	s.appendGlobalLog(b.String())
	s.savePreferences()
	s.setStatus("工具检测完成")
}

func desktopToolMissingCheckText(label, flag, brewPackage, explicit string) string {
	parts := []string{"未找到"}
	if flag != "" {
		parts = append(parts, "可在输入框填写完整路径或用 "+flag)
	}
	if runtime.GOOS == "darwin" && brewPackage != "" {
		parts = append(parts, "macOS 可执行 brew install "+brewPackage)
	} else if strings.TrimSpace(label) != "" {
		parts = append(parts, "请安装 "+label)
	}
	explicit = strings.TrimSpace(explicit)
	if explicit != "" && !isBareToolName(explicit) {
		// long 2026-06-28 02:43:00：用户手动填错绝对路径时，短提示仍要指出当前值不可用，否则会误以为只是系统 PATH 没配好。
		parts = append(parts, "当前填写路径不存在："+explicit)
	}
	return strings.Join(parts, "；")
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
	return desktopFirstNonEmptyLine(string(out))
}

func desktopFirstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func (s *desktopState) copySelectedCommand() {
	task := s.selectedTask()
	command := taskCommandText(task)
	if command == "" {
		s.setStatus("没有可复制的命令")
		return
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(command)
		s.setStatus("已复制命令")
	}
}

func (s *desktopState) copySelectedLog() {
	task := s.selectedTask()
	if task == nil || strings.TrimSpace(task.Log.String()) == "" {
		s.setStatus("没有可复制的日志")
		return
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(task.Log.String())
		s.setStatus("已复制日志")
	}
}

func (s *desktopState) copyDoctorReport() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	report, err := s.desktopDoctorJSON(ctx)
	if err != nil {
		s.setStatus("复制诊断失败：" + err.Error())
		s.appendGlobalLog("复制诊断失败：" + err.Error() + "\n")
		return
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(report)
		s.setStatus("已复制诊断")
		s.appendGlobalLog(doctorReportLogSummary(report))
	}
}

func (s *desktopState) desktopDoctorJSON(ctx context.Context) (string, error) {
	runtimeDir, helper, err := prepareRuntime()
	if err != nil {
		return "", err
	}
	args := []string{"doctor", "--json"}
	addToolPathArg := func(flag string, entry *widget.Entry) {
		if entry == nil {
			return
		}
		if value := desktopToolPathValue(entry.Text); value != "" {
			args = append(args, flag, value)
		}
	}
	addToolPathArg("--ffmpeg-path", s.ffmpegPathEntry)
	addToolPathArg("--mp4box-path", s.mp4boxPathEntry)
	addToolPathArg("--aria2c-path", s.aria2cPathEntry)
	cmd := exec.CommandContext(ctx, helper, args...)
	cmd.Dir = runtimeDir
	cmd.Env = append(desktopCommandEnv(), forcePlainTextEnv)
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			return "", fmt.Errorf("%w: %s", err, text)
		}
		return "", err
	}
	return string(out), nil
}

func desktopToolPathValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "自动" {
		return ""
	}
	return value
}

func doctorReportLogSummary(report string) string {
	type tool struct {
		Name        string `json:"name"`
		Found       bool   `json:"found"`
		Path        string `json:"path"`
		Version     string `json:"version"`
		Input       string `json:"input"`
		Flag        string `json:"flag"`
		InstallHint string `json:"installHint"`
	}
	type payload struct {
		Tools []tool `json:"tools"`
	}
	var data payload
	if err := json.Unmarshal([]byte(report), &data); err != nil || len(data.Tools) == 0 {
		return "已复制诊断到剪贴板。\n"
	}
	parts := make([]string, 0, len(data.Tools))
	for _, item := range data.Tools {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = "工具"
		}
		if item.Found {
			path := strings.TrimSpace(item.Path)
			if path == "" {
				path = "已找到"
			}
			if version := strings.TrimSpace(item.Version); version != "" {
				path += " [" + version + "]"
			}
			parts = append(parts, fmt.Sprintf("%s=%s", name, path))
			continue
		}
		missing := "未找到"
		hints := make([]string, 0, 3)
		if input := strings.TrimSpace(item.Input); input != "" {
			hints = append(hints, "当前指定值不可用: "+input)
		}
		if flag := strings.TrimSpace(item.Flag); flag != "" {
			hints = append(hints, "可用 "+flag)
		}
		if installHint := strings.TrimSpace(item.InstallHint); installHint != "" {
			hints = append(hints, installHint)
		}
		if len(hints) > 0 {
			// long 2026-06-28 03:04:00：复制诊断的日志摘要是用户最容易看到的排障入口；缺失工具时直接露出坏输入、参数名和安装建议，避免必须展开完整 JSON 才能知道下一步。
			missing += "（" + strings.Join(hints, "；") + "）"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", name, missing))
	}
	// long 2026-06-27 04:31:00：完整 doctor JSON 可能很长，只在日志里留下工具命中摘要，方便用户回看 ffmpeg 这类依赖是否被桌面端识别。
	return "已复制诊断到剪贴板；" + strings.Join(parts, "，") + "。\n"
}

func (s *desktopState) selectedTask() *desktopTask {
	if s.selectedTaskIndex < 0 || s.selectedTaskIndex >= len(s.tasks) {
		return nil
	}
	return s.tasks[s.selectedTaskIndex]
}

func (s *desktopState) selectedFile() *managedFile {
	task := s.selectedTask()
	if task == nil || s.selectedFileIndex < 0 || s.selectedFileIndex >= len(task.Files) {
		return nil
	}
	return &task.Files[s.selectedFileIndex]
}

func (s *desktopState) refreshAll() {
	s.taskList.Refresh()
	s.refreshDetails()
	s.refreshActions()
}

func (s *desktopState) refreshDetailsIfSelected(task *desktopTask) {
	if s.selectedTask() == task {
		s.refreshDetails()
	}
}

func (s *desktopState) scheduleTaskRefresh(task *desktopTask) {
	if s.uiRefreshPending {
		return
	}
	s.uiRefreshPending = true
	go func() {
		time.Sleep(80 * time.Millisecond)
		fyne.Do(func() {
			s.uiRefreshPending = false
			if s.taskList != nil {
				s.taskList.Refresh()
			}
			s.refreshDetailsIfSelected(task)
		})
	}()
}

func (s *desktopState) refreshDetails() {
	now := time.Now()
	s.summaryLabel.SetText(s.taskSummary())
	task := s.selectedTask()
	if task == nil {
		s.taskTitleLabel.SetText("未选择任务")
		s.taskMetaLabel.SetText("")
		s.taskStatusLabel.SetText("状态 · 未选择")
		s.taskStatusLabel.Importance = widget.LowImportance
		s.taskStatusLabel.Refresh()
		s.syncSelectedTaskLog(nil)
		s.bytesLabel.SetText("已下载 0 B")
		s.speedLabel.SetText("速度 0 B/s")
		s.elapsedLabel.SetText("耗时 00:00")
		s.refreshStreamIndexOptions(nil)
		s.progress.SetValue(0)
		s.progress.Hide()
		s.qrImage.Hide()
		s.qrImage.Refresh()
		s.fileList.Refresh()
		s.refreshActions()
		return
	}
	s.taskTitleLabel.SetText(fmt.Sprintf("#%d %s", task.ID, taskDisplayTitle(task)))
	s.taskMetaLabel.SetText(taskMeta(task))
	s.taskStatusLabel.SetText("状态 · " + string(task.Status))
	s.taskStatusLabel.Importance = taskStatusImportance(task.Status)
	s.taskStatusLabel.Refresh()
	s.syncSelectedTaskLog(task)
	s.refreshStreamIndexOptions(task)
	s.bytesLabel.SetText("已下载 " + formatBytes(task.Bytes))
	s.speedLabel.SetText(taskSpeedText(task, now))
	s.elapsedLabel.SetText("耗时 " + formatElapsed(taskElapsed(task, now)))
	s.progress.SetValue(task.Progress)
	s.normalizeSelectedFileIndex(task)
	s.progress.Show()
	s.reloadQRCode(task)
	s.fileList.Refresh()
	if s.selectedFileIndex >= 0 && s.selectedFileIndex < len(task.Files) {
		s.fileList.Select(s.selectedFileIndex)
	}
	s.refreshActions()
}

func (s *desktopState) syncSelectedTaskLog(task *desktopTask) {
	if s.logEntry == nil {
		return
	}
	if task == nil {
		if s.logEntry.Text() != "" {
			s.logEntry.SetText("")
		}
		s.renderedLogTaskID = -1
		s.renderedLogBytes = 0
		return
	}
	text := task.Log.String()
	// long 2026-07-26 16:18:00：下载日志会持续增长；同一任务只追加新增片段，避免每秒把整段日志重新复制到 TextGrid 导致长任务界面卡顿。
	if s.renderedLogTaskID == task.ID && s.renderedLogBytes <= len(text) && len(s.logEntry.Text()) == s.renderedLogBytes {
		if delta := text[s.renderedLogBytes:]; delta != "" {
			current := s.logEntry.Text()
			if strings.HasSuffix(current, "\n") && len(s.logEntry.Rows) > 0 && len(s.logEntry.Rows[len(s.logEntry.Rows)-1].Cells) == 0 {
				// long 2026-07-26 16:53:00：TextGrid.Append 固定从新行开始；先移除尾部换行生成的空行，才能按日志原文追加而不凭空多出一行。
				s.logEntry.Rows = s.logEntry.Rows[:len(s.logEntry.Rows)-1]
				s.logEntry.Append(delta)
			} else {
				s.logEntry.SetText(text)
			}
		}
	} else if s.logEntry.Text() != text {
		s.logEntry.SetText(text)
	}
	if s.logFollowCheck != nil && s.logFollowCheck.Checked {
		s.logEntry.ScrollToBottom()
	}
	s.renderedLogTaskID = task.ID
	s.renderedLogBytes = len(text)
}

func (s *desktopState) refreshActions() {
	task := s.selectedTask()
	hasTask := task != nil
	running := s.runningTask != nil
	canStart := hasTask && !running && task.Status != statusRunning && task.Status != statusStopping
	canStop := hasTask && (task.Status == statusRunning || task.Status == statusStopping)
	canRemove := hasTask && !canStop
	canRetry := hasTask && !running && task.Status != statusPending && task.Status != statusRunning && task.Status != statusStopping
	hasFile := s.selectedFile() != nil
	setButtonState(s.startButton, canStart)
	setButtonState(s.nextButton, !running)
	setButtonState(s.fillFormBtn, hasTask && !running && task.Channel != "账号")
	setButtonState(s.stopButton, canStop)
	setButtonState(s.retryButton, canRetry)
	setButtonState(s.retryFailedBtn, !running && hasRetryableDesktopTask(s.tasks))
	setButtonState(s.removeButton, canRemove)
	setButtonState(s.clearEndedBtn, hasEndedDesktopTask(s.tasks))
	setButtonState(s.openFileBtn, hasFile)
	setButtonState(s.revealFileBtn, hasFile)
	setButtonState(s.copyFilePathBtn, hasFile)
	setButtonState(s.copyAllFilesBtn, hasTask && allFilePathsText(task.Files) != "")
	setButtonState(s.copyCommandBtn, hasTask && taskCommandText(task) != "")
	setButtonState(s.copyLogBtn, hasTask && strings.TrimSpace(task.Log.String()) != "")
	setButtonState(s.copyDoctorBtn, !running)
	setButtonState(s.checkToolsBtn, !running)
}

func (s *desktopState) refreshStreamIndexOptions(task *desktopTask) {
	if s.videoIndexEntry == nil || s.audioIndexEntry == nil {
		return
	}
	if task == nil {
		s.videoIndexEntry.SetOptions(withAutoStreamOption(nil))
		s.audioIndexEntry.SetOptions(withAutoStreamOption(nil))
		return
	}
	videoOptions, audioOptions := streamIndexOptionsFromLog(task.Log.String())
	s.videoIndexEntry.SetOptions(withAutoStreamOption(videoOptions))
	s.audioIndexEntry.SetOptions(withAutoStreamOption(audioOptions))
}

func withAutoStreamOption(options []string) []string {
	out := make([]string, 0, len(options)+1)
	out = append(out, "自动")
	out = append(out, options...)
	return out
}

func (s *desktopState) normalizeSelectedFileIndex(task *desktopTask) {
	if task == nil || s.selectedFileIndex >= len(task.Files) {
		s.selectedFileIndex = -1
	}
}

func (s *desktopState) reloadQRCode(task *desktopTask) {
	if task == nil || task.RuntimeDir == "" {
		s.qrImage.Hide()
		s.qrImage.Refresh()
		return
	}
	path := filepath.Join(task.RuntimeDir, "qrcode.png")
	if _, err := os.Stat(path); err != nil {
		s.qrImage.Hide()
		s.qrImage.Refresh()
		return
	}
	s.qrImage.File = path
	s.qrImage.Show()
	s.qrImage.Refresh()
}

func (s *desktopState) hasRunningTask() bool {
	return s.runningTask != nil
}

func (s *desktopState) setStatus(status string) {
	if s.statusLabel == nil {
		return
	}
	s.statusLabel.SetText(status)
	s.statusLabel.Importance = statusMessageImportance(status)
	s.statusLabel.Refresh()
}

func (s *desktopState) appendGlobalLog(text string) {
	task := s.selectedTask()
	if task == nil {
		s.logEntry.SetText(text)
		if s.logFollowCheck != nil && s.logFollowCheck.Checked {
			s.logEntry.ScrollToBottom()
		}
		s.renderedLogTaskID = -1
		s.renderedLogBytes = len(text)
		return
	}
	task.Log.WriteString(text)
	s.refreshDetails()
}

func (s *desktopState) loadPreferences() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	prefs := app.Preferences()
	s.workDirEntry.SetText(prefs.StringWithFallback("work_dir", s.workDirEntry.Text))
	s.pageEntry.SetText(prefs.StringWithFallback("select_page", s.pageEntry.Text))
	s.videoIndexEntry.SetText(prefs.StringWithFallback("video_index", s.videoIndexEntry.Text))
	s.audioIndexEntry.SetText(prefs.StringWithFallback("audio_index", s.audioIndexEntry.Text))
	s.dfnEntry.SetText(prefs.StringWithFallback("dfn_priority", s.dfnEntry.Text))
	s.encodingEntry.SetText(prefs.StringWithFallback("encoding_priority", s.encodingEntry.Text))
	s.extraArgsEntry.SetText(prefs.StringWithFallback("extra_args", s.extraArgsEntry.Text))
	s.ffmpegPathEntry.SetText(prefs.StringWithFallback("ffmpeg_path", s.ffmpegPathEntry.Text))
	s.mp4boxPathEntry.SetText(prefs.StringWithFallback("mp4box_path", s.mp4boxPathEntry.Text))
	s.aria2cPathEntry.SetText(prefs.StringWithFallback("aria2c_path", s.aria2cPathEntry.Text))
	s.modeSelect.SetSelected(prefs.StringWithFallback("mode", s.modeSelect.Selected))
	s.channelSelect.SetSelected(prefs.StringWithFallback("channel", s.channelSelect.Selected))
	s.danmakuCheck.SetChecked(prefs.BoolWithFallback("download_danmaku", s.danmakuCheck.Checked))
	s.skipSubtitleCheck.SetChecked(prefs.BoolWithFallback("skip_subtitle", s.skipSubtitleCheck.Checked))
	s.skipCoverCheck.SetChecked(prefs.BoolWithFallback("skip_cover", s.skipCoverCheck.Checked))
	s.skipMuxCheck.SetChecked(prefs.BoolWithFallback("skip_mux", s.skipMuxCheck.Checked))
	s.aria2cCheck.SetChecked(prefs.BoolWithFallback("use_aria2c", s.aria2cCheck.Checked))
	s.autoQueueCheck.SetChecked(prefs.BoolWithFallback("auto_queue", s.autoQueueCheck.Checked))
}

func (s *desktopState) refreshToolPathEntries() {
	normalizeToolEntry(s.ffmpegPathEntry, "ffmpeg")
	normalizeToolEntry(s.mp4boxPathEntry, "mp4box", "MP4Box", "MP4box")
	normalizeToolEntry(s.aria2cPathEntry, "aria2c")
}

func normalizeToolEntry(entry *widget.Entry, names ...string) {
	if entry == nil {
		return
	}
	current := strings.TrimSpace(entry.Text)
	if current != "" && fileExists(current) {
		return
	}
	if current != "" && isBareToolName(current) {
		if path := detectToolPath(append([]string{current}, names...)...); path != "" {
			entry.SetText(path)
			return
		}
	}
	if path := detectToolPath(names...); path != "" {
		entry.SetText(path)
	}
}

func setToolEntryFromTask(entry *widget.Entry, value string, names ...string) {
	if entry == nil {
		return
	}
	if strings.TrimSpace(value) != "" {
		entry.SetText(strings.TrimSpace(value))
		normalizeToolEntry(entry, names...)
		return
	}
	normalizeToolEntry(entry, names...)
}

func (s *desktopState) savePreferences() {
	app := fyne.CurrentApp()
	if app == nil || !s.canSavePreferences() {
		return
	}
	prefs := app.Preferences()
	prefs.SetString("work_dir", strings.TrimSpace(s.workDirEntry.Text))
	prefs.SetString("select_page", strings.TrimSpace(s.pageEntry.Text))
	prefs.SetString("video_index", strings.TrimSpace(s.videoIndexEntry.Text))
	prefs.SetString("audio_index", strings.TrimSpace(s.audioIndexEntry.Text))
	prefs.SetString("dfn_priority", strings.TrimSpace(s.dfnEntry.Text))
	prefs.SetString("encoding_priority", strings.TrimSpace(s.encodingEntry.Text))
	prefs.SetString("extra_args", s.extraArgsEntry.Text)
	prefs.SetString("ffmpeg_path", desktopToolPathValue(s.ffmpegPathEntry.Text))
	prefs.SetString("mp4box_path", desktopToolPathValue(s.mp4boxPathEntry.Text))
	prefs.SetString("aria2c_path", desktopToolPathValue(s.aria2cPathEntry.Text))
	prefs.SetString("mode", defaultText(s.modeSelect.Selected, "下载"))
	prefs.SetString("channel", defaultText(s.channelSelect.Selected, "WEB"))
	prefs.SetBool("download_danmaku", s.danmakuCheck.Checked)
	prefs.SetBool("skip_subtitle", s.skipSubtitleCheck.Checked)
	prefs.SetBool("skip_cover", s.skipCoverCheck.Checked)
	prefs.SetBool("skip_mux", s.skipMuxCheck.Checked)
	prefs.SetBool("use_aria2c", s.aria2cCheck.Checked)
	prefs.SetBool("auto_queue", s.autoQueueCheck.Checked)
}

func (s *desktopState) canSavePreferences() bool {
	return s.workDirEntry != nil &&
		s.pageEntry != nil &&
		s.videoIndexEntry != nil &&
		s.audioIndexEntry != nil &&
		s.dfnEntry != nil &&
		s.encodingEntry != nil &&
		s.extraArgsEntry != nil &&
		s.ffmpegPathEntry != nil &&
		s.mp4boxPathEntry != nil &&
		s.aria2cPathEntry != nil &&
		s.modeSelect != nil &&
		s.channelSelect != nil &&
		s.danmakuCheck != nil &&
		s.skipSubtitleCheck != nil &&
		s.skipCoverCheck != nil &&
		s.skipMuxCheck != nil &&
		s.aria2cCheck != nil &&
		s.autoQueueCheck != nil
}

func (s *desktopState) loadTaskHistory() error {
	path, err := taskHistoryPath()
	if err != nil {
		return err
	}
	tasks, nextID, err := loadTaskHistoryFile(path)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return nil
	}
	s.tasks = tasks
	s.nextID = nextID
	s.selectedTaskIndex = len(tasks) - 1
	s.selectedFileIndex = -1
	if s.taskList != nil {
		s.taskList.Refresh()
		s.taskList.Select(s.selectedTaskIndex)
	}
	return nil
}

func (s *desktopState) saveTaskHistory() {
	s.saveTaskHistoryAt(time.Now())
}

func (s *desktopState) saveTaskHistoryAt(now time.Time) {
	path, err := taskHistoryPath()
	if err != nil {
		s.setStatus("保存任务历史失败：" + err.Error())
		return
	}
	if err := saveTaskHistoryFile(path, s.tasks); err != nil {
		s.setStatus("保存任务历史失败：" + err.Error())
		return
	}
	s.lastHistorySave = now
}

func (s *desktopState) saveRunningTaskHistoryIfDue(now time.Time) {
	if s.runningTask == nil || !shouldSaveTaskHistory(s.lastHistorySave, now, desktopTaskHistorySaveGap) {
		return
	}
	s.saveTaskHistoryAt(now)
}

func shouldSaveTaskHistory(last, now time.Time, interval time.Duration) bool {
	if interval <= 0 || last.IsZero() {
		return true
	}
	if now.Before(last) {
		return true
	}
	return now.Sub(last) >= interval
}

func taskHistoryPath() (string, error) {
	dir, err := runtimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, desktopTaskHistoryFile), nil
}

func saveTaskHistoryFile(path string, tasks []*desktopTask) error {
	tasks = trimTaskHistoryTasks(tasks, desktopTaskHistoryLimit)
	history := desktopTaskHistory{
		Version: 1,
		Tasks:   make([]persistedDesktopTask, 0, len(tasks)),
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		history.Tasks = append(history.Tasks, persistDesktopTask(task))
	}
	data, err := json.MarshalIndent(history, "", "  ")
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
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString("\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func loadTaskHistoryFile(path string) ([]*desktopTask, int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 1, nil
	}
	if err != nil {
		return nil, 1, err
	}
	var history desktopTaskHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, 1, err
	}
	tasks := make([]*desktopTask, 0, len(history.Tasks))
	nextID := 1
	for _, item := range history.Tasks {
		task := restoreDesktopTask(item)
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

func persistDesktopTask(task *desktopTask) persistedDesktopTask {
	return persistDesktopTaskWithLogLimit(task, desktopTaskHistoryLogSize)
}

func persistDesktopTaskWithLogLimit(task *desktopTask, logLimit int) persistedDesktopTask {
	status := task.Status
	if status == statusRunning || status == statusStopping {
		// long 2026-06-27 11:51:00：桌面历史只描述已经落盘的任务状态；旧进程无法跨 app 重启继续控制，保存为已停止可以避免误导用户。
		status = statusStopped
	}
	return persistedDesktopTask{
		ID:               task.ID,
		URL:              task.URL,
		WorkDir:          task.WorkDir,
		Mode:             task.Mode,
		Channel:          task.Channel,
		SelectPage:       task.SelectPage,
		VideoIndex:       task.VideoIndex,
		AudioIndex:       task.AudioIndex,
		DfnPriority:      task.DfnPriority,
		EncodingPriority: task.EncodingPriority,
		ExtraArgs:        task.ExtraArgs,
		FFmpegPath:       task.FFmpegPath,
		MP4BoxPath:       task.MP4BoxPath,
		Aria2cPath:       task.Aria2cPath,
		Args:             append([]string(nil), task.Args...),
		Status:           status,
		Bytes:            task.Bytes,
		Progress:         task.Progress,
		Log:              trimTaskHistoryLog(task.Log.String(), logLimit),
		Files:            append([]managedFile(nil), task.Files...),
		CreatedAt:        task.CreatedAt,
		StartedAt:        task.StartedAt,
		EndedAt:          task.EndedAt,
	}
}

func restoreDesktopTask(item persistedDesktopTask) *desktopTask {
	status := item.Status
	if status == "" {
		status = statusPending
	}
	if status == statusRunning || status == statusStopping {
		status = statusStopped
	}
	task := &desktopTask{
		ID:               item.ID,
		URL:              item.URL,
		WorkDir:          item.WorkDir,
		Mode:             item.Mode,
		Channel:          item.Channel,
		SelectPage:       item.SelectPage,
		VideoIndex:       item.VideoIndex,
		AudioIndex:       item.AudioIndex,
		DfnPriority:      item.DfnPriority,
		EncodingPriority: item.EncodingPriority,
		ExtraArgs:        item.ExtraArgs,
		FFmpegPath:       item.FFmpegPath,
		MP4BoxPath:       item.MP4BoxPath,
		Aria2cPath:       item.Aria2cPath,
		Args:             append([]string(nil), item.Args...),
		Status:           status,
		Bytes:            item.Bytes,
		Progress:         item.Progress,
		ProgressState:    desktopProgressState{Progress: item.Progress},
		Files:            append([]managedFile(nil), item.Files...),
		CreatedAt:        item.CreatedAt,
		StartedAt:        item.StartedAt,
		EndedAt:          item.EndedAt,
	}
	task.Log.WriteString(item.Log)
	return task
}

func trimTaskHistoryTasks(tasks []*desktopTask, limit int) []*desktopTask {
	if limit <= 0 || len(tasks) <= limit {
		return tasks
	}
	return tasks[len(tasks)-limit:]
}

func trimTaskHistoryLog(logText string, limit int) string {
	if limit <= 0 || len(logText) <= limit {
		return logText
	}
	marker := fmt.Sprintf("... 日志过长，仅保留最后 %s ...\n", formatBytes(int64(limit)))
	keepBytes := limit - len(marker)
	if keepBytes <= 0 {
		return marker
	}
	start := len(logText) - keepBytes
	for start < len(logText) && !utf8.RuneStart(logText[start]) {
		start++
	}
	// long 2026-06-27 12:07:00：历史文件只需要保留最近排障信息，按 UTF-8 边界截尾可以避免 JSON 中出现被切断的中文字符。
	return marker + logText[start:]
}

func (s *desktopState) startRuntimeTicker() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			fyne.Do(func() {
				if s.runningTask == nil {
					return
				}
				now := time.Now()
				applyFileGrowthFallbackToTask(s.runningTask, now)
				s.taskList.Refresh()
				s.refreshDetailsIfSelected(s.runningTask)
				s.saveRunningTaskHistoryIfDue(now)
			})
		}
	}()
}

func (s *desktopState) taskSummary() string {
	if len(s.tasks) == 0 {
		return "任务 0"
	}
	counts := map[taskStatus]int{}
	for _, task := range s.tasks {
		counts[task.Status]++
	}
	return fmt.Sprintf(
		"任务 %d · 等待 %d · 下载 %d · 完成 %d · 失败 %d · 停止 %d",
		len(s.tasks),
		counts[statusPending],
		counts[statusRunning]+counts[statusStopping],
		counts[statusSuccess],
		counts[statusFailed],
		counts[statusStopped],
	)
}

func taskCompletionStatus(task *desktopTask) string {
	if task == nil {
		return "任务已结束"
	}
	switch task.Status {
	case statusSuccess:
		return fmt.Sprintf("任务 #%d 已完成", task.ID)
	case statusFailed:
		return fmt.Sprintf("任务 #%d 失败，请查看运行日志", task.ID)
	case statusStopped:
		return fmt.Sprintf("任务 #%d 已停止", task.ID)
	default:
		return fmt.Sprintf("任务 #%d 状态：%s", task.ID, task.Status)
	}
}

func taskStatusImportance(status taskStatus) widget.Importance {
	switch status {
	case statusRunning:
		return widget.HighImportance
	case statusStopping:
		return widget.WarningImportance
	case statusSuccess:
		return widget.SuccessImportance
	case statusFailed:
		return widget.DangerImportance
	case statusPending, statusStopped:
		return widget.LowImportance
	default:
		return widget.MediumImportance
	}
}

func statusMessageImportance(status string) widget.Importance {
	status = strings.TrimSpace(status)
	switch {
	case status == "":
		return widget.LowImportance
	case strings.Contains(status, "失败"), strings.Contains(status, "错误"), strings.Contains(status, "未找到"), strings.Contains(status, "请输入"), strings.Contains(status, "不可用"):
		return widget.DangerImportance
	case strings.Contains(status, "正在停止"), strings.Contains(status, "运行中"), strings.Contains(status, "等待"):
		return widget.WarningImportance
	case strings.Contains(status, "完成"), strings.Contains(status, "已复制"), strings.Contains(status, "已打开"), strings.Contains(status, "已加入"), strings.Contains(status, "已填入"), strings.Contains(status, "已清理"), strings.Contains(status, "已删除"):
		return widget.SuccessImportance
	default:
		return widget.MediumImportance
	}
}

func setButtonState(button *widget.Button, enabled bool) {
	if button == nil {
		return
	}
	if enabled {
		button.Enable()
	} else {
		button.Disable()
	}
}

func setCheckState(check *widget.Check, checked bool) {
	if check == nil {
		return
	}
	check.SetChecked(checked)
}

func taskDisplayTitle(task *desktopTask) string {
	if task.URL == "" {
		return task.Mode
	}
	return task.URL
}

func taskCommandText(task *desktopTask) string {
	if task == nil {
		return ""
	}
	args := desktopTaskArgsForCommand(task)
	if len(args) == 0 {
		return ""
	}
	return shellJoin(append([]string{taskCommandHelper(task)}, args...))
}

func taskCommandHelper(task *desktopTask) string {
	if task != nil && strings.TrimSpace(task.RuntimeDir) != "" {
		runtimeHelper := filepath.Join(task.RuntimeDir, helperName())
		if fileExists(runtimeHelper) {
			return runtimeHelper
		}
	}
	if helper, err := locateHelper(); err == nil && strings.TrimSpace(helper) != "" {
		return helper
	}
	return helperName()
}

func (task *desktopTask) cloneForRetry(id int) *desktopTask {
	return &desktopTask{
		ID:               id,
		URL:              task.URL,
		WorkDir:          task.WorkDir,
		Mode:             task.Mode,
		Channel:          task.Channel,
		SelectPage:       task.SelectPage,
		VideoIndex:       task.VideoIndex,
		AudioIndex:       task.AudioIndex,
		DfnPriority:      task.DfnPriority,
		EncodingPriority: task.EncodingPriority,
		ExtraArgs:        task.ExtraArgs,
		FFmpegPath:       task.FFmpegPath,
		MP4BoxPath:       task.MP4BoxPath,
		Aria2cPath:       task.Aria2cPath,
		Args:             append([]string(nil), task.Args...),
		Status:           statusPending,
		CreatedAt:        time.Now(),
	}
}

func ensureDesktopTaskArgs(task *desktopTask) error {
	if task == nil || len(task.Args) > 0 {
		return nil
	}
	args, err := rebuildDesktopTaskArgs(task)
	if err != nil {
		return err
	}
	task.Args = args
	return nil
}

func desktopTaskArgsForCommand(task *desktopTask) []string {
	if task == nil {
		return nil
	}
	if len(task.Args) > 0 {
		return task.Args
	}
	args, err := rebuildDesktopTaskArgs(task)
	if err != nil {
		return nil
	}
	return args
}

func rebuildDesktopTaskArgs(task *desktopTask) ([]string, error) {
	if task == nil {
		return nil, nil
	}
	if command := loginCommandFromDesktopTask(task); command != "" {
		return []string{command}, nil
	}
	if strings.TrimSpace(task.URL) == "" {
		return nil, errors.New("任务缺少视频地址，无法重建命令参数")
	}
	extraArgs, err := splitArgs(task.ExtraArgs)
	if err != nil {
		return nil, fmt.Errorf("重建任务参数失败：%w", err)
	}
	return buildDesktopTaskArgs(task, extraArgs, desktopTaskArgSwitches{}), nil
}

type desktopTaskArgSwitches struct {
	DownloadDanmaku bool
	SkipSubtitle    bool
	SkipCover       bool
	SkipMux         bool
	UseAria2c       bool
}

func buildDesktopTaskArgs(task *desktopTask, extraArgs []string, switches desktopTaskArgSwitches) []string {
	if task == nil {
		return nil
	}
	args := []string{}
	if task.Mode == "仅查看" {
		args = append(args, "info")
	}
	if strings.TrimSpace(task.WorkDir) != "" {
		args = append(args, "--work-dir", strings.TrimSpace(task.WorkDir))
	}
	appendPair := func(flag, value string) {
		if strings.TrimSpace(value) != "" {
			args = append(args, flag, strings.TrimSpace(value))
		}
	}
	appendPair("--select-page", task.SelectPage)
	appendPair("--dfn-priority", task.DfnPriority)
	appendPair("--encoding-priority", task.EncodingPriority)
	if task.Mode != "仅查看" {
		appendPair("--video-index", task.VideoIndex)
		appendPair("--audio-index", task.AudioIndex)
		appendPair("--ffmpeg-path", desktopToolPathValue(task.FFmpegPath))
		appendPair("--mp4box-path", desktopToolPathValue(task.MP4BoxPath))
		appendPair("--aria2c-path", desktopToolPathValue(task.Aria2cPath))
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
	args = append(args, task.URL)
	return args
}

func loginCommandFromDesktopTask(task *desktopTask) string {
	if task == nil || task.Channel != "账号" {
		return ""
	}
	text := strings.TrimSpace(task.Mode + " " + task.URL)
	if strings.Contains(text, "TV") {
		return "logintv"
	}
	if strings.Contains(strings.ToUpper(text), "WEB") {
		return "login"
	}
	return ""
}

func taskMeta(task *desktopTask) string {
	parts := []string{string(task.Status), task.Mode, task.Channel, progressPercentText(task.Progress), formatBytes(task.Bytes)}
	if !task.StartedAt.IsZero() {
		parts = append(parts, "开始 "+task.StartedAt.Format("15:04:05"))
	}
	if !task.EndedAt.IsZero() {
		parts = append(parts, "结束 "+task.EndedAt.Format("15:04:05"))
	}
	if len(task.Files) > 0 {
		parts = append(parts, fmt.Sprintf("%d 个文件", len(task.Files)))
	}
	return strings.Join(parts, " · ")
}

func taskSpeedText(task *desktopTask, now time.Time) string {
	current := currentTaskSpeed(task, now)
	average := averageTaskSpeed(task, now)
	if average > 0 {
		return fmt.Sprintf("速度 %s/s · 平均 %s/s", formatBytes(int64(current)), formatBytes(int64(average)))
	}
	return fmt.Sprintf("速度 %s/s", formatBytes(int64(current)))
}

func selectedFilePathText(file *managedFile) string {
	if file == nil {
		return ""
	}
	return strings.TrimSpace(file.Path)
}

func allFilePathsText(files []managedFile) string {
	text, _ := allFilePathsTextWithCount(files)
	return text
}

func allFilePathsTextWithCount(files []managedFile) (string, int) {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if path := strings.TrimSpace(file.Path); path != "" {
			paths = append(paths, path)
		}
	}
	return strings.Join(paths, "\n"), len(paths)
}

func currentTaskSpeed(task *desktopTask, now time.Time) float64 {
	if task == nil || task.Status != statusRunning && task.Status != statusStopping {
		return 0
	}
	if task.LastTransferAt.IsZero() || now.Sub(task.LastTransferAt) > 3*time.Second {
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

func parseTransferEvent(line string) (transferEvent, bool) {
	payload := strings.TrimPrefix(line, serverTransferEventPrefix)
	if payload == line {
		return transferEvent{}, false
	}
	var event transferEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return transferEvent{}, false
	}
	if strings.TrimSpace(event.Path) == "" || event.Bytes <= 0 {
		return transferEvent{}, false
	}
	return event, true
}

func applyDesktopProgressLine(task *desktopTask, line string) bool {
	if task == nil {
		return false
	}
	state := &task.ProgressState
	line = cleanDesktopProgressLogLine(line)
	if match := desktopPageStartPattern.FindStringSubmatch(line); match != nil {
		currentPage := desktopParseInt(match[1])
		totalPages := desktopParseInt(match[3])
		if totalPages > 0 {
			state.TotalPages = totalPages
		}
		if currentPage > 0 {
			state.CurrentPage = currentPage
		}
		return setDesktopProgress(task, desktopProgressForPageStart(currentPage, state.TotalPages))
	}
	if match := desktopPageVideoStartPattern.FindStringSubmatch(line); match != nil {
		return setDesktopProgress(task, desktopProgressForPageMediaStage(desktopParseInt(match[1]), state, desktopProgressStageVideoDownloaded))
	}
	if match := desktopPageAudioStartPattern.FindStringSubmatch(line); match != nil {
		return setDesktopProgress(task, desktopProgressForPageMediaStage(desktopParseInt(match[1]), state, desktopProgressStageAudioDownloaded))
	}
	if match := desktopPageExtraAudioStartPattern.FindStringSubmatch(line); match != nil {
		return setDesktopProgress(task, desktopProgressForPageMediaStage(desktopParseInt(match[1]), state, desktopProgressStageExtraAudio))
	}
	if match := desktopClipStartPattern.FindStringSubmatch(line); match != nil {
		pageIndex := desktopParseInt(match[1])
		clipIndex := desktopParseInt(match[2])
		clipTotal := desktopParseInt(match[3])
		if state.TotalPages == 0 {
			return false
		}
		if pageIndex > 0 {
			state.CurrentPage = pageIndex
		}
		return setDesktopProgress(task, desktopProgressForClip(pageIndex, clipIndex, clipTotal, state.TotalPages))
	}
	if match := desktopPageDonePattern.FindStringSubmatch(line); match != nil {
		currentPage := desktopParseInt(match[1])
		if state.TotalPages == 0 {
			state.TotalPages = currentPage
		}
		if currentPage > 0 {
			state.CurrentPage = currentPage
		}
		return setDesktopProgress(task, desktopProgressForPageStage(currentPage, state.TotalPages, desktopProgressStagePageDownloaded))
	}
	if desktopMuxStartPattern.MatchString(line) {
		return setDesktopProgress(task, desktopProgressForPageStage(state.CurrentPage, state.TotalPages, desktopProgressStageMuxStarted))
	}
	if strings.Contains(line, "任务完成") {
		return setDesktopProgress(task, 1)
	}
	return false
}

func cleanDesktopProgressLogLine(line string) string {
	line = strings.TrimSpace(line)
	if closeIndex := strings.Index(line, "] - "); strings.HasPrefix(line, "[") && closeIndex >= 0 {
		return strings.TrimSpace(line[closeIndex+4:])
	}
	return line
}

func setDesktopProgress(task *desktopTask, progress float64) bool {
	if task == nil {
		return false
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	if progress <= task.ProgressState.Progress {
		return false
	}
	task.ProgressState.Progress = progress
	task.Progress = progress
	return true
}

func desktopProgressForPageStart(pageIndex, totalPages int) float64 {
	if totalPages <= 0 || pageIndex <= 1 {
		return 0
	}
	return float64(pageIndex-1) / float64(totalPages)
}

func desktopProgressForPageMediaStage(pageIndex int, state *desktopProgressState, stage float64) float64 {
	if state == nil || state.TotalPages <= 0 {
		return 0
	}
	if pageIndex > 0 {
		state.CurrentPage = pageIndex
	}
	return desktopProgressForPageStage(pageIndex, state.TotalPages, stage)
}

func desktopProgressForClip(pageIndex, clipIndex, clipTotal, totalPages int) float64 {
	if totalPages <= 0 || clipTotal <= 0 {
		return desktopProgressForPageStart(pageIndex, totalPages)
	}
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if clipIndex <= 0 {
		clipIndex = 1
	}
	if clipIndex > clipTotal {
		clipIndex = clipTotal
	}
	withinPage := float64(clipIndex) / float64(clipTotal+1) * desktopProgressStagePageDownloaded
	return desktopProgressForPageStage(pageIndex, totalPages, withinPage)
}

func desktopProgressForPageStage(pageIndex, totalPages int, stage float64) float64 {
	if totalPages <= 0 {
		return 0
	}
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if stage < 0 {
		stage = 0
	}
	if stage > 1 {
		stage = 1
	}
	return (float64(pageIndex-1) + stage) / float64(totalPages)
}

func desktopParseInt(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func applyTransferEventToTask(task *desktopTask, event transferEvent, observedAt time.Time) bool {
	if task == nil || strings.TrimSpace(event.Path) == "" || event.Bytes <= 0 {
		return false
	}
	if task.TransferPathBytes == nil {
		task.TransferPathBytes = make(map[string]int64)
	}
	key := transferPathKey(task, event.Path)
	delta := event.Bytes
	if event.Delta {
		task.TransferPathBytes[key] += delta
	} else {
		// long 2026-06-25 13:18:00：完成事件是单个资源的当前大小，按路径扣掉任务启动前的基线，避免已有文件或本地混流产物被当成本次网络下载。
		totalGrowth := event.Bytes - transferBaselineBytes(task, key)
		if totalGrowth <= 0 {
			return false
		}
		seen := task.TransferPathBytes[key]
		if totalGrowth <= seen {
			return false
		}
		delta = totalGrowth - seen
		task.TransferPathBytes[key] = totalGrowth
	}
	task.EventBytes += delta
	task.Bytes = task.EventBytes
	if !observedAt.IsZero() {
		last := task.LastTransferAt
		if last.IsZero() {
			last = task.StartedAt
		}
		if !last.IsZero() {
			elapsed := observedAt.Sub(last).Seconds()
			if elapsed > 0 {
				task.LastSpeedBytes = float64(delta) / elapsed
			}
		}
		task.LastTransferAt = observedAt
	}
	return true
}

func applyFileGrowthFallbackToTask(task *desktopTask, observedAt time.Time) bool {
	if task == nil || strings.TrimSpace(task.WorkDir) == "" || task.EventBytes > 0 || len(task.TransferPathBytes) > 0 {
		return false
	}
	growth := totalFileGrowthBytes(task.WorkDir, task.Baseline)
	if growth <= task.FileGrowthBytes {
		return false
	}
	delta := growth - task.FileGrowthBytes
	task.FileGrowthBytes = growth
	task.Bytes = growth
	if !observedAt.IsZero() {
		last := task.LastTransferAt
		if last.IsZero() {
			last = task.StartedAt
		}
		if !last.IsZero() {
			elapsed := observedAt.Sub(last).Seconds()
			if elapsed > 0 {
				task.LastSpeedBytes = float64(delta) / elapsed
			}
		}
		task.LastTransferAt = observedAt
	}
	return true
}

func totalFileGrowthBytes(root string, baseline map[string]fileSnapshot) int64 {
	if strings.TrimSpace(root) == "" {
		return 0
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	var total int64
	_ = filepath.WalkDir(rootAbs, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if !isDownloadMonitorFile(path) {
			return nil
		}
		abs, _ := filepath.Abs(path)
		before := baseline[abs]
		if info.Size() > before.Size {
			total += info.Size() - before.Size
		}
		return nil
	})
	return total
}

func isDownloadMonitorFile(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".m4a", ".flv", ".jpg", ".jpeg", ".png", ".webp", ".xml", ".ass", ".srt", ".tmp", ".vclip", ".aclip":
		return true
	default:
		return false
	}
}

func transferPathKey(task *desktopTask, path string) string {
	candidate := path
	if task != nil && task.WorkDir != "" && !filepath.IsAbs(candidate) {
		candidate = filepath.Join(task.WorkDir, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return candidate
	}
	return abs
}

func transferBaselineBytes(task *desktopTask, key string) int64 {
	if task == nil || task.Baseline == nil {
		return 0
	}
	if before, ok := task.Baseline[key]; ok {
		return before.Size
	}
	return 0
}

func prepareRuntime() (string, string, error) {
	runtimeDir, err := runtimeDir()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", "", err
	}
	source, err := locateHelper()
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(runtimeDir, helperName())
	if err := copyHelperIfNeeded(source, target); err != nil {
		return "", "", err
	}
	return runtimeDir, target, nil
}

func runtimeDir() (string, error) {
	if env := strings.TrimSpace(os.Getenv(desktopRuntimeDirEnv)); env != "" {
		return env, nil
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return "", errors.New("无法定位用户配置目录")
	}
	return filepath.Join(base, "BBDown Go"), nil
}

func locateHelper() (string, error) {
	if env := strings.TrimSpace(os.Getenv("BBDOWN_GO_HELPER")); env != "" {
		if fileExists(env) {
			return env, nil
		}
		return "", fmt.Errorf("BBDOWN_GO_HELPER 指向的文件不存在：%s", env)
	}
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(exeDir, helperName()),
		filepath.Join(exeDir, "..", "Resources", helperName()),
		filepath.Join(".", helperName()),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			abs, _ := filepath.Abs(candidate)
			return abs, nil
		}
	}
	return "", errors.New("找不到 BB-DL helper；请把 CLI 放在桌面程序同目录，或设置 BBDOWN_GO_HELPER")
}

func copyHelperIfNeeded(source, target string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	if targetInfo, err := os.Stat(target); err == nil {
		if sourceInfo.Size() == targetInfo.Size() && sourceInfo.ModTime().Equal(targetInfo.ModTime()) {
			same, err := sameFileContent(source, target)
			if err != nil {
				return err
			}
			// long 2026-06-27 15:02:00：桌面版会把内置 CLI helper 复制到用户配置目录；发布包升级后即使文件大小和时间戳碰巧一致，也必须按内容确认，避免继续调用旧 helper 而保留旧的 ffmpeg 查找问题。
			if same {
				return nil
			}
		}
	}
	input, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, input, 0o755); err != nil {
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

func snapshotFiles(root string) map[string]fileSnapshot {
	result := map[string]fileSnapshot{}
	if strings.TrimSpace(root) == "" {
		return result
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	_ = filepath.WalkDir(rootAbs, func(path string, entry os.DirEntry, err error) error {
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

func changedFiles(root string, baseline map[string]fileSnapshot) []managedFile {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	files := make([]managedFile, 0)
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
		files = append(files, managedFile{Path: abs, RelPath: rel, Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})
	if len(files) > 200 {
		return files[:200]
	}
	return files
}

func isManagedOutputFile(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	switch name {
	case "qrcode.png", "bbdown.data", "bbdowntv.data", "bbdownapp.data", "bbdown.config", "bbdown.archives":
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".tmp", ".vclip", ".aclip", ".aria2":
		return false
	}
	return isDownloadMonitorFile(path)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func helperName() string {
	if runtime.GOOS == "windows" {
		return "BB-DL-cli.exe"
	}
	return "BB-DL-cli"
}

func desktopCommandEnv() []string {
	toolDirs := desktopExtraToolDirs()
	env := envWithPath(os.Environ(), mergeToolPath(os.Getenv("PATH"), toolDirs...))
	if len(toolDirs) > 0 {
		env = envWithValue(env, desktopToolDirsEnv, strings.Join(toolDirs, string(os.PathListSeparator)))
	}
	return env
}

func envWithPath(env []string, pathValue string) []string {
	return envWithValue(env, "PATH", pathValue)
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
	parts := make([]string, 0)
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
	for _, dir := range loginShellToolDirs("ffmpeg", "MP4Box", "MP4box", "mp4box", "aria2c") {
		add(dir)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

func desktopExtraToolDirs() []string {
	seen := map[string]bool{}
	dirs := make([]string, 0, 8)
	add := func(dir string) {
		dir = strings.TrimSpace(filepath.Clean(dir))
		if dir == "" || dir == "." || seen[dir] {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	if wd, err := os.Getwd(); err == nil {
		add(wd)
	}
	if exe, err := os.Executable(); err == nil && strings.TrimSpace(exe) != "" {
		exeDir := filepath.Dir(exe)
		add(exeDir)
		add(filepath.Join(exeDir, "bin"))
		add(filepath.Join(exeDir, "..", "Resources"))
		add(filepath.Join(exeDir, "..", "Resources", "bin"))
		add(filepath.Join(exeDir, "..", "Resources", "ffmpeg"))
	}
	if helper, err := locateHelper(); err == nil {
		helperDir := filepath.Dir(helper)
		add(helperDir)
		add(filepath.Join(helperDir, "bin"))
		add(filepath.Join(helperDir, "ffmpeg"))
	}
	if dir, err := runtimeDir(); err == nil {
		// long 2026-06-27 23:10:00：桌面端会把 helper 放进用户配置目录；把这个目录也传给 CLI，便于用户临时把 ffmpeg 放到同一运行目录排障。
		add(dir)
		add(filepath.Join(dir, "bin"))
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
	if path := detectToolPathFromLoginShell(names...); path != "" {
		return path
	}
	return ""
}

func detectToolPathFromLoginShell(names ...string) string {
	shell := strings.TrimSpace(desktopLoginShellPath)
	if shell == "" || !fileExists(shell) {
		return ""
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || !isBareToolName(name) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		out, err := exec.CommandContext(ctx, shell, "-lc", "command -v -- "+shellQuote(name)).Output()
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
	dirs := make([]string, 0, len(names))
	for _, name := range names {
		path := detectToolPathFromLoginShell(name)
		if path == "" {
			continue
		}
		dir := filepath.Dir(path)
		if dir == "." || seen[dir] {
			continue
		}
		// long 2026-06-27 11:36:00：Finder 启动的桌面 app 不继承终端 PATH；把登录 shell 能解析到的工具目录补进 helper 环境，避免下载阶段再次找不到 ffmpeg。
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	return dirs
}

func (s *desktopState) resolveTaskToolPaths(task *desktopTask) error {
	if task == nil || task.Channel == "账号" || task.Mode == "仅查看" {
		return nil
	}
	if taskNeedsMuxTool(task.Args) {
		if boolArgEnabled(task.Args, "--use-mp4box") {
			explicit := desktopToolPathValue(effectiveValueFlag(task.Args, "--mp4box-path", task.MP4BoxPath))
			path, err := resolveDesktopToolPath(explicit, "mp4box", "MP4Box", "MP4box")
			if err != nil {
				return err
			}
			task.MP4BoxPath = path
			task.Args = upsertValueFlag(task.Args, "--mp4box-path", path)
			if s.mp4boxPathEntry != nil {
				s.mp4boxPathEntry.SetText(path)
			}
		} else {
			explicit := desktopToolPathValue(effectiveValueFlag(task.Args, "--ffmpeg-path", task.FFmpegPath))
			path, err := resolveDesktopToolPath(explicit, "ffmpeg")
			if err != nil {
				path = deferredDesktopToolPath(explicit, "ffmpeg")
				if path == "" {
					return err
				}
			}
			task.FFmpegPath = path
			task.Args = upsertValueFlag(task.Args, "--ffmpeg-path", path)
			if s.ffmpegPathEntry != nil {
				s.ffmpegPathEntry.SetText(path)
			}
		}
	}
	if taskNeedsAria2cTool(task.Args) {
		explicit := desktopToolPathValue(effectiveValueFlag(task.Args, "--aria2c-path", task.Aria2cPath))
		path, err := resolveDesktopToolPath(explicit, "aria2c")
		if err != nil {
			return err
		}
		task.Aria2cPath = path
		task.Args = upsertValueFlag(task.Args, "--aria2c-path", path)
		if s.aria2cPathEntry != nil {
			s.aria2cPathEntry.SetText(path)
		}
	}
	s.savePreferences()
	return nil
}

func deferredDesktopToolPath(explicit string, names ...string) string {
	if len(names) == 0 {
		return ""
	}
	name := strings.TrimSpace(names[0])
	if name == "" {
		return ""
	}
	explicit = strings.TrimSpace(explicit)
	if explicit != "" && isBareToolName(explicit) {
		return explicit
	}
	// long 2026-06-27 23:58:00：桌面壳和 helper 的 PATH 来源不完全一致；GUI 预检找不到默认工具时，交给同版本 helper 做最终解析，避免 Finder 环境差异提前阻断下载。
	return name
}

func effectiveValueFlag(args []string, flag, fallback string) string {
	value := strings.TrimSpace(fallback)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == flag {
			if i+1 < len(args) {
				// long 2026-06-28 00:34:00：桌面表单先生成常用参数，额外参数随后追加；按 CLI 解析顺序读取最后一个显式值，才能让高级用户在额外参数里覆盖表单工具路径。
				value = strings.TrimSpace(args[i+1])
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, flag+"=") {
			value = strings.TrimSpace(strings.TrimPrefix(arg, flag+"="))
		}
	}
	return value
}

func taskNeedsMuxTool(args []string) bool {
	if boolArgEnabled(args, "--skip-mux") ||
		taskOnlyShowsInfo(args) ||
		boolArgEnabled(args, "--cover-only") ||
		boolArgEnabled(args, "--danmaku-only") ||
		boolArgEnabled(args, "--sub-only") ||
		boolArgEnabled(args, "--audio-only") ||
		boolArgEnabled(args, "--video-only") {
		return false
	}
	return true
}

func taskNeedsAria2cTool(args []string) bool {
	return boolAnyArgEnabled(args, "--use-aria2c", "--aria2") && !taskOnlyShowsInfo(args)
}

func taskOnlyShowsInfo(args []string) bool {
	if len(args) > 0 && args[0] == "info" {
		return true
	}
	return boolArgEnabled(args, "--only-show-info") || boolArgEnabled(args, "--info")
}

func resolveDesktopToolPath(explicit string, names ...string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if path := findDesktopToolPath(explicit, names...); path != "" {
		return path, nil
	}
	if explicit != "" && !isBareToolName(explicit) {
		if path := findDesktopToolPath("", names...); path != "" {
			// long 2026-06-27 23:44:00：桌面历史和偏好里可能留着旧机器或旧 Homebrew 前缀的绝对路径；只要本机还能自动找到同名工具，就修正任务参数继续下载。
			return path, nil
		}
	}
	name := "工具"
	if len(names) > 0 {
		name = names[0]
	}
	label := desktopToolDisplayName(name)
	message := fmt.Sprintf("找不到可执行的%s文件。请在 %s 输入框填写完整路径，或安装后重试；已检查当前目录、程序目录、PATH、常见目录：%s", name, label, strings.Join(commonToolDirs(), "、"))
	if runtime.GOOS == "darwin" {
		message += "；macOS 下也会读取 zsh 登录环境中的 command -v 结果"
		if strings.EqualFold(name, "ffmpeg") {
			message += "；未安装时可先执行 brew install ffmpeg"
		}
	}
	if explicit != "" && !isBareToolName(explicit) {
		message += "；显式路径不存在：" + explicit
	}
	return "", errors.New(message)
}

func findDesktopToolPath(explicit string, names ...string) string {
	if path := bbdown.FindBinaryPath(explicit, names...); path != "" {
		return path
	}
	explicit = strings.TrimSpace(explicit)
	candidates := make([]string, 0, len(names)+1)
	if explicit != "" && isBareToolName(explicit) {
		candidates = append(candidates, explicit)
	}
	candidates = append(candidates, names...)
	for _, dir := range desktopExtraToolDirs() {
		for _, name := range candidates {
			name = strings.TrimSpace(name)
			if name == "" || !isBareToolName(name) {
				continue
			}
			candidate := filepath.Join(dir, name)
			if fileExists(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func desktopToolDisplayName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ffmpeg":
		return "FFmpeg"
	case "mp4box":
		return "MP4Box"
	case "aria2c":
		return "aria2c"
	default:
		return defaultText(name, "工具")
	}
}

func hasArgFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

func boolArgEnabled(args []string, flag string) bool {
	return boolAnyArgEnabled(args, flag)
}

func boolAnyArgEnabled(args []string, flags ...string) bool {
	enabled := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		for _, flag := range flags {
			if arg == flag {
				if i+1 < len(args) {
					if value, ok := parseDesktopBoolLiteral(args[i+1]); ok {
						// long 2026-06-28 00:12:00：桌面“额外参数”要跟 CLI 一样理解 --flag false；否则预检会把用户显式关闭的 aria2c/skip-mux 当成开启。
						enabled = value
						i++
						break
					}
				}
				enabled = true
				continue
			}
			prefix := flag + "="
			if strings.HasPrefix(arg, prefix) {
				value := strings.TrimSpace(strings.TrimPrefix(arg, prefix))
				// long 2026-06-27 20:32:00：同一个 CLI 选项存在正式名和短别名时，桌面端按命令行实际出现顺序取最后一次生效值，避免额外参数里关闭别名后预检仍误认为需要 aria2c。
				enabled, _ = parseDesktopBoolLiteral(value)
			}
		}
	}
	return enabled
}

func parseDesktopBoolLiteral(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

func upsertValueFlag(args []string, flag, value string) []string {
	cleaned := make([]string, 0, len(args)+2)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == flag {
			i++
			continue
		}
		if strings.HasPrefix(arg, flag+"=") {
			continue
		}
		cleaned = append(cleaned, arg)
	}
	insertAt := len(cleaned)
	if insertAt > 0 {
		insertAt = len(cleaned) - 1
	}
	cleaned = append(cleaned, "", "")
	copy(cleaned[insertAt+2:], cleaned[insertAt:])
	cleaned[insertAt] = flag
	cleaned[insertAt+1] = value
	return cleaned
}

func isBareToolName(name string) bool {
	return !strings.ContainsAny(name, `/\`)
}

func commonToolDirs() []string {
	dirs := []string{}
	if home := userHomeDir(); home != "." {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin"))
	}
	switch runtime.GOOS {
	case "darwin":
		// long 2026-06-26 00:21:48：macOS GUI app 不读取用户 shell 初始化文件，Homebrew/Port 常见目录需要主动补进子进程环境。
		dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin")
	case "linux":
		dirs = append(dirs, "/usr/local/bin", "/usr/bin", "/bin", "/snap/bin")
	case "windows":
		dirs = append(dirs, `C:\ffmpeg\bin`, `C:\Program Files\ffmpeg\bin`)
	}
	return dirs
}

func openPath(path string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", path).Start()
	case "windows":
		_ = exec.Command("cmd", "/C", "start", "", path).Start()
	default:
		_ = exec.Command("xdg-open", path).Start()
	}
}

func revealPath(path string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", "-R", path).Start()
	case "windows":
		_ = exec.Command("explorer", "/select,", path).Start()
	default:
		_ = exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}

func shellJoin(args []string) string {
	return shellJoinForOS(runtime.GOOS, args)
}

func shellJoinForOS(goos string, args []string) string {
	items := make([]string, 0, len(args))
	for _, arg := range args {
		items = append(items, shellQuoteForOS(goos, arg))
	}
	return strings.Join(items, " ")
}

func shellQuote(value string) string {
	return shellQuoteForOS(runtime.GOOS, value)
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
	backslashes := 0
	for _, r := range value {
		if r == '\\' {
			backslashes++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
			continue
		}
		if backslashes > 0 {
			b.WriteString(strings.Repeat(`\`, backslashes))
			backslashes = 0
		}
		b.WriteRune(r)
	}
	if backslashes > 0 {
		// long 2026-06-28 01:29:00：Windows 双引号参数末尾的反斜杠需要翻倍，否则会转义结束引号，复制命令后 helper 收到的路径会少一个路径分隔符。
		b.WriteString(strings.Repeat(`\`, backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}

func shellSafeArg(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			continue
		case r >= 'A' && r <= 'Z':
			continue
		case r >= '0' && r <= '9':
			continue
		case strings.ContainsRune("_@%+=:,./-", r):
			continue
		default:
			return false
		}
	}
	return true
}

func splitArgs(input string) ([]string, error) {
	args := []string{}
	var current strings.Builder
	var quote rune
	tokenStarted := false
	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\':
			if i+1 < len(runes) && splitArgBackslashEscapes(runes[i+1], quote) {
				i++
				current.WriteRune(runes[i])
			} else {
				current.WriteRune(r)
			}
			tokenStarted = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
				tokenStarted = true
			}
		case r == '\'' || r == '"':
			quote = r
			tokenStarted = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if tokenStarted {
				args = append(args, current.String())
				current.Reset()
				tokenStarted = false
			}
		default:
			current.WriteRune(r)
			tokenStarted = true
		}
	}
	if quote != 0 {
		return nil, errors.New("额外参数引号未闭合")
	}
	// long 2026-06-28 01:02:00：空引号是有效 CLI 值，例如 --download-danmaku-formats ""；桌面额外参数不能把它丢掉，否则下一个 URL 会被误当成该选项的值。
	if tokenStarted {
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

func defaultText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func streamIndexValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "自动" {
		return ""
	}
	digits := leadingDigits(trimmed)
	if digits == "" {
		return trimmed
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, digits))
	if rest == "" || strings.HasPrefix(rest, ".") {
		return digits
	}
	return trimmed
}

func streamIndexOptionsFromLog(logText string) ([]string, []string) {
	var videoOptions []string
	var audioOptions []string
	section := ""
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
		switch section {
		case "video":
			videoOptions = append(videoOptions, option)
		case "audio":
			audioOptions = append(audioOptions, option)
		}
	}
	return videoOptions, audioOptions
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
	for i, r := range value {
		if r < '0' || r > '9' {
			return value[:i]
		}
	}
	return value
}

func progressPercentText(progress float64) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return fmt.Sprintf("%.0f%%", progress*100)
}

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

func formatElapsed(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	totalSeconds := int(value.Round(time.Second).Seconds())
	hours := totalSeconds / 3600
	minutes := totalSeconds % 3600 / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
