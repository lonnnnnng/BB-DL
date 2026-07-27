package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type TaskManager struct {
	mu              sync.Mutex
	ctx             context.Context
	tasks           []*desktopTask
	nextID          int
	runningTaskID   int
	preferences     Preferences
	status          string
	lastHistorySave time.Time
	historyPath     string
	backgroundWG    sync.WaitGroup
	shuttingDown    bool
	tickerStop      chan struct{}
	nextLoginID     int
	login           *loginSession
}

func NewTaskManager() *TaskManager {
	historyPath, _ := taskHistoryPath()
	return &TaskManager{nextID: 1, nextLoginID: 1, preferences: defaultPreferences(), status: "就绪", historyPath: historyPath}
}

func (manager *TaskManager) Startup(ctx context.Context) {
	manager.mu.Lock()
	manager.shuttingDown = false
	manager.ctx = ctx
	manager.status = "就绪"
	if path, err := preferencesPath(); err == nil {
		if preferences, err := loadPreferencesFile(path); err == nil {
			manager.preferences = preferences
		} else {
			manager.status = "加载偏好失败：" + err.Error()
		}
	}
	if path := manager.historyPath; path != "" {
		if tasks, nextID, err := loadTaskHistoryFile(path); err == nil {
			manager.tasks, manager.nextID = downloadTasksOnly(tasks), nextID
			// long 2026-07-26 20:35:19：Wails 登录已迁为临时会话，旧版账号任务需要从历史中永久移除，避免下次启动再次污染下载列表。
			if len(manager.tasks) != len(tasks) {
				_ = saveTaskHistoryFile(path, manager.tasks)
			}
		} else {
			manager.status = "加载任务历史失败：" + err.Error()
		}
	}
	if manager.tickerStop == nil {
		manager.tickerStop = make(chan struct{})
		manager.backgroundWG.Add(1)
		go func(stop <-chan struct{}) {
			defer manager.backgroundWG.Done()
			manager.runTicker(stop)
		}(manager.tickerStop)
	}
	manager.mu.Unlock()
}

func (manager *TaskManager) Shutdown() {
	manager.mu.Lock()
	manager.shuttingDown = true
	if manager.tickerStop != nil {
		close(manager.tickerStop)
		manager.tickerStop = nil
	}
	var handle commandHandle
	var loginHandle commandHandle
	if task := manager.taskByIDLocked(manager.runningTaskID); task != nil {
		handle = task.command
		task.StopRequested = true
		task.Status = statusStopped
		task.command = nil
		task.EndedAt = time.Now()
	}
	manager.runningTaskID = 0
	if manager.login != nil && manager.login.Running {
		manager.login.StopRequested = true
		manager.login.Running = false
		manager.login.Status = "已取消"
		loginHandle = manager.login.command
		manager.login.command = nil
	}
	manager.saveHistoryLocked(time.Now())
	manager.mu.Unlock()
	if handle != nil {
		_ = handle.Kill()
	}
	if loginHandle != nil {
		_ = loginHandle.Kill()
	}
	// long 2026-07-27 01:41:55：应用退出和测试清理前必须等 helper 输出、Wait 和历史落盘全部结束，避免后台收尾继续写入已经释放的运行目录。
	manager.backgroundWG.Wait()
}

func (manager *TaskManager) Bootstrap() Bootstrap {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return Bootstrap{Preferences: manager.preferences, Tasks: manager.taskDTOsLocked(), Summary: manager.summaryLocked(), Version: appVersion(), BuildTime: appBuildTime(), Status: manager.status}
}

func (manager *TaskManager) Preferences() Preferences {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.preferences
}

func (manager *TaskManager) SavePreferences(preferences Preferences) error {
	preferences = normalizePreferences(preferences)
	path, err := preferencesPath()
	if err != nil {
		return err
	}
	if err := saveJSONAtomic(path, preferences); err != nil {
		return err
	}
	manager.mu.Lock()
	manager.preferences = preferences
	manager.status = "设置已保存"
	manager.mu.Unlock()
	manager.emit("preferences:updated", preferences)
	manager.emitStatus("设置已保存")
	return nil
}

