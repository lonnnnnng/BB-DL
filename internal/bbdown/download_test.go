package bbdown

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGetSelectedPagesRangeAndLatestAliases(t *testing.T) {
	vInfo := &VInfo{PagesInfo: []Page{{Index: 1}, {Index: 2}, {Index: 3}, {Index: 4}}}
	got := GetSelectedPages(&MyOption{SelectPage: "2-LATEST"}, vInfo, "")
	if !slices.Equal(got, []string{"2", "3", "4"}) {
		t.Fatalf("selected pages = %v", got)
	}
}

func TestGetSelectedPagesRangeTrimsEndpointsLikeOriginal(t *testing.T) {
	vInfo := &VInfo{PagesInfo: []Page{{Index: 1}, {Index: 2}, {Index: 3}}}
	got := GetSelectedPages(&MyOption{SelectPage: "1 - 3"}, vInfo, "")
	if !slices.Equal(got, []string{"1", "2", "3"}) {
		t.Fatalf("selected pages = %v", got)
	}
}

func TestGetSelectedPagesInvalidRangeFallsBackToAll(t *testing.T) {
	vInfo := &VInfo{PagesInfo: []Page{{Index: 1}, {Index: 2}}}
	out := captureStdout(t, func() {
		got := GetSelectedPages(&MyOption{SelectPage: "x-2"}, vInfo, "")
		if got != nil {
			t.Fatalf("selected pages = %v, want nil", got)
		}
	})
	if !strings.Contains(out, "解析分P参数时失败了~") {
		t.Fatalf("output = %q", out)
	}
}

func TestGetSelectedPagesKeepsCommaListLikeOriginal(t *testing.T) {
	vInfo := &VInfo{PagesInfo: []Page{{Index: 1}, {Index: 2}, {Index: 3}}}
	got := GetSelectedPages(&MyOption{SelectPage: "1,, 3"}, vInfo, "")
	if !slices.Equal(got, []string{"1", "", " 3"}) {
		t.Fatalf("selected pages = %v", got)
	}
}

func TestPrepareDownloadURLAllowPcdnDoesNotReplacePCDN(t *testing.T) {
	rawURL := "https://pcdn.bilivideo.com:4483/upgcxcode/path/video.m4s"
	got := PrepareDownloadURL(rawURL, &MyOption{AllowPcdn: true}, &Config{})
	if got != rawURL {
		t.Fatalf("PrepareDownloadURL = %q, want %q", got, rawURL)
	}
}

func TestPrepareDownloadURLAreaAkamaiReplacementIgnoresAllowPcdn(t *testing.T) {
	rawURL := "https://upos-hz-mirrorakam.akamaized.net/upgcxcode/path/video.m4s"
	got := PrepareDownloadURL(rawURL, &MyOption{AllowPcdn: true}, &Config{Area: "th"})
	want := "https://" + BackupHost + "/upgcxcode/path/video.m4s"
	if got != want {
		t.Fatalf("PrepareDownloadURL = %q, want %q", got, want)
	}
}

func TestPrepareDownloadURLForceHTTPAppliesToNonMediaURL(t *testing.T) {
	rawURL := "https://comment.bilibili.com/123.xml"
	got := PrepareDownloadURL(rawURL, &MyOption{ForceHttp: true}, &Config{})
	if got != "http://comment.bilibili.com/123.xml" {
		t.Fatalf("PrepareDownloadURL = %q", got)
	}
}

func TestPrepareDownloadURLForceHTTPSkipsMcdnPortURL(t *testing.T) {
	rawURL := "https://upos.mcdn.bilivideo.cn:4483/upgcxcode/path/video.m4s"
	got := PrepareDownloadURL(rawURL, &MyOption{ForceHttp: true, AllowPcdn: true}, &Config{})
	if got != rawURL {
		t.Fatalf("PrepareDownloadURL = %q, want %q", got, rawURL)
	}
}

func TestPrepareDownloadURLMultiThreadCmccDisablesForceHTTP(t *testing.T) {
	rawURL := "https://upos-cmcc-bilivideo.com/upgcxcode/path/video.m4s"
	got := PrepareDownloadURL(rawURL, &MyOption{ForceHttp: true, MultiThread: true, AllowPcdn: true}, &Config{})
	if got != rawURL {
		t.Fatalf("PrepareDownloadURL = %q, want %q", got, rawURL)
	}

	got = PrepareDownloadURL(rawURL, &MyOption{ForceHttp: true, MultiThread: false, AllowPcdn: true}, &Config{})
	if got != "http://upos-cmcc-bilivideo.com/upgcxcode/path/video.m4s" {
		t.Fatalf("PrepareDownloadURL without multithread = %q", got)
	}
}

