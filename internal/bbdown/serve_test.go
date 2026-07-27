package bbdown

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestServeOptionsToArgs(t *testing.T) {
	args := serveOptionsToArgs(MyOption{
		UseTvApi:               true,
		DanmakuOnly:            true,
		DownloadDanmakuFormats: "ass",
		WorkDir:                "/tmp/bbdown-test",
		SelectPage:             "1",
		VideoIndex:             "3",
		AudioIndex:             "2",
		FileExistsAction:       FileExistsActionOverwrite,
		Aria2cProxy:            "http://127.0.0.1:7890",
	})

	for _, want := range []string{"--use-tv-api", "--danmaku-only", "--download-danmaku-formats", "ass", "--work-dir", "/tmp/bbdown-test", "--select-page", "1", "--video-index", "3", "--audio-index", "2", "--file-exists-action", "overwrite", "--aria2c-proxy", "http://127.0.0.1:7890"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args %v missing %q", args, want)
		}
	}
}

func TestValidateServeRequestOptionsRejectsInvalidFileExistsAction(t *testing.T) {
	if err := validateServeRequestOptions(ServeRequestOptions{MyOption: MyOption{FileExistsAction: "invalid"}}); err == nil {
		t.Fatal("invalid file exists action should be rejected before starting a server task")
	}
}

func TestServeOptionsToArgsDefaultTrueValues(t *testing.T) {
	opt := DefaultMyOption()
	opt.MultiThread = false
	opt.ForceHttp = false
	opt.SkipAi = false
	opt.ForceReplaceHost = false
	opt.AddDfnSubfix = true
	args := serveOptionsToArgs(opt)

	for _, want := range []string{"--multi-thread=false", "--force-http=false", "--skip-ai=false", "--force-replace-host=false", "--add-dfn-subfix"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args %v missing %q", args, want)
		}
	}
}

func TestNormalizeOptionsForCompatibilityIsIdempotentForServe(t *testing.T) {
	opt := MyOption{
		Aria2cArgs:        "-x 4",
		Aria2cProxy:       "http://127.0.0.1:7890",
		AddDfnSubfix:      true,
		NoPaddingPageNum:  true,
		OnlyHevc:          true,
		BandwithAscending: true,
	}

	normalizeOptionsForCompatibility(&opt, false)
	normalizeOptionsForCompatibility(&opt, false)

	proxyArg := `--all-proxy="http://127.0.0.1:7890"`
	if strings.Count(opt.Aria2cArgs, proxyArg) != 1 {
		t.Fatalf("Aria2cArgs = %q, want one %s", opt.Aria2cArgs, proxyArg)
	}
	if opt.FilePattern != "<videoTitle>[<dfn>]" || opt.MultiFilePattern != "<videoTitle>/[P<pageNumber>]<pageTitle>[<dfn>]" {
		t.Fatalf("patterns = %q %q", opt.FilePattern, opt.MultiFilePattern)
	}
	if opt.EncodingPriority != "hevc" {
		t.Fatalf("EncodingPriority = %q, want hevc", opt.EncodingPriority)
	}
	if !opt.VideoAscending || !opt.AudioAscending {
		t.Fatalf("ascending flags = video:%v audio:%v, want both true", opt.VideoAscending, opt.AudioAscending)
	}
}

func TestServeOptionsToArgsKeepsServerDefaultPriorityOrder(t *testing.T) {
	args := serveOptionsToArgs(MyOption{
		EncodingPriority: "hevc,avc",
		DfnPriority:      "1080P 高清,720P 高清",
	})
	dfnIndex := slices.Index(args, "--dfn-priority")
	encodingIndex := slices.Index(args, "--encoding-priority")
	if dfnIndex < 0 || encodingIndex < 0 {
		t.Fatalf("priority args missing: %v", args)
	}
	if dfnIndex > encodingIndex {
		t.Fatalf("server generated args should keep dfn priority first: %v", args)
	}
}

func TestServeOptionsToArgsClearsUnusedStreamIndexes(t *testing.T) {
	tests := []struct {
		name         string
		opt          MyOption
		wantPresent  []string
		wantAdjacent [][2]string
		wantMissing  []string
	}{
		{
			name:         "audio only ignores video index",
			opt:          MyOption{AudioOnly: true, VideoIndex: "bad", AudioIndex: "1"},
			wantPresent:  []string{"--audio-only"},
			wantAdjacent: [][2]string{{"--audio-index", "1"}},
			wantMissing:  []string{"--video-index"},
		},
		{
			name:         "video only ignores audio index",
			opt:          MyOption{VideoOnly: true, VideoIndex: "1", AudioIndex: "bad"},
			wantPresent:  []string{"--video-only"},
			wantAdjacent: [][2]string{{"--video-index", "1"}},
			wantMissing:  []string{"--audio-index"},
		},
		{
			name:        "info ignores both indexes",
			opt:         MyOption{OnlyShowInfo: true, VideoIndex: "bad", AudioIndex: "bad"},
			wantPresent: []string{"--only-show-info"},
			wantMissing: []string{"--video-index", "--audio-index"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := serveOptionsToArgs(tc.opt)
			for _, want := range tc.wantPresent {
				if !slices.Contains(args, want) {
					t.Fatalf("args %v missing %q", args, want)
				}
			}
			for _, pair := range tc.wantAdjacent {
				if !containsAdjacentArgs(args, pair[0], pair[1]) {
					t.Fatalf("args %v missing %s %s", args, pair[0], pair[1])
				}
			}
			for _, missing := range tc.wantMissing {
				if slices.Contains(args, missing) {
					t.Fatalf("args %v should not contain %q", args, missing)
				}
			}
		})
	}
}

func TestServeOptionsToArgsNormalizesExclusiveOnlyFlagsBeforeStreamIndexes(t *testing.T) {
	args := serveOptionsToArgs(MyOption{
		AudioOnly:  true,
		VideoOnly:  true,
		VideoIndex: "1",
		AudioIndex: "2",
	})

	for _, unwanted := range []string{"--audio-only", "--video-only"} {
		if slices.Contains(args, unwanted) {
			t.Fatalf("exclusive only flags should be normalized away before args are generated: %v", args)
		}
	}
	for _, pair := range [][2]string{{"--video-index", "1"}, {"--audio-index", "2"}} {
		if !containsAdjacentArgs(args, pair[0], pair[1]) {
			t.Fatalf("normal download after compatibility should keep %s %s: %v", pair[0], pair[1], args)
		}
	}
}

func TestValidateServeRequestOptionsChecksIndexesAfterCompatibility(t *testing.T) {
	err := validateServeRequestOptions(ServeRequestOptions{MyOption: MyOption{
		Url:        "BV1J9EB6xEAB",
		AudioOnly:  true,
		VideoOnly:  true,
		VideoIndex: "bad",
		AudioIndex: "0",
	}})

	if err == nil || !strings.Contains(err.Error(), "视频流序号无效") {
		t.Fatalf("exclusive only flags normalize to normal download, bad video index should fail: %v", err)
	}
}