func (manager *TaskManager) CreateTask(input TaskInput, startNow bool) (TaskDTO, error) {
	manager.mu.Lock()
	id := manager.nextID
	manager.nextID++
	manager.mu.Unlock()
	task, err := taskFromInput(id, input)
	if err != nil {
		return TaskDTO{}, err
	}
	manager.mu.Lock()
	manager.tasks = append(manager.tasks, task)
	manager.preferences = normalizePreferences(input.Preferences)
	manager.status = fmt.Sprintf("任务 #%d 已加入队列", task.ID)
	manager.saveHistoryLocked(time.Now())
	dto := manager.taskDTOLocked(task)
	manager.mu.Unlock()
	if path, pathErr := preferencesPath(); pathErr == nil {
		_ = saveJSONAtomic(path, manager.Preferences())
	}
	manager.emit("task:added", dto)
	manager.emitSummary()
	manager.emitStatus(fmt.Sprintf("任务 #%d 已加入队列", task.ID))
	if startNow {
		if err := manager.StartTask(task.ID); err != nil {
			return manager.Task(task.ID), err
		}
		return manager.Task(task.ID), nil
	}
	return dto, nil
}

func (manager *TaskManager) StartTask(id int) error {
	manager.mu.Lock()
	if manager.shuttingDown {
		manager.mu.Unlock()
		return errors.New("应用正在退出")
	}
	task := manager.taskByIDLocked(id)
	if task == nil {
		manager.mu.Unlock()
		return errors.New("任务不存在")
	}
	if manager.runningTaskID != 0 {
		manager.mu.Unlock()
		return errors.New("已有任务运行中")
	}
	if len(task.Args) == 0 {
		args, err := rebuildTaskArgs(task)
		if err != nil {
			return manager.completeFailedStartLocked(task, err)
		}
		task.Args = args
	}
	if err := resolveTaskToolPaths(task); err != nil {
		return manager.completeFailedStartLocked(task, err)
	}
	runtimeDir, helper, err := prepareRuntime()
	if err != nil {
		return manager.completeFailedStartLocked(task, err)
	}
	if task.WorkDir != "" {
		if err := os.MkdirAll(task.WorkDir, 0o755); err != nil {
			return manager.completeFailedStartLocked(task, err)
		}
		task.Baseline = snapshotFiles(task.WorkDir)
	}
	command := exec.Command(helper, task.Args...)
	command.Dir = runtimeDir
	command.Env = append(desktopCommandEnv(), serverChildEnv, forcePlainTextEnv)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return manager.completeFailedStartLocked(task, err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return manager.completeFailedStartLocked(task, err)
	}
	task.Status = statusRunning
	task.StartedAt = time.Now()
	task.EndedAt = time.Time{}
	task.RuntimeDir = runtimeDir
	task.StopRequested = false
	task.Bytes = 0
	task.EventBytes = 0
	task.FileGrowthBytes = 0
	task.Progress = 0
	task.ProgressState = progressState{}
	task.TransferPathBytes = map[string]int64{}
	task.LastTransferAt = time.Time{}
	task.LastSpeedBytes = 0
	task.Files = nil
	task.Log.Reset()
	task.Log.WriteString("$ " + shellJoin(append([]string{helper}, task.Args...)) + "\n")
	manager.runningTaskID = id
	if err := command.Start(); err != nil {
		return manager.completeFailedStartLocked(task, err)
	}
	task.command = processHandle{process: command.Process}
	manager.backgroundWG.Add(1)
	manager.status = fmt.Sprintf("任务 #%d 已开始", id)
	manager.saveHistoryLocked(time.Now())
	manager.mu.Unlock()
	manager.emit("task:log-reset", map[string]any{"taskID": id, "log": "$ " + shellJoin(append([]string{helper}, task.Args...)) + "\n"})
	manager.notifyTask(id, "task:updated")
	manager.emitTasksReset()
	manager.emitSummary()
	manager.emitStatus(fmt.Sprintf("任务 #%d 已开始", id))
	var outputWG sync.WaitGroup
	outputWG.Add(2)
	go func() { defer outputWG.Done(); manager.consumeOutput(id, stdout) }()
	go func() { defer outputWG.Done(); manager.consumeOutput(id, stderr) }()
	go func() {
		defer manager.backgroundWG.Done()
		// long 2026-07-26 22:54:26：helper 退出前必须先排空两条输出管道，否则 Linux 会在 Wait 关闭管道时丢失末尾传输事件和完成日志。
		outputWG.Wait()
		manager.finishTask(id, command.Wait())
	}()
	return nil
}

