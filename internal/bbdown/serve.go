package bbdown

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	servePageStartPattern           = regexp.MustCompile(`^开始解析P(\d+): .* \((\d+) of (\d+)\)$`)
	servePageVideoStartPattern      = regexp.MustCompile(`^开始下载P(\d+)视频\.\.\.$`)
	servePageAudioStartPattern      = regexp.MustCompile(`^开始下载P(\d+)音频\.\.\.$`)
	servePageExtraAudioStartPattern = regexp.MustCompile(`^开始下载P(\d+)(背景配音|配音\[.*\])\.\.\.$`)
	serveClipStartPattern           = regexp.MustCompile(`^开始下载P(\d+)视频, 片段\((\d+)/(\d+)\)\.\.\.$`)
	servePageDonePattern            = regexp.MustCompile(`^下载P(\d+)完毕$`)
	serveMuxStartPattern            = regexp.MustCompile(`^开始(合并音视频.*|合并分段|混流视频.*)\.\.\.$`)
	taskSecretParamPattern          = regexp.MustCompile(`(?i)\b(access_token|access_key|token|SESSDATA|bili_jct|DedeUserID|DedeUserID__ckMd5|sid)=([^&;\s"']+)`)
	serverTransferEventMu           sync.Mutex
)

const serverTransferEventPrefix = "__BBDOWN_GO_SERVER_TRANSFER__ "

// long 2026-06-18 21:37:12：服务模式只能从子进程日志估算页内状态；这些阶段让普通 DASH 任务在下载、合并期间都有可见进度，同时保留“任务完成”才到 100%。
const (
	serveProgressStageVideoDownloaded = 0.12
	serveProgressStageAudioDownloaded = 0.35
	serveProgressStageExtraAudio      = 0.55
	serveProgressStagePageDownloaded  = 0.75
	serveProgressStageMuxStarted      = 0.90
)

type ServeRequestOptions struct {
	MyOption
	CallBackWebHook string `json:"CallBackWebHook"`
}

// long: server 任务复用 CLI 下载链路时标记子进程，避免每个任务重复输出启动横幅和重复触发版本检查。
const ServerChildEnv = "BBDOWN_GO_SERVER_CHILD"

func IsServerChildProcess() bool {
	return os.Getenv(ServerChildEnv) == "1"
}

func (o *ServeRequestOptions) UnmarshalJSON(data []byte) error {
	type optionAlias MyOption
	type serveRequestOptionsJSON struct {
		optionAlias
		CallBackWebHook string `json:"CallBackWebHook"`
	}
	payload := serveRequestOptionsJSON{
		optionAlias: optionAlias(DefaultMyOption()),
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	o.MyOption = MyOption(payload.optionAlias)
	o.CallBackWebHook = payload.CallBackWebHook
	return nil
}

type DownloadTask struct {
	Aid                  string   `json:"Aid"`
	Url                  string   `json:"Url"`
	TaskCreateTime       int64    `json:"TaskCreateTime"`
	Title                *string  `json:"Title"`
	Pic                  *string  `json:"Pic"`
	VideoPubTime         *int64   `json:"VideoPubTime"`
	TaskFinishTime       *int64   `json:"TaskFinishTime"`
	Progress             float64  `json:"Progress"`
	DownloadSpeed        float64  `json:"DownloadSpeed"`
	TotalDownloadedBytes float64  `json:"TotalDownloadedBytes"`
	IsSuccessful         bool     `json:"IsSuccessful"`
	SavePaths            []string `json:"SavePaths"`
	ErrorStage           *string  `json:"ErrorStage"`
	ErrorMessage         *string  `json:"ErrorMessage"`
}

const (
	taskErrorStageResolve  = "resolve"
	taskErrorStageInfo     = "info"
	taskErrorStageDownload = "download"
)

func (t DownloadTask) MarshalJSON() ([]byte, error) {
	type downloadTaskAlias DownloadTask
	payload := downloadTaskAlias(t)
	if payload.SavePaths == nil {
		payload.SavePaths = []string{}
	}
	return json.Marshal(payload)
}

type DownloadTaskCollection struct {
	Running  []DownloadTask `json:"Running"`
	Finished []DownloadTask `json:"Finished"`
}

type taskProgressState struct {
	totalPages  int
	currentPage int
	progress    float64
}

type taskTransferState struct {
	lastBytes              float64
	lastAt                 time.Time
	totalTransferredBytes  float64
	sawDownloadFilePattern bool
	baselineBytes          map[string]int64
	workDir                string
	sawTransferEvent       bool
	eventTransferredBytes  float64
	eventPathBytes         map[string]int64
}

type taskTransferSnapshot struct {
	bytes                   float64
	hasDownloadFilePattern  bool
	hasPredictedOutputFiles bool
}

type serverTransferEvent struct {
	Path  string `json:"Path"`
	Bytes int64  `json:"Bytes"`
	Delta bool   `json:"Delta,omitempty"`
}

func newTaskTransferState(workDir string, paths []string) *taskTransferState {
	state := &taskTransferState{}
	state.workDir, _ = ResolveWorkDir(workDir)
	state.baselineBytes = taskTransferBaselineBytes(workDir, paths)
	return state
}

func EmitServerTransferEvent(path string) {
	if !IsServerChildProcess() {
		return
	}
	event, ok := serverTransferEventFromPath(path)
	if !ok {
		return
	}
	emitServerTransferEvent(event)
}

func serverTransferWriter(writer io.Writer, path string) (io.Writer, func()) {
	return transferWriter(writer, path, nil)
}

func transferWriter(writer io.Writer, path string, progress *downloadProgress) (io.Writer, func()) {
	reporter := newServerTransferReporter(path)
	if reporter == nil && progress == nil {
		return writer, func() {}
	}
	return &serverTransferCountingWriter{writer: writer, reporter: reporter, progress: progress}, func() {
		if reporter != nil {
			reporter.Flush()
		}
		if progress != nil {
			progress.Flush()
		}
	}
}

type serverTransferCountingWriter struct {
	writer   io.Writer
	reporter *serverTransferReporter
	progress *downloadProgress
}

func (w *serverTransferCountingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		if w.reporter != nil {
			w.reporter.Add(int64(n))
		}
		if w.progress != nil {
			w.progress.Add(int64(n))
		}
	}
	return n, err
}