func TestHandleSelectedTrackHostsLogsDefaultForceReplacementLikeOriginal(t *testing.T) {
	opt := DefaultMyOption()
	video := &Video{BaseURL: "https://upos-hz-mirrorakam.bilivideo.com/upgcxcode/path/video.m4s"}
	audio := &Audio{BaseURL: "https://upos-hz-mirrorakam.bilivideo.com/upgcxcode/path/audio.m4a"}

	out := captureStdout(t, func() {
		HandleSelectedTrackHosts(&opt, &Config{}, video, audio)
	})
	wantVideoURL := "https://" + BackupHost + "/upgcxcode/path/video.m4s"
	if video.BaseURL != wantVideoURL {
		t.Fatalf("video URL = %q, want %q", video.BaseURL, wantVideoURL)
	}
	wantAudioURL := "https://" + BackupHost + "/upgcxcode/path/audio.m4a"
	if audio.BaseURL != wantAudioURL {
		t.Fatalf("audio URL = %q, want %q", audio.BaseURL, wantAudioURL)
	}
	for _, want := range []string{
		"尝试将视频流强制替换为" + BackupHost + "……",
		"尝试将音频流强制替换为" + BackupHost + "……",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestDownloadResourceStreamsNonMediaWithoutContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "flush unsupported", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		_, _ = w.Write([]byte("<i><d p=\"0\">hello</d></i>"))
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	outPath := filepath.Join(t.TempDir(), "danmaku.xml")
	if err := DownloadResource(context.Background(), httpc, cfg, server.URL+"/123.xml", outPath, &MyOption{MultiThread: true}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `<i><d p="0">hello</d></i>` {
		t.Fatalf("body = %q", body)
	}
}

func TestDownloadResourceEmptyURLSkipsAria2cLikeOriginal(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "missing-dir", "cover.jpg")
	err := DownloadResource(context.Background(), NewHTTPClient(NewConfig()), NewConfig(), "", outPath, &MyOption{
		UseAria2c:  true,
		Aria2cPath: filepath.Join(t.TempDir(), "missing-aria2c"),
	})
	if err != nil {
		t.Fatalf("empty URL should be ignored before aria2c like original DownloadFileAsync: %v", err)
	}
	if _, statErr := os.Stat(outPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("empty URL should not create output file, stat err = %v", statErr)
	}
}

func TestApplyDownloadHeadersSkipsRefererForAndroidPlatform(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.test/video.m4s?platform=android", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyDownloadHeaders(req, &Config{Cookie: "SESSDATA=test"})
	if got := req.Header.Get("Referer"); got != "" {
		t.Fatalf("Referer = %q, want empty", got)
	}
	if got := req.Header.Get("User-Agent"); got != "Mozilla/5.0" {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := req.Header.Get("Cookie"); got != "SESSDATA=test" {
		t.Fatalf("Cookie = %q", got)
	}
}

func TestApplyDownloadHeadersKeepsRefererForWebURL(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.test/video.m4s", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyDownloadHeaders(req, &Config{})
	if got := req.Header.Get("Referer"); got != "https://www.bilibili.com" {
		t.Fatalf("Referer = %q", got)
	}
}

func TestBuildAria2cArgsSkipsRefererForTVPlatform(t *testing.T) {
	args := buildAria2cArgs("https://example.test/video.m4s?platform=android_tv_yst", "/tmp/video.m4s", "", "")
	if slices.Contains(args, "--header=Referer: https://www.bilibili.com") {
		t.Fatalf("aria2c args should not include Referer: %v", args)
	}
	if !slices.Contains(args, "--header=User-Agent: Mozilla/5.0") || !slices.Contains(args, "--header=Cookie: ") {
		t.Fatalf("aria2c args missing original headers: %v", args)
	}
}

func TestBuildAria2cArgsHeaderOrderMatchesOriginal(t *testing.T) {
	args := buildAria2cArgs("https://example.test/video.m4s", "/tmp/video.m4s", "", "SESSDATA=test")
	refererIndex := slices.Index(args, "--header=Referer: https://www.bilibili.com")
	userAgentIndex := slices.Index(args, "--header=User-Agent: Mozilla/5.0")
	cookieIndex := slices.Index(args, "--header=Cookie: SESSDATA=test")
	if refererIndex < 0 || userAgentIndex < 0 || cookieIndex < 0 {
		t.Fatalf("aria2c args missing original headers: %v", args)
	}
	if !(refererIndex < userAgentIndex && userAgentIndex < cookieIndex) {
		t.Fatalf("aria2c header order = %v, want Referer before User-Agent before Cookie", args)
	}
}

func TestBuildAria2cArgsKeepsQuotedExtraArgs(t *testing.T) {
	args := buildAria2cArgs("https://example.test/video.m4s", "/tmp/video.m4s", `--all-proxy="http://127.0.0.1:7890" --header="X-Test: a b" --conf-path=C:\aria2\aria2.conf`, "")
	for _, want := range []string{`--all-proxy=http://127.0.0.1:7890`, `--header=X-Test: a b`} {
		if !slices.Contains(args, want) {
			t.Fatalf("args %v missing quoted extra arg %q", args, want)
		}
	}
	if !slices.Contains(args, `--conf-path=C:\aria2\aria2.conf`) {
		t.Fatalf("windows-style backslashes should be preserved: %v", args)
	}
	if slices.Contains(args, `--header="X-Test:`) || slices.Contains(args, "a") || slices.Contains(args, `b"`) {
		t.Fatalf("quoted extra arg should not be split by spaces: %v", args)
	}
}

func TestDownloadByAria2cIgnoresExitCodeLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	aria2cPath := filepath.Join(dir, "aria2c")
	script := "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do\n  case \"$1\" in\n    -d) shift; dir=\"$1\" ;;\n    -o) shift; out=\"$1\" ;;\n  esac\n  shift\ndone\nprintf ok > \"$dir/$out\"\nexit 23\n"
	if err := os.WriteFile(aria2cPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "video.m4s")
	if err := DownloadByAria2c("https://example.test/video.m4s", outPath, aria2cPath, "", ""); err != nil {
		t.Fatalf("aria2c non-zero exit with complete output should follow original success path: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
}

func TestDownloadByAria2cFailsWhenOutputMissing(t *testing.T) {
	dir := t.TempDir()
	aria2cPath := filepath.Join(dir, "aria2c")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(aria2cPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err := DownloadByAria2c("https://example.test/video.m4s", filepath.Join(dir, "video.m4s"), aria2cPath, "", "")
	if err == nil || !strings.Contains(err.Error(), "aria2下载可能存在错误") {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadByAria2cFailsWhenAria2ControlFileRemains(t *testing.T) {
	dir := t.TempDir()
	aria2cPath := filepath.Join(dir, "aria2c")
	argsPath := filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\ndir=''\nout=''\nwhile [ \"$#\" -gt 0 ]; do\n  case \"$1\" in\n    -d) shift; dir=\"$1\" ;;\n    -o) shift; out=\"$1\" ;;\n  esac\n  shift\ndone\ntouch \"$dir/$out\"\ntouch \"$dir/$out.aria2\"\nexit 0\n"
	if err := os.WriteFile(aria2cPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err := DownloadByAria2c("https://example.test/video.m4s", filepath.Join(dir, "video.m4s"), aria2cPath, "", "")
	if err == nil || !strings.Contains(err.Error(), "aria2下载可能存在错误") {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadResourceMultiThreadFailureDoesNotFallbackToSingleDownload(t *testing.T) {
	var gotRangeRequest atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", "20")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotRangeRequest.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	httpc := &HTTPClient{client: &http.Client{Transport: rewriteHostTransport{target: target, base: server.Client().Transport}}, config: cfg}
	err = DownloadResource(context.Background(), httpc, cfg, "http://upos-test.bilivideo.com/video.m4s", filepath.Join(t.TempDir(), "video.m4s"), &MyOption{MultiThread: true})
	if err == nil || !strings.Contains(err.Error(), "服务器可能并不支持多线程下载") {
		t.Fatalf("expected multi-thread range failure to be returned, got %v", err)
	}
	if !gotRangeRequest.Load() {
		t.Fatal("media DownloadResource should attempt range requests through multi-thread downloader")
	}
}

func TestDownloadResourceCMCCDisablesForceHTTPForLaterTracksLikeOriginal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=0-" {
			t.Fatalf("Range = %q", got)
		}
		w.Header().Set("Content-Length", "2")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	httpc := &HTTPClient{client: &http.Client{Transport: rewriteHostTransport{target: target, base: server.Client().Transport}}, config: cfg}
	opt := &MyOption{ForceHttp: true, MultiThread: true, AllowPcdn: true}
	if err := DownloadResource(context.Background(), httpc, cfg, "https://upos-cmcc-bilivideo.com/upgcxcode/path/video.m4s", filepath.Join(t.TempDir(), "video.m4s"), opt); err != nil {
		t.Fatal(err)
	}
	if opt.ForceHttp {
		t.Fatal("cmcc fallback should disable ForceHttp on the shared option like original DownloadConfig")
	}
	next := PrepareDownloadURL("https://upos-bilivideo.com/upgcxcode/path/audio.m4s", opt, cfg)
	if next != "https://upos-bilivideo.com/upgcxcode/path/audio.m4s" {
		t.Fatalf("next URL = %q, want https preserved after cmcc fallback", next)
	}
}

func TestDownloadResourceCombinesMultiThreadMediaParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusPartialContent)
		switch got := r.Header.Get("Range"); got {
		case "bytes=0-":
			_, _ = w.Write([]byte("helloworld"))
		default:
			t.Fatalf("unexpected Range = %q", got)
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	httpc := &HTTPClient{client: &http.Client{Transport: rewriteHostTransport{target: target, base: server.Client().Transport}}, config: cfg}
	dir := t.TempDir()
	outPath := filepath.Join(dir, "video.mp4")
	unrelatedPart := filepath.Join(dir, "00000_other.vclip")
	if err := os.WriteFile(unrelatedPart, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DownloadResource(context.Background(), httpc, cfg, "http://upos-test.bilivideo.com/video.m4s", outPath, &MyOption{MultiThread: true}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "helloworld" {
		t.Fatalf("body = %q", body)
	}
	if files := multiThreadCleanupFiles(outPath); len(files) != 0 {
		t.Fatalf("part files should be cleaned after media download, got %v", files)
	}
	if body, err := os.ReadFile(unrelatedPart); err != nil || string(body) != "keep" {
		t.Fatalf("unrelated part should remain, body = %q, err = %v", body, err)
	}
}

func TestDownloadMediaMultiThreadCombinesAndCleansParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusPartialContent)
		switch got := r.Header.Get("Range"); got {
		case "bytes=0-5":
			_, _ = w.Write([]byte("hellow"))
		case "bytes=6-":
			_, _ = w.Write([]byte("orld"))
		default:
			t.Fatalf("unexpected Range = %q", got)
		}
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	dir := t.TempDir()
	outPath := filepath.Join(dir, "video.mp4")
	unrelatedPart := filepath.Join(dir, "00000_other.vclip")
	if err := os.WriteFile(unrelatedPart, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DownloadMediaMultiThread(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, 5, 1); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "helloworld" {
		t.Fatalf("body = %q", body)
	}
	if files := multiThreadCleanupFiles(outPath); len(files) != 0 {
		t.Fatalf("part files should be cleaned after merge, got %v", files)
	}
	if body, err := os.ReadFile(unrelatedPart); err != nil || string(body) != "keep" {
		t.Fatalf("unrelated part should remain, body = %q, err = %v", body, err)
	}
}

type rewriteHostTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	return t.base.RoundTrip(clone)
}

func TestDownloadFileMultiThreadRangeNotSupportedMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", "20")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("range ignored"))
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	err := DownloadFileMultiThread(context.Background(), httpc, cfg, server.URL+"/video.m4s", filepath.Join(t.TempDir(), "video.m4s"), 5, 1)
	if err == nil || !strings.Contains(err.Error(), "服务器可能并不支持多线程下载, 请使用 --multi-thread false 关闭多线程") {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadFileMultiThreadUsesRangeForSmallFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusOK)
			return
		}
		if got := r.Header.Get("Range"); got != "bytes=0-" {
			t.Fatalf("Range = %q", got)
		}
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	outPath := filepath.Join(t.TempDir(), "video.m4s")
	if err := DownloadFileMultiThread(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, 20*1024*1024, 1); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(multiThreadPartPath(outPath, 0))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q", body)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("multi-thread utility should leave merge to caller, stat err = %v", err)
	}
}

func TestDownloadFileMultiThreadUsesOpenRangeForLastPart(t *testing.T) {
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			return
		}
		ranges = append(ranges, rangeHeader)
		w.WriteHeader(http.StatusPartialContent)
		switch rangeHeader {
		case "bytes=0-5":
			_, _ = w.Write([]byte("hellow"))
		case "bytes=6-":
			_, _ = w.Write([]byte("orld"))
		default:
			t.Fatalf("unexpected Range = %q", rangeHeader)
		}
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	outPath := filepath.Join(t.TempDir(), "video.m4s")
	if err := DownloadFileMultiThread(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, 5, 1); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ranges, []string{"bytes=0-5", "bytes=6-"}) {
		t.Fatalf("ranges = %v", ranges)
	}
	if body := readJoinedMultiThreadParts(t, outPath); body != "helloworld" {
		t.Fatalf("body = %q", body)
	}
}

func TestDownloadFileMultiThreadDoesNotRequestEmptyTailAfterInclusiveRanges(t *testing.T) {
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", "11")
			w.WriteHeader(http.StatusOK)
			return
		}
		ranges = append(ranges, rangeHeader)
		w.WriteHeader(http.StatusPartialContent)
		switch rangeHeader {
		case "bytes=0-5":
			_, _ = w.Write([]byte("hello "))
		case "bytes=6-":
			_, _ = w.Write([]byte("world"))
		default:
			t.Fatalf("unexpected Range = %q", rangeHeader)
		}
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	outPath := filepath.Join(t.TempDir(), "video.m4s")
	if err := DownloadFileMultiThread(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, 5, 1); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ranges, []string{"bytes=0-5", "bytes=6-"}) {
		t.Fatalf("ranges = %v", ranges)
	}
	if body := readJoinedMultiThreadParts(t, outPath); body != "hello world" {
		t.Fatalf("body = %q", body)
	}
}