func (manager *TaskManager) consumeOutput(id int, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		manager.handleLine(id, scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
		manager.appendLog(id, "读取输出失败："+err.Error())
	}
}

func (manager *TaskManager) handleLine(id int, line string) {
	if event, ok := parseTransferEvent(line); ok {
		manager.mu.Lock()
		task := manager.taskByIDLocked(id)
		changed := applyTransferEvent(task, event, time.Now())
		manager.mu.Unlock()
		if changed {
			manager.notifyTask(id, "task:updated")
		}
		return
	}
	manager.mu.Lock()
	task := manager.taskByIDLocked(id)
	if task == nil {
		manager.mu.Unlock()
		return
	}
	task.Log.WriteString(line + "\n")
	progressChanged := applyProgressLine(task, line)
	manager.mu.Unlock()
	manager.emit("task:log", map[string]any{"taskID": id, "line": line})
	if progressChanged {
		manager.notifyTask(id, "task:updated")
	}
}

func (manager *TaskManager) appendLog(id int, line string) {
	manager.mu.Lock()
	if task := manager.taskByIDLocked(id); task != nil {
		task.Log.WriteString(line + "\n")
	}
	manager.mu.Unlock()
	manager.emit("task:log", map[string]any{"taskID": id, "line": line})
}

func (manager *TaskManager) finishTask(id int, runErr error) {
	manager.mu.Lock()
	task := manager.taskByIDLocked(id)
	if task == nil {
		manager.mu.Unlock()
		return
	}
	task.EndedAt = time.Now()
	task.command = nil
	task.Files = changedFiles(task.WorkDir, task.Baseline)
	task.LastSpeedBytes = 0
	finalLogLines := []string{""}
	if task.StopRequested {
		task.Status = statusStopped
		task.Log.WriteString("\n任务已停止。\n")
		finalLogLines = append(finalLogLines, "任务已停止。")
	} else if runErr != nil {
		task.Status = statusFailed
		task.Log.WriteString("\n任务失败：" + runErr.Error() + "\n")
		finalLogLines = append(finalLogLines, "任务失败："+runErr.Error())
		if hint := desktopStartupHint(errors.New(task.Log.String())); hint != "" {
			task.Log.WriteString(hint + "\n")
			finalLogLines = append(finalLogLines, hint)
		}
	} else {
		task.Status = statusSuccess
		setProgress(task, 1)
		task.Log.WriteString("\n任务完成。\n")
		finalLogLines = append(finalLogLines, "任务完成。")
	}
	if manager.runningTaskID == id {
		manager.runningTaskID = 0
	}
	manager.status = completionStatus(task)
	autoQueue := !manager.shuttingDown && manager.preferences.AutoQueue && task.Status != statusStopped
	manager.saveHistoryLocked(time.Now())
	manager.mu.Unlock()
	for _, line := range finalLogLines {
		manager.emit("task:log", map[string]any{"taskID": id, "line": line})
	}
	manager.notifyTask(id, "task:finished")
	manager.emitTasksReset()
	manager.emitSummary()
	manager.emitStatus(completionStatus(task))
	if autoQueue {
		_ = manager.StartNextPending()
	}
}

func (manager *TaskManager) failBeforeStartLocked(task *desktopTask, err error) []string {
	lines := []string{"启动失败：" + err.Error()}
	task.Status = statusFailed
	task.EndedAt = time.Now()
	task.command = nil
	task.Log.WriteString(lines[0] + "\n")
	if hint := desktopStartupHint(err); hint != "" {
		task.Log.WriteString(hint + "\n")
		lines = append(lines, hint)
		manager.status = hint
	} else {
		manager.status = "启动失败：" + err.Error()
	}
	if manager.runningTaskID == task.ID {
		manager.runningTaskID = 0
	}
	manager.saveHistoryLocked(time.Now())
	return lines
}

// long 2026-07-26 20:35:19：启动前失败也属于队列中的一次终态，必须先完整广播失败结果，再按自动队列顺序继续，避免坏任务把后续下载全部卡住。
func (manager *TaskManager) completeFailedStartLocked(task *desktopTask, err error) error {
	id := task.ID
	lines := manager.failBeforeStartLocked(task, err)
	autoQueue := !manager.shuttingDown && manager.preferences.AutoQueue
	status := manager.status
	manager.mu.Unlock()
	for _, line := range lines {
		manager.emit("task:log", map[string]any{"taskID": id, "line": line})
	}
	manager.notifyTask(id, "task:finished")
	manager.emitTasksReset()
	manager.emitSummary()
	manager.emitStatus(status)
	if autoQueue {
		_ = manager.StartNextPending()
	}
	return err
}