type serverTransferReporter struct {
	path       string
	interval   time.Duration
	lastEmitAt time.Time
	pending    int64
	mu         sync.Mutex
}

func newServerTransferReporter(path string) *serverTransferReporter {
	if !IsServerChildProcess() || strings.TrimSpace(path) == "" {
		return nil
	}
	return &serverTransferReporter{
		path:       path,
		interval:   time.Second,
		lastEmitAt: time.Now(),
	}
}

func (r *serverTransferReporter) Add(bytes int64) {
	if r == nil || bytes <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending += bytes
	now := time.Now()
	if now.Sub(r.lastEmitAt) >= r.interval {
		r.emitLocked(now)
	}
}

func (r *serverTransferReporter) Flush() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emitLocked(time.Now())
}

func (r *serverTransferReporter) emitLocked(now time.Time) {
	if r.pending <= 0 {
		return
	}
	// long 2026-06-20 14:55:26：服务模式子进程按写入增量上报下载字节，让父进程在长任务运行中就能刷新速度；完成事件仍保留，用来补齐被节流或异常退出前漏掉的最终大小。
	emitServerTransferEvent(serverTransferEvent{Path: r.path, Bytes: r.pending, Delta: true})
	r.pending = 0
	r.lastEmitAt = now
}

func emitServerTransferEvent(event serverTransferEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	serverTransferEventMu.Lock()
	defer serverTransferEventMu.Unlock()
	fmt.Println(serverTransferEventPrefix + string(payload))
}

func serverTransferEventFromPath(path string) (serverTransferEvent, bool) {
	if strings.TrimSpace(path) == "" {
		return serverTransferEvent{}, false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() <= 0 {
		return serverTransferEvent{}, false
	}
	return serverTransferEvent{Path: path, Bytes: info.Size()}, true
}

func parseServerTransferEvent(line string) (serverTransferEvent, bool) {
	payload := strings.TrimPrefix(line, serverTransferEventPrefix)
	if payload == line {
		return serverTransferEvent{}, false
	}
	var event serverTransferEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return serverTransferEvent{}, false
	}
	if strings.TrimSpace(event.Path) == "" || event.Bytes <= 0 {
		return serverTransferEvent{}, false
	}
	return event, true
}

func applyServerTransferEvent(task *DownloadTask, state *taskTransferState, event serverTransferEvent) bool {
	return applyServerTransferEventAt(task, state, event, time.Now())
}

func applyServerTransferEventAt(task *DownloadTask, state *taskTransferState, event serverTransferEvent, observedAt time.Time) bool {
	if task == nil || state == nil || strings.TrimSpace(event.Path) == "" || event.Bytes <= 0 {
		return false
	}
	// long 2026-06-20 14:55:26：增量事件贴近原版 ProgressBar 的 RelatedTask 更新；完成事件只补该路径尚未上报的差额，避免下载过程增量和最终文件大小重复计入。
	state.sawTransferEvent = true
	if state.eventPathBytes == nil {
		state.eventPathBytes = make(map[string]int64)
	}
	eventPath := eventTransferPathKey(state, event.Path)
	delta := event.Bytes
	if event.Delta {
		state.eventPathBytes[eventPath] += event.Bytes
	} else {
		totalGrowth := event.Bytes - eventTransferBaselineBytes(state, eventPath)
		if totalGrowth <= 0 {
			return false
		}
		seen := state.eventPathBytes[eventPath]
		if totalGrowth <= seen {
			return false
		}
		delta = totalGrowth - seen
		state.eventPathBytes[eventPath] = totalGrowth
	}
	state.eventTransferredBytes += float64(delta)
	if state.eventTransferredBytes > state.totalTransferredBytes {
		state.totalTransferredBytes = state.eventTransferredBytes
	}
	changed := false
	if state.totalTransferredBytes > task.TotalDownloadedBytes {
		task.TotalDownloadedBytes = state.totalTransferredBytes
		changed = true
	}
	if !observedAt.IsZero() {
		if !state.lastAt.IsZero() {
			elapsed := observedAt.Sub(state.lastAt).Seconds()
			if elapsed > 0 {
				speed := float64(delta) / elapsed
				if speed != task.DownloadSpeed {
					task.DownloadSpeed = speed
					changed = true
				}
			}
		}
		state.lastAt = observedAt
		state.lastBytes = state.totalTransferredBytes
	}
	return changed
}

func eventTransferPathKey(state *taskTransferState, path string) string {
	if state != nil && state.workDir != "" && !filepath.IsAbs(path) {
		return filepath.Join(state.workDir, path)
	}
	return path
}

func eventTransferBaselineBytes(state *taskTransferState, path string) int64 {
	if state == nil || state.baselineBytes == nil {
		return 0
	}
	return state.baselineBytes[path]
}

func (c DownloadTaskCollection) MarshalJSON() ([]byte, error) {
	type downloadTaskCollectionAlias DownloadTaskCollection
	payload := downloadTaskCollectionAlias(c)
	if payload.Running == nil {
		payload.Running = []DownloadTask{}
	}
	if payload.Finished == nil {
		payload.Finished = []DownloadTask{}
	}
	return json.Marshal(payload)
}

type ApiServer struct {
	cfg     *Config
	httpc   *HTTPClient
	mu      sync.Mutex
	running []DownloadTask
	done    []DownloadTask
	srv     *http.Server
}

func NewApiServer(cfg *Config, httpc *HTTPClient) *ApiServer {
	return &ApiServer{cfg: cfg, httpc: httpc}
}

func NormalizeListenAddr(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "http://0.0.0.0:23333"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return "", fmt.Errorf("%s不是合法的http URL，url示例：http://0.0.0.0:5000\n如果您需要https，请额外配置反向代理", raw)
	}
	return parsed.Host, nil
}

func (s *ApiServer) Run(addr string) error {
	s.srv = &http.Server{Addr: addr, Handler: s.routes()}
	return s.srv.ListenAndServe()
}