func TestDownloadFileMultiThreadResumesExistingPartFile(t *testing.T) {
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			return
		}
		ranges = append(ranges, rangeHeader)
		w.WriteHeader(http.StatusPartialContent)
		switch rangeHeader {
		case "bytes=2-5":
			if got := r.Header.Get("If-Range"); got == "" {
				t.Fatal("resume request should carry If-Range like original")
			}
			_, _ = w.Write([]byte("llow"))
		case "bytes=6-":
			_, _ = w.Write([]byte("orld"))
		default:
			t.Fatalf("unexpected Range = %q", rangeHeader)
		}
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	dir := t.TempDir()
	outPath := filepath.Join(dir, "video.m4s")
	if err := os.WriteFile(multiThreadPartPath(outPath, 0), []byte("he"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DownloadFileMultiThread(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, 5, 1); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ranges, []string{"bytes=2-5", "bytes=6-"}) {
		t.Fatalf("ranges = %v", ranges)
	}
	if body := readJoinedMultiThreadParts(t, outPath); body != "helloworld" {
		t.Fatalf("body = %q", body)
	}
}

func TestMultiThreadPartPathMatchesOriginalClipNames(t *testing.T) {
	dir := t.TempDir()
	if got := multiThreadPartPath(filepath.Join(dir, "foo.video.mp4"), 3); got != filepath.Join(dir, "00003_foo.video.vclip") {
		t.Fatalf("video part path = %q", got)
	}
	if got := multiThreadPartPath(filepath.Join(dir, "foo.audio.m4a"), 12); got != filepath.Join(dir, "00012_foo.audio.aclip") {
		t.Fatalf("audio part path = %q", got)
	}
}

