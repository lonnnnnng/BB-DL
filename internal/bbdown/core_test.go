package bbdown

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatFileSize(t *testing.T) {
	cases := map[float64]string{
		42:                     "42 bytes",
		1.5:                    "1.5 bytes",
		1023.5:                 "1023.5 bytes",
		2048:                   "2.00 KB",
		20 * 1024 * 1024:       "20.00 MB",
		3 * 1024 * 1024 * 1024: "3.00 GB",
	}
	for input, want := range cases {
		if got := FormatFileSize(input); got != want {
			t.Fatalf("FormatFileSize(%v) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatFileSizePanicsOnNegativeLikeOriginal(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("FormatFileSize(-1) should panic like original ArgumentOutOfRangeException")
		}
	}()
	_ = FormatFileSize(-1)
}

func TestFormatTimeMatchesOriginalDisplay(t *testing.T) {
	cases := []struct {
		name     string
		seconds  int
		absolute bool
		want     string
	}{
		{name: "sub-minute", seconds: 59, want: "00m59s"},
		{name: "minute-boundary", seconds: 60, want: "01m00s"},
		{name: "relative-hours", seconds: 3661, want: "1h01m01s"},
		{name: "absolute-hours", seconds: 3661, absolute: true, want: "01:01:01"},
		{name: "absolute-over-day", seconds: 26*3600 + 2*60 + 3, absolute: true, want: "26:02:03"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatTime(tc.seconds, tc.absolute); got != tc.want {
				t.Fatalf("FormatTime(%d, %v) = %q, want %q", tc.seconds, tc.absolute, got, tc.want)
			}
		})
	}
}

func TestFormatTimeStampMatchesOriginalNullAndLocalTime(t *testing.T) {
	useBeijingLocalForPathTest(t)

	if got := FormatTimeStamp(0, "2006-01-02 15:04:05 -0700"); got != "null" {
		t.Fatalf("zero timestamp = %q, want null", got)
	}
	if got := FormatTimeStamp(1717286400, "2006-01-02 15:04:05 -0700"); got != "2024-06-02 08:00:00 +0800" {
		t.Fatalf("timestamp = %q", got)
	}
}

func TestFormatSelectedTrackLines(t *testing.T) {
	video := &Video{Dfn: "1080P 高码率", Res: "1920x1080", Codecs: "HEVC", Fps: "", Bandwidth: 1024, Dur: 10}
	if got := formatVideoTrackLine("[视频]", video, 0); got != "[视频] [1080P 高码率] [1920x1080] [HEVC] [1024 kbps] [~1.25 MB]" {
		t.Fatalf("video line = %q", got)
	}

	audio := &Audio{Codecs: "M4A", Bandwidth: 128, Dur: 10}
	if got := formatAudioTrackLine("[音频]", audio, 0); got != "[音频] [M4A] [128 kbps] [~160.00 KB]" {
		t.Fatalf("audio line = %q", got)
	}
}

func TestSortTracksVideoWithPriorityOrder(t *testing.T) {
	items := []Video{
		{ID: "80", Dfn: "1080P 高清", Codecs: "AVC", Bandwidth: 1000},
		{ID: "64", Dfn: "720P 高清", Codecs: "HEVC", Bandwidth: 800},
	}
	dfnPriority := ParseDfnPriority("1080P 高清,720P 高清")
	encodingPriority := ParseEncodingPriority("hevc,avc")

	dfnFirst := SortTracksVideoWithPriorityOrder(items, dfnPriority, encodingPriority, false, false)
	if dfnFirst[0].Dfn != "1080P 高清" {
		t.Fatalf("dfn-first selected %q, want 1080P 高清", dfnFirst[0].Dfn)
	}

	encodingFirst := SortTracksVideoWithPriorityOrder(items, dfnPriority, encodingPriority, false, true)
	if encodingFirst[0].Codecs != "HEVC" {
		t.Fatalf("encoding-first selected %q, want HEVC", encodingFirst[0].Codecs)
	}
}

func TestSortTracksVideoRejectsNonIntegerIDLikeOriginal(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("SortTracksVideo should panic when video id is not a strict integer")
		}
	}()

	_ = SortTracksVideo([]Video{
		{ID: "80abc", Dfn: "1080P 高清", Codecs: "AVC", Bandwidth: 1000},
		{ID: "64", Dfn: "720P 高清", Codecs: "AVC", Bandwidth: 800},
	}, false)
}

func TestPrintVideoInfoRejectsNonIntegerAidLikeOriginal(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("PrintVideoInfo should panic when first page aid is not a strict integer")
		}
	}()

	PrintVideoInfo(&VInfo{
		Title:     "标题",
		PagesInfo: []Page{{Index: 1, Aid: "80433022abc", Title: "P1"}},
	}, "WEB", false)
}

func TestFirstEncodingPriority(t *testing.T) {
	cases := map[string]string{
		"hevc,av1,avc": "HEVC",
		"e-ac-3,m4a":   "EAC3",
		"， av1，hevc":   "AV1",
		"":             "",
	}
	for input, want := range cases {
		if got := FirstEncodingPriority(input); got != want {
			t.Fatalf("FirstEncodingPriority(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEncodingPriorityRejectsExplicitEmptyListLikeOriginal(t *testing.T) {
	for name, fn := range map[string]func(){
		"first": func() { _ = FirstEncodingPriority(" ，,, ") },
		"parse": func() { _ = ParseEncodingPriority(" ，,, ") },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("explicit empty encoding priority should panic like original First()")
				}
			}()
			fn()
		})
	}
}