func (s *ApiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /get-tasks/{$}", s.handleTasks)
	mux.HandleFunc("GET /get-tasks/running", s.handleRunning)
	mux.HandleFunc("GET /get-tasks/finished", s.handleFinished)
	mux.HandleFunc("GET /get-tasks/{id}", s.handleTaskByID)
	mux.HandleFunc("POST /add-task", s.handleAddTask)
	mux.HandleFunc("GET /remove-finished", s.handleRemoveFinished)
	mux.HandleFunc("GET /remove-finished/{$}", s.handleRemoveFinished)
	mux.HandleFunc("GET /remove-finished/failed", s.handleRemoveFinished)
	mux.HandleFunc("GET /remove-finished/{id}", s.handleRemoveFinished)
	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func cloneTasks(tasks []DownloadTask) []DownloadTask {
	if len(tasks) == 0 {
		return []DownloadTask{}
	}
	return append([]DownloadTask(nil), tasks...)
}

func (s *ApiServer) handleTasks(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, DownloadTaskCollection{Running: cloneTasks(s.running), Finished: cloneTasks(s.done)})
}

func (s *ApiServer) handleRunning(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, cloneTasks(s.running))
}

func (s *ApiServer) handleFinished(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, cloneTasks(s.done))
}

func (s *ApiServer) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range append(s.running, s.done...) {
		if t.Aid == id {
			writeJSON(w, t)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *ApiServer) handleAddTask(w http.ResponseWriter, r *http.Request) {
	var req ServeRequestOptions
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Url == "" {
		http.Error(w, "输入有误", http.StatusBadRequest)
		return
	}
	if err := validateServeRequestOptions(req); err != nil {
		http.Error(w, "输入有误: "+err.Error(), http.StatusBadRequest)
		return
	}
	go s.addTask(req)
	w.WriteHeader(http.StatusOK)
}

func validateServeRequestOptions(req ServeRequestOptions) error {
	opt := req.MyOption
	normalizeOptionsForCompatibility(&opt, false)
	NormalizeTrackIndexOptions(&opt)
	if err := ValidateFileExistsAction(opt.FileExistsAction); err != nil {
		return err
	}
	return ValidateTrackIndexSyntax(&opt)
}

func (s *ApiServer) handleRemoveFinished(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := r.PathValue("id")
	if strings.HasSuffix(r.URL.Path, "/failed") {
		var keep []DownloadTask
		for _, t := range s.done {
			if t.IsSuccessful {
				keep = append(keep, t)
			}
		}
		s.done = keep
	} else if id == "" || id == "/" {
		s.done = nil
	} else {
		var keep []DownloadTask
		for _, t := range s.done {
			if t.Aid != id {
				keep = append(keep, t)
			}
		}
		s.done = keep
	}
	w.WriteHeader(http.StatusOK)
}

func (s *ApiServer) addTask(req ServeRequestOptions) {
	// long 2026-06-18 23:16:28：服务父进程会先预测 SavePaths 和选轨，废弃参数必须先按 CLI 语义归一化，避免父进程结果和子进程真实下载路径不一致。
	normalizeOptionsForCompatibility(&req.MyOption, false)
	ctx := context.Background()
	taskCreateTime := time.Now().Unix()
	aid, err := ResolveAvid(ctx, req.Url, s.httpc, s.cfg)
	if err != nil {
		task := DownloadTask{
			Aid:            fallbackTaskAid(req.Url),
			Url:            req.Url,
			TaskCreateTime: taskCreateTime,
			TaskFinishTime: ptrTo(time.Now().Unix()),
		}
		setTaskError(&task, taskErrorStageResolve, err, req, nil)
		s.mu.Lock()
		s.done = append(s.done, task)
		s.mu.Unlock()
		if req.CallBackWebHook != "" {
			s.postCallback(req.CallBackWebHook, task)
		}
		return
	}
	s.mu.Lock()
	for _, running := range s.running {
		if running.Aid == aid {
			s.mu.Unlock()
			if req.CallBackWebHook != "" {
				s.postCallback(req.CallBackWebHook, running)
			}
			return
		}
	}
	s.mu.Unlock()

	task := DownloadTask{Aid: aid, Url: req.Url, TaskCreateTime: taskCreateTime}
	s.mu.Lock()
	s.running = append(s.running, task)
	s.mu.Unlock()

	aidOri, vInfo, apiType, err := GetVideoInfo(ctx, s.cfg, s.httpc, &req.MyOption, aid, req.Url)
	if err != nil {
		setTaskError(&task, taskErrorStageInfo, err, req, nil)
	} else if vInfo == nil {
		setTaskError(&task, taskErrorStageInfo, fmt.Errorf("未获取到视频信息"), req, nil)
	} else if len(vInfo.PagesInfo) == 0 {
		setTaskError(&task, taskErrorStageInfo, fmt.Errorf("未获取到分P信息"), req, nil)
	} else {
		task.Title = ptrTo(vInfo.Title)
		task.Pic = ptrTo(vInfo.Pic)
		task.VideoPubTime = ptrTo(vInfo.PubTime)
		task.SavePaths = s.predictSavePaths(ctx, &req.MyOption, aidOri, vInfo, apiType)
		s.updateRunning(task)
		progressState := &taskProgressState{}
		transferState := newTaskTransferState(req.WorkDir, task.SavePaths)
		lineTail := newTaskLogTail(20)
		var taskMu sync.Mutex
		stopTransfer := make(chan struct{})
		transferDone := make(chan struct{})
		go s.watchTaskTransfer(&task, &taskMu, transferState, req.WorkDir, stopTransfer, transferDone)
		err = s.runDownloadProcess(req, func(line string) {
			taskMu.Lock()
			if event, ok := parseServerTransferEvent(line); ok {
				changed := applyServerTransferEventAt(&task, transferState, event, time.Now())
				snapshot := task
				taskMu.Unlock()
				if changed {
					s.updateRunning(snapshot)
				}
				return
			}
			lineTail.add(line)
			changed := s.applyProgressLine(&task, progressState, line)
			snapshot := task
			taskMu.Unlock()
			if changed {
				s.updateRunning(snapshot)
			}
		})
		close(stopTransfer)
		<-transferDone
		taskMu.Lock()
		_ = applyTransferSnapshot(&task, transferState, taskTransferBytes(req.WorkDir, task.SavePaths, transferState), time.Now())
		taskMu.Unlock()
		task.IsSuccessful = err == nil
		if task.IsSuccessful {
			task.Progress = 1
			finalizeFinalOutputTransferBytes(&task, req.MyOption, req.WorkDir, transferState)
		} else {
			setTaskError(&task, taskErrorStageDownload, err, req, lineTail.snapshot())
		}
	}
	task.TaskFinishTime = ptrTo(time.Now().Unix())
	if task.IsSuccessful && task.TaskFinishTime != nil && *task.TaskFinishTime > task.TaskCreateTime {
		task.DownloadSpeed = task.TotalDownloadedBytes / float64(*task.TaskFinishTime-task.TaskCreateTime)
	}
	s.mu.Lock()
	s.running = removeTask(s.running, aid)
	s.done = append(s.done, task)
	s.mu.Unlock()
	if req.CallBackWebHook != "" {
		s.postCallback(req.CallBackWebHook, task)
	}
}

func (s *ApiServer) watchTaskTransfer(task *DownloadTask, taskMu *sync.Mutex, state *taskTransferState, workDir string, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	update := func(now time.Time) {
		taskMu.Lock()
		// long 2026-06-20 14:55:26：server 任务优先消费子进程下载增量事件；文件轮询只兜底事件缺失或外部下载器场景，避免把混流最终文件的本地写入误算成下载流量。
		changed := applyTransferSnapshot(task, state, taskTransferBytes(workDir, task.SavePaths, state), now)
		snapshot := *task
		taskMu.Unlock()
		if changed {
			s.updateRunning(snapshot)
		}
	}

	update(time.Now())
	for {
		select {
		case now := <-ticker.C:
			update(now)
		case <-stop:
			update(time.Now())
			return
		}
	}
}

func (s *ApiServer) updateRunning(task DownloadTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.running {
		if s.running[i].Aid == task.Aid {
			s.running[i] = task
			return
		}
	}
}

func (s *ApiServer) predictSavePaths(ctx context.Context, opt *MyOption, aidOri string, vInfo *VInfo, apiType string) []string {
	if opt != nil && opt.OnlyShowInfo {
		// long 2026-06-18 22:10:42：原版服务模式在 OnlyShowInfo 打印轨道后立即返回，不会把未下载的媒体路径写进任务结果。
		return []string{}
	}
	pages := vInfo.PagesInfo
	totalPagesCount := len(pages)
	if selected := GetSelectedPages(opt, vInfo, opt.Url); selected != nil {
		filtered := make([]Page, 0, len(selected))
		for _, p := range pages {
			for _, s := range selected {
				if fmt.Sprintf("%d", p.Index) == s {
					filtered = append(filtered, p)
				}
			}
		}
		pages = filtered
	}
	encodingPriority := ParseEncodingPriority(opt.EncodingPriority)
	firstEncoding := FirstEncodingPriority(opt.EncodingPriority)
	dfnPriority := ParseDfnPriority(opt.DfnPriority)
	paths := make([]string, 0, len(pages))
	reservedOutputGroups := make(map[string]struct{}, len(pages))
	for _, p := range pages {
		downloadTitle := DownloadTitle(vInfo.Title)
		if opt.SaveArchivesToFile && CheckAidFromArchive(p.Aid) {
			// long 2026-06-18 22:23:56：归档命中的分 P 在原版服务里会在下载前跳过，任务结果不能暴露未生成的预测产物。
			continue
		}
		if opt.SubOnly && !opt.SkipSubtitle && !opt.DanmakuOnly && !opt.CoverOnly {
			path := FormatSavePath(serverSavePathPattern(opt, vInfo, totalPagesCount), downloadTitle, nil, nil, p, len(pages), apiType, vInfo.PubTime)
			// long 2026-07-27 00:55:09：同一服务任务的多个分 P 可能使用相同模板；预测阶段先预留已分配基名，才能与子进程依次产生的 (1)、(2) 路径一致。
			path, err := resolveFileExistsPathInDir(path, opt.FileExistsAction, opt.WorkDir, reservedOutputGroups)
			if err != nil {
				continue
			}
			subs, err := GetSubtitles(ctx, s.httpc, s.cfg, p.Aid, p.Cid, p.Epid, p.Index, opt.UseIntlApi)
			if err != nil {
				continue
			}
			paths = append(paths, serverPredictedSubtitleOutputPaths(path, subs, opt.SkipAi)...)
			continue
		}
		tracks, err := ExtractTracks(ctx, s.cfg, s.httpc, aidOri, p.Aid, p.Cid, p.Epid, opt.UseTvApi, opt.UseIntlApi, opt.UseAppApi, firstEncoding, "")
		if err != nil {
			continue
		}
		applyServerOnlyModeToTracks(opt, tracks)
		tracks.VideoTracks = SortTracksVideoWithPriority(tracks.VideoTracks, dfnPriority, encodingPriority, opt.VideoAscending)
		tracks.AudioTracks = SortTracksAudioWithPriority(tracks.AudioTracks, encodingPriority, opt.AudioAscending)
		videoIndex, audioIndex, err := SelectedTrackIndexes(opt, tracks)
		if err != nil {
			continue
		}
		var video *Video
		if len(tracks.VideoTracks) > 0 {
			video = &tracks.VideoTracks[videoIndex]
		}
		var audio *Audio
		if len(tracks.AudioTracks) > 0 {
			audio = &tracks.AudioTracks[audioIndex]
		}
		path := FormatSavePath(serverSavePathPattern(opt, vInfo, totalPagesCount), downloadTitle, video, audio, p, len(pages), apiType, vInfo.PubTime)
		path, err = resolveFileExistsPathInDir(path, opt.FileExistsAction, opt.WorkDir, reservedOutputGroups)
		if err != nil {
			continue
		}
		paths = append(paths, serverPredictedOutputPaths(opt, vInfo, p, tracks, path)...)
	}
	return paths
}

func serverPredictedSubtitleOutputPaths(path string, subs []Subtitle, skipAI bool) []string {
	paths := make([]string, 0, len(subs))
	for _, sub := range subs {
		if skipAI && strings.HasPrefix(sub.Lan, "ai-") {
			continue
		}
		// long: CLI 的字幕 only 不会生成 mp4，而是把每条字幕保存到以视频基名派生的语言后缀文件；服务任务必须暴露这个真实产物。
		paths = append(paths, SubtitleOutputPath(path, sub))
	}
	return paths
}

func serverPredictedOutputPaths(opt *MyOption, vInfo *VInfo, page Page, tracks *ParsedTracks, path string) []string {
	if opt == nil {
		return []string{path}
	}
	if opt.OnlyShowInfo {
		return []string{}
	}
	base := strings.TrimSuffix(path, ".mp4")
	if opt.DanmakuOnly {
		formats := ParseDanmakuFormats(opt.DownloadDanmakuFormats)
		paths := make([]string, 0, 2)
		if formats[DanmakuXML] {
			paths = append(paths, base+".xml")
		}
		if formats[DanmakuASS] {
			paths = append(paths, base+".ass")
		}
		return paths
	}
	if opt.CoverOnly {
		coverURL := ""
		if vInfo != nil {
			coverURL = vInfo.Pic
		}
		if coverURL == "" {
			coverURL = page.Cover
		}
		return []string{base + CoverExt(coverURL)}
	}
	if opt.SkipMux && tracks != nil && len(tracks.Clips) > 0 {
		// long: FLV 分段任务在 CLI 中即使跳过混流，也会先合并成可播放的保留产物；服务端对外路径必须指向这个真实文件。
		return []string{base + ".flv-merged.mp4"}
	}
	if opt.SkipMux && tracks != nil {
		// long: DASH 跳过混流时 CLI 保留的是音视频下载产物，不会生成最终 mp4；服务端任务结果要直接暴露这些可检查文件。
		paths := make([]string, 0, 2)
		if !opt.AudioOnly && len(tracks.VideoTracks) > 0 {
			paths = append(paths, base+".video.mp4")
		}
		if !opt.VideoOnly && len(tracks.AudioTracks) > 0 {
			paths = append(paths, base+".audio.m4a")
		}
		if len(paths) > 0 {
			return paths
		}
	}
	if opt.AudioOnly {
		return []string{base + ".m4a"}
	}
	return []string{path}
}

func serverSavePathPattern(opt *MyOption, vInfo *VInfo, totalPagesCount int) string {
	if totalPagesCount > 1 || (vInfo != nil && vInfo.IsBangumi && !vInfo.IsBangumiEnd) {
		if opt.MultiFilePattern != "" {
			return opt.MultiFilePattern
		}
		return "<videoTitle>/[P<pageNumberWithZero>]<pageTitle>"
	}
	return opt.FilePattern
}

func applyServerOnlyModeToTracks(opt *MyOption, tracks *ParsedTracks) {
	if opt == nil || tracks == nil || len(tracks.Clips) > 0 {
		return
	}
	if opt.AudioOnly {
		tracks.VideoTracks = nil
	}
	if opt.VideoOnly {
		tracks.AudioTracks = nil
		tracks.BackgroundAudios = nil
		tracks.RoleAudioLists = nil
	}
}

func (s *ApiServer) runDownloadProcess(req ServeRequestOptions, onLine func(string)) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := serveOptionsToArgs(req.MyOption)
	args = append(args, req.Url)
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), ServerChildEnv+"=1")
	cmd.Stdin = os.Stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	stream := func(dst io.Writer, src io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(src)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if _, ok := parseServerTransferEvent(line); ok {
				if onLine != nil {
					onLine(line)
				}
				continue
			}
			// long: serve 模式复用 CLI 子进程下载，父进程必须一边转发日志给终端，一边从日志里提取任务进度。
			fmt.Fprintln(dst, line)
			if onLine != nil {
				onLine(line)
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			errCh <- scanErr
		}
	}

	wg.Add(2)
	go stream(os.Stdout, stdout)
	go stream(os.Stderr, stderr)

	waitErr := cmd.Wait()
	wg.Wait()
	close(errCh)
	if waitErr != nil {
		return waitErr
	}
	for scanErr := range errCh {
		if scanErr != nil {
			if isBenignPipeCloseError(scanErr) {
				continue
			}
			return scanErr
		}
	}
	return nil
}