func TestMultiThreadPartFilesMatchesOriginalCaseInsensitiveExtension(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		filepath.Join(dir, "00001_demo.VCLIP"),
		filepath.Join(dir, "00000_demo.vclip"),
		filepath.Join(dir, "00000_other.vclip"),
		filepath.Join(dir, "00002_demo.txt"),
	}
	for _, file := range files {
		if err := os.WriteFile(file, []byte(filepath.Base(file)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := multiThreadPartFiles(filepath.Join(dir, "demo.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "00000_demo.vclip"), filepath.Join(dir, "00001_demo.VCLIP")}
	if !slices.Equal(got, want) {
		t.Fatalf("part files = %v, want %v", got, want)
	}
}

func TestCombineFilesMovesSingleFileLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "00000_demo.vclip")
	outPath := filepath.Join(dir, "demo.mp4")
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CombineFiles([]string{source}, outPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("single source should be moved away, stat err = %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q", body)
	}
}

func TestDownloadFileMultiThreadSkipsExistingSameSizeFile(t *testing.T) {
	var gotRangeGET bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotRangeGET = true
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	outPath := filepath.Join(t.TempDir(), "video.m4s")
	if err := os.WriteFile(outPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DownloadFileMultiThread(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, 20*1024*1024, 1); err != nil {
		t.Fatal(err)
	}
	if gotRangeGET {
		t.Fatal("existing same-size file should skip range downloads")
	}
}

func TestDownloadFileMultiThreadUsesGETForContentLengthLikeOriginal(t *testing.T) {
	var gotHEAD bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			gotHEAD = true
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	outPath := filepath.Join(t.TempDir(), "video.m4s")
	if err := DownloadFileMultiThread(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, 20*1024*1024, 1); err != nil {
		t.Fatal(err)
	}
	if gotHEAD {
		t.Fatal("multi-thread content length probe should use GET like original BBDown, not HEAD")
	}
	body, err := os.ReadFile(multiThreadPartPath(outPath, 0))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q", body)
	}
}

func readJoinedMultiThreadParts(t *testing.T, outPath string) string {
	t.Helper()
	files, err := multiThreadPartFiles(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(body)
	}
	return b.String()
}

func TestDownloadFileRetriesToTempFileThenMovesToTarget(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	dir := t.TempDir()
	outPath := filepath.Join(dir, "video.m4s")
	if err := DownloadFile(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, false); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "video.tmp")); !os.IsNotExist(err) {
		t.Fatalf("tmp file should be moved away, stat err = %v", err)
	}
}