func TestPrintTracksIncludesDubbingInfo(t *testing.T) {
	out := captureStdout(t, func() {
		PrintTracks(&ParsedTracks{
			BackgroundAudios: []Audio{{Codecs: "M4A", Bandwidth: 64, Dur: 10}},
			RoleAudioLists: []AudioMaterialInfo{{
				Title: "角色音频",
				Audio: []Audio{{Codecs: "M4A", Bandwidth: 96, Dur: 10}},
			}},
		}, 0, false)
	})
	for _, want := range []string{"共计1条背景音频流.", "共计1条配音, 每条包含1条配音流.", "[M4A] [64 kbps]", "[M4A] [96 kbps]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("PrintTracks output missing %q: %s", want, out)
		}
	}
}

func TestResolveWorkDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BBDOWN_GO_TEST_DIR", dir)
	for _, input := range []string{"$BBDOWN_GO_TEST_DIR/out", "%BBDOWN_GO_TEST_DIR%/out"} {
		got, err := ResolveWorkDir(input)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(dir, "out")
		if got != want {
			t.Fatalf("ResolveWorkDir(%q) = %q, want %q", input, got, want)
		}
	}

	got, err := ResolveWorkDir("relative-out")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) || filepath.Base(got) != "relative-out" {
		t.Fatalf("relative workdir not absolutized: %q", got)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err = ResolveWorkDir("~/BB-DL-test")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, "BB-DL-test") {
		t.Fatalf("tilde workdir = %q", got)
	}
}

func TestLoadCredentialsLoadsLocalCookieAndToken(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := os.WriteFile(filepath.Join(dir, "BBDown.data"), []byte("SESSDATA=test-cookie"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "BBDownTV.data"), []byte("access_token=tv-token"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "BBDownApp.data"), []byte("access_token=app-token"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := NewConfig()
	out := captureStdout(t, func() {
		LoadCredentials(cfg, &MyOption{UseTvApi: true})
	})
	if cfg.Cookie != "SESSDATA=test-cookie" || cfg.Token != "tv-token" {
		t.Fatalf("credentials = cookie %q token %q", cfg.Cookie, cfg.Token)
	}
	if !strings.Contains(out, "加载本地cookie...") || !strings.Contains(out, "加载本地token...") {
		t.Fatalf("credential load output = %q", out)
	}

	cfg = NewConfig()
	LoadCredentials(cfg, &MyOption{UseAppApi: true})
	if cfg.Token != "app-token" {
		t.Fatalf("app token = %q", cfg.Token)
	}
}

func TestLoadCredentialsKeepsExplicitValues(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := os.WriteFile(filepath.Join(dir, "BBDown.data"), []byte("SESSDATA=local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "BBDownTV.data"), []byte("access_token=local-token"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := NewConfig()
	cfg.Cookie = "SESSDATA=explicit"
	cfg.Token = "explicit-token"
	out := captureStdout(t, func() {
		LoadCredentials(cfg, &MyOption{UseTvApi: true})
	})
	if cfg.Cookie != "SESSDATA=explicit" || cfg.Token != "explicit-token" {
		t.Fatalf("explicit credentials overwritten: %+v", cfg)
	}
	if out != "" {
		t.Fatalf("explicit credentials should not log local loading: %q", out)
	}
}

func TestFixAvidConvertsCopyrightRedirectToEp(t *testing.T) {
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/video/av123/") {
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"https://www.bilibili.com/bangumi/play/ep456"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	got, err := FixAvid(context.Background(), httpc, "123")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ep:456" {
		t.Fatalf("FixAvid = %q, want ep:456", got)
	}
}

func TestFixAvidProbesEmptyAvidLikeOriginal(t *testing.T) {
	var sawEmptyAvid bool
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/video/av/" {
				t.Fatalf("unexpected empty avid probe: %s", req.URL.String())
			}
			sawEmptyAvid = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	got, err := FixAvid(context.Background(), httpc, "")
	if err != nil {
		t.Fatal(err)
	}
	if !sawEmptyAvid {
		t.Fatal("FixAvid should probe empty avid because C# string.All returns true for empty strings")
	}
	if got != "" {
		t.Fatalf("FixAvid empty avid = %q, want empty string", got)
	}
}

func TestFixAvidKeepsEmptyEpRedirectGroupLikeOriginal(t *testing.T) {
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/video/av123/") {
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"https://www.bilibili.com/bangumi/play/episode"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	got, err := FixAvid(context.Background(), httpc, "123")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ep:" {
		t.Fatalf("FixAvid empty ep redirect = %q, want ep:", got)
	}
}

func TestApplySteinGateTVFallback(t *testing.T) {
	opt := &MyOption{UseTvApi: true}
	out := captureStdout(t, func() {
		if got := ApplySteinGateTVFallback(&VInfo{IsSteinGate: true}, opt); got != "WEB" {
			t.Fatalf("api type = %q", got)
		}
	})
	if opt.UseTvApi {
		t.Fatal("stein gate video should disable tv api")
	}
	if !strings.Contains(out, "视频为互动视频，暂时不支持tv下载，修改为默认下载") {
		t.Fatalf("fallback output = %q", out)
	}

	opt = &MyOption{UseTvApi: true}
	if got := ApplySteinGateTVFallback(&VInfo{}, opt); got != "TV" || !opt.UseTvApi {
		t.Fatalf("non stein gate fallback = %q opt=%+v", got, opt)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