func containsAdjacentArgs(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func TestIsServerChildProcess(t *testing.T) {
	if IsServerChildProcess() {
		t.Fatal("server child process marker should be disabled by default")
	}
	t.Setenv(ServerChildEnv, "1")
	if !IsServerChildProcess() {
		t.Fatal("server child process marker should be enabled from env")
	}
}

func TestParseServerTransferEvent(t *testing.T) {
	event, ok := parseServerTransferEvent(serverTransferEventPrefix + `{"Path":"demo.video.mp4","Bytes":123}`)
	if !ok {
		t.Fatal("server transfer event should parse")
	}
	if event.Path != "demo.video.mp4" || event.Bytes != 123 {
		t.Fatalf("event = %+v", event)
	}
	if _, ok := parseServerTransferEvent("普通日志"); ok {
		t.Fatal("normal log line should not parse as a transfer event")
	}
	if _, ok := parseServerTransferEvent(serverTransferEventPrefix + `{"Path":"demo.video.mp4","Bytes":0}`); ok {
		t.Fatal("zero-byte transfer event should be ignored")
	}
	event, ok = parseServerTransferEvent(serverTransferEventPrefix + `{"Path":"demo.video.mp4","Bytes":45,"Delta":true}`)
	if !ok || !event.Delta || event.Bytes != 45 {
		t.Fatalf("delta event = %+v, ok = %v", event, ok)
	}
	if _, ok := parseServerTransferEvent(serverTransferEventPrefix + `{"Path":"","Bytes":45,"Delta":true}`); ok {
		t.Fatal("empty path transfer event should be ignored")
	}
}

func TestServerTransferWriterEmitsDeltaEvents(t *testing.T) {
	t.Setenv(ServerChildEnv, "1")
	output := captureStdout(t, func() {
		var body strings.Builder
		writer, flush := serverTransferWriter(&body, "demo.video.mp4")
		if _, err := writer.Write([]byte("hello")); err != nil {
			t.Fatal(err)
		}
		flush()
		if body.String() != "hello" {
			t.Fatalf("writer body = %q", body.String())
		}
	})

	event, ok := parseServerTransferEvent(strings.TrimSpace(output))
	if !ok {
		t.Fatalf("transfer output did not parse: %q", output)
	}
	if !event.Delta || event.Path != "demo.video.mp4" || event.Bytes != 5 {
		t.Fatalf("event = %+v, want 5-byte delta for demo.video.mp4", event)
	}
}

func TestServeRequestOptionsJSONDefaults(t *testing.T) {
	var req ServeRequestOptions
	if err := json.Unmarshal([]byte(`{"Url":"BV1xx"}`), &req); err != nil {
		t.Fatal(err)
	}
	if !req.MultiThread || !req.ForceHttp || !req.SkipAi || !req.ForceReplaceHost {
		t.Fatalf("default true options not applied: %+v", req.MyOption)
	}
	if req.DelayPerPage != "0" || req.Host != "api.bilibili.com" || req.EpHost != "api.bilibili.com" || req.TvHost != "api.snm0516.aisee.tv" {
		t.Fatalf("default string options not applied: %+v", req.MyOption)
	}

	if err := json.Unmarshal([]byte(`{"Url":"BV1xx","MultiThread":false,"ForceHttp":false,"SkipAi":false,"ForceReplaceHost":false}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.MultiThread || req.ForceHttp || req.SkipAi || req.ForceReplaceHost {
		t.Fatalf("explicit false options should be preserved: %+v", req.MyOption)
	}
}

func TestDownloadTaskJSONIncludesNullFields(t *testing.T) {
	body, err := json.Marshal(DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{`"Title":null`, `"Pic":null`, `"VideoPubTime":null`, `"TaskFinishTime":null`, `"ErrorStage":null`, `"ErrorMessage":null`} {
		if !strings.Contains(text, want) {
			t.Fatalf("download task json missing %s: %s", want, text)
		}
	}
	if !strings.Contains(text, `"SavePaths":[]`) {
		t.Fatalf("download task json should encode empty SavePaths as []: %s", text)
	}
}

func TestDownloadTaskCollectionJSONUsesEmptyArrays(t *testing.T) {
	body, err := json.Marshal(DownloadTaskCollection{})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `"Running":[]`) || !strings.Contains(text, `"Finished":[]`) {
		t.Fatalf("task collection json should encode empty arrays: %s", text)
	}
}

func TestApiServerTaskHandlersReturnJSONArrays(t *testing.T) {
	server := NewApiServer(NewConfig(), nil)
	req := httptest.NewRequest(http.MethodGet, "/get-tasks/", nil)
	rec := httptest.NewRecorder()
	server.handleTasks(rec, req)

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"Running":[]`) || !strings.Contains(body, `"Finished":[]`) {
		t.Fatalf("empty task response should use arrays: %s", body)
	}
}

func TestApiServerTaskCollectionHandlers(t *testing.T) {
	server := NewApiServer(NewConfig(), nil)
	server.running = []DownloadTask{{Aid: "running-aid", Url: "BV-running", IsSuccessful: false}}
	server.done = []DownloadTask{{Aid: "finished-aid", Url: "BV-finished", IsSuccessful: true}}

	rec := httptest.NewRecorder()
	server.handleTasks(rec, httptest.NewRequest(http.MethodGet, "/get-tasks/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("task collection status = %d", rec.Code)
	}
	var collection DownloadTaskCollection
	if err := json.Unmarshal(rec.Body.Bytes(), &collection); err != nil {
		t.Fatal(err)
	}
	if got := taskAids(collection.Running); !slices.Equal(got, []string{"running-aid"}) {
		t.Fatalf("collection running aids = %v", got)
	}
	if got := taskAids(collection.Finished); !slices.Equal(got, []string{"finished-aid"}) {
		t.Fatalf("collection finished aids = %v", got)
	}

	rec = httptest.NewRecorder()
	server.handleRunning(rec, httptest.NewRequest(http.MethodGet, "/get-tasks/running", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("running task status = %d", rec.Code)
	}
	var running []DownloadTask
	if err := json.Unmarshal(rec.Body.Bytes(), &running); err != nil {
		t.Fatal(err)
	}
	if got := taskAids(running); !slices.Equal(got, []string{"running-aid"}) {
		t.Fatalf("running aids = %v", got)
	}

	rec = httptest.NewRecorder()
	server.handleFinished(rec, httptest.NewRequest(http.MethodGet, "/get-tasks/finished", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("finished task status = %d", rec.Code)
	}
	var finished []DownloadTask
	if err := json.Unmarshal(rec.Body.Bytes(), &finished); err != nil {
		t.Fatal(err)
	}
	if got := taskAids(finished); !slices.Equal(got, []string{"finished-aid"}) {
		t.Fatalf("finished aids = %v", got)
	}
}

func TestApiServerTaskRoutes(t *testing.T) {
	server := NewApiServer(NewConfig(), nil)
	server.running = []DownloadTask{{Aid: "same-aid", Url: "BV-running", IsSuccessful: false}}
	server.done = []DownloadTask{
		{Aid: "finished-aid", Url: "BV-finished", IsSuccessful: true},
		{Aid: "same-aid", Url: "BV-finished-same", IsSuccessful: true},
	}
	routes := server.routes()

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/get-tasks/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("task collection route status = %d", rec.Code)
	}
	var collection DownloadTaskCollection
	if err := json.Unmarshal(rec.Body.Bytes(), &collection); err != nil {
		t.Fatal(err)
	}
	if got := taskAids(collection.Running); !slices.Equal(got, []string{"same-aid"}) {
		t.Fatalf("task collection route running aids = %v", got)
	}
	if got := taskAids(collection.Finished); !slices.Equal(got, []string{"finished-aid", "same-aid"}) {
		t.Fatalf("task collection route finished aids = %v", got)
	}

	rec = httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/get-tasks/running", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("running route status = %d", rec.Code)
	}
	var running []DownloadTask
	if err := json.Unmarshal(rec.Body.Bytes(), &running); err != nil {
		t.Fatal(err)
	}
	if got := taskAids(running); !slices.Equal(got, []string{"same-aid"}) {
		t.Fatalf("running route aids = %v", got)
	}

	rec = httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/get-tasks/finished", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("finished route status = %d", rec.Code)
	}
	var finished []DownloadTask
	if err := json.Unmarshal(rec.Body.Bytes(), &finished); err != nil {
		t.Fatal(err)
	}
	if got := taskAids(finished); !slices.Equal(got, []string{"finished-aid", "same-aid"}) {
		t.Fatalf("finished route aids = %v", got)
	}

	rec = httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/get-tasks/same-aid", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("task by aid route status = %d", rec.Code)
	}
	var task DownloadTask
	if err := json.Unmarshal(rec.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.Url != "BV-running" {
		t.Fatalf("running task should win on the by-aid route, got %+v", task)
	}

	rec = httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/get-tasks/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing task route status = %d, want 404", rec.Code)
	}
}

func TestApiServerAddTaskRoutes(t *testing.T) {
	cfg := NewConfig()
	server := NewApiServer(cfg, NewHTTPClient(cfg))
	routes := server.routes()

	for name, body := range map[string]string{
		"malformed json": "{",
		"empty url":      `{"Url":""}`,
		"bad video index": `{
			"Url":"BV1J9EB6xEAB",
			"VideoIndex":"bad"
		}`,
		"bad index after exclusive only normalization": `{
			"Url":"BV1J9EB6xEAB",
			"AudioOnly":true,
			"VideoOnly":true,
			"VideoIndex":"bad",
			"AudioIndex":"0"
		}`,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/add-task", strings.NewReader(body))
		routes.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s add-task status = %d, want 400", name, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "输入有误") {
			t.Fatalf("%s add-task body = %q", name, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/add-task", strings.NewReader(`{"Url":"not-a-bili-id"}`))
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid add-task envelope status = %d, want 200", rec.Code)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		server.mu.Lock()
		running := cloneTasks(server.running)
		done := cloneTasks(server.done)
		server.mu.Unlock()
		if len(done) == 1 {
			if len(running) != 0 {
				t.Fatalf("failed task should not stay running: %+v", running)
			}
			task := done[0]
			if task.Aid != "not-a-bili-id" || task.Url != "not-a-bili-id" || task.IsSuccessful {
				t.Fatalf("finished failed task = %+v", task)
			}
			if task.ErrorMessage == nil || !strings.Contains(*task.ErrorMessage, "输入有误") {
				t.Fatalf("finished failed task should keep input error: %+v", task.ErrorMessage)
			}
			if task.ErrorStage == nil || *task.ErrorStage != taskErrorStageResolve {
				t.Fatalf("finished failed task should mark resolve stage: %+v", task.ErrorStage)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("valid add-task envelope did not finish the failed task")
}

func TestValidateServeRequestOptionsIgnoresUnusedStreamIndex(t *testing.T) {
	err := validateServeRequestOptions(ServeRequestOptions{MyOption: MyOption{
		Url:        "BV1J9EB6xEAB",
		AudioOnly:  true,
		VideoIndex: "bad",
		AudioIndex: "0",
	}})
	if err != nil {
		t.Fatalf("audio-only service task should ignore video index: %v", err)
	}
}

func TestApiServerCORS(t *testing.T) {
	nextCalled := false
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	assertCORSHeaders := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		for name, want := range map[string]string{
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "*",
			"Access-Control-Allow-Headers": "*",
		} {
			if got := rec.Header().Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/get-tasks/", nil))
	if !nextCalled {
		t.Fatal("normal requests should continue to the API handler")
	}
	assertCORSHeaders(t, rec)

	nextCalled = false
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/get-tasks/", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want 204", rec.Code)
	}
	if nextCalled {
		t.Fatal("preflight requests should be handled before route dispatch")
	}
	assertCORSHeaders(t, rec)
}

func TestApiServerTaskByIDHandler(t *testing.T) {
	server := NewApiServer(NewConfig(), nil)
	server.running = []DownloadTask{{Aid: "same-aid", Url: "BV-running", IsSuccessful: false}}
	server.done = []DownloadTask{
		{Aid: "finished-aid", Url: "BV-finished", IsSuccessful: true},
		{Aid: "same-aid", Url: "BV-finished-same", IsSuccessful: true},
	}

	req := httptest.NewRequest(http.MethodGet, "/get-tasks/finished-aid", nil)
	req.SetPathValue("id", "finished-aid")
	rec := httptest.NewRecorder()
	server.handleTaskByID(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("finished task lookup status = %d", rec.Code)
	}
	var task DownloadTask
	if err := json.Unmarshal(rec.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.Aid != "finished-aid" || task.Url != "BV-finished" {
		t.Fatalf("finished task lookup = %+v", task)
	}

	req = httptest.NewRequest(http.MethodGet, "/get-tasks/same-aid", nil)
	req.SetPathValue("id", "same-aid")
	rec = httptest.NewRecorder()
	server.handleTaskByID(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("running task lookup status = %d", rec.Code)
	}
	task = DownloadTask{}
	if err := json.Unmarshal(rec.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.Url != "BV-running" {
		t.Fatalf("running task should win when Aid exists in both queues, got %+v", task)
	}

	req = httptest.NewRequest(http.MethodGet, "/get-tasks/missing", nil)
	req.SetPathValue("id", "missing")
	rec = httptest.NewRecorder()
	server.handleTaskByID(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing task lookup status = %d, want 404", rec.Code)
	}
}

func TestApiServerRemoveFinishedHandlers(t *testing.T) {
	seedDone := func() []DownloadTask {
		return []DownloadTask{
			{Aid: "success-1", Url: "BV1", IsSuccessful: true},
			{Aid: "failed-1", Url: "BV2", IsSuccessful: false},
			{Aid: "success-2", Url: "BV3", IsSuccessful: true},
		}
	}

	server := NewApiServer(NewConfig(), nil)
	server.done = seedDone()
	rec := httptest.NewRecorder()
	server.handleRemoveFinished(rec, httptest.NewRequest(http.MethodGet, "/remove-finished/failed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("remove failed status = %d", rec.Code)
	}
	if got := finishedTaskAids(server.done); !slices.Equal(got, []string{"success-1", "success-2"}) {
		t.Fatalf("remove failed should keep only successful tasks, got %v", got)
	}

	server.done = seedDone()
	req := httptest.NewRequest(http.MethodGet, "/remove-finished/failed-1", nil)
	req.SetPathValue("id", "failed-1")
	rec = httptest.NewRecorder()
	server.handleRemoveFinished(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove by aid status = %d", rec.Code)
	}
	if got := finishedTaskAids(server.done); !slices.Equal(got, []string{"success-1", "success-2"}) {
		t.Fatalf("remove by aid should drop only matched task, got %v", got)
	}

	server.done = seedDone()
	rec = httptest.NewRecorder()
	server.handleRemoveFinished(rec, httptest.NewRequest(http.MethodGet, "/remove-finished/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("remove all status = %d", rec.Code)
	}
	if len(server.done) != 0 {
		t.Fatalf("remove all should clear finished tasks, got %+v", server.done)
	}
}

func TestApiServerRemoveFinishedRoutes(t *testing.T) {
	seedDone := func() []DownloadTask {
		return []DownloadTask{
			{Aid: "success-1", Url: "BV1", IsSuccessful: true},
			{Aid: "failed-1", Url: "BV2", IsSuccessful: false},
			{Aid: "success-2", Url: "BV3", IsSuccessful: true},
		}
	}

	server := NewApiServer(NewConfig(), nil)
	server.done = seedDone()
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/remove-finished/failed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("remove failed route status = %d", rec.Code)
	}
	if got := taskAids(server.done); !slices.Equal(got, []string{"success-1", "success-2"}) {
		t.Fatalf("remove failed route should keep only successful tasks, got %v", got)
	}

	server.done = seedDone()
	rec = httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/remove-finished/failed-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("remove by aid route status = %d", rec.Code)
	}
	if got := taskAids(server.done); !slices.Equal(got, []string{"success-1", "success-2"}) {
		t.Fatalf("remove by aid route should drop only matched task, got %v", got)
	}

	server.done = seedDone()
	rec = httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/remove-finished/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("remove all route status = %d", rec.Code)
	}
	if len(server.done) != 0 {
		t.Fatalf("remove all route should clear finished tasks, got %+v", server.done)
	}

	server.done = seedDone()
	rec = httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/remove-finished", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("remove all route without trailing slash status = %d", rec.Code)
	}
	if len(server.done) != 0 {
		t.Fatalf("remove all route without trailing slash should clear finished tasks, got %+v", server.done)
	}
}

func finishedTaskAids(tasks []DownloadTask) []string {
	return taskAids(tasks)
}

func taskAids(tasks []DownloadTask) []string {
	aids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		aids = append(aids, task.Aid)
	}
	return aids
}

func TestTaskErrorSummaryUsesLastLineAndRedactsSecrets(t *testing.T) {
	req := ServeRequestOptions{
		MyOption: MyOption{
			Cookie:      "SESSDATA=cookie-secret; bili_jct=jct-secret",
			AccessToken: "access_token=token-secret",
		},
	}
	summary := taskErrorSummary(errors.New("exit status 1"), req, []string{
		"开始下载",
		"接口失败 access_key=token-secret Cookie: SESSDATA=cookie-secret; bili_jct=jct-secret",
	})
	for _, leaked := range []string{"token-secret", "cookie-secret", "jct-secret"} {
		if strings.Contains(summary, leaked) {
			t.Fatalf("summary leaked secret %q: %s", leaked, summary)
		}
	}
	for _, want := range []string{"接口失败", "access_key=<redacted>", "Cookie:", "SESSDATA=<redacted>", "bili_jct=<redacted>", "exit status 1"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q: %s", want, summary)
		}
	}
}

func TestSetTaskErrorRecordsStageAndSummary(t *testing.T) {
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	setTaskError(&task, taskErrorStageDownload, errors.New("exit status 1"), ServeRequestOptions{}, []string{"最后一行失败"})

	if task.ErrorStage == nil || *task.ErrorStage != taskErrorStageDownload {
		t.Fatalf("error stage = %+v, want download", task.ErrorStage)
	}
	if task.ErrorMessage == nil || !strings.Contains(*task.ErrorMessage, "最后一行失败") || !strings.Contains(*task.ErrorMessage, "exit status 1") {
		t.Fatalf("error summary = %+v", task.ErrorMessage)
	}
}

func TestAddTaskRecordsResolveErrorAsFinishedTask(t *testing.T) {
	cfg := NewConfig()
	server := NewApiServer(cfg, NewHTTPClient(cfg))
	server.addTask(ServeRequestOptions{MyOption: MyOption{Url: "not-a-bili-id"}})

	if len(server.running) != 0 {
		t.Fatalf("invalid task should not stay running: %+v", server.running)
	}
	if len(server.done) != 1 {
		t.Fatalf("invalid task should be recorded as finished, got %d", len(server.done))
	}
	task := server.done[0]
	if task.Aid != "not-a-bili-id" || task.Url != "not-a-bili-id" {
		t.Fatalf("failed task identity = %+v", task)
	}
	if task.IsSuccessful {
		t.Fatal("invalid task should be marked unsuccessful")
	}
	if task.TaskFinishTime == nil {
		t.Fatal("failed task should have finish time")
	}
	if task.ErrorMessage == nil || !strings.Contains(*task.ErrorMessage, "输入有误") {
		t.Fatalf("failed task should include error summary: %+v", task.ErrorMessage)
	}
	if task.ErrorStage == nil || *task.ErrorStage != taskErrorStageResolve {
		t.Fatalf("failed task should include resolve stage: %+v", task.ErrorStage)
	}
}

func TestAddTaskRecordsInfoErrorStage(t *testing.T) {
	cfg := NewConfig()
	httpc := NewHTTPClient(cfg)
	httpc.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := ""
		switch {
		case req.Method == http.MethodGet && req.URL.Host == "api.bilibili.com" && req.URL.Path == "/x/web-interface/nav":
			body = `{"data":{"isLogin":true,"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/img.png","sub_url":"https://i0.hdslb.com/bfs/wbi/sub.png"}}}`
		case req.Method == http.MethodHead && req.URL.Host == "www.bilibili.com" && strings.HasPrefix(req.URL.Path, "/video/av123"):
			body = ""
		case req.Method == http.MethodGet && req.URL.Host == "api.bilibili.com" && req.URL.Path == "/x/web-interface/view":
			body = `{"code":0,"data":{"title":"空分P样本","desc":"","pic":"","pubdate":0,"bvid":"BV1xx","cid":"0","pages":[]}}`
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	server := NewApiServer(cfg, httpc)
	server.addTask(ServeRequestOptions{MyOption: MyOption{Url: "av123"}})

	if len(server.running) != 0 {
		t.Fatalf("info failed task should not stay running: %+v", server.running)
	}
	if len(server.done) != 1 {
		t.Fatalf("info failed task should be recorded as finished, got %d", len(server.done))
	}
	task := server.done[0]
	if task.Aid != "123" || task.IsSuccessful {
		t.Fatalf("info failed task identity = %+v", task)
	}
	if task.ErrorStage == nil || *task.ErrorStage != taskErrorStageInfo {
		t.Fatalf("info failed task should include info stage: %+v", task.ErrorStage)
	}
	if task.ErrorMessage == nil || !strings.Contains(*task.ErrorMessage, "未获取到分P信息") {
		t.Fatalf("info failed task should include page error: %+v", task.ErrorMessage)
	}
}

func TestAddTaskResolveErrorIgnoresInvalidCallbackURL(t *testing.T) {
	cfg := NewConfig()
	server := NewApiServer(cfg, NewHTTPClient(cfg))
	server.addTask(ServeRequestOptions{
		MyOption:        MyOption{Url: "not-a-bili-id"},
		CallBackWebHook: "://bad callback",
	})

	if len(server.running) != 0 {
		t.Fatalf("invalid task should not stay running: %+v", server.running)
	}
	if len(server.done) != 1 {
		t.Fatalf("invalid callback should not drop finished failed task, got %d", len(server.done))
	}
	task := server.done[0]
	if task.Aid != "not-a-bili-id" || task.ErrorMessage == nil || !strings.Contains(*task.ErrorMessage, "输入有误") {
		t.Fatalf("finished failed task = %+v", task)
	}
	if task.ErrorStage == nil || *task.ErrorStage != taskErrorStageResolve {
		t.Fatalf("finished failed task should keep resolve stage: %+v", task.ErrorStage)
	}
}

func TestPostCallbackWithClient(t *testing.T) {
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	var gotMethod, gotType, gotBody string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod = req.Method
		gotType = req.Header.Get("Content-Type")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		gotBody = string(body)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}

	server := NewApiServer(NewConfig(), nil)
	server.postCallbackWithClient("http://callback.local/hook", task, client)

	if gotMethod != http.MethodPost || gotType != "application/json" {
		t.Fatalf("callback request method/type = %s %s", gotMethod, gotType)
	}
	if !strings.Contains(gotBody, `"Aid":"1"`) || !strings.Contains(gotBody, `"SavePaths":[]`) {
		t.Fatalf("callback body = %s", gotBody)
	}
}

func TestPostCallbackWithClientIgnoresInvalidURL(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}

	server := NewApiServer(NewConfig(), nil)
	server.postCallbackWithClient("://bad callback", DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}, client)

	if called {
		t.Fatal("invalid callback URL should not issue an HTTP request")
	}
}