func desktopStartupHint(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if strings.Contains(text, "找不到可执行的ffmpeg文件") {
		return missingToolHint("完整下载需要 FFmpeg 混流", "FFmpeg", "ffmpeg")
	}
	if strings.Contains(text, "找不到可执行的mp4box文件") {
		return missingToolHint("当前任务需要 MP4Box 混流", "MP4Box", "gpac")
	}
	if strings.Contains(text, "找不到可执行的aria2c文件") {
		return missingToolHint("当前任务启用了 aria2c 下载", "aria2c", "aria2")
	}
	return ""
}
func missingToolHint(reason, label, brew string) string {
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf("提示：%s；请在设置中检测工具，或执行 brew install %s 后重试。", reason, brew)
	}
	return fmt.Sprintf("提示：%s；请安装 %s 后在设置中填写完整路径。", reason, label)
}

func (manager *TaskManager) StopTask(id int) error {
	manager.mu.Lock()
	task := manager.taskByIDLocked(id)
	if task == nil {
		manager.mu.Unlock()
		return errors.New("任务不存在")
	}
	if task.Status != statusRunning && task.Status != statusStopping {
		manager.mu.Unlock()
		return errors.New("任务当前未运行")
	}
	task.StopRequested = true
	task.Status = statusStopping
	handle := task.command
	task.Log.WriteString("\n已请求停止当前任务。\n")
	manager.status = fmt.Sprintf("正在停止任务 #%d", id)
	manager.saveHistoryLocked(time.Now())
	manager.mu.Unlock()
	if handle != nil {
		_ = handle.Kill()
	}
	manager.emit("task:log", map[string]any{"taskID": id, "line": "\n已请求停止当前任务。"})
	manager.notifyTask(id, "task:updated")
	manager.emitStatus(fmt.Sprintf("正在停止任务 #%d", id))
	return nil
}

func (manager *TaskManager) StopRunningTask() {
	manager.mu.Lock()
	id := manager.runningTaskID
	manager.mu.Unlock()
	if id != 0 {
		_ = manager.StopTask(id)
	}
}
func (manager *TaskManager) HasRunningTask() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.runningTaskID != 0
}

func (manager *TaskManager) StartNextPending() error {
	manager.mu.Lock()
	if manager.runningTaskID != 0 {
		manager.mu.Unlock()
		return errors.New("已有任务运行中")
	}
	id := 0
	for _, task := range manager.tasks {
		if task.Status == statusPending {
			id = task.ID
			break
		}
	}
	manager.mu.Unlock()
	if id == 0 {
		return errors.New("没有等待中的任务")
	}
	return manager.StartTask(id)
}

func (manager *TaskManager) RetryTask(id int) (TaskDTO, error) {
	manager.mu.Lock()
	if manager.runningTaskID != 0 {
		manager.mu.Unlock()
		return TaskDTO{}, errors.New("已有任务运行中")
	}
	source := manager.taskByIDLocked(id)
	if source == nil {
		manager.mu.Unlock()
		return TaskDTO{}, errors.New("任务不存在")
	}
	if !isRetryableStatus(source.Status) && source.Status != statusSuccess {
		manager.mu.Unlock()
		return TaskDTO{}, errors.New("当前任务不可重试")
	}
	retry := source.cloneForRetry(manager.nextID)
	manager.nextID++
	manager.tasks = append(manager.tasks, retry)
	manager.saveHistoryLocked(time.Now())
	dto := manager.taskDTOLocked(retry)
	manager.mu.Unlock()
	manager.emit("task:added", dto)
	manager.emitSummary()
	if err := manager.StartTask(retry.ID); err != nil {
		return manager.Task(retry.ID), err
	}
	return manager.Task(retry.ID), nil
}