func setTaskError(task *DownloadTask, stage string, err error, req ServeRequestOptions, recentLines []string) {
	if task == nil {
		return
	}
	if strings.TrimSpace(stage) != "" {
		task.ErrorStage = ptrTo(stage)
	}
	// long: 服务调用方需要稳定的阶段字段做任务看板筛选，中文错误摘要仍只承担排障提示，并继续统一走脱敏逻辑。
	task.ErrorMessage = ptrTo(taskErrorSummary(err, req, recentLines))
}

func isBenignPipeCloseError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, os.ErrClosed) || strings.Contains(err.Error(), "file already closed")
}

func (s *ApiServer) applyProgressLine(task *DownloadTask, state *taskProgressState, line string) bool {
	if task == nil || state == nil {
		return false
	}
	line = cleanProgressLogLine(line)
	// long: 子进程不会直接回写 DownloadTask，按页和 FLV 分段日志单调推进 Progress，避免服务端任务长时间停在 0。
	if match := servePageStartPattern.FindStringSubmatch(line); match != nil {
		currentPage := parseIntOrZero(match[1])
		totalPages := parseIntOrZero(match[3])
		if totalPages > 0 {
			state.totalPages = totalPages
		}
		if currentPage > 0 {
			state.currentPage = currentPage
		}
		return s.setTaskProgress(task, state, progressForPageStart(currentPage, state.totalPages))
	}
	if match := servePageVideoStartPattern.FindStringSubmatch(line); match != nil {
		return s.setTaskProgress(task, state, progressForPageMediaStage(parseIntOrZero(match[1]), state, serveProgressStageVideoDownloaded))
	}
	if match := servePageAudioStartPattern.FindStringSubmatch(line); match != nil {
		return s.setTaskProgress(task, state, progressForPageMediaStage(parseIntOrZero(match[1]), state, serveProgressStageAudioDownloaded))
	}
	if match := servePageExtraAudioStartPattern.FindStringSubmatch(line); match != nil {
		return s.setTaskProgress(task, state, progressForPageMediaStage(parseIntOrZero(match[1]), state, serveProgressStageExtraAudio))
	}
	if match := serveClipStartPattern.FindStringSubmatch(line); match != nil {
		pageIndex := parseIntOrZero(match[1])
		clipIndex := parseIntOrZero(match[2])
		clipTotal := parseIntOrZero(match[3])
		if state.totalPages == 0 {
			return false
		}
		if pageIndex > 0 {
			state.currentPage = pageIndex
		}
		return s.setTaskProgress(task, state, progressForClip(pageIndex, clipIndex, clipTotal, state.totalPages))
	}
	if match := servePageDonePattern.FindStringSubmatch(line); match != nil {
		currentPage := parseIntOrZero(match[1])
		if state.totalPages == 0 {
			state.totalPages = currentPage
		}
		if currentPage > 0 {
			state.currentPage = currentPage
		}
		return s.setTaskProgress(task, state, progressForPageDone(currentPage, state.totalPages))
	}
	if serveMuxStartPattern.MatchString(line) {
		return s.setTaskProgress(task, state, progressForPageStage(state.currentPage, state.totalPages, serveProgressStageMuxStarted))
	}
	if strings.Contains(line, "任务完成") {
		return s.setTaskProgress(task, state, 1)
	}
	return false
}

