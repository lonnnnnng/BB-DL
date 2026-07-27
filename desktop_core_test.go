package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTaskInputUsesDetectedStreamIndexes(t *testing.T) {
	task, err := taskFromInput(1, TaskInput{
		URL: "BV1J9EB6xEAB",
		Preferences: Preferences{
			WorkDir: "/tmp/downloads", Mode: "下载", Channel: "WEB",
			VideoIndex: "1. 1920x1080 AVC", AudioIndex: "0. 192K AAC",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.VideoIndex != "1" || task.AudioIndex != "0" {
		t.Fatalf("unexpected indexes: video=%q audio=%q", task.VideoIndex, task.AudioIndex)
	}
	joined := strings.Join(task.Args, " ")
	if !strings.Contains(joined, "--video-index 1") || !strings.Contains(joined, "--audio-index 0") {
		t.Fatalf("stream indexes missing from args: %s", joined)
	}
}

func TestTaskFileExistsActionSurvivesHistoryRetryAndFormFill(t *testing.T) {
	task, err := taskFromInput(7, TaskInput{URL: "BV1J9EB6xEAB", Preferences: Preferences{
		WorkDir: "/tmp/downloads", Mode: "下载", Channel: "WEB", FileExistsAction: "rename",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if task.FileExistsAction != "rename" || !strings.Contains(strings.Join(task.Args, " "), "--file-exists-action rename") {
		t.Fatalf("task action was not included in args: action=%q args=%v", task.FileExistsAction, task.Args)
	}
	restored := restoreTask(persistTask(task))
	if restored.FileExistsAction != "rename" {
		t.Fatalf("restored action = %q", restored.FileExistsAction)
	}
	if retry := restored.cloneForRetry(8); retry.FileExistsAction != "rename" {
		t.Fatalf("retry action = %q", retry.FileExistsAction)
	}
	if form := taskInputFromTask(restored); form.FileExistsAction != "rename" {
		t.Fatalf("filled form action = %q", form.FileExistsAction)
	}
	if got := normalizePreferences(Preferences{}).FileExistsAction; got != "skip" {
		t.Fatalf("legacy preferences action = %q", got)
	}
}

func TestFillFormDataUsesSavedFileExistsActionForNewTask(t *testing.T) {
	manager := NewTaskManager()
	manager.preferences = defaultPreferences()
	manager.preferences.FileExistsAction = "rename"
	oldTask, err := taskFromInput(1, TaskInput{URL: "BV1J9EB6xEAB", Preferences: Preferences{
		WorkDir: "/tmp/downloads", Mode: "下载", Channel: "WEB", FileExistsAction: "skip",
	}})
	if err != nil {
		t.Fatal(err)
	}
	manager.tasks = []*desktopTask{oldTask}

	input, err := manager.FillFormData(oldTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if input.FileExistsAction != "rename" {
		t.Fatalf("filled form action = %q, want saved default rename", input.FileExistsAction)
	}
	if retry := oldTask.cloneForRetry(2); retry.FileExistsAction != "skip" {
		t.Fatalf("retry action = %q, want original task snapshot skip", retry.FileExistsAction)
	}
	newTask, err := taskFromInput(3, input)
	if err != nil {
		t.Fatal(err)
	}
	if newTask.FileExistsAction != "rename" || !strings.Contains(strings.Join(newTask.Args, " "), "--file-exists-action rename") {
		t.Fatalf("new task did not use saved default: action=%q args=%v", newTask.FileExistsAction, newTask.Args)
	}
}

func TestStreamOptionsAndProgressFollowHelperProtocol(t *testing.T) {
	logText := "共计2条视频流.\n  0. 1920x1080 HEVC\n  1. 1920x1080 AVC\n共计2条音频流.\n  0. 192K AAC\n  1. 132K AAC\n"
	video, audio := streamIndexOptionsFromLog(logText)
	if len(video) != 2 || len(audio) != 2 {
		t.Fatalf("unexpected options: video=%v audio=%v", video, audio)
	}
	task := &desktopTask{}
	applyProgressLine(task, "开始解析P1: 测试视频 (1 of 1)")
	if task.ProgressState.CurrentPage != 1 || task.ProgressState.TotalPages != 1 {
		t.Fatalf("page state not initialised: %+v", task.ProgressState)
	}
	if !applyProgressLine(task, "开始下载P1视频...") {
		t.Fatal("video stage should advance progress")
	}
	if task.Progress < progressStageVideoDownloaded {
		t.Fatalf("unexpected progress: %f", task.Progress)
	}
	if !applyProgressLine(task, "任务完成") || task.Progress != 1 {
		t.Fatalf("completion progress=%f", task.Progress)
	}
}

func TestTaskManagerRunsHelperAndCollectsTransferData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试 helper 使用 POSIX shell")
	}
	tempDir := t.TempDir()
	helper := filepath.Join(tempDir, "fake-BB-DL")
	script := `#!/bin/sh
echo '共计2条视频流.'
echo '  0. 1920x1080 HEVC'
echo '  1. 1920x1080 AVC'
echo '共计2条音频流.'
echo '  0. 192K AAC'
echo '  1. 132K AAC'
echo '开始解析P1: 测试视频 (1 of 1)'
echo '__BBDOWN_GO_SERVER_TRANSFER__ {"path":"video.m4s","bytes":2048,"delta":true}'
echo '开始下载P1视频...'
echo '任务完成'
`
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BBDOWN_GO_HELPER", helper)
	t.Setenv(desktopRuntimeDirEnv, filepath.Join(tempDir, "runtime"))

	manager := NewTaskManager()
	task, err := manager.CreateTask(TaskInput{URL: "BV1J9EB6xEAB", Preferences: Preferences{WorkDir: filepath.Join(tempDir, "downloads"), Mode: "仅查看", Channel: "WEB", AutoQueue: false}}, true)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		task = manager.Task(task.ID)
		if task.Status == string(statusSuccess) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if task.Status != string(statusSuccess) {
		t.Fatalf("task did not finish: status=%s log=%s", task.Status, mustTaskLog(t, manager, task.ID))
	}
	if task.Bytes != 2048 || task.Progress != 1 {
		t.Fatalf("unexpected metrics: bytes=%d progress=%f", task.Bytes, task.Progress)
	}
	if len(task.VideoOptions) != 2 || len(task.AudioOptions) != 2 {
		t.Fatalf("stream options not collected: video=%v audio=%v", task.VideoOptions, task.AudioOptions)
	}
}

func TestTaskManagerContinuesQueueAfterPreflightFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试 helper 使用 POSIX shell")
	}
	tempDir := t.TempDir()
	helper := filepath.Join(tempDir, "fake-BB-DL")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\necho '任务完成'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BBDOWN_GO_HELPER", helper)
	t.Setenv(desktopRuntimeDirEnv, filepath.Join(tempDir, "runtime"))
	blockedWorkDir := filepath.Join(tempDir, "blocked-work-dir")
	if err := os.WriteFile(blockedWorkDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewTaskManager()
	failed, err := manager.CreateTask(TaskInput{URL: "BV1J9EB6xEAB", Preferences: Preferences{WorkDir: blockedWorkDir, Mode: "仅查看", Channel: "WEB", AutoQueue: true}}, false)
	if err != nil {
		t.Fatal(err)
	}
	next, err := manager.CreateTask(TaskInput{URL: "BV1J9EB6xEAB", Preferences: Preferences{WorkDir: filepath.Join(tempDir, "downloads"), Mode: "仅查看", Channel: "WEB", AutoQueue: true}}, false)
	if err != nil {
		t.Fatal(err)
	}

	// long 2026-07-26 20:35:19：首条任务预检失败不能阻断队列，后续等待任务仍应由自动队列完成。
	if err := manager.StartTask(failed.ID); err == nil {
		t.Fatal("expected invalid work directory preflight failure")
	}
	waitForTaskStatus(t, manager, next.ID, statusSuccess)
	if task := manager.Task(failed.ID); task.Status != string(statusFailed) {
		t.Fatalf("preflight task status=%s", task.Status)
	}
}

func TestRetryFailedTasksStartsOldestPendingTask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试 helper 使用 POSIX shell")
	}
	tempDir := t.TempDir()
	helper := filepath.Join(tempDir, "fake-BB-DL")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nsleep 1\necho '任务完成'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BBDOWN_GO_HELPER", helper)
	t.Setenv(desktopRuntimeDirEnv, filepath.Join(tempDir, "runtime"))
	blockedWorkDir := filepath.Join(tempDir, "blocked-work-dir")
	if err := os.WriteFile(blockedWorkDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewTaskManager()
	defer manager.Shutdown()
	pending, err := manager.CreateTask(TaskInput{URL: "BV1J9EB6xEAB", Preferences: Preferences{WorkDir: filepath.Join(tempDir, "downloads"), Mode: "仅查看", Channel: "WEB", AutoQueue: false}}, false)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := manager.CreateTask(TaskInput{URL: "BV1J9EB6xEAB", Preferences: Preferences{WorkDir: blockedWorkDir, Mode: "仅查看", Channel: "WEB", AutoQueue: false}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartTask(failed.ID); err == nil {
		t.Fatal("expected invalid work directory preflight failure")
	}
	manager.mu.Lock()
	manager.preferences.AutoQueue = true
	manager.mu.Unlock()

	added, err := manager.RetryFailedTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 {
		t.Fatalf("added retries=%d", len(added))
	}
	// long 2026-07-26 20:35:19：失败副本追加到队尾后，自动启动必须尊重此前已经等待的任务。
	if task := manager.Task(pending.ID); task.Status != string(statusRunning) {
		t.Fatalf("oldest pending task status=%s, retry status=%s", task.Status, manager.Task(added[0].ID).Status)
	}
}

func TestLoginSessionDoesNotCreateDownloadTask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试 helper 使用 POSIX shell")
	}
	for _, kind := range []string{"WEB", "TV"} {
		t.Run(kind, func(t *testing.T) {
			tempDir := t.TempDir()
			helper := filepath.Join(tempDir, "fake-BB-DL")
			script := "#!/bin/sh\ncase \"$1\" in login|logintv) ;; *) exit 2 ;; esac\necho '获取登录地址...'\necho '登录成功, 已保存登录凭据'\n"
			if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			runtimeDir := filepath.Join(tempDir, "runtime")
			t.Setenv("BBDOWN_GO_HELPER", helper)
			t.Setenv(desktopRuntimeDirEnv, runtimeDir)

			manager := NewTaskManager()
			before := manager.summaryLocked()
			session, err := manager.StartLogin(kind)
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				session = manager.LoginSession()
				if !session.Running {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !session.Successful || session.Status != "登录成功" {
				t.Fatalf("login session=%+v", session)
			}
			// long 2026-07-26 20:35:19：账号登录不能占用下载任务编号，也不能改变任务列表、队列汇总或落盘任务历史。
			if len(manager.Tasks()) != 0 || manager.summaryLocked() != before || manager.nextID != 1 {
				t.Fatalf("login polluted download queue: tasks=%d summary=%+v nextID=%d", len(manager.Tasks()), manager.summaryLocked(), manager.nextID)
			}
			historyPath, err := taskHistoryPath()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(historyPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("login created task history: %v", err)
			}
		})
	}
}

func TestTaskManagerKeepsHistoryPathAfterRuntimeEnvironmentChanges(t *testing.T) {
	firstRuntime := filepath.Join(t.TempDir(), "first-runtime")
	secondRuntime := filepath.Join(t.TempDir(), "second-runtime")
	t.Setenv(desktopRuntimeDirEnv, firstRuntime)
	manager := NewTaskManager()
	t.Setenv(desktopRuntimeDirEnv, secondRuntime)

	manager.mu.Lock()
	manager.tasks = []*desktopTask{{ID: 1, URL: "BV1J9EB6xEAB", Status: statusPending, CreatedAt: time.Now()}}
	manager.saveHistoryLocked(time.Now())
	manager.mu.Unlock()

	if _, err := os.Stat(filepath.Join(firstRuntime, desktopTaskHistoryFile)); err != nil {
		t.Fatalf("manager history should stay in its original runtime directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(secondRuntime, desktopTaskHistoryFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manager history leaked into the new runtime directory: %v", err)
	}
}

func TestDownloadTasksOnlyRemovesLegacyLoginTasks(t *testing.T) {
	tasks := []*desktopTask{
		{ID: 1, Channel: "WEB", URL: "BV1J9EB6xEAB"},
		{ID: 2, Channel: "账号", URL: "WEB 登录"},
		{ID: 3, Channel: "账号", URL: "TV 登录"},
	}
	filtered := downloadTasksOnly(tasks)
	if len(filtered) != 1 || filtered[0].ID != 1 {
		t.Fatalf("filtered tasks=%+v", filtered)
	}
}

func TestLoginSessionTreatsExpiredQRCodeAsUnsuccessful(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试 helper 使用 POSIX shell")
	}
	tempDir := t.TempDir()
	helper := filepath.Join(tempDir, "fake-BB-DL")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\necho '二维码已过期, 请重新执行登录指令.'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BBDOWN_GO_HELPER", helper)
	t.Setenv(desktopRuntimeDirEnv, filepath.Join(tempDir, "runtime"))

	manager := NewTaskManager()
	if _, err := manager.StartLogin("TV"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		session := manager.LoginSession()
		if !session.Running {
			if session.Successful || session.Status != "二维码已失效" {
				t.Fatalf("expired login session=%+v", session)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expired login session did not finish")
}

func waitForTaskStatus(t *testing.T, manager *TaskManager, id int, expected taskStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if task := manager.Task(id); task.Status == string(expected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	task := manager.Task(id)
	t.Fatalf("task #%d status=%s, expected=%s, log=%s", id, task.Status, expected, mustTaskLog(t, manager, id))
}

func mustTaskLog(t *testing.T, manager *TaskManager, id int) string {
	t.Helper()
	text, err := manager.TaskLog(id)
	if err != nil {
		t.Fatal(err)
	}
	return text
}