func (manager *TaskManager) RetryFailedTasks() ([]TaskDTO, error) {
	manager.mu.Lock()
	if manager.runningTaskID != 0 {
		manager.mu.Unlock()
		return nil, errors.New("已有任务运行中")
	}
	added := []TaskDTO{}
	existing := append([]*desktopTask(nil), manager.tasks...)
	for _, task := range existing {
		if isRetryableStatus(task.Status) {
			retry := task.cloneForRetry(manager.nextID)
			manager.nextID++
			manager.tasks = append(manager.tasks, retry)
			added = append(added, manager.taskDTOLocked(retry))
		}
	}
	if len(added) == 0 {
		manager.mu.Unlock()
		return nil, errors.New("没有失败或已停止任务可重试")
	}
	auto := manager.preferences.AutoQueue
	manager.saveHistoryLocked(time.Now())
	manager.mu.Unlock()
	for _, task := range added {
		manager.emit("task:added", task)
	}
	manager.emitSummary()
	if auto {
		// long 2026-07-26 20:35:19：批量重试只负责追加副本；实际启动仍从最早的等待任务开始，保留用户已经排好的队列顺序。
		_ = manager.StartNextPending()
	}
	return added, nil
}

func (manager *TaskManager) DeleteTask(id int) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for index, task := range manager.tasks {
		if task.ID != id {
			continue
		}
		if task.Status == statusRunning || task.Status == statusStopping {
			return errors.New("运行中的任务不能删除")
		}
		manager.tasks = append(manager.tasks[:index], manager.tasks[index+1:]...)
		manager.saveHistoryLocked(time.Now())
		go manager.emit("task:deleted", map[string]int{"taskID": id})
		go manager.emitSummary()
		return nil
	}
	return errors.New("任务不存在")
}

func (manager *TaskManager) ClearEndedTasks() (int, error) {
	manager.mu.Lock()
	active := make([]*desktopTask, 0, len(manager.tasks))
	removed := 0
	for _, task := range manager.tasks {
		if isActiveStatus(task.Status) {
			active = append(active, task)
		} else {
			removed++
		}
	}
	manager.tasks = active
	manager.saveHistoryLocked(time.Now())
	manager.mu.Unlock()
	manager.emit("tasks:reset", manager.Tasks())
	manager.emitSummary()
	return removed, nil
}

func (manager *TaskManager) FillFormData(id int) (TaskInput, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	task := manager.taskByIDLocked(id)
	if task == nil {
		return TaskInput{}, errors.New("任务不存在")
	}
	if task.Channel == "账号" {
		return TaskInput{}, errors.New("登录任务不能填入下载表单")
	}
	input := taskInputFromTask(task)
	// long 2026-07-27 14:35:00：填入表单用于创建新任务，同名文件策略应采用当前已保存默认值，避免旧任务的 skip 反向覆盖用户刚保存的 rename/overwrite。
	input.FileExistsAction = manager.preferences.FileExistsAction
	input.AutoQueue = manager.preferences.AutoQueue
	input.Theme = manager.preferences.Theme
	return input, nil
}
func (manager *TaskManager) TaskLog(id int) (string, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	task := manager.taskByIDLocked(id)
	if task == nil {
		return "", errors.New("任务不存在")
	}
	return task.Log.String(), nil
}
func (manager *TaskManager) TaskCommand(id int) (string, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	task := manager.taskByIDLocked(id)
	if task == nil {
		return "", errors.New("任务不存在")
	}
	return manager.taskCommandLocked(task), nil
}

func (manager *TaskManager) Task(id int) TaskDTO {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.taskDTOLocked(manager.taskByIDLocked(id))
}
func (manager *TaskManager) Tasks() []TaskDTO {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.taskDTOsLocked()
}
func (manager *TaskManager) taskByIDLocked(id int) *desktopTask {
	for _, task := range manager.tasks {
		if task.ID == id {
			return task
		}
	}
	return nil
}
func (manager *TaskManager) taskDTOsLocked() []TaskDTO {
	result := make([]TaskDTO, 0, len(manager.tasks))
	for _, task := range manager.tasks {
		result = append(result, manager.taskDTOLocked(task))
	}
	return result
}