func TestDownloadFileFailureDoesNotOverwriteExistingTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	outPath := filepath.Join(t.TempDir(), "video.m4s")
	if err := os.WriteFile(outPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DownloadFile(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, false); err == nil {
		t.Fatal("expected download failure")
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "old" {
		t.Fatalf("existing target was overwritten: %q", body)
	}
}

func TestDownloadFileResumesTempFileWithRange(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		switch attempts {
		case 1:
			if got := r.Header.Get("Range"); got != "bytes=0-" {
				t.Fatalf("first Range = %q", got)
			}
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
		case 2:
			if got := r.Header.Get("Range"); got != "bytes=5-" {
				t.Fatalf("resume Range = %q", got)
			}
			if got := r.Header.Get("If-Range"); got == "" {
				t.Fatal("resume request should carry If-Range like original")
			}
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("world"))
		default:
			t.Fatalf("unexpected attempt %d", attempts)
		}
	}))
	defer server.Close()
	cfg := &Config{}
	httpc := &HTTPClient{client: server.Client(), config: cfg}
	outPath := filepath.Join(t.TempDir(), "video.m4s")
	if err := DownloadFile(context.Background(), httpc, cfg, server.URL+"/video.m4s", outPath, false); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "helloworld" {
		t.Fatalf("body = %q", body)
	}
}

func TestMuxByFFmpegIncludesAudioMaterials(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "ffmpeg-args.txt")
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err := MuxAVWithOptions(MuxOptions{
		FFmpegPath: ffmpegPath,
		VideoPath:  filepath.Join(dir, "video.mp4"),
		AudioPath:  filepath.Join(dir, "audio.m4a"),
		OutPath:    filepath.Join(dir, "out.mp4"),
		AudioMaterials: []AudioMaterial{
			{Title: "背景音频", Path: filepath.Join(dir, "background.m4a")},
			{Title: "角色音频", PersonName: "long", Path: filepath.Join(dir, "role.m4a")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	for _, want := range []string{
		filepath.Join(dir, "background.m4a"),
		filepath.Join(dir, "role.m4a"),
		"1:a:0",
		"2:a:0",
		"-metadata:s:a:0",
		"title=原音频",
		"-metadata:s:a:1",
		"title=背景音频",
		"-metadata:s:a:2",
		"title=角色音频",
		"artist=long",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("ffmpeg args missing %q: %v", want, args)
		}
	}
}

func TestMuxByFFmpegEscapesMetadataLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "ffmpeg-args.txt")
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err := MuxAVWithOptions(MuxOptions{
		FFmpegPath:  ffmpegPath,
		VideoPath:   filepath.Join(dir, "video.mp4"),
		AudioPath:   filepath.Join(dir, "audio.m4a"),
		OutPath:     filepath.Join(dir, "out.mp4"),
		Title:       `视频"标题\路径`,
		Description: `简介"内容\路径`,
		EpisodeID:   `EP"01\A`,
		BVID:        "BV1qt4y1X7TW",
		Author:      "UP主",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	for _, want := range []string{
		`title=EP'01\\A`,
		`description=简介'内容\\路径`,
		`album=视频'标题\\路径`,
		"comment=https://www.bilibili.com/video/BV1qt4y1X7TW/",
		"artist=UP主",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("ffmpeg args missing escaped metadata %q: %v", want, args)
		}
	}
}

func TestMuxByFFmpegKeepsMissingCoverInputLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "ffmpeg-args.txt")
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	coverPath := filepath.Join(dir, "missing-cover.jpg")
	err := MuxAVWithOptions(MuxOptions{
		FFmpegPath: ffmpegPath,
		VideoPath:  filepath.Join(dir, "video.mp4"),
		AudioPath:  filepath.Join(dir, "audio.m4a"),
		CoverPath:  coverPath,
		OutPath:    filepath.Join(dir, "out.mp4"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	for _, want := range []string{coverPath, "-disposition:v:1", "attached_pic"} {
		if !slices.Contains(args, want) {
			t.Fatalf("ffmpeg args missing original missing-cover item %q: %v", want, args)
		}
	}
}

func TestMuxByFFmpegIgnoresExitCodeLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "ffmpeg-args.txt")
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 7\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err := MuxAVWithOptions(MuxOptions{
		FFmpegPath: ffmpegPath,
		VideoPath:  filepath.Join(dir, "video.mp4"),
		AudioPath:  filepath.Join(dir, "audio.m4a"),
		OutPath:    filepath.Join(dir, "out.mp4"),
	})
	if err != nil {
		t.Fatalf("MuxAVWithOptions should ignore started process exit code like original RunExe, got %v", err)
	}
	if _, err := os.ReadFile(argsPath); err != nil {
		t.Fatalf("ffmpeg script did not run: %v", err)
	}
}