func TestApplyProgressLineTracksPageAndClipProgress(t *testing.T) {
	server := NewApiServer(NewConfig(), nil)
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	state := &taskProgressState{}

	if server.applyProgressLine(&task, state, "开始解析P1: 1... (1 of 3)") {
		t.Fatal("first page start should keep progress at zero")
	}
	if state.totalPages != 3 {
		t.Fatalf("totalPages = %d, want 3", state.totalPages)
	}
	if !server.applyProgressLine(&task, state, "[2026-06-27 11:02:03.004] - 开始下载P1视频...") {
		t.Fatal("timestamped child log should update progress")
	}
	if !server.applyProgressLine(&task, state, "下载P1完毕") {
		t.Fatal("page completion should update progress")
	}
	if task.Progress < 0.24 || task.Progress > 0.26 {
		t.Fatalf("after first page downloads progress = %v, want about 1/4", task.Progress)
	}
	if !server.applyProgressLine(&task, state, "开始下载P2视频, 片段(1/3)...") {
		t.Fatal("clip progress should update within current page")
	}
	if task.Progress <= 1.0/3.0 || task.Progress >= 0.50 {
		t.Fatalf("clip progress = %v, want early progress within page 2", task.Progress)
	}
	if !server.applyProgressLine(&task, state, "下载P2完毕") {
		t.Fatal("second page completion should update progress")
	}
	if task.Progress < 0.58 || task.Progress > 0.59 {
		t.Fatalf("after second page downloads progress = %v, want about 7/12", task.Progress)
	}
	if !server.applyProgressLine(&task, state, "任务完成") {
		t.Fatal("task completion should update progress")
	}
	if task.Progress != 1 {
		t.Fatalf("final progress = %v, want 1", task.Progress)
	}
}

