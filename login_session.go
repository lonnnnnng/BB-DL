package main

import (
	"bufio"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type LoginSessionDTO struct {
	ID         int    `json:"id"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	Log        string `json:"log"`
	Running    bool   `json:"running"`
	Successful bool   `json:"successful"`
}

type loginSession struct {
	LoginSessionDTO
	RuntimeDir    string
	StopRequested bool
	command       commandHandle
}

// long 2026-07-26 20:35:19：登录是账号级临时会话，不属于下载任务；独立子进程避免污染任务统计、历史记录和自动队列顺序。
func (manager *TaskManager) StartLogin(kind string) (LoginSessionDTO, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	commandName := "login"
	if kind == "TV" {
		commandName = "logintv"
	} else if kind != "WEB" {
		return LoginSessionDTO{}, errors.New("登录类型只能是 WEB 或 TV")
	}

	manager.mu.Lock()
	if manager.shuttingDown {
		manager.mu.Unlock()
		return LoginSessionDTO{}, errors.New("应用正在退出")
	}
	if manager.login != nil && manager.login.Running {
		manager.mu.Unlock()
		return LoginSessionDTO{}, errors.New("已有登录会话正在进行")
	}
	runtimeDir, helper, err := prepareRuntime()
	if err != nil {
		manager.mu.Unlock()
		return LoginSessionDTO{}, err
	}
	_ = os.Remove(filepath.Join(runtimeDir, "qrcode.png"))
	command := exec.Command(helper, commandName)
	command.Dir = runtimeDir
	command.Env = append(desktopCommandEnv(), serverChildEnv, forcePlainTextEnv)
	stdout, err := command.StdoutPipe()
	if err != nil {
		manager.mu.Unlock()
		return LoginSessionDTO{}, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		manager.mu.Unlock()
		return LoginSessionDTO{}, err
	}
	if err := command.Start(); err != nil {
		manager.mu.Unlock()
		return LoginSessionDTO{}, err
	}
	session := &loginSession{
		LoginSessionDTO: LoginSessionDTO{ID: manager.nextLoginID, Kind: kind, Status: "正在生成二维码", Message: "正在请求登录二维码", Running: true},
		RuntimeDir:      runtimeDir,
		command:         processHandle{process: command.Process},
	}
	manager.nextLoginID++
	manager.login = session
	manager.backgroundWG.Add(1)
	dto := session.LoginSessionDTO
	manager.mu.Unlock()
	manager.emit("login:updated", dto)
	var outputWG sync.WaitGroup
	outputWG.Add(2)
	go func() { defer outputWG.Done(); manager.consumeLoginOutput(session.ID, stdout) }()
	go func() { defer outputWG.Done(); manager.consumeLoginOutput(session.ID, stderr) }()
	go func() {
		defer manager.backgroundWG.Done()
		// long 2026-07-26 22:54:26：登录成功标记位于输出末尾，先排空管道再 Wait，避免进程退出时把已确认的扫码结果误判为登录结束。
		outputWG.Wait()
		manager.finishLogin(session.ID, command.Wait())
	}()
	return dto, nil
}

func (manager *TaskManager) consumeLoginOutput(id int, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		manager.handleLoginLine(id, scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
		manager.handleLoginLine(id, "读取登录输出失败："+err.Error())
	}
}

func (manager *TaskManager) handleLoginLine(id int, line string) {
	if looksLikeConsoleQRCode(line) {
		return
	}
	manager.mu.Lock()
	session := manager.login
	if session == nil || session.ID != id {
		manager.mu.Unlock()
		return
	}
	session.Log += line + "\n"
	session.Message = loginMessageForLine(line, session.Message)
	dto := session.LoginSessionDTO
	runtimeDir := session.RuntimeDir
	manager.mu.Unlock()
	manager.emit("login:updated", dto)
	if strings.Contains(line, "生成二维码成功") {
		if dataURL, err := loginQRCodeDataURL(runtimeDir); err == nil {
			manager.emit("login:qrcode", map[string]any{"id": id, "dataURL": dataURL})
		}
	}
}

func (manager *TaskManager) finishLogin(id int, runErr error) {
	manager.mu.Lock()
	session := manager.login
	if session == nil || session.ID != id {
		manager.mu.Unlock()
		return
	}
	session.Running = false
	session.command = nil
	switch {
	case session.StopRequested:
		session.Status = "已取消"
		session.Message = "登录已取消"
	case runErr != nil:
		session.Status = "登录失败"
		session.Message = runErr.Error()
		session.Log += "登录失败：" + runErr.Error() + "\n"
	case strings.Contains(session.Log, "登录成功"):
		session.Status = "登录成功"
		session.Message = "登录凭据已保存到本机"
		session.Successful = true
	case strings.Contains(session.Log, "二维码已过期"):
		session.Status = "二维码已失效"
		session.Message = "二维码已失效，请重新生成"
	default:
		session.Status = "登录已结束"
		session.Message = "登录进程已结束，请查看状态后重试"
	}
	dto := session.LoginSessionDTO
	runtimeDir := session.RuntimeDir
	manager.mu.Unlock()
	_ = os.Remove(filepath.Join(runtimeDir, "qrcode.png"))
	manager.emit("login:updated", dto)
	manager.emit("login:finished", dto)
}

func (manager *TaskManager) StopLogin() error {
	manager.mu.Lock()
	if manager.login == nil || !manager.login.Running {
		manager.mu.Unlock()
		return errors.New("当前没有进行中的登录")
	}
	manager.login.StopRequested = true
	manager.login.Status = "正在取消"
	manager.login.Message = "正在停止登录会话"
	handle := manager.login.command
	dto := manager.login.LoginSessionDTO
	manager.mu.Unlock()
	manager.emit("login:updated", dto)
	if handle != nil {
		if err := handle.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
	}
	return nil
}

func (manager *TaskManager) LoginSession() LoginSessionDTO {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.login == nil {
		return LoginSessionDTO{}
	}
	return manager.login.LoginSessionDTO
}

func loginMessageForLine(line, current string) string {
	switch {
	case strings.Contains(line, "获取登录地址"):
		return "正在请求登录二维码"
	case strings.Contains(line, "生成二维码成功"), strings.Contains(line, "等待扫码"):
		return "请使用哔哩哔哩客户端扫码"
	case strings.Contains(line, "扫码成功"), strings.Contains(line, "等待手机端确认"):
		return "已扫码，请在客户端确认"
	case strings.Contains(line, "登录成功"):
		return "登录凭据已保存到本机"
	case strings.Contains(line, "二维码已过期"):
		return "二维码已失效，请重新生成"
	default:
		return current
	}
}

func looksLikeConsoleQRCode(line string) bool {
	return strings.ContainsAny(line, "█▀▄")
}

func loginQRCodeDataURL(runtimeDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(runtimeDir, "qrcode.png"))
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), nil
}

func downloadTasksOnly(tasks []*desktopTask) []*desktopTask {
	filtered := make([]*desktopTask, 0, len(tasks))
	for _, task := range tasks {
		if task != nil && task.Channel != "账号" {
			filtered = append(filtered, task)
		}
	}
	return filtered
}