func TestMuxByFFmpegUsesOriginalChapterFileName(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "ffmpeg-args.txt")
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	chapterPath := filepath.Join(dir, "chapters")
	err := MuxAVWithOptions(MuxOptions{
		FFmpegPath: ffmpegPath,
		VideoPath:  filepath.Join(dir, "video.mp4"),
		AudioPath:  filepath.Join(dir, "audio.m4a"),
		OutPath:    filepath.Join(dir, "out.mp4"),
		Chapters:   []ViewPoint{{Title: "片头", Start: 1, End: 12}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	if !slices.Contains(args, chapterPath) {
		t.Fatalf("ffmpeg args should use original chapter file name %q: %v", chapterPath, args)
	}
	if slices.Contains(args, chapterPath+".ffmetadata") || slices.Contains(args, chapterPath+".txt") {
		t.Fatalf("ffmpeg args should not use Go-specific chapter extension: %v", args)
	}
	if _, err := os.Stat(chapterPath); !os.IsNotExist(err) {
		t.Fatalf("chapter temp file should be removed after mux, stat err = %v", err)
	}
}

func TestMuxByFFmpegSkipsEmptySubtitleFiles(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "ffmpeg-args.txt")
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	emptySub := filepath.Join(dir, "empty.srt")
	validSub := filepath.Join(dir, "zh.srt")
	if err := os.WriteFile(emptySub, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(validSub, []byte("1\n00:00:00,000 --> 00:00:01,000\n你好\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := MuxAVWithOptions(MuxOptions{
		FFmpegPath: ffmpegPath,
		VideoPath:  filepath.Join(dir, "video.mp4"),
		AudioPath:  filepath.Join(dir, "audio.m4a"),
		OutPath:    filepath.Join(dir, "out.mp4"),
		Subtitles: []Subtitle{
			{Lan: "en-US", Path: emptySub},
			{Lan: "zh-CN", Path: validSub},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	if slices.Contains(args, emptySub) {
		t.Fatalf("ffmpeg args should skip empty subtitle %q: %v", emptySub, args)
	}
	for _, want := range []string{
		validSub,
		"2:s:0",
		"-metadata:s:s:0",
		"language=chi",
		"title=中文（简体）",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("ffmpeg args missing %q: %v", want, args)
		}
	}
	if slices.Contains(args, "-metadata:s:s:1") {
		t.Fatalf("subtitle metadata should follow included subtitle tracks: %v", args)
	}
}

func TestMuxByMP4BoxSubtitleUdtaCountsExistingTracks(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "mp4box-args.txt")
	mp4boxPath := filepath.Join(dir, "mp4box")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 0\n"
	if err := os.WriteFile(mp4boxPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	subCN := filepath.Join(dir, "zh.srt")
	subEN := filepath.Join(dir, "en.srt")
	if err := os.WriteFile(subCN, []byte("1\n00:00:00,000 --> 00:00:01,000\n你好\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subEN, []byte("1\n00:00:00,000 --> 00:00:01,000\nhello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := MuxAVWithOptions(MuxOptions{
		UseMP4Box:  true,
		MP4BoxPath: mp4boxPath,
		VideoPath:  filepath.Join(dir, "video.mp4"),
		AudioPath:  filepath.Join(dir, "audio.m4a"),
		OutPath:    filepath.Join(dir, "out.mp4"),
		Subtitles: []Subtitle{
			{Lan: "zh-CN", Path: subCN},
			{Lan: "en-US", Path: subEN},
		},
		SimplyMux: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	for _, want := range []string{
		"3:type=name:str=中文（简体）",
		"4:type=name:str=English(USA)",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("mp4box args missing %q: %v", want, args)
		}
	}
	if slices.Contains(args, "1:type=name:str=中文（简体）") || slices.Contains(args, "2:type=name:str=中文（简体）") {
		t.Fatalf("subtitle udta should not target existing video/audio tracks: %v", args)
	}
}

func TestMuxByMP4BoxWritesMetadataWhenSimplyMuxLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "mp4box-args.txt")
	mp4boxPath := filepath.Join(dir, "mp4box")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 0\n"
	if err := os.WriteFile(mp4boxPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	err := MuxAVWithOptions(MuxOptions{
		UseMP4Box:   true,
		MP4BoxPath:  mp4boxPath,
		VideoPath:   filepath.Join(dir, "video.mp4"),
		AudioPath:   filepath.Join(dir, "audio.m4a"),
		OutPath:     filepath.Join(dir, "out.mp4"),
		Title:       "标题",
		Description: "简介",
		BVID:        "BV1qt4y1X7TW",
		Author:      "UP主",
		SimplyMux:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	if !slices.Contains(args, "-itags") {
		t.Fatalf("mp4box args should keep metadata even with SimplyMux like original: %v", args)
	}
	if !slices.Contains(args, "tool=:title=标题:sdesc=简介:comment=https://www.bilibili.com/video/BV1qt4y1X7TW/:artist=UP主") {
		t.Fatalf("mp4box args missing original -itags payload: %v", args)
	}
}

func TestMuxByMP4BoxKeepsMissingCoverTagLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "mp4box-args.txt")
	mp4boxPath := filepath.Join(dir, "mp4box")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 0\n"
	if err := os.WriteFile(mp4boxPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	coverPath := filepath.Join(dir, "missing-cover.jpg")

	err := MuxAVWithOptions(MuxOptions{
		UseMP4Box:  true,
		MP4BoxPath: mp4boxPath,
		VideoPath:  filepath.Join(dir, "video.mp4"),
		AudioPath:  filepath.Join(dir, "audio.m4a"),
		CoverPath:  coverPath,
		OutPath:    filepath.Join(dir, "out.mp4"),
		Title:      "标题",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	if !slices.Contains(args, "-itags") {
		t.Fatalf("mp4box args should include -itags with missing cover like original: %v", args)
	}
	if !slices.Contains(args, "tool=:cover="+coverPath+":title=标题:sdesc=:comment=https://www.bilibili.com/video//:artist=") {
		t.Fatalf("mp4box args missing original missing-cover tag: %v", args)
	}
}

func TestMuxByMP4BoxIgnoresExitCodeLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "mp4box-args.txt")
	mp4boxPath := filepath.Join(dir, "mp4box")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 7\n"
	if err := os.WriteFile(mp4boxPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	err := MuxAVWithOptions(MuxOptions{
		UseMP4Box:  true,
		MP4BoxPath: mp4boxPath,
		VideoPath:  filepath.Join(dir, "video.mp4"),
		AudioPath:  filepath.Join(dir, "audio.m4a"),
		OutPath:    filepath.Join(dir, "out.mp4"),
	})
	if err != nil {
		t.Fatalf("MuxAVWithOptions should ignore started process exit code like original RunExe, got %v", err)
	}
	if _, err := os.ReadFile(argsPath); err != nil {
		t.Fatalf("mp4box script did not run: %v", err)
	}
}

func TestMuxByMP4BoxUsesOriginalChapterFileName(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "mp4box-args.txt")
	mp4boxPath := filepath.Join(dir, "mp4box")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nexit 0\n"
	if err := os.WriteFile(mp4boxPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	chapterPath := filepath.Join(dir, "chapters")
	err := MuxAVWithOptions(MuxOptions{
		UseMP4Box:  true,
		MP4BoxPath: mp4boxPath,
		VideoPath:  filepath.Join(dir, "video.mp4"),
		AudioPath:  filepath.Join(dir, "audio.m4a"),
		OutPath:    filepath.Join(dir, "out.mp4"),
		Chapters:   []ViewPoint{{Title: "片头", Start: 1, End: 12}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	if !slices.Contains(args, chapterPath) {
		t.Fatalf("mp4box args should use original chapter file name %q: %v", chapterPath, args)
	}
	if slices.Contains(args, chapterPath+".ffmetadata") || slices.Contains(args, chapterPath+".txt") {
		t.Fatalf("mp4box args should not use Go-specific chapter extension: %v", args)
	}
	if _, err := os.Stat(chapterPath); !os.IsNotExist(err) {
		t.Fatalf("chapter temp file should be removed after mux, stat err = %v", err)
	}
}

func TestFormatMP4BoxTagsEscapesMetadataLikeOriginal(t *testing.T) {
	tags := formatMP4BoxTags(MuxOptions{
		Title:       `标题"Q\Z`,
		EpisodeID:   `EP"1\2`,
		Description: `简介"3\4`,
		BVID:        "BV1qt4y1X7TW",
		Author:      "UP主",
	})
	for _, want := range []string{
		`tool=`,
		`album=标题'Q\\Z`,
		`title=EP'1\\2`,
		`sdesc=简介'3\\4`,
		"comment=https://www.bilibili.com/video/BV1qt4y1X7TW/",
		"artist=UP主",
	} {
		if !strings.Contains(tags, want) {
			t.Fatalf("mp4box tags %q missing escaped metadata %q", tags, want)
		}
	}
}

func TestFormatMP4BoxTagsKeepsOriginalEmptyFields(t *testing.T) {
	tags := formatMP4BoxTags(MuxOptions{})
	want := "tool=:title=:sdesc=:comment=https://www.bilibili.com/video//:artist="
	if tags != want {
		t.Fatalf("mp4box empty tags = %q, want %q", tags, want)
	}
}

func TestFormatFFmpegChaptersMatchesOriginal(t *testing.T) {
	got := formatFFmpegChapters([]ViewPoint{
		{Title: "片头", Start: 1, End: 12},
		{Title: "零长章节", Start: 12, End: 12},
	})
	want := ";FFMETADATA\n" +
		"[CHAPTER]\n" +
		"TIMEBASE=1/1000\n" +
		"START=1000\n" +
		"END=12000\n" +
		"title=片头\n\n" +
		"[CHAPTER]\n" +
		"TIMEBASE=1/1000\n" +
		"START=12000\n" +
		"END=12000\n" +
		"title=零长章节\n\n"
	if got != want {
		t.Fatalf("ffmpeg chapters = %q, want %q", got, want)
	}
}

func TestFormatMP4BoxChaptersMatchesOriginal(t *testing.T) {
	got := formatMP4BoxChapters([]ViewPoint{
		{Title: "片头", Start: 1, End: 12},
		{Title: "超过一天", Start: 26*60*60 + 2*60 + 3, End: 26*60*60 + 3*60},
	})
	want := "00:00:01 片头\n26:02:03 超过一天\n"
	if got != want {
		t.Fatalf("mp4box chapters = %q, want %q", got, want)
	}
}

func TestFFmpegVersionSupportsDOVI(t *testing.T) {
	cases := []struct {
		name string
		info string
		want bool
	}{
		{name: "ffmpeg 5.0", info: "libavutil      57. 17.100 / 57. 17.100", want: true},
		{name: "original 57.x bug", info: "libavutil      57.  0.100 / 57.  0.100", want: true},
		{name: "ffmpeg 4.x", info: "libavutil      56. 70.100 / 56. 70.100", want: false},
		{name: "newer major", info: "libavutil      58. 29.100 / 58. 29.100", want: true},
		{name: "invalid", info: "ffmpeg version n/a", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ffmpegVersionSupportsDOVI(tc.info); got != tc.want {
				t.Fatalf("ffmpegVersionSupportsDOVI = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckFFmpegDOVIUsesVersionOutput(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' 'libavutil      57. 17.100 / 57. 17.100'\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if !CheckFFmpegDOVI(ffmpegPath) {
		t.Fatal("CheckFFmpegDOVI should accept libavutil 57.17+")
	}
}

func TestIsDolbyVisionTrack(t *testing.T) {
	if !IsDolbyVisionTrack(&Video{ID: "126"}) {
		t.Fatal("id 126 should be treated as Dolby Vision")
	}
	if !IsDolbyVisionTrack(&Video{Dfn: "杜比视界"}) {
		t.Fatal("dfn 杜比视界 should be treated as Dolby Vision")
	}
	if IsDolbyVisionTrack(&Video{ID: "125", Dfn: "HDR 真彩"}) {
		t.Fatal("HDR should not be treated as Dolby Vision")
	}
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func TestMergeFLVSingleFileMovesInput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "clip.mp4")
	out := filepath.Join(dir, "merged.mp4")
	if err := os.WriteFile(in, []byte("clip"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MergeFLV([]string{in}, out, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(in); !os.IsNotExist(err) {
		t.Fatalf("input still exists or unexpected stat error: %v", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "clip" {
		t.Fatalf("merged content = %q, want clip", string(body))
	}
}

func TestMergeFLVMultiFileWithFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found")
	}
	dir := t.TempDir()
	clip1 := filepath.Join(dir, "clip1.mp4")
	clip2 := filepath.Join(dir, "clip2.mp4")
	out := filepath.Join(dir, "merged.mp4")
	makeClip := func(path, color string) {
		cmd := exec.Command(ffmpeg, "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c="+color+":s=16x16:d=0.2", "-c:v", "libx264", "-pix_fmt", "yuv420p", path)
		if err := cmd.Run(); err != nil {
			t.Fatalf("make clip %s: %v", color, err)
		}
	}
	makeClip(clip1, "black")
	makeClip(clip2, "white")

	if err := MergeFLV([]string{clip1, clip2}, out, ffmpeg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(clip1); !os.IsNotExist(err) {
		t.Fatalf("clip1 still exists or unexpected stat error: %v", err)
	}
	if _, err := os.Stat(clip2); !os.IsNotExist(err) {
		t.Fatalf("clip2 still exists or unexpected stat error: %v", err)
	}
	if info, err := os.Stat(out); err != nil || info.Size() == 0 {
		t.Fatalf("merged output invalid: info=%v err=%v", info, err)
	}
	if err := exec.Command(ffprobe, "-v", "error", out).Run(); err != nil {
		t.Fatalf("ffprobe merged output: %v", err)
	}
}

func TestMergeFLVIgnoresTranscodeExitCodeLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := `#!/bin/sh
out=""
for arg do
  out="$arg"
done
printf 'ts' > "$out"
exit 7
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	clip1 := filepath.Join(dir, "clip1.mp4")
	clip2 := filepath.Join(dir, "clip2.mp4")
	out := filepath.Join(dir, "merged.mp4")
	if err := os.WriteFile(clip1, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clip2, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MergeFLV([]string{clip1, clip2}, out, ffmpegPath); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "tsts" {
		t.Fatalf("merged body = %q, want tsts", body)
	}
}