func cleanProgressLogLine(line string) string {
	line = strings.TrimSpace(line)
	if closeIndex := strings.Index(line, "] - "); strings.HasPrefix(line, "[") && closeIndex >= 0 {
		return strings.TrimSpace(line[closeIndex+4:])
	}
	return line
}

func (s *ApiServer) setTaskProgress(task *DownloadTask, state *taskProgressState, progress float64) bool {
	if task == nil || state == nil {
		return false
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	if progress <= state.progress {
		return false
	}
	state.progress = progress
	task.Progress = progress
	return true
}

func progressForPageStart(pageIndex, totalPages int) float64 {
	if totalPages <= 0 {
		return 0
	}
	if pageIndex <= 1 {
		return 0
	}
	return float64(pageIndex-1) / float64(totalPages)
}

func progressForPageDone(pageIndex, totalPages int) float64 {
	if totalPages <= 0 {
		totalPages = pageIndex
	}
	return progressForPageStage(pageIndex, totalPages, serveProgressStagePageDownloaded)
}

func progressForClip(pageIndex, clipIndex, clipTotal, totalPages int) float64 {
	if totalPages <= 0 || clipTotal <= 0 {
		return progressForPageStart(pageIndex, totalPages)
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
	withinPage := float64(clipIndex) / float64(clipTotal+1) * serveProgressStagePageDownloaded
	return progressForPageStage(pageIndex, totalPages, withinPage)
}

func progressForPageMediaStage(pageIndex int, state *taskProgressState, stage float64) float64 {
	if state == nil || state.totalPages <= 0 {
		return 0
	}
	if pageIndex > 0 {
		state.currentPage = pageIndex
	}
	return progressForPageStage(pageIndex, state.totalPages, stage)
}

func progressForPageStage(pageIndex, totalPages int, stage float64) float64 {
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
	base := float64(pageIndex - 1)
	return (base + stage) / float64(totalPages)
}

func parseIntOrZero(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func applyTransferSnapshot(task *DownloadTask, state *taskTransferState, snapshot taskTransferSnapshot, observedAt time.Time) bool {
	if task == nil || state == nil || observedAt.IsZero() {
		return false
	}
	bytes := snapshot.bytes
	if bytes < 0 {
		bytes = 0
	}
	changed := false
	if snapshot.hasDownloadFilePattern {
		state.sawDownloadFilePattern = true
	}
	if state.sawTransferEvent {
		return false
	}
	if state.lastAt.IsZero() {
		if bytes > 0 {
			state.totalTransferredBytes += bytes
			task.TotalDownloadedBytes = state.totalTransferredBytes
			changed = true
		}
	} else {
		elapsed := observedAt.Sub(state.lastAt).Seconds()
		delta := bytes - state.lastBytes
		if delta > 0 {
			state.totalTransferredBytes += delta
			if state.totalTransferredBytes > task.TotalDownloadedBytes {
				task.TotalDownloadedBytes = state.totalTransferredBytes
				changed = true
			}
			if elapsed > 0 {
				speed := delta / elapsed
				if speed != task.DownloadSpeed {
					task.DownloadSpeed = speed
					changed = true
				}
			}
		}
	}
	state.lastBytes = bytes
	state.lastAt = observedAt
	return changed
}

func finalizeFinalOutputTransferBytes(task *DownloadTask, opt MyOption, workDir string, state *taskTransferState) bool {
	resourceOnly := opt.CoverOnly || opt.DanmakuOnly || opt.SubOnly
	singleStreamMux := !opt.SkipMux && (opt.AudioOnly || opt.VideoOnly) && !(opt.AudioOnly && opt.VideoOnly)
	// long: 资源 only 和单轨音/视频任务的最终产物基本等同于下载结果，可用来补齐短任务被轮询间隔漏掉的字节；普通双流混流仍不能这样算。
	finalOutputCanRepresentDownload := resourceOnly || singleStreamMux
	if task == nil || !finalOutputCanRepresentDownload {
		return false
	}
	bytes := totalExistingGrowthBytes(workDir, task.SavePaths, state)
	if bytes <= task.TotalDownloadedBytes {
		return false
	}
	task.TotalDownloadedBytes = bytes
	return true
}

func serveOptionsToArgs(opt MyOption) []string {
	normalizeOptionsForCompatibility(&opt, false)
	NormalizeTrackIndexOptions(&opt)
	var args []string
	addBool := func(name string, value bool) {
		if value {
			args = append(args, name)
		}
	}
	addBoolValue := func(name string, value bool) {
		args = append(args, fmt.Sprintf("%s=%t", name, value))
	}
	addString := func(name, value string) {
		if strings.TrimSpace(value) != "" {
			args = append(args, name, value)
		}
	}
	addBool("--use-tv-api", opt.UseTvApi)
	addBool("--use-app-api", opt.UseAppApi)
	addBool("--use-intl-api", opt.UseIntlApi)
	addBool("--use-mp4box", opt.UseMP4box)
	// long: API 请求体没有命令行参数先后顺序，原版 server 因此按默认“清晰度优先”选流；这里生成子进程参数时保持同样语义，避免保存路径预测和实际下载选中不同流。
	addString("--dfn-priority", opt.DfnPriority)
	addString("--encoding-priority", opt.EncodingPriority)
	addBool("--only-show-info", opt.OnlyShowInfo)
	addBool("--show-all", opt.ShowAll)
	addBool("--use-aria2c", opt.UseAria2c)
	addBool("--interactive", opt.Interactive)
	addBool("--hide-streams", opt.HideStreams)
	addBoolValue("--multi-thread", opt.MultiThread)
	addBool("--video-only", opt.VideoOnly)
	addBool("--audio-only", opt.AudioOnly)
	addBool("--danmaku-only", opt.DanmakuOnly)
	addBool("--cover-only", opt.CoverOnly)
	addBool("--sub-only", opt.SubOnly)
	addBool("--debug", opt.Debug)
	addBool("--skip-mux", opt.SkipMux)
	addBool("--skip-subtitle", opt.SkipSubtitle)
	addBool("--skip-cover", opt.SkipCover)
	addBoolValue("--force-http", opt.ForceHttp)
	addBool("--download-danmaku", opt.DownloadDanmaku)
	addString("--download-danmaku-formats", opt.DownloadDanmakuFormats)
	addBoolValue("--skip-ai", opt.SkipAi)
	addBool("--video-ascending", opt.VideoAscending)
	addBool("--audio-ascending", opt.AudioAscending)
	addBool("--allow-pcdn", opt.AllowPcdn)
	addBoolValue("--force-replace-host", opt.ForceReplaceHost)
	addBool("--save-archives-to-file", opt.SaveArchivesToFile)
	addBool("--simply-mux", opt.SimplyMux)
	addString("--file-exists-action", opt.FileExistsAction)
	addBool("--only-hevc", opt.OnlyHevc)
	addBool("--only-avc", opt.OnlyAvc)
	addBool("--only-av1", opt.OnlyAv1)
	addBool("--add-dfn-subfix", opt.AddDfnSubfix)
	addBool("--no-padding-page-num", opt.NoPaddingPageNum)
	addBool("--bandwith-ascending", opt.BandwithAscending)
	addString("--file-pattern", opt.FilePattern)
	addString("--multi-file-pattern", opt.MultiFilePattern)
	addString("--select-page", opt.SelectPage)
	addString("--video-index", opt.VideoIndex)
	addString("--audio-index", opt.AudioIndex)
	addString("--language", opt.Language)
	addString("--user-agent", opt.UserAgent)
	addString("--cookie", opt.Cookie)
	addString("--access-token", opt.AccessToken)
	addString("--aria2c-args", opt.Aria2cArgs)
	addString("--aria2c-proxy", opt.Aria2cProxy)
	addString("--work-dir", opt.WorkDir)
	addString("--ffmpeg-path", opt.FFmpegPath)
	addString("--mp4box-path", opt.Mp4boxPath)
	addString("--aria2c-path", opt.Aria2cPath)
	addString("--upos-host", opt.UposHost)
	addString("--delay-per-page", opt.DelayPerPage)
	addString("--host", opt.Host)
	addString("--ep-host", opt.EpHost)
	addString("--tv-host", opt.TvHost)
	addString("--area", opt.Area)
	addString("--config-file", opt.ConfigFile)
	return args
}

func totalTaskBytes(workDir string, paths []string) float64 {
	var total int64
	candidates := taskByteCandidatePaths(workDir, paths)
	for candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			total += info.Size()
		}
	}
	return float64(total)
}

func taskTransferBytes(workDir string, paths []string, state *taskTransferState) taskTransferSnapshot {
	var relatedTotal int64
	var predictedTotal int64
	var fallbackTotal int64
	var hasRelated bool
	var hasPredicted bool
	var hasFallback bool

	predicted, related, fallback := taskTransferCandidatePathSets(workDir, paths)
	for candidate := range related {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			relatedTotal += transferGrowthBytes(state, candidate, info.Size())
			hasRelated = true
		}
	}
	for candidate := range fallback {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			fallbackTotal += transferGrowthBytes(state, candidate, info.Size())
			hasFallback = true
		}
	}
	for candidate := range predicted {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			predictedTotal += transferGrowthBytes(state, candidate, info.Size())
			hasPredicted = true
		}
	}

	if hasRelated {
		return taskTransferSnapshot{
			bytes:                  float64(relatedTotal),
			hasDownloadFilePattern: true,
		}
	}
	if state != nil && state.sawDownloadFilePattern {
		return taskTransferSnapshot{}
	}
	if hasFallback {
		return taskTransferSnapshot{
			bytes:                   float64(fallbackTotal),
			hasPredictedOutputFiles: true,
		}
	}
	return taskTransferSnapshot{
		bytes:                   float64(predictedTotal),
		hasPredictedOutputFiles: hasPredicted,
	}
}