func TestApplyProgressLineTracksDASHMediaStages(t *testing.T) {
	server := NewApiServer(NewConfig(), nil)
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	state := &taskProgressState{}

	if server.applyProgressLine(&task, state, "开始解析P1: 1... (1 of 2)") {
		t.Fatal("first page start should keep progress at zero")
	}
	if !server.applyProgressLine(&task, state, "开始下载P1视频...") {
		t.Fatal("video download start should update progress")
	}
	videoProgress := task.Progress
	if videoProgress <= 0 || videoProgress >= 0.10 {
		t.Fatalf("video progress = %v, want early page progress", videoProgress)
	}
	if !server.applyProgressLine(&task, state, "开始下载P1音频...") {
		t.Fatal("audio download start should update progress")
	}
	audioProgress := task.Progress
	if audioProgress <= videoProgress || audioProgress >= 0.20 {
		t.Fatalf("audio progress = %v, video progress = %v", audioProgress, videoProgress)
	}
	if !server.applyProgressLine(&task, state, "下载P1完毕") {
		t.Fatal("download completion should update progress")
	}
	downloadProgress := task.Progress
	if downloadProgress <= audioProgress || downloadProgress >= 0.40 {
		t.Fatalf("download progress = %v, audio progress = %v", downloadProgress, audioProgress)
	}
	if !server.applyProgressLine(&task, state, "开始合并音视频和字幕...") {
		t.Fatal("mux start should update progress")
	}
	if task.Progress <= downloadProgress || task.Progress >= 0.50 {
		t.Fatalf("mux progress = %v, download progress = %v", task.Progress, downloadProgress)
	}
	if server.applyProgressLine(&task, state, "下载P1完毕") {
		t.Fatal("older repeated progress should not move backwards")
	}
	if !server.applyProgressLine(&task, state, "开始解析P2: 2... (2 of 2)") {
		t.Fatal("second page start should advance after first page mux")
	}
	if task.Progress < 0.50 || task.Progress > 0.51 {
		t.Fatalf("second page start progress = %v, want about 1/2", task.Progress)
	}
}