func (manager *TaskManager) taskDTOLocked(task *desktopTask) TaskDTO {
	if task == nil {
		return TaskDTO{}
	}
	now := time.Now()
	video, audio := streamIndexOptionsFromLog(task.Log.String())
	running := manager.runningTaskID != 0
	canStop := task.Status == statusRunning || task.Status == statusStopping
	return TaskDTO{ID: task.ID, URL: task.URL, WorkDir: task.WorkDir, Mode: task.Mode, Channel: task.Channel, SelectPage: task.SelectPage, VideoIndex: task.VideoIndex, AudioIndex: task.AudioIndex, DfnPriority: task.DfnPriority, EncodingPriority: task.EncodingPriority, ExtraArgs: task.ExtraArgs, FFmpegPath: task.FFmpegPath, MP4BoxPath: task.MP4BoxPath, Aria2cPath: task.Aria2cPath, FileExistsAction: task.FileExistsAction, Args: append([]string(nil), task.Args...), Status: string(task.Status), Bytes: task.Bytes, Progress: task.Progress, Files: append([]ManagedFile(nil), task.Files...), CreatedAt: task.CreatedAt, StartedAt: task.StartedAt, EndedAt: task.EndedAt, CurrentSpeed: currentTaskSpeed(task, now), AverageSpeed: averageTaskSpeed(task, now), ElapsedSeconds: int64(taskElapsed(task, now).Seconds()), VideoOptions: video, AudioOptions: audio, Command: manager.taskCommandLocked(task), Actions: TaskActions{CanStart: !running && !canStop, CanStop: canStop, CanRetry: !running && isRetryableStatus(task.Status), CanDelete: !canStop}}
}

func (manager *TaskManager) taskCommandLocked(task *desktopTask) string {
	if task == nil || len(task.Args) == 0 {
		return ""
	}
	helper := helperName()
	if task.RuntimeDir != "" && fileExists(filepath.Join(task.RuntimeDir, helperName())) {
		helper = filepath.Join(task.RuntimeDir, helperName())
	} else if located, err := locateHelper(); err == nil {
		helper = located
	}
	return shellJoin(append([]string{helper}, task.Args...))
}

func (manager *TaskManager) summaryLocked() QueueSummary {
	summary := QueueSummary{Total: len(manager.tasks)}
	for _, task := range manager.tasks {
		switch task.Status {
		case statusPending:
			summary.Pending++
		case statusRunning, statusStopping:
			summary.Running++
		case statusSuccess:
			summary.Success++
		case statusFailed:
			summary.Failed++
		case statusStopped:
			summary.Stopped++
		}
	}
	return summary
}
func (manager *TaskManager) saveHistoryLocked(now time.Time) {
	path := manager.historyPath
	if path == "" {
		var err error
		path, err = taskHistoryPath()
		if err != nil {
			manager.status = "保存任务历史失败：" + err.Error()
			return
		}
		manager.historyPath = path
	}
	// long 2026-07-27 01:01:46：helper 收尾在后台 goroutine 中执行，固定管理器所属历史路径，避免环境变量切换后把旧任务写进另一个运行目录。
	if err := saveTaskHistoryFile(path, manager.tasks); err != nil {
		manager.status = "保存任务历史失败：" + err.Error()
		return
	}
	manager.lastHistorySave = now
}

func (manager *TaskManager) runTicker(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			manager.mu.Lock()
			id := manager.runningTaskID
			if task := manager.taskByIDLocked(id); task != nil {
				applyFileGrowthFallback(task, now)
				if shouldSaveTaskHistory(manager.lastHistorySave, now) {
					manager.saveHistoryLocked(now)
				}
			}
			manager.mu.Unlock()
			if id != 0 {
				manager.notifyTask(id, "task:updated")
			}
		case <-stop:
			return
		}
	}
}

func (manager *TaskManager) notifyTask(id int, event string) {
	dto := manager.Task(id)
	if dto.ID != 0 {
		manager.emit(event, dto)
	}
}
func (manager *TaskManager) emitSummary() {
	manager.mu.Lock()
	summary := manager.summaryLocked()
	manager.mu.Unlock()
	manager.emit("queue:summary", summary)
}
func (manager *TaskManager) emitTasksReset() {
	manager.emit("tasks:reset", manager.Tasks())
}
func (manager *TaskManager) emitStatus(message string) {
	manager.mu.Lock()
	manager.status = message
	manager.mu.Unlock()
	manager.emit("app:status", map[string]string{"message": message})
}
func (manager *TaskManager) emit(name string, payload any) {
	manager.mu.Lock()
	ctx := manager.ctx
	manager.mu.Unlock()
	if ctx != nil {
		wailsruntime.EventsEmit(ctx, name, payload)
	}
}

func completionStatus(task *desktopTask) string {
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