func taskTransferBaselineBytes(workDir string, paths []string) map[string]int64 {
	baseline := make(map[string]int64)
	predicted, related, fallback := taskTransferCandidatePathSets(workDir, paths)
	for _, set := range []map[string]struct{}{predicted, related, fallback} {
		for candidate := range set {
			if _, seen := baseline[candidate]; seen {
				continue
			}
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				baseline[candidate] = info.Size()
			}
		}
	}
	return baseline
}

func transferGrowthBytes(state *taskTransferState, path string, currentSize int64) int64 {
	if currentSize <= 0 {
		return 0
	}
	var baseline int64
	if state != nil && state.baselineBytes != nil {
		baseline = state.baselineBytes[path]
	}
	if currentSize <= baseline {
		return 0
	}
	return currentSize - baseline
}

func taskTransferCandidatePathSets(workDir string, paths []string) (map[string]struct{}, map[string]struct{}, map[string]struct{}) {
	predicted := make(map[string]struct{})
	related := make(map[string]struct{})
	fallback := make(map[string]struct{})
	for _, path := range paths {
		for _, resolved := range resolveTaskPathCandidates(workDir, path) {
			predicted[resolved] = struct{}{}
			addDownloadRelatedTransferPathCandidates(related, fallback, resolved)
		}
	}
	return predicted, related, fallback
}