func TestApplyProgressLineTracksExtraAudioAndFLVMuxStages(t *testing.T) {
	server := NewApiServer(NewConfig(), nil)
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	state := &taskProgressState{}

	_ = server.applyProgressLine(&task, state, "开始解析P1: 1... (1 of 1)")
	if !server.applyProgressLine(&task, state, "开始下载P1背景配音...") {
		t.Fatal("background audio start should update progress")
	}
	backgroundProgress := task.Progress
	if backgroundProgress < 0.54 || backgroundProgress > 0.56 {
		t.Fatalf("background audio progress = %v, want about 0.55", backgroundProgress)
	}
	changed := server.applyProgressLine(&task, state, "开始下载P1配音[旁白]...")
	if changed {
		t.Fatal("same-stage role audio should not report a new progress change")
	}
	if task.Progress != backgroundProgress {
		t.Fatalf("same-stage role audio should keep progress = %v, got %v", backgroundProgress, task.Progress)
	}
	if !server.applyProgressLine(&task, state, "开始合并分段...") {
		t.Fatal("FLV merge start should update progress")
	}
	if task.Progress < 0.89 || task.Progress > 0.91 {
		t.Fatalf("FLV merge progress = %v, want about 0.90", task.Progress)
	}

	roleTask := DownloadTask{Aid: "2", Url: "BV2xx", TaskCreateTime: 456}
	roleState := &taskProgressState{}
	_ = server.applyProgressLine(&roleTask, roleState, "开始解析P1: 1... (1 of 1)")
	if !server.applyProgressLine(&roleTask, roleState, "开始下载P1配音[旁白]...") {
		t.Fatal("role audio start should update progress when it is the first extra audio stage")
	}
	if roleTask.Progress < 0.54 || roleTask.Progress > 0.56 {
		t.Fatalf("role audio progress = %v, want about 0.55", roleTask.Progress)
	}
}

func TestApplyTransferSnapshotUpdatesBytesAndSpeed(t *testing.T) {
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	state := &taskTransferState{}
	start := time.Date(2026, 6, 18, 11, 30, 0, 0, time.Local)

	if !applyTransferSnapshot(&task, state, taskTransferSnapshot{bytes: 100}, start) {
		t.Fatal("first byte snapshot should update total bytes")
	}
	if task.TotalDownloadedBytes != 100 || task.DownloadSpeed != 0 {
		t.Fatalf("first snapshot task = %+v", task)
	}
	if !applyTransferSnapshot(&task, state, taskTransferSnapshot{bytes: 500}, start.Add(2*time.Second)) {
		t.Fatal("second byte snapshot should update speed")
	}
	if task.TotalDownloadedBytes != 500 || task.DownloadSpeed != 200 {
		t.Fatalf("second snapshot task = %+v, want 500 bytes and 200 B/s", task)
	}
	if applyTransferSnapshot(&task, state, taskTransferSnapshot{bytes: 400}, start.Add(3*time.Second)) {
		t.Fatal("shrinking temporary file snapshot should not lower bytes or speed")
	}
	if task.TotalDownloadedBytes != 500 || task.DownloadSpeed != 200 {
		t.Fatalf("shrinking snapshot task = %+v", task)
	}
	if !applyTransferSnapshot(&task, state, taskTransferSnapshot{bytes: 450}, start.Add(4*time.Second)) {
		t.Fatal("growth after a shrink should count only the new delta")
	}
	if task.TotalDownloadedBytes != 550 || task.DownloadSpeed != 50 {
		t.Fatalf("post-shrink growth task = %+v, want 550 bytes and 50 B/s", task)
	}
}

func TestApplyTransferSnapshotCountsGrowthWithoutElapsed(t *testing.T) {
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	state := &taskTransferState{}
	now := time.Date(2026, 6, 18, 17, 5, 0, 0, time.Local)

	if !applyTransferSnapshot(&task, state, taskTransferSnapshot{bytes: 100}, now) {
		t.Fatal("first snapshot should update total bytes")
	}
	if !applyTransferSnapshot(&task, state, taskTransferSnapshot{bytes: 250}, now) {
		t.Fatal("final same-time growth should still update total bytes")
	}
	if task.TotalDownloadedBytes != 250 {
		t.Fatalf("total downloaded = %v, want 250", task.TotalDownloadedBytes)
	}
	if task.DownloadSpeed != 0 {
		t.Fatalf("download speed = %v, want unchanged zero when no elapsed time exists", task.DownloadSpeed)
	}
}

func TestApplyServerTransferEventCompletesFastDownloadsWithoutDoubleCounting(t *testing.T) {
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	state := &taskTransferState{}
	now := time.Date(2026, 6, 18, 22, 55, 0, 0, time.Local)

	if !applyTransferSnapshot(&task, state, taskTransferSnapshot{bytes: 100}, now) {
		t.Fatal("partial file snapshot should update bytes before event source appears")
	}
	if !applyServerTransferEvent(&task, state, serverTransferEvent{Path: "demo.video.mp4", Bytes: 250}) {
		t.Fatal("completed download event should raise total bytes to the completed file size")
	}
	if task.TotalDownloadedBytes != 250 {
		t.Fatalf("total downloaded after first event = %v, want 250", task.TotalDownloadedBytes)
	}
	if applyTransferSnapshot(&task, state, taskTransferSnapshot{bytes: 500}, now.Add(time.Second)) {
		t.Fatal("file polling after transfer events should not double count completed downloads")
	}
	if !applyServerTransferEvent(&task, state, serverTransferEvent{Path: "demo.audio.m4a", Bytes: 50}) {
		t.Fatal("second completed download event should add another file")
	}
	if task.TotalDownloadedBytes != 300 {
		t.Fatalf("total downloaded after second event = %v, want 300", task.TotalDownloadedBytes)
	}
}

func TestApplyServerTransferEventTracksDeltaSpeedAndDedupesCompletedFile(t *testing.T) {
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	state := &taskTransferState{}
	start := time.Date(2026, 6, 20, 14, 55, 0, 0, time.Local)

	if !applyServerTransferEventAt(&task, state, serverTransferEvent{Path: "demo.video.mp4", Bytes: 100, Delta: true}, start) {
		t.Fatal("first delta event should update total bytes")
	}
	if task.TotalDownloadedBytes != 100 || task.DownloadSpeed != 0 {
		t.Fatalf("first delta task = %+v", task)
	}
	if !applyServerTransferEventAt(&task, state, serverTransferEvent{Path: "demo.video.mp4", Bytes: 50, Delta: true}, start.Add(time.Second)) {
		t.Fatal("second delta event should update speed")
	}
	if task.TotalDownloadedBytes != 150 || task.DownloadSpeed != 50 {
		t.Fatalf("second delta task = %+v, want 150 bytes and 50 B/s", task)
	}
	if applyServerTransferEventAt(&task, state, serverTransferEvent{Path: "demo.video.mp4", Bytes: 150}, start.Add(2*time.Second)) {
		t.Fatal("completed event for already reported bytes should not double count")
	}
	if task.TotalDownloadedBytes != 150 || task.DownloadSpeed != 50 {
		t.Fatalf("deduped completed task = %+v", task)
	}
	if !applyServerTransferEventAt(&task, state, serverTransferEvent{Path: "demo.video.mp4", Bytes: 200}, start.Add(2*time.Second)) {
		t.Fatal("completed event should add bytes missed by throttled deltas")
	}
	if task.TotalDownloadedBytes != 200 || task.DownloadSpeed != 50 {
		t.Fatalf("completed top-up task = %+v, want 200 bytes and 50 B/s", task)
	}
}

