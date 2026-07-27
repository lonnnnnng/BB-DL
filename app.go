package main

import (
	"context"
	"strings"

	"github.com/lonnnnnng/BB-DL/internal/bbdown"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	manager *TaskManager
}

func NewApp() *App {
	return &App{manager: NewTaskManager()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.manager.Startup(ctx)
}

func (a *App) shutdown(context.Context) {
	a.manager.Shutdown()
}

func (a *App) beforeClose(ctx context.Context) bool {
	if !a.manager.HasRunningTask() {
		return false
	}
	result, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "任务仍在运行",
		Message:       "退出会停止当前下载任务，确定退出吗？",
		Buttons:       []string{"继续下载", "退出并停止"},
		DefaultButton: "继续下载",
		CancelButton:  "继续下载",
	})
	if err != nil || result != "退出并停止" {
		return true
	}
	a.manager.StopRunningTask()
	return false
}

func (a *App) GetBootstrap() Bootstrap {
	return a.manager.Bootstrap()
}

func (a *App) SavePreferences(preferences Preferences) error {
	return a.manager.SavePreferences(preferences)
}

func (a *App) ChooseWorkDir() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "选择视频保存目录",
		DefaultDirectory:     a.manager.Preferences().WorkDir,
		CanCreateDirectories: true,
	})
}

func (a *App) OpenWorkDir(path string) error {
	path, err := ensureWorkDir(path)
	if err != nil {
		return err
	}
	return openPath(path)
}

func (a *App) CreateTask(input TaskInput, startNow bool) (TaskDTO, error) {
	return a.manager.CreateTask(input, startNow)
}

func (a *App) StartLogin(kind string) (LoginSessionDTO, error) {
	return a.manager.StartLogin(kind)
}

func (a *App) StopLogin() error {
	return a.manager.StopLogin()
}

func (a *App) GetLoginSession() LoginSessionDTO {
	return a.manager.LoginSession()
}

func (a *App) StartTask(id int) error {
	return a.manager.StartTask(id)
}

func (a *App) StartNextPending() error {
	return a.manager.StartNextPending()
}

func (a *App) StopTask(id int) error {
	return a.manager.StopTask(id)
}

func (a *App) RetryTask(id int) (TaskDTO, error) {
	return a.manager.RetryTask(id)
}

func (a *App) RetryFailedTasks() ([]TaskDTO, error) {
	return a.manager.RetryFailedTasks()
}

func (a *App) DeleteTask(id int) error {
	return a.manager.DeleteTask(id)
}

func (a *App) ClearEndedTasks() (int, error) {
	return a.manager.ClearEndedTasks()
}

func (a *App) FillFormData(id int) (TaskInput, error) {
	return a.manager.FillFormData(id)
}

func (a *App) GetTaskLog(id int) (string, error) {
	return a.manager.TaskLog(id)
}

func (a *App) GetTaskCommand(id int) (string, error) {
	return a.manager.TaskCommand(id)
}

func (a *App) CheckTools(paths ToolPaths) []ToolDiagnostic {
	return checkTools(paths)
}

func (a *App) Doctor(paths ToolPaths) (string, error) {
	return desktopDoctorJSON(a.ctx, paths)
}

func (a *App) OpenFile(path string) error {
	return openPath(path)
}

func (a *App) RevealFile(path string) error {
	return revealPath(path)
}

func (a *App) VersionInfo() VersionInfo {
	return VersionInfo{Version: appVersion(), BuildTime: appBuildTime()}
}

func appVersion() string {
	return bbdown.Version
}

func appBuildTime() string {
	if strings.TrimSpace(bbdown.BuildTime) == "" {
		return "development"
	}
	return bbdown.BuildTime
}