func addDownloadRelatedTransferPathCandidates(candidates, fallback map[string]struct{}, path string) {
	ext := filepath.Ext(path)
	basePath := strings.TrimSuffix(path, ext)
	if ext != "" {
		candidates[basePath+".tmp"] = struct{}{}
	}
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	clipBase := flvClipBaseFromOutputBase(base)
	for _, related := range []string{
		filepath.Join(dir, base+".video.mp4"),
		filepath.Join(dir, base+".audio.m4a"),
	} {
		candidates[related] = struct{}{}
		relatedExt := filepath.Ext(related)
		if relatedExt != "" {
			candidates[strings.TrimSuffix(related, relatedExt)+".tmp"] = struct{}{}
		}
	}
	fallback[filepath.Join(dir, clipBase+".flv-merged.mp4")] = struct{}{}
	addDynamicDownloadFiles(candidates, dir, base)
	if clipBase != base {
		addDynamicDownloadFiles(candidates, dir, clipBase)
	}
}

func taskByteCandidatePaths(workDir string, paths []string) map[string]struct{} {
	candidates := make(map[string]struct{})
	for _, path := range paths {
		for _, resolved := range resolveTaskPathCandidates(workDir, path) {
			addDownloadRelatedPathCandidates(candidates, resolved)
		}
	}
	return candidates
}