func TestApplyServerTransferEventSubtractsPreExistingBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.video.mp4")
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	state := &taskTransferState{
		workDir:       dir,
		baselineBytes: map[string]int64{path: 100},
	}
	now := time.Date(2026, 6, 20, 14, 57, 0, 0, time.Local)

	if applyServerTransferEventAt(&task, state, serverTransferEvent{Path: "demo.video.mp4", Bytes: 100}, now) {
		t.Fatal("completed event matching pre-existing baseline should not count as this task download")
	}
	if task.TotalDownloadedBytes != 0 {
		t.Fatalf("total downloaded = %v, want 0", task.TotalDownloadedBytes)
	}
	if !applyServerTransferEventAt(&task, state, serverTransferEvent{Path: "demo.video.mp4", Bytes: 150}, now.Add(time.Second)) {
		t.Fatal("completed event should count only growth beyond baseline")
	}
	if task.TotalDownloadedBytes != 50 {
		t.Fatalf("total downloaded = %v, want only 50 new bytes", task.TotalDownloadedBytes)
	}
}

func TestFinalizeFinalOutputTransferBytesUsesResourceOutputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.xml"), []byte("xml-body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out.ass"), []byte("ass-body"), 0o644); err != nil {
		t.Fatal(err)
	}

	task := DownloadTask{
		Aid:                  "1",
		Url:                  "BV1xx",
		TaskCreateTime:       123,
		TotalDownloadedBytes: 4,
		SavePaths:            []string{"out.xml", "out.ass"},
	}
	if !finalizeFinalOutputTransferBytes(&task, MyOption{DanmakuOnly: true}, dir, nil) {
		t.Fatal("resource-only final outputs should update total bytes")
	}
	if task.TotalDownloadedBytes != float64(len("xml-bodyass-body")) {
		t.Fatalf("total downloaded = %v, want final output bytes", task.TotalDownloadedBytes)
	}
}

func TestFinalizeFinalOutputTransferBytesUsesSubtitleOutputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.ai-zh.srt"), []byte("subtitle-body"), 0o644); err != nil {
		t.Fatal(err)
	}

	task := DownloadTask{
		Aid:                  "1",
		Url:                  "BV1xx",
		TaskCreateTime:       123,
		TotalDownloadedBytes: 0,
		SavePaths:            []string{"out.ai-zh.srt"},
	}
	if !finalizeFinalOutputTransferBytes(&task, MyOption{SubOnly: true}, dir, nil) {
		t.Fatal("subtitle-only final output should update total bytes")
	}
	if task.TotalDownloadedBytes != float64(len("subtitle-body")) {
		t.Fatalf("total downloaded = %v, want final subtitle bytes", task.TotalDownloadedBytes)
	}
}

func TestFinalizeFinalOutputTransferBytesUsesAudioOnlyOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.m4a"), []byte("audio-only-output"), 0o644); err != nil {
		t.Fatal(err)
	}

	task := DownloadTask{
		Aid:                  "1",
		Url:                  "BV1xx",
		TaskCreateTime:       123,
		TotalDownloadedBytes: 3,
		SavePaths:            []string{"out.m4a"},
	}
	if !finalizeFinalOutputTransferBytes(&task, MyOption{AudioOnly: true}, dir, nil) {
		t.Fatal("audio-only final output should update total bytes")
	}
	if task.TotalDownloadedBytes != float64(len("audio-only-output")) {
		t.Fatalf("total downloaded = %v, want final audio bytes", task.TotalDownloadedBytes)
	}
}

func TestFinalizeFinalOutputTransferBytesUsesVideoOnlyOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.mp4"), []byte("video-only-output"), 0o644); err != nil {
		t.Fatal(err)
	}

	task := DownloadTask{
		Aid:                  "1",
		Url:                  "BV1xx",
		TaskCreateTime:       123,
		TotalDownloadedBytes: 3,
		SavePaths:            []string{"out.mp4"},
	}
	if !finalizeFinalOutputTransferBytes(&task, MyOption{VideoOnly: true}, dir, nil) {
		t.Fatal("video-only final output should update total bytes")
	}
	if task.TotalDownloadedBytes != float64(len("video-only-output")) {
		t.Fatalf("total downloaded = %v, want final video bytes", task.TotalDownloadedBytes)
	}
}

func TestFinalizeFinalOutputTransferBytesDoesNotCountMuxOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.mp4"), []byte("mux-output"), 0o644); err != nil {
		t.Fatal(err)
	}

	task := DownloadTask{
		Aid:                  "1",
		Url:                  "BV1xx",
		TaskCreateTime:       123,
		TotalDownloadedBytes: 4,
		SavePaths:            []string{"out.mp4"},
	}
	if finalizeFinalOutputTransferBytes(&task, MyOption{}, dir, nil) {
		t.Fatal("non-resource task should not count mux output as downloaded bytes")
	}
	if task.TotalDownloadedBytes != 4 {
		t.Fatalf("total downloaded = %v, want unchanged", task.TotalDownloadedBytes)
	}
}

func TestFinalizeFinalOutputTransferBytesDoesNotCountSkipMuxAudioOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.audio.m4a"), []byte("skip-mux-audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	task := DownloadTask{
		Aid:                  "1",
		Url:                  "BV1xx",
		TaskCreateTime:       123,
		TotalDownloadedBytes: 4,
		SavePaths:            []string{"out.audio.m4a"},
	}
	if finalizeFinalOutputTransferBytes(&task, MyOption{AudioOnly: true, SkipMux: true}, dir, nil) {
		t.Fatal("skip-mux audio-only is already tracked as a download artifact")
	}
	if task.TotalDownloadedBytes != 4 {
		t.Fatalf("total downloaded = %v, want unchanged", task.TotalDownloadedBytes)
	}
}

func TestFinalizeFinalOutputTransferBytesSubtractsPreExistingOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.m4a")
	if err := os.WriteFile(path, []byte("existing-output"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := newTaskTransferState(dir, []string{"out.m4a"})
	task := DownloadTask{
		Aid:            "1",
		Url:            "BV1xx",
		TaskCreateTime: 123,
		SavePaths:      []string{"out.m4a"},
	}

	if finalizeFinalOutputTransferBytes(&task, MyOption{AudioOnly: true}, dir, state) {
		t.Fatal("pre-existing audio-only output should not be counted as this task's downloaded bytes")
	}
	if task.TotalDownloadedBytes != 0 {
		t.Fatalf("total downloaded = %v, want unchanged zero", task.TotalDownloadedBytes)
	}
	if err := os.WriteFile(path, []byte("existing-output-new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !finalizeFinalOutputTransferBytes(&task, MyOption{AudioOnly: true}, dir, state) {
		t.Fatal("new growth beyond the baseline should still be counted")
	}
	if task.TotalDownloadedBytes != float64(len("-new")) {
		t.Fatalf("total downloaded = %v, want only growth bytes", task.TotalDownloadedBytes)
	}
}

func TestTotalTaskBytesIncludesRelatedTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"demo.video.tmp":             "video",
		"00000_demo.audio.aclip":     "audio",
		"demo.clip00000.mp4":         "clip",
		"00000_demo.clip00001.vclip": "part",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := totalTaskBytes("", []string{filepath.Join(dir, "demo.mp4")})
	if got != float64(len("videoaudioclippart")) {
		t.Fatalf("totalTaskBytes = %v, want related temporary file bytes", got)
	}
}

func TestTotalTaskBytesUsesWorkDirForRelativePaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.tmp"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := totalTaskBytes(dir, []string{"demo.mp4"})
	if got != float64(len("partial")) {
		t.Fatalf("totalTaskBytes with work dir = %v, want relative temp bytes", got)
	}
}

func TestTaskTransferBytesPrefersDownloadArtifactsOverMuxOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.video.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.audio.m4a"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.mp4"), []byte("mux-output-should-not-count"), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot := taskTransferBytes(dir, []string{"demo.mp4"}, &taskTransferState{})
	if snapshot.bytes != float64(len("videoaudio")) {
		t.Fatalf("transfer bytes = %v, want only download artifacts", snapshot.bytes)
	}
	if !snapshot.hasDownloadFilePattern {
		t.Fatal("download artifacts should be reported")
	}
}

