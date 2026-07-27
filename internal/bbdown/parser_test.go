package bbdown

import (
	"context"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestGetAppSign(t *testing.T) {
	got := GetAppSign("appkey=1d8b6e7d45233436&build=7320200")
	want := "366b9f1bb50b321ba65af67d8021c400"
	if got != want {
		t.Fatalf("GetAppSign = %s, want %s", got, want)
	}
}

func TestWriteDebugPlayJSON(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	path, err := writeDebugPlayJSON(`{"code":0}`)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^debug_\d{17}\.json$`).MatchString(path) {
		t.Fatalf("debug path = %q", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"code":0}` {
		t.Fatalf("debug body = %q", string(body))
	}
}

func TestParseDashLikeAppDubbingInfo(t *testing.T) {
	body := `{"code":0,"data":{"video_info":{"timelength":10000,"stream_list":[{"stream_info":{"quality":"80"},"dash_video":{"base_url":"https://video.example/80.m4s","backup_url":[],"bandwidth":1000000,"codecid":7,"size":123}}],"dash_audio":[{"id":30280,"base_url":"https://audio.example/main.m4a","backup_url":[],"bandwidth":128000,"codecs":"M4A"}]},"dubbing_info":{"background_audio":[{"id":1,"base_url":"https://audio.example/bg.m4a","backup_url":[],"bandwidth":64000,"codecs":"M4A"}],"role_audio_list":[{"title":"角色音频","person_name":"long","audio_id":7,"audio":[{"id":30280,"base_url":"https://audio.example/role.m4a","backup_url":[],"bandwidth":96000,"codecs":"M4A"}]}]}}}`
	tracks, err := parseDashLike(context.Background(), nil, body, NewConfig(), "ep:1", "100", "200", "1", false, false, true, &ParsedTracks{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks.BackgroundAudios) != 1 || tracks.BackgroundAudios[0].BaseURL != "https://audio.example/bg.m4a" {
		t.Fatalf("background audios = %+v", tracks.BackgroundAudios)
	}
	if len(tracks.RoleAudioLists) != 1 {
		t.Fatalf("role audio lists = %+v", tracks.RoleAudioLists)
	}
	role := tracks.RoleAudioLists[0]
	if role.Title != "角色音频" || role.PersonName != "long" || role.Path != "100/100.200.7.m4a" || len(role.Audio) != 1 || role.Audio[0].BaseURL != "https://audio.example/role.m4a" {
		t.Fatalf("role audio = %+v", role)
	}
}

func TestParseDashLikeAppGrpcDubbingInfoFields(t *testing.T) {
	body := `{"code":0,"data":{"video_info":{"timelength":10000,"stream_list":[],"dash_audio":[]},"dubbing_info":{"role_audio_list":[{"title":"角色音频","personName":"long","path":"100/custom-role.m4a","audio":[{"id":30280,"base_url":"https://audio.example/role.m4a","backup_url":[],"bandwidth":96000,"codecs":"M4A"}]}]}}}`
	tracks, err := parseDashLike(context.Background(), nil, body, NewConfig(), "ep:1", "100", "200", "1", false, false, true, &ParsedTracks{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks.RoleAudioLists) != 1 {
		t.Fatalf("role audio lists = %+v", tracks.RoleAudioLists)
	}
	role := tracks.RoleAudioLists[0]
	if role.PersonName != "long" || role.Path != "100/custom-role.m4a" {
		t.Fatalf("role audio grpc fields = %+v", role)
	}
}

func TestParseDashLikeAppDubbingInfoFromDataWrapper(t *testing.T) {
	body := `{"code":0,"data":{"dash":{"duration":10,"video":[{"id":80,"base_url":"https://video.example/80.m4s","backup_url":[],"bandwidth":1000000,"codecid":7,"size":100}],"audio":[{"id":30280,"base_url":"https://audio.example/main.m4a","backup_url":[],"bandwidth":128000,"codecs":"mp4a.40.2"}]},"dubbing_info":{"background_audio":[{"id":9,"base_url":"https://audio.example/bg.m4a","backup_url":[],"bandwidth":64000,"codecs":"M4A"}],"role_audio_list":[{"title":"粤语","person_name":"演员","audio_id":8,"audio":[{"id":30281,"base_url":"https://audio.example/role.m4a","backup_url":[],"bandwidth":96000,"codecs":"M4A"}]}]}}}`
	tracks, err := parseDashLike(context.Background(), nil, body, NewConfig(), "ep:1", "100", "200", "1", false, false, true, &ParsedTracks{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks.BackgroundAudios) != 1 || tracks.BackgroundAudios[0].BaseURL != "https://audio.example/bg.m4a" {
		t.Fatalf("background audios = %+v", tracks.BackgroundAudios)
	}
	if len(tracks.RoleAudioLists) != 1 {
		t.Fatalf("role audio lists = %+v", tracks.RoleAudioLists)
	}
	role := tracks.RoleAudioLists[0]
	if role.Title != "粤语" || role.PersonName != "演员" || role.Path != "100/100.200.8.m4a" || len(role.Audio) != 1 || role.Audio[0].BaseURL != "https://audio.example/role.m4a" {
		t.Fatalf("role audio = %+v", role)
	}
}

func TestExtractTracksReparsesDashWithMaxQn(t *testing.T) {
	var requestedQn []string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			qn := req.URL.Query().Get("qn")
			requestedQn = append(requestedQn, qn)
			body := dashPlayJSON("80", "https://video.example/80.m4s", "https://audio.example/initial.m4a")
			if qn == GetMaxQn() {
				body = dashPlayJSON("127", "https://video.example/127.m4s", "https://audio.example/max.m4a")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	tracks, err := ExtractTracks(context.Background(), NewConfig(), httpc, "123", "123", "456", "", false, false, false, "", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(requestedQn) != 2 || requestedQn[0] != "0" || requestedQn[1] != GetMaxQn() {
		t.Fatalf("requested qn = %v, want [0 %s]", requestedQn, GetMaxQn())
	}
	if len(tracks.VideoTracks) != 2 {
		t.Fatalf("video tracks = %+v, want initial and max-qn tracks", tracks.VideoTracks)
	}
	if len(tracks.AudioTracks) != 1 || tracks.AudioTracks[0].BaseURL != "https://audio.example/max.m4a" {
		t.Fatalf("audio tracks = %+v, want audio from max-qn response", tracks.AudioTracks)
	}
	if !strings.Contains(tracks.WebJSONString, "127.m4s") {
		t.Fatalf("WebJSONString should keep reparsed response: %s", tracks.WebJSONString)
	}
}

func TestAppendUniqueTrackIgnoresURLLikeOriginalEquals(t *testing.T) {
	videos := appendUniqueVideo(nil, Video{ID: "64", Dfn: "720P", Res: "1280x720", Fps: "30", Codecs: "AVC", Bandwidth: 800, Dur: 10, BaseURL: "https://cdn-a.example/video.m4s", Size: 1})
	videos = appendUniqueVideo(videos, Video{ID: "64", Dfn: "720P", Res: "1280x720", Fps: "30", Codecs: "AVC", Bandwidth: 800, Dur: 10, BaseURL: "https://cdn-b.example/video.m4s", Size: 2})
	if len(videos) != 1 {
		t.Fatalf("video tracks = %+v, want one track when only url/size changes", videos)
	}

	audios := appendUniqueAudio(nil, Audio{ID: "30280", Dfn: "30280", Codecs: "M4A", Bandwidth: 128, Dur: 10, BaseURL: "https://cdn-a.example/audio.m4a"})
	audios = appendUniqueAudio(audios, Audio{ID: "30280", Dfn: "30280", Codecs: "M4A", Bandwidth: 128, Dur: 10, BaseURL: "https://cdn-b.example/audio.m4a"})
	if len(audios) != 1 {
		t.Fatalf("audio tracks = %+v, want one track when only url changes", audios)
	}
}

func TestExtractTracksReparsesFLVWithMaxQn(t *testing.T) {
	var requestedQn []string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			qn := req.URL.Query().Get("qn")
			requestedQn = append(requestedQn, qn)
			body := flvPlayJSON("16", "https://flv.example/low.flv", 100, 1000)
			if qn == GetMaxQn() {
				body = flvPlayJSON("80", "https://flv.example/max.flv", 300, 3000)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	tracks, err := ExtractTracks(context.Background(), NewConfig(), httpc, "123", "123", "456", "", false, false, false, "", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(requestedQn) != 2 || requestedQn[0] != "0" || requestedQn[1] != GetMaxQn() {
		t.Fatalf("requested qn = %v, want [0 %s]", requestedQn, GetMaxQn())
	}
	if len(tracks.Clips) != 1 || tracks.Clips[0] != "https://flv.example/max.flv" {
		t.Fatalf("clips = %+v, want max-qn clip", tracks.Clips)
	}
	if len(tracks.VideoTracks) != 1 || tracks.VideoTracks[0].ID != "80" || tracks.VideoTracks[0].Dur != 3 || tracks.VideoTracks[0].Size != 300 {
		t.Fatalf("video tracks = %+v, want max-qn metadata", tracks.VideoTracks)
	}
	if len(tracks.Dfns) != 2 || tracks.Dfns[0] != "80" || tracks.Dfns[1] != "64" {
		t.Fatalf("dfns = %+v, want qn_extras from max-qn response", tracks.Dfns)
	}
}

func TestExtractTracksFallsBackToLegacyUGCPlayURLForVoucher(t *testing.T) {
	var requests []string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests = append(requests, req.URL.Path+"?qn="+req.URL.Query().Get("qn"))
			body := `{"code":0,"data":{"v_voucher":"voucher_test"}}`
			if req.URL.Path == "/x/player/playurl" {
				body = flvPlayJSON("32", "https://flv.example/legacy-low.flv", 100, 1000)
				if req.URL.Query().Get("qn") == GetMaxQn() {
					body = flvPlayJSON("64", "https://flv.example/legacy-max.mp4", 500, 3000)
				}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	tracks, err := ExtractTracks(context.Background(), NewConfig(), httpc, "149", "149", "3642987", "", false, false, false, "", "0")
	if err != nil {
		t.Fatal(err)
	}
	wantRequests := []string{
		"/x/player/wbi/playurl?qn=0",
		"/x/player/playurl?qn=0",
		"/x/player/wbi/playurl?qn=" + GetMaxQn(),
		"/x/player/playurl?qn=" + GetMaxQn(),
	}
	if strings.Join(requests, "\n") != strings.Join(wantRequests, "\n") {
		t.Fatalf("requests = %v, want %v", requests, wantRequests)
	}
	if len(tracks.Clips) != 1 || tracks.Clips[0] != "https://flv.example/legacy-max.mp4" {
		t.Fatalf("clips = %+v, want legacy max clip", tracks.Clips)
	}
	if len(tracks.VideoTracks) != 1 || tracks.VideoTracks[0].ID != "64" || tracks.VideoTracks[0].Dfn != "720P 高清" {
		t.Fatalf("video tracks = %+v, want legacy FLV metadata", tracks.VideoTracks)
	}
}

func TestFetchPointsParsesViewPoints(t *testing.T) {
	var requestedPath string
	var requestedQuery string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedPath = req.URL.Path
			requestedQuery = req.URL.RawQuery
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":0,"data":{"view_points":[{"content":"片头","from":1,"to":12},{"content":"正片","from":12,"to":60}]}}`)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	points := FetchPoints(context.Background(), httpc, NewConfig(), "456", "123")
	if requestedPath != "/x/player/wbi/v2" || requestedQuery != "cid=456&aid=123" {
		t.Fatalf("requested %s?%s, want original player v2 parameters", requestedPath, requestedQuery)
	}
	want := []ViewPoint{{Title: "片头", Start: 1, End: 12}, {Title: "正片", Start: 12, End: 60}}
	if !sameViewPoints(points, want) {
		t.Fatalf("points = %+v, want %+v", points, want)
	}
}

func TestFetchPointsIgnoresErrors(t *testing.T) {
	if got := FetchPoints(context.Background(), nil, NewConfig(), "456", "123"); got != nil {
		t.Fatalf("FetchPoints with nil client = %+v, want nil", got)
	}
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`not json`)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}
	if got := FetchPoints(context.Background(), httpc, NewConfig(), "456", "123"); got != nil {
		t.Fatalf("FetchPoints with bad JSON = %+v, want nil", got)
	}
}

func TestExtractTracksIntlReparsesPreferCodeType(t *testing.T) {
	var requestedCodes []string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			code := req.URL.Query().Get("prefer_code_type")
			requestedCodes = append(requestedCodes, code)
			body := intlStreamPlayJSON("80", "https://intl.example/avc.m4s")
			if code == "1" {
				body = intlStreamPlayJSON("80", "https://intl.example/hevc.m4s")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	tracks, err := ExtractTracks(context.Background(), NewConfig(), httpc, "ep:1", "123", "456", "789", false, true, false, "", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(requestedCodes) != 2 || requestedCodes[0] != "0" || requestedCodes[1] != "1" {
		t.Fatalf("prefer_code_type requests = %v, want [0 1]", requestedCodes)
	}
	if len(tracks.VideoTracks) != 2 {
		t.Fatalf("video tracks = %+v, want both intl code variants", tracks.VideoTracks)
	}
	if len(tracks.AudioTracks) != 1 {
		t.Fatalf("audio tracks should be de-duplicated: %+v", tracks.AudioTracks)
	}
	if !strings.Contains(tracks.WebJSONString, "hevc.m4s") {
		t.Fatalf("WebJSONString should keep second intl response: %s", tracks.WebJSONString)
	}
}

func TestExtractTracksBiliPlusIntlSignsRequests(t *testing.T) {
	cases := []struct {
		name     string
		area     string
		wantArea string
	}{
		{name: "default area", area: "", wantArea: "th"},
		{name: "explicit area", area: "tw", wantArea: "tw"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rawQueries []string
			var requestedCodes []string
			httpc := &HTTPClient{
				client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host != "biliplus.example" {
						t.Fatalf("host = %q, want biliplus.example", req.URL.Host)
					}
					if req.URL.Path != "/intl/gateway/v2/ogv/playurl" {
						t.Fatalf("path = %q", req.URL.Path)
					}
					rawQueries = append(rawQueries, req.URL.RawQuery)
					requestedCodes = append(requestedCodes, req.URL.Query().Get("prefer_code_type"))
					body := intlStreamPlayJSON("80", "https://biliplus.example/avc.m4s")
					if req.URL.Query().Get("prefer_code_type") == "1" {
						body = intlStreamPlayJSON("80", "https://biliplus.example/hevc.m4s")
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(body)),
						Request:    req,
					}, nil
				})},
				config: NewConfig(),
			}
			cfg := NewConfig()
			cfg.Host = "biliplus.example"
			cfg.Area = tc.area
			cfg.Token = "intl-token"

			tracks, err := ExtractTracks(context.Background(), cfg, httpc, "ep:1", "123", "456", "789", false, true, false, "", "0")
			if err != nil {
				t.Fatal(err)
			}
			if len(tracks.VideoTracks) != 2 {
				t.Fatalf("video tracks = %+v, want both BiliPlus code variants", tracks.VideoTracks)
			}
			if strings.Join(requestedCodes, ",") != "0,1" {
				t.Fatalf("prefer_code_type requests = %v, want [0 1]", requestedCodes)
			}
			for _, rawQuery := range rawQueries {
				params, sign, ok := strings.Cut(rawQuery, "&sign=")
				if !ok {
					t.Fatalf("raw query missing sign: %s", rawQuery)
				}
				if got := GetSign(params, true); got != sign {
					t.Fatalf("sign = %s, want %s for params %s", sign, got, params)
				}
				for _, want := range []string{
					"access_key=intl-token",
					"aid=123",
					"appkey=7d089525d3611b1c",
					"area=" + tc.wantArea,
					"cid=456",
					"ep_id=789",
					"platform=android",
					"qn=0",
					"s_locale=zh_SG",
					"ts=",
				} {
					if !strings.Contains(params, want) {
						t.Fatalf("params %q missing %q", params, want)
					}
				}
			}
		})
	}
}

func TestParseDashLikeMergesDolbyAndFlacAudio(t *testing.T) {
	body := `{"code":0,"data":{"dash":{"duration":10,"video":[{"id":80,"base_url":"https://video.example/80.m4s","backup_url":[],"bandwidth":1000000,"codecid":7,"size":100,"width":1920,"height":1080,"frame_rate":"30"}],"audio":[{"id":30280,"base_url":"https://audio.example/main.m4a","backup_url":[],"bandwidth":128000,"codecs":"mp4a.40.2"}],"dolby":{"audio":[{"id":30250,"base_url":"https://audio.example/dolby.eac3","backup_url":[],"bandwidth":448000,"codecs":"ec-3"}]},"flac":{"audio":{"id":30251,"base_url":"https://audio.example/flac.m4a","backup_url":[],"bandwidth":900000,"codecs":"fLaC"}}}}}`
	tracks, err := parseDashLike(context.Background(), nil, body, NewConfig(), "123", "123", "456", "", false, false, true, &ParsedTracks{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks.AudioTracks) != 3 {
		t.Fatalf("audio tracks = %+v, want main + dolby + flac", tracks.AudioTracks)
	}
	got := map[string]Audio{}
	for _, audio := range tracks.AudioTracks {
		got[audio.ID] = audio
	}
	if got["30280"].Codecs != "M4A" || got["30280"].BaseURL != "https://audio.example/main.m4a" {
		t.Fatalf("main audio = %+v", got["30280"])
	}
	if got["30250"].Codecs != "E-AC-3" || got["30250"].BaseURL != "https://audio.example/dolby.eac3" {
		t.Fatalf("dolby audio = %+v", got["30250"])
	}
	if got["30251"].Codecs != "FLAC" || got["30251"].BaseURL != "https://audio.example/flac.m4a" {
		t.Fatalf("flac audio = %+v", got["30251"])
	}
}

func TestParseDashLikeLabelsDolbyVisionVideo(t *testing.T) {
	body := `{"code":0,"data":{"dash":{"duration":10,"video":[{"id":126,"base_url":"https://video.example/dovi.m4s","backup_url":[],"bandwidth":20000000,"codecid":12,"size":25000000,"width":3840,"height":2160,"frame_rate":"60"}],"audio":[{"id":30280,"base_url":"https://audio.example/main.m4a","backup_url":[],"bandwidth":128000,"codecs":"mp4a.40.2"}]}}}`
	tracks, err := parseDashLike(context.Background(), nil, body, NewConfig(), "123", "123", "456", "", false, false, true, &ParsedTracks{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks.VideoTracks) != 1 {
		t.Fatalf("video tracks = %+v, want one Dolby Vision track", tracks.VideoTracks)
	}
	video := tracks.VideoTracks[0]
	if video.ID != "126" || video.Dfn != "杜比视界" || video.Codecs != "HEVC" || video.Bandwidth != 20000 || !IsDolbyVisionTrack(&video) {
		t.Fatalf("dolby vision video = %+v", video)
	}
}

func TestParseDashLikeSkipsDolbyAndFlacForTV(t *testing.T) {
	body := `{"code":0,"data":{"dash":{"duration":10,"video":[{"id":80,"base_url":"https://video.example/80.m4s","backup_url":[],"bandwidth":1000000,"codecid":7,"size":100}],"audio":[{"id":30280,"base_url":"https://audio.example/main.m4a","backup_url":[],"bandwidth":128000,"codecs":"mp4a.40.2"}],"dolby":{"audio":[{"id":30250,"base_url":"https://audio.example/dolby.eac3","backup_url":[],"bandwidth":448000,"codecs":"ec-3"}]},"flac":{"audio":{"id":30251,"base_url":"https://audio.example/flac.m4a","backup_url":[],"bandwidth":900000,"codecs":"fLaC"}}}}}`
	tracks, err := parseDashLike(context.Background(), nil, body, NewConfig(), "123", "123", "456", "", true, false, true, &ParsedTracks{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks.AudioTracks) != 1 || tracks.AudioTracks[0].ID != "30280" {
		t.Fatalf("tv audio tracks = %+v, want only dash.audio", tracks.AudioTracks)
	}
}

func TestBuildBangumiChapterPoints(t *testing.T) {
	rawArray := []any{
		map[string]any{"toastText": "即将跳过片尾", "start": float64(90), "end": float64(100)},
		map[string]any{"toastText": "即将跳过片头", "start": float64(10), "end": float64(20)},
	}
	got := buildBangumiChapterPoints(rawArray)
	want := []ViewPoint{
		{Title: "正片", Start: 0, End: 10},
		{Title: "片头", Start: 10, End: 20},
		{Title: "正片", Start: 20, End: 90},
		{Title: "片尾", Start: 90, End: 100},
	}
	if !sameViewPoints(got, want) {
		t.Fatalf("array chapter points = %+v, want %+v", got, want)
	}

	rawItems := map[string]any{"items": rawArray}
	if got := buildBangumiChapterPoints(rawItems); !sameViewPoints(got, want) {
		t.Fatalf("items chapter points = %+v, want %+v", got, want)
	}
}

func dashPlayJSON(qn, videoURL, audioURL string) string {
	return `{"code":0,"data":{"dash":{"duration":10,"video":[{"id":` + qn + `,"base_url":"` + videoURL + `","backup_url":[],"bandwidth":1000000,"codecid":7,"size":100,"width":1920,"height":1080,"frame_rate":"30"}],"audio":[{"id":30280,"base_url":"` + audioURL + `","backup_url":[],"bandwidth":128000,"codecs":"mp4a.40.2"}]}}}`
}

func intlStreamPlayJSON(qn, videoURL string) string {
	return `{"code":0,"data":{"video_info":{"timelength":10000,"stream_list":[{"stream_info":{"quality":"` + qn + `"},"dash_video":{"base_url":"` + videoURL + `","backup_url":[],"bandwidth":1000000,"codecid":7,"size":100}}],"dash_audio":[{"id":30280,"base_url":"https://intl.example/audio.m4a","backup_url":[],"bandwidth":128000}]}}}`
}

func flvPlayJSON(qn, clipURL string, size, length int) string {
	return `{"code":0,"data":{"quality":` + qn + `,"video_codecid":7,"durl":[{"url":"` + clipURL + `","size":` + strconv.Itoa(size) + `,"length":` + strconv.Itoa(length) + `}],"qn_extras":[{"qn":80},{"qn":64}]}}`
}

func sameViewPoints(a, b []ViewPoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