func resolveTaskPathCandidates(workDir, path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	resolvedWorkDir, _ := ResolveWorkDir(workDir)
	if resolvedWorkDir != "" && !filepath.IsAbs(path) {
		return []string{filepath.Join(resolvedWorkDir, path)}
	}
	return []string{path}
}

func addDownloadRelatedPathCandidates(candidates map[string]struct{}, path string) {
	addDownloadFileCandidates(candidates, path)
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	clipBase := flvClipBaseFromOutputBase(base)
	for _, related := range []string{
		filepath.Join(dir, base+".video.mp4"),
		filepath.Join(dir, base+".audio.m4a"),
		filepath.Join(dir, clipBase+".flv-merged.mp4"),
	} {
		addDownloadFileCandidates(candidates, related)
	}
	addDynamicDownloadFiles(candidates, dir, base)
	if clipBase != base {
		addDynamicDownloadFiles(candidates, dir, clipBase)
	}
}

func flvClipBaseFromOutputBase(base string) string {
	return strings.TrimSuffix(base, ".flv-merged")
}

func addDownloadFileCandidates(candidates map[string]struct{}, path string) {
	candidates[path] = struct{}{}
	ext := filepath.Ext(path)
	if ext != "" {
		basePath := strings.TrimSuffix(path, ext)
		candidates[basePath+".tmp"] = struct{}{}
		addDynamicDownloadFiles(candidates, filepath.Dir(path), filepath.Base(basePath))
	}
}

func addDynamicDownloadFiles(candidates map[string]struct{}, dir, base string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// long: DASH、FLV 和多线程下载都会先写临时文件；按最终文件 basename 限定扫描范围，避免把同目录其它任务误计入当前任务。
		if isRelatedClipFile(name, base) || isRelatedMultiThreadPart(name, base) {
			candidates[filepath.Join(dir, name)] = struct{}{}
		}
	}
}

func isRelatedClipFile(name, base string) bool {
	return strings.HasPrefix(name, base+".clip") && (strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".tmp"))
}

func isRelatedMultiThreadPart(name, base string) bool {
	if len(name) <= 6 || name[5] != '_' {
		return false
	}
	for _, r := range name[:5] {
		if r < '0' || r > '9' {
			return false
		}
	}
	stem := name[6:]
	if !strings.HasPrefix(stem, base) {
		return false
	}
	return strings.HasSuffix(name, ".vclip") || strings.HasSuffix(name, ".aclip")
}

func totalExistingBytes(workDir string, paths []string) float64 {
	var total int64
	seen := make(map[string]struct{})
	for _, path := range paths {
		for _, candidate := range resolveTaskPathCandidates(workDir, path) {
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				total += info.Size()
				break
			}
		}
	}
	return float64(total)
}

func totalExistingGrowthBytes(workDir string, paths []string, state *taskTransferState) float64 {
	var total int64
	seen := make(map[string]struct{})
	for _, path := range paths {
		for _, candidate := range resolveTaskPathCandidates(workDir, path) {
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				total += transferGrowthBytes(state, candidate, info.Size())
				break
			}
		}
	}
	return float64(total)
}

type taskLogTail struct {
	limit int
	lines []string
}

func newTaskLogTail(limit int) *taskLogTail {
	if limit <= 0 {
		limit = 1
	}
	return &taskLogTail{limit: limit}
}

func (t *taskLogTail) add(line string) {
	if t == nil {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	line = truncateRunes(line, 500)
	t.lines = append(t.lines, line)
	if overflow := len(t.lines) - t.limit; overflow > 0 {
		t.lines = append([]string(nil), t.lines[overflow:]...)
	}
}

func (t *taskLogTail) snapshot() []string {
	if t == nil || len(t.lines) == 0 {
		return nil
	}
	return append([]string(nil), t.lines...)
}

func taskErrorSummary(err error, req ServeRequestOptions, lines []string) string {
	var parts []string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			parts = append(parts, line)
			break
		}
	}
	if err != nil {
		parts = append(parts, err.Error())
	}
	if len(parts) == 0 {
		parts = append(parts, "任务执行失败")
	}
	summary := strings.Join(parts, ": ")
	return truncateRunes(redactTaskSecrets(summary, req), 800)
}

func redactTaskSecrets(text string, req ServeRequestOptions) string {
	// long: 服务任务错误会出现在查询接口和 webhook 中，只保留排障所需摘要，避免把 Cookie 或 token 随任务状态扩散出去。
	text = taskSecretParamPattern.ReplaceAllString(text, "${1}=<redacted>")
	for _, secret := range taskSecretValues(req) {
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, "<redacted>")
	}
	return text
}

func taskSecretValues(req ServeRequestOptions) []string {
	var values []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if len(value) >= 4 {
			values = append(values, value)
		}
	}
	add(req.Cookie)
	add(req.AccessToken)
	add(strings.TrimPrefix(req.AccessToken, "access_token="))
	for _, part := range strings.Split(req.Cookie, ";") {
		if _, value, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
			add(value)
		}
	}
	return values
}

func fallbackTaskAid(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "unknown"
	}
	return rawURL
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func (s *ApiServer) postCallback(callback string, task DownloadTask) {
	// long 2026-06-18 21:24:28：callback 是服务任务的收尾通知，失败不能反向拖住任务协程；设置超时保持原版“回调失败不影响任务状态”的边界。
	s.postCallbackWithClient(callback, task, &http.Client{Timeout: 10 * time.Second})
}

func (s *ApiServer) postCallbackWithClient(callback string, task DownloadTask, client *http.Client) {
	if _, err := url.ParseRequestURI(callback); err != nil {
		return
	}
	body, err := json.Marshal(task)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, callback, strings.NewReader(string(body)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func removeTask(tasks []DownloadTask, aid string) []DownloadTask {
	out := tasks[:0]
	for _, t := range tasks {
		if t.Aid != aid {
			out = append(out, t)
		}
	}
	return out
}

func ptrTo[T any](value T) *T {
	return &value
}