func TestTaskTransferBytesIgnoresMuxOutputAfterDownloadArtifactsWereSeen(t *testing.T) {
	dir := t.TempDir()
	state := &taskTransferState{}
	if err := os.WriteFile(filepath.Join(dir, "demo.video.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := taskTransferBytes(dir, []string{"demo.mp4"}, state)
	task := DownloadTask{Aid: "1", Url: "BV1xx", TaskCreateTime: 123}
	if !applyTransferSnapshot(&task, state, first, time.Date(2026, 6, 18, 13, 50, 0, 0, time.Local)) {
		t.Fatal("first artifact snapshot should update task")
	}
	if err := os.Remove(filepath.Join(dir, "demo.video.mp4")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.mp4"), []byte("mux-output"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := taskTransferBytes(dir, []string{"demo.mp4"}, state)
	if second.bytes != 0 {
		t.Fatalf("transfer bytes after cleanup = %v, want 0 to avoid counting mux output", second.bytes)
	}
	if applyTransferSnapshot(&task, state, second, time.Date(2026, 6, 18, 13, 50, 1, 0, time.Local)) {
		t.Fatal("mux output after cleanup should not update transfer totals")
	}
	if task.TotalDownloadedBytes != float64(len("video")) {
		t.Fatalf("total downloaded = %v, want original artifact bytes", task.TotalDownloadedBytes)
	}
}

func TestTaskTransferBytesFallsBackToPredictedOutputWhenArtifactsWereMissed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.mp4"), []byte("final"), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot := taskTransferBytes(dir, []string{"demo.mp4"}, &taskTransferState{})
	if snapshot.bytes != float64(len("final")) {
		t.Fatalf("fallback transfer bytes = %v, want final output bytes", snapshot.bytes)
	}
	if !snapshot.hasPredictedOutputFiles {
		t.Fatal("fallback should mark predicted output files")
	}
}

func TestTaskTransferBytesSubtractsPreExistingPredictedOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.mp4")
	if err := os.WriteFile(path, []byte("existing-output"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := newTaskTransferState(dir, []string{"demo.mp4"})

	snapshot := taskTransferBytes(dir, []string{"demo.mp4"}, state)
	if snapshot.bytes != 0 {
		t.Fatalf("pre-existing output bytes = %v, want 0", snapshot.bytes)
	}
	if !snapshot.hasPredictedOutputFiles {
		t.Fatal("existing predicted output should still be detected as an output file")
	}
	if err := os.WriteFile(path, []byte("existing-output-new"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot = taskTransferBytes(dir, []string{"demo.mp4"}, state)
	if snapshot.bytes != float64(len("-new")) {
		t.Fatalf("growth bytes = %v, want only bytes written after task start", snapshot.bytes)
	}
}

func TestTaskTransferBytesUsesOriginalFLVClipBaseForMergedSavePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.clip00000.mp4"), []byte("clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.flv-merged.mp4"), []byte("local-merged-output"), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot := taskTransferBytes(dir, []string{"demo.flv-merged.mp4"}, &taskTransferState{})
	if snapshot.bytes != float64(len("clip")) {
		t.Fatalf("transfer bytes = %v, want original FLV clip bytes", snapshot.bytes)
	}
	if !snapshot.hasDownloadFilePattern {
		t.Fatal("FLV clip artifact should be reported as download file pattern")
	}
}

func TestAddTaskDuplicateRunningPostsCallback(t *testing.T) {
	bodyCh := make(chan string, 1)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		bodyCh <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer callback.Close()

	server := NewApiServer(NewConfig(), NewHTTPClient(NewConfig()))
	server.running = []DownloadTask{{Aid: "80433022", Url: "BV1GJ411x7h7", TaskCreateTime: 123}}
	server.addTask(ServeRequestOptions{
		MyOption:        MyOption{Url: "BV1GJ411x7h7"},
		CallBackWebHook: callback.URL,
	})

	select {
	case body := <-bodyCh:
		if !strings.Contains(body, `"Aid":"80433022"`) {
			t.Fatalf("callback body = %s", body)
		}
	default:
		t.Fatal("duplicate running task should post callback")
	}
	if len(server.running) != 1 || len(server.done) != 0 {
		t.Fatalf("duplicate task should stay running only, running=%d done=%d", len(server.running), len(server.done))
	}
}

func TestNormalizeListenAddr(t *testing.T) {
	cases := map[string]string{
		"":                        "0.0.0.0:23333",
		"http://0.0.0.0:5000":     "0.0.0.0:5000",
		"http://127.0.0.1:23333/": "127.0.0.1:23333",
	}
	for input, want := range cases {
		got, err := NormalizeListenAddr(input)
		if err != nil {
			t.Fatalf("NormalizeListenAddr(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeListenAddr(%q) = %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"https://127.0.0.1:23333", "127.0.0.1:24444", ":25555"} {
		_, err := NormalizeListenAddr(input)
		if err == nil {
			t.Fatalf("expected %q listen URL to be rejected", input)
		}
		if !strings.Contains(err.Error(), "如果您需要https，请额外配置反向代理") {
			t.Fatalf("listen error should include reverse proxy hint: %v", err)
		}
	}
}

func TestServerSavePathPatternKeepsMultiDefaultAfterSelection(t *testing.T) {
	opt := &MyOption{}
	vInfo := &VInfo{Title: "合集"}
	pattern := serverSavePathPattern(opt, vInfo, 8)
	got := FormatSavePath(pattern, vInfo.Title, nil, nil, Page{Index: 3, Title: "第三集", Aid: "1"}, 1, "WEB", 0)
	if got != "合集/[P3]第三集.mp4" {
		t.Fatalf("server predicted path = %q", got)
	}
}

func TestServerSavePathPatternUsesMultiDefaultForUnfinishedBangumi(t *testing.T) {
	opt := &MyOption{}
	vInfo := &VInfo{Title: "番剧", IsBangumi: true, IsBangumiEnd: false}
	pattern := serverSavePathPattern(opt, vInfo, 1)
	got := FormatSavePath(pattern, vInfo.Title, nil, nil, Page{Index: 1, Title: "第一集", Aid: "1"}, 1, "WEB", 0)
	if got != "番剧/[P1]第一集.mp4" {
		t.Fatalf("server bangumi predicted path = %q", got)
	}
}

func TestServerSavePathPredictionUsesDownloadTitleLikeOriginal(t *testing.T) {
	opt := &MyOption{}
	vInfo := &VInfo{Title: ".合集."}
	pattern := serverSavePathPattern(opt, vInfo, 1)
	got := FormatSavePath(pattern, DownloadTitle(vInfo.Title), nil, nil, Page{Index: 1, Title: "第一集", Aid: "1"}, 1, "WEB", 0)
	if got != "_.合集._fix.mp4" {
		t.Fatalf("server dot-title predicted path = %q", got)
	}
}

func TestServerSavePathPatternKeepsWhitespaceMultiFilePatternLikeOriginal(t *testing.T) {
	pattern := serverSavePathPattern(&MyOption{FilePattern: "<videoTitle>", MultiFilePattern: "   "}, &VInfo{Title: "标题"}, 2)
	if pattern != "   " {
		t.Fatalf("serverSavePathPattern = %q, want explicit whitespace multi-file pattern", pattern)
	}
}

func TestApplyServerOnlyModeToTracks(t *testing.T) {
	tracks := &ParsedTracks{
		VideoTracks: []Video{{Dfn: "1080P"}},
		AudioTracks: []Audio{{Codecs: "M4A"}},
	}
	applyServerOnlyModeToTracks(&MyOption{AudioOnly: true}, tracks)
	if len(tracks.VideoTracks) != 0 || len(tracks.AudioTracks) != 1 {
		t.Fatalf("audio-only server tracks = %+v", tracks)
	}

	tracks = &ParsedTracks{
		VideoTracks:      []Video{{Dfn: "1080P"}},
		AudioTracks:      []Audio{{Codecs: "M4A"}},
		BackgroundAudios: []Audio{{Codecs: "EAC3"}},
		RoleAudioLists:   []AudioMaterialInfo{{Title: "角色音频", Audio: []Audio{{Codecs: "M4A"}}}},
	}
	applyServerOnlyModeToTracks(&MyOption{VideoOnly: true}, tracks)
	if len(tracks.VideoTracks) != 1 || len(tracks.AudioTracks) != 0 || len(tracks.BackgroundAudios) != 0 || len(tracks.RoleAudioLists) != 0 {
		t.Fatalf("video-only server tracks = %+v", tracks)
	}
}

func TestServerCoverOnlyPredictionUsesSharedCoverExt(t *testing.T) {
	opt := &MyOption{CoverOnly: true}
	vInfo := &VInfo{Title: "标题"}
	path := FormatSavePath(serverSavePathPattern(opt, vInfo, 1), vInfo.Title, nil, nil, Page{Index: 1, Aid: "1"}, 1, "WEB", 0)
	got := strings.TrimSuffix(path, ".mp4") + CoverExt("https://i0.hdslb.com/a.jpg@672w_378h_1c.webp?x=1")
	if got != "标题.webp" {
		t.Fatalf("server cover predicted path = %q", got)
	}
}

func TestServerPredictedSubtitleOutputPathsMatchCLI(t *testing.T) {
	subs := []Subtitle{
		{Lan: "ai-zh", URL: "https://example.test/ai.json"},
		{Lan: "en-US", URL: "https://example.test/en.json"},
		{Lan: "zh-HK", URL: "https://example.test/zh.ass", Path: "1/2.zh-HK.ass"},
	}
	got := serverPredictedSubtitleOutputPaths("标题.mp4", subs, false)
	want := []string{"标题.ai-zh.srt", "标题.en-US.srt", "标题.zh-HK.ass"}
	if !slices.Equal(got, want) {
		t.Fatalf("server predicted subtitle paths = %v, want %v", got, want)
	}

	got = serverPredictedSubtitleOutputPaths("标题.mp4", subs, true)
	want = []string{"标题.en-US.srt", "标题.zh-HK.ass"}
	if !slices.Equal(got, want) {
		t.Fatalf("server predicted subtitle paths with SkipAi = %v, want %v", got, want)
	}
}

func TestPredictSavePathsSkipsOnlyShowInfo(t *testing.T) {
	server := NewApiServer(NewConfig(), nil)
	got := server.predictSavePaths(
		t.Context(),
		&MyOption{OnlyShowInfo: true},
		"123",
		&VInfo{Title: "标题", PagesInfo: []Page{{Index: 1, Aid: "123", Cid: "456"}}},
		"WEB",
	)
	if len(got) != 0 {
		t.Fatalf("OnlyShowInfo service task should not predict output paths: %v", got)
	}

	got = serverPredictedOutputPaths(
		&MyOption{OnlyShowInfo: true},
		&VInfo{Title: "标题"},
		Page{Index: 1, Aid: "123"},
		&ParsedTracks{VideoTracks: []Video{{Dfn: "360P"}}, AudioTracks: []Audio{{Dfn: "M4A"}}},
		"标题.mp4",
	)
	if len(got) != 0 {
		t.Fatalf("OnlyShowInfo output prediction should stay empty: %v", got)
	}
}

func TestPredictSavePathsSkipsArchivedPages(t *testing.T) {
	archiveFile := archivePath()
	_ = os.Remove(archiveFile)
	t.Cleanup(func() { _ = os.Remove(archiveFile) })
	if err := SaveAidToArchive("123"); err != nil {
		t.Fatal(err)
	}

	server := NewApiServer(NewConfig(), nil)
	got := server.predictSavePaths(
		t.Context(),
		&MyOption{SaveArchivesToFile: true, CoverOnly: true},
		"123",
		&VInfo{Title: "标题", Pic: "https://i0.hdslb.com/a.jpg", PagesInfo: []Page{{Index: 1, Aid: "123", Cid: "456"}}},
		"WEB",
	)
	if len(got) != 0 {
		t.Fatalf("archived service task should not predict skipped output paths: %v", got)
	}
}

func TestServerPredictedOutputPathsUsesFLVMergedForSkipMuxClips(t *testing.T) {
	got := serverPredictedOutputPaths(
		&MyOption{SkipMux: true},
		&VInfo{Title: "标题"},
		Page{Index: 1, Aid: "1"},
		&ParsedTracks{Clips: []string{"https://example.com/clip.flv"}},
		"标题.mp4",
	)
	if len(got) != 1 || got[0] != "标题.flv-merged.mp4" {
		t.Fatalf("server predicted FLV skip-mux paths = %v", got)
	}
}

func TestServerPredictedOutputPathsUsesDASHPartsForSkipMux(t *testing.T) {
	tracks := &ParsedTracks{
		VideoTracks: []Video{{Dfn: "360P"}},
		AudioTracks: []Audio{{Dfn: "M4A"}},
	}
	got := serverPredictedOutputPaths(&MyOption{SkipMux: true}, nil, Page{}, tracks, "标题.mp4")
	want := []string{"标题.video.mp4", "标题.audio.m4a"}
	if !slices.Equal(got, want) {
		t.Fatalf("server predicted DASH skip-mux paths = %v, want %v", got, want)
	}

	got = serverPredictedOutputPaths(&MyOption{SkipMux: true, AudioOnly: true}, nil, Page{}, tracks, "标题.mp4")
	want = []string{"标题.audio.m4a"}
	if !slices.Equal(got, want) {
		t.Fatalf("server predicted DASH audio-only skip-mux paths = %v, want %v", got, want)
	}

	got = serverPredictedOutputPaths(&MyOption{SkipMux: true, VideoOnly: true}, nil, Page{}, tracks, "标题.mp4")
	want = []string{"标题.video.mp4"}
	if !slices.Equal(got, want) {
		t.Fatalf("server predicted DASH video-only skip-mux paths = %v, want %v", got, want)
	}
}

func TestServerRenamePredictionReservesSamePatternAcrossPages(t *testing.T) {
	dir := t.TempDir()
	opt := &MyOption{CoverOnly: true, FileExistsAction: FileExistsActionRename, WorkDir: dir}
	vInfo := &VInfo{Title: "同名", Pic: "https://i0.hdslb.com/cover.jpg"}
	reserved := make(map[string]struct{})
	var got []string
	for pageIndex := 1; pageIndex <= 3; pageIndex++ {
		path := FormatSavePath("<videoTitle>", vInfo.Title, nil, nil, Page{Index: pageIndex, Aid: "1"}, 3, "WEB", 0)
		resolved, err := resolveFileExistsPathInDir(path, opt.FileExistsAction, opt.WorkDir, reserved)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, serverPredictedOutputPaths(opt, vInfo, Page{Index: pageIndex}, nil, resolved)...)
	}
	want := []string{"同名.jpg", "同名 (1).jpg", "同名 (2).jpg"}
	if !slices.Equal(got, want) {
		t.Fatalf("reserved multi-page predictions = %v, want %v", got, want)
	}
}

func TestSelectedTrackIndexesForServePrediction(t *testing.T) {
	tracks := &ParsedTracks{
		VideoTracks: []Video{{Dfn: "360P"}, {Dfn: "480P"}},
		AudioTracks: []Audio{{ID: "0"}, {ID: "1"}, {ID: "2"}},
	}
	videoIndex, audioIndex, err := SelectedTrackIndexes(&MyOption{VideoIndex: "1", AudioIndex: "2"}, tracks)
	if err != nil || videoIndex != 1 || audioIndex != 2 {
		t.Fatalf("indexes = video %d audio %d err %v, want 1 2 nil", videoIndex, audioIndex, err)
	}
	videoIndex, audioIndex, err = SelectedTrackIndexes(&MyOption{AudioOnly: true, VideoIndex: "9", AudioIndex: "1"}, tracks)
	if err != nil || videoIndex != 0 || audioIndex != 1 {
		t.Fatalf("audio-only indexes = video %d audio %d err %v, want 0 1 nil", videoIndex, audioIndex, err)
	}
	videoIndex, audioIndex, err = SelectedTrackIndexes(&MyOption{VideoOnly: true, VideoIndex: "1", AudioIndex: "9"}, tracks)
	if err != nil || videoIndex != 1 || audioIndex != 0 {
		t.Fatalf("video-only indexes = video %d audio %d err %v, want 1 0 nil", videoIndex, audioIndex, err)
	}
	if _, _, err := SelectedTrackIndexes(&MyOption{VideoIndex: "9"}, tracks); err == nil {
		t.Fatal("out-of-range explicit video index should stop SavePaths prediction")
	}
	if _, _, err := SelectedTrackIndexes(&MyOption{AudioIndex: "abc"}, tracks); err == nil {
		t.Fatal("invalid explicit audio index should stop SavePaths prediction")
	}
}

func TestIsBenignPipeCloseError(t *testing.T) {
	if !isBenignPipeCloseError(&os.PathError{Op: "read", Path: "|0", Err: os.ErrClosed}) {
		t.Fatal("closed stdout/stderr pipe should be treated as a benign child-process shutdown error")
	}
	if isBenignPipeCloseError(os.ErrNotExist) {
		t.Fatal("unrelated errors should not be treated as benign pipe closes")
	}
}

func TestTotalExistingBytesWithWorkDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BBDOWN_GO_TOTAL_DIR", dir)
	path := filepath.Join(dir, "out.ass")
	if err := os.WriteFile(path, []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}

	total := totalExistingBytes("$BBDOWN_GO_TOTAL_DIR", []string{"out.ass"})
	if total != 5 {
		t.Fatalf("totalExistingBytes = %v, want 5", total)
	}
}
