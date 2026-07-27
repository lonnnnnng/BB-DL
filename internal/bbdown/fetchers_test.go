package bbdown

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNormalInfoFetcherSetsBangumiEpidForSingleRedirectPage(t *testing.T) {
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `{"data":{"title":"番剧普通入口","desc":"简介","pic":"cover.jpg","pubdate":1717200000,"bvid":"BV1xx","cid":222,"redirect_url":"https://www.bilibili.com/bangumi/play/ep456","owner":{"name":"UP主","mid":42},"rights":{"is_stein_gate":0},"pages":[{"page":1,"cid":222,"part":"正片","duration":66,"dimension":{"width":1920,"height":1080}}]}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	info, err := (&NormalInfoFetcher{httpc: httpc}).Fetch(context.Background(), "111")
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsBangumi || len(info.PagesInfo) != 1 {
		t.Fatalf("normal redirect info = %+v", info)
	}
	if info.PagesInfo[0].Epid != "456" {
		t.Fatalf("page epid = %q, want 456", info.PagesInfo[0].Epid)
	}
}

func TestNormalInfoFetcherAppendsSteinGateChoicePages(t *testing.T) {
	var paths []string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			paths = append(paths, req.URL.Path)
			var body string
			switch req.URL.Path {
			case "/x/web-interface/view":
				body = `{"data":{"title":"互动视频","desc":"简介","pic":"cover.jpg","pubdate":1717200000,"bvid":"BV1xx","cid":222,"redirect_url":"","owner":{"name":"UP主","mid":42},"rights":{"is_stein_gate":1},"pages":[{"page":1,"cid":222,"part":"开场","duration":66,"dimension":{"width":1920,"height":1080}}]}}`
			case "/x/player.so":
				if req.URL.Query().Get("bvid") != "BV1xx" || req.URL.Query().Get("id") != "cid:222" {
					t.Fatalf("player.so query = %s", req.URL.RawQuery)
				}
				body = `<interaction>{"graph_version":987}</interaction>`
			case "/x/stein/edgeinfo_v2":
				if req.URL.Query().Get("graph_version") != "987" || req.URL.Query().Get("bvid") != "BV1xx" {
					t.Fatalf("edgeinfo query = %s", req.URL.RawQuery)
				}
				body = `{"data":{"edges":{"questions":[{"choices":[{"cid":333,"option":"左边"},{"cid":444,"option":"右边"}]}]}}}`
			default:
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	info, err := (&NormalInfoFetcher{httpc: httpc}).Fetch(context.Background(), "111")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/x/web-interface/view", "/x/player.so", "/x/stein/edgeinfo_v2"} {
		if !slices.Contains(paths, want) {
			t.Fatalf("request paths %v missing %s", paths, want)
		}
	}
	if !info.IsSteinGate || len(info.PagesInfo) != 3 {
		t.Fatalf("stein gate info = %+v", info)
	}
	if info.PagesInfo[1].Index != 2 || info.PagesInfo[1].Cid != "333" || info.PagesInfo[1].Title != "左边" || info.PagesInfo[1].OwnerName != "UP主" {
		t.Fatalf("first choice page = %+v", info.PagesInfo[1])
	}
	if info.PagesInfo[2].Index != 3 || info.PagesInfo[2].Cid != "444" || info.PagesInfo[2].Title != "右边" {
		t.Fatalf("second choice page = %+v", info.PagesInfo[2])
	}
}

func TestNormalInfoFetcherFallsBackToPlayerV2ForSteinGateGraphVersion(t *testing.T) {
	var paths []string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			paths = append(paths, req.URL.Path)
			var body string
			switch req.URL.Path {
			case "/x/web-interface/view":
				body = `{"data":{"title":"互动视频","desc":"简介","pic":"cover.jpg","pubdate":1717200000,"bvid":"BV1xx","cid":222,"redirect_url":"","owner":{"name":"UP主","mid":42},"rights":{"is_stein_gate":1},"pages":[{"page":1,"cid":222,"part":"开场","duration":66,"dimension":{"width":1920,"height":1080}}]}}`
			case "/x/player.so":
				body = `<html><title>出错啦!</title></html>`
			case "/x/player/v2":
				if req.URL.Query().Get("aid") != "111" || req.URL.Query().Get("cid") != "222" {
					t.Fatalf("player/v2 query = %s", req.URL.RawQuery)
				}
				body = `{"data":{"interaction":{"graph_version":542689}}}`
			case "/x/stein/edgeinfo_v2":
				if req.URL.Query().Get("graph_version") != "542689" || req.URL.Query().Get("bvid") != "BV1xx" {
					t.Fatalf("edgeinfo query = %s", req.URL.RawQuery)
				}
				body = `{"data":{"edges":{"questions":[{"choices":[{"cid":333,"option":"A 华强"},{"cid":444,"option":"B 瓜摊老板"}]}]}}}`
			default:
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	info, err := (&NormalInfoFetcher{httpc: httpc}).Fetch(context.Background(), "111")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/x/web-interface/view", "/x/player.so", "/x/player/v2", "/x/stein/edgeinfo_v2"} {
		if !slices.Contains(paths, want) {
			t.Fatalf("request paths %v missing %s", paths, want)
		}
	}
	if !info.IsSteinGate || len(info.PagesInfo) != 3 {
		t.Fatalf("stein gate fallback info = %+v", info)
	}
	if info.PagesInfo[1].Cid != "333" || info.PagesInfo[1].Title != "A 华强" {
		t.Fatalf("first fallback choice page = %+v", info.PagesInfo[1])
	}
	if info.PagesInfo[2].Cid != "444" || info.PagesInfo[2].Title != "B 瓜摊老板" {
		t.Fatalf("second fallback choice page = %+v", info.PagesInfo[2])
	}
}

func TestBangumiInfoFetcherUsesSectionWhenEpisodeMissing(t *testing.T) {
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `{"result":{"cover":"cover.jpg","title":"番剧标题","evaluate":"简介","publish":{"pub_time":"2024-06-01 12:00:00"},"episodes":[{"badge":"","aid":1,"cid":2,"id":101,"title":"1","long_title":"正片","pub_time":1717200000,"dimension":{"width":1920,"height":1080}}],"section":[{"title":"番外","episodes":[{"badge":"","aid":3,"cid":4,"id":202,"title":"SP","long_title":"特别篇","pub_time":1717286400,"dimension":{"width":1280,"height":720}}]}]}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	info, err := (&BangumiInfoFetcher{httpc: httpc, cfg: NewConfig()}).Fetch(context.Background(), "ep:202")
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "番剧标题[番外]" {
		t.Fatalf("title = %q, want section suffix", info.Title)
	}
	if info.Index != "1" || len(info.PagesInfo) != 1 {
		t.Fatalf("section episode selection = index %q pages %+v", info.Index, info.PagesInfo)
	}
	page := info.PagesInfo[0]
	if page.Aid != "3" || page.Cid != "4" || page.Epid != "202" || page.Title != "SP 特别篇" || page.Res != "1280x720" {
		t.Fatalf("section page = %+v", page)
	}
}

func TestIntlBangumiInfoFetcherUsesModuleEpisodeAndAccessKey(t *testing.T) {
	var requestedAccessKey string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedAccessKey = req.URL.Query().Get("access_key")
			body := `{"result":{"season_id":123,"cover":"intl-cover.jpg","title":"国际版标题","evaluate":"国际版简介","publish":{"pub_time":"2024-06-01 12:00:00"},"episodes":[{"badge":"","aid":1,"cid":2,"id":101,"title":"1","long_title":"正片","pub_time":1717200000,"dimension":{"width":1920,"height":1080}}],"modules":[{"data":{"episodes":[{"badge":"","aid":3,"cid":4,"id":202,"link":"https://www.bilibili.tv/play/123/202","title":"SP","long_title":"特别篇","pub_time":1717286400,"dimension":{"width":1280,"height":720}}]}}]}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}
	cfg := NewConfig()
	cfg.Token = "intl-token"

	info, err := (&IntlBangumiInfoFetcher{httpc: httpc, cfg: cfg}).Fetch(context.Background(), "ep:202")
	if err != nil {
		t.Fatal(err)
	}
	if requestedAccessKey != "intl-token" {
		t.Fatalf("access_key = %q, want token", requestedAccessKey)
	}
	if info.Title != "国际版标题" || info.Desc != "国际版简介" || info.Pic != "intl-cover.jpg" {
		t.Fatalf("metadata = title %q desc %q pic %q", info.Title, info.Desc, info.Pic)
	}
	if info.Index != "1" || len(info.PagesInfo) != 1 {
		t.Fatalf("module episode selection = index %q pages %+v", info.Index, info.PagesInfo)
	}
	page := info.PagesInfo[0]
	if page.Aid != "3" || page.Cid != "4" || page.Epid != "202" || page.Title != "SP 特别篇" || page.Res != "1280x720" {
		t.Fatalf("module page = %+v", page)
	}
}

func TestIntlBangumiInfoFetcherUsesInitialStateWhenCoverMissing(t *testing.T) {
	webFallbackRequested := false
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.Host {
			case "api.bilibili.tv":
				body = `{"result":{"season_id":123,"cover":"","title":"接口标题","evaluate":"接口简介","publish":{"pub_time":"2024-06-01 12:00:00"},"episodes":[{"badge":"","aid":1,"cid":2,"id":202,"title":"1","long_title":"正片","pub_time":1717200000,"dimension":{"width":1920,"height":1080}}]}}`
			case "bangumi.bilibili.com":
				webFallbackRequested = true
				if req.URL.Path != "/anime/123" {
					t.Fatalf("fallback path = %q", req.URL.Path)
				}
				body = `<html><script>window.__INITIAL_STATE__={"mediaInfo":{"cover":"web-cover.jpg","title":"网页标题","evaluate":"网页简介"}};(function(){})</script></html>`
			default:
				t.Fatalf("unexpected host %q", req.URL.Host)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	info, err := (&IntlBangumiInfoFetcher{httpc: httpc, cfg: NewConfig()}).Fetch(context.Background(), "ep:202")
	if err != nil {
		t.Fatal(err)
	}
	if !webFallbackRequested {
		t.Fatal("expected bangumi web metadata fallback request")
	}
	if info.Title != "网页标题" || info.Desc != "网页简介" || info.Pic != "web-cover.jpg" {
		t.Fatalf("fallback metadata = title %q desc %q pic %q", info.Title, info.Desc, info.Pic)
	}
	if info.Index != "1" || len(info.PagesInfo) != 1 {
		t.Fatalf("episode selection = index %q pages %+v", info.Index, info.PagesInfo)
	}
}

func TestMediaListFetcherFallsBackToSeriesWhenCollectionInfoMissing(t *testing.T) {
	sawSeriesList := false
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.Path {
			case "/x/v1/medialist/info":
				switch req.URL.Query().Get("type") {
				case "8":
					body = `{"code":-404,"message":"collection missing","data":null}`
				case "5":
					body = `{"code":0,"data":{"title":"系列标题","intro":"系列简介","ctime":1717200000}}`
				default:
					t.Fatalf("unexpected info type %q", req.URL.Query().Get("type"))
				}
			case "/x/v2/medialist/resource/list":
				if req.URL.Query().Get("type") != "5" || req.URL.Query().Get("desc") != "true" {
					t.Fatalf("series fallback query = %s", req.URL.RawQuery)
				}
				sawSeriesList = true
				body = `{"code":0,"data":{"has_more":false,"media_list":[{"attr":0,"page":1,"id":111,"title":"视频标题","intro":"视频简介","pubtime":1717200100,"cover":"cover.jpg","upper":{"name":"UP主","mid":42},"pages":[{"id":222,"page":1,"title":"正片","duration":66,"dimension":{"width":1920,"height":1080}}]}]}}`
			default:
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	info, err := (&MediaListFetcher{httpc: httpc}).Fetch(context.Background(), "listBizId:2045")
	if err != nil {
		t.Fatal(err)
	}
	if !sawSeriesList {
		t.Fatal("expected media list fallback to series resource list")
	}
	if info.Title != "系列标题" || info.Desc != "系列简介" || len(info.PagesInfo) != 1 {
		t.Fatalf("fallback info = title %q desc %q pages %+v", info.Title, info.Desc, info.PagesInfo)
	}
	page := info.PagesInfo[0]
	if page.Index != 1 || page.Aid != "111" || page.Cid != "222" || page.Title != "视频标题" || page.Res != "1920x1080" {
		t.Fatalf("fallback page = %+v", page)
	}
}

func TestMediaListFetcherSkipsDuplicatePagesByAidCidEpid(t *testing.T) {
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.Path {
			case "/x/v1/medialist/info":
				body = `{"code":0,"data":{"title":"合集标题","intro":"合集简介","ctime":1717200000}}`
			case "/x/v2/medialist/resource/list":
				body = `{"code":0,"data":{"has_more":false,"media_list":[{"attr":0,"page":1,"id":111,"title":"重复视频A","intro":"简介A","pubtime":1717200100,"cover":"a.jpg","upper":{"name":"UP主","mid":42},"pages":[{"id":222,"page":1,"title":"正片","duration":66,"dimension":{"width":1920,"height":1080}}]},{"attr":0,"page":1,"id":111,"title":"重复视频B","intro":"简介B","pubtime":1717200200,"cover":"b.jpg","upper":{"name":"UP主","mid":42},"pages":[{"id":222,"page":1,"title":"正片","duration":77,"dimension":{"width":1280,"height":720}}]},{"attr":0,"page":1,"id":333,"title":"新视频","intro":"简介C","pubtime":1717200300,"cover":"c.jpg","upper":{"name":"UP主","mid":42},"pages":[{"id":444,"page":1,"title":"正片","duration":88,"dimension":{"width":640,"height":360}}]}]}}`
			default:
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	info, err := (&MediaListFetcher{httpc: httpc}).Fetch(context.Background(), "listBizId:2045")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.PagesInfo) != 2 {
		t.Fatalf("pages len = %d, pages %+v", len(info.PagesInfo), info.PagesInfo)
	}
	if info.PagesInfo[0].Index != 1 || info.PagesInfo[0].Aid != "111" || info.PagesInfo[0].Cid != "222" {
		t.Fatalf("first page = %+v", info.PagesInfo[0])
	}
	if info.PagesInfo[1].Index != 2 || info.PagesInfo[1].Aid != "333" || info.PagesInfo[1].Cid != "444" {
		t.Fatalf("second page = %+v", info.PagesInfo[1])
	}
}

func TestSpaceVideoFetcherReturnsDownloadablePagesAndURLList(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	cfg := NewConfig()
	cfg.WBISalt = "test-salt"
	var requestedPaths []string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedPaths = append(requestedPaths, req.URL.Path)
			var body string
			switch req.URL.Path {
			case "/live_user/v1/Master/info":
				if req.URL.Query().Get("uid") != "42" {
					t.Fatalf("user info uid = %q", req.URL.Query().Get("uid"))
				}
				body = `{"data":{"info":{"uname":"UP/主"}}}`
			case "/x/space/wbi/arc/search":
				switch req.URL.Query().Get("pn") {
				case "1":
					body = `{"data":{"page":{"count":51},"list":{"vlist":[{"aid":111,"first_cid":222,"title":"单P投稿","length":"01:06","created":1717200000,"pic":"cover1.jpg","description":"简介1","author":"UP主","mid":42,"videos":1}]}}}`
				case "2":
					body = `{"data":{"page":{"count":51},"list":{"vlist":[{"aid":333,"title":"多P投稿","length":"01:17","created":1717200100,"pic":"cover2.jpg","description":"简介2","author":"UP主","mid":42,"videos":2}]}}}`
				default:
					t.Fatalf("unexpected space page %q", req.URL.Query().Get("pn"))
				}
			case "/x/web-interface/view":
				if req.URL.Query().Get("aid") != "333" {
					t.Fatalf("normal info aid = %q", req.URL.Query().Get("aid"))
				}
				body = `{"data":{"title":"多P投稿","desc":"多P简介","pic":"cover2.jpg","pubdate":1717200100,"bvid":"BV1xx","cid":444,"redirect_url":"","owner":{"name":"UP主","mid":42},"rights":{"is_stein_gate":0},"pages":[{"page":1,"cid":444,"part":"第一段","duration":10,"dimension":{"width":1920,"height":1080}},{"page":2,"cid":555,"part":"第二段","duration":20,"dimension":{"width":1280,"height":720}}]}}`
			default:
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: cfg,
	}

	out := captureStdout(t, func() {
		info, err := (&SpaceVideoFetcher{httpc: httpc, cfg: cfg}).Fetch(context.Background(), "mid:42")
		if err != nil {
			t.Fatal(err)
		}
		if info.Title != "UP.主的投稿视频" {
			t.Fatalf("title = %q", info.Title)
		}
		if len(info.PagesInfo) != 3 {
			t.Fatalf("pages len = %d, pages %+v", len(info.PagesInfo), info.PagesInfo)
		}
		want := []Page{
			{Index: 1, Aid: "111", Cid: "222", Title: "单P投稿", Dur: 66, PubTime: 1717200000, Cover: "cover1.jpg", Desc: "简介1", OwnerName: "UP主", OwnerMid: "42"},
			{Index: 2, Aid: "333", Cid: "444", Title: "多P投稿_P1_第一段", Dur: 10, Res: "1920x1080", PubTime: 1717200100, Cover: "cover2.jpg", Desc: "多P简介", OwnerName: "UP主", OwnerMid: "42"},
			{Index: 3, Aid: "333", Cid: "555", Title: "多P投稿_P2_第二段", Dur: 20, Res: "1280x720", PubTime: 1717200100, Cover: "cover2.jpg", Desc: "多P简介", OwnerName: "UP主", OwnerMid: "42"},
		}
		for i := range want {
			if info.PagesInfo[i] != want[i] {
				t.Fatalf("page %d = %+v, want %+v", i, info.PagesInfo[i], want[i])
			}
		}
	})
	if !strings.Contains(out, "已获取该用户的全部投稿视频地址并保存至：UP.主的投稿视频.txt") {
		t.Fatalf("space fetch output = %q", out)
	}
	listBody, err := os.ReadFile(filepath.Join(tmp, "UP.主的投稿视频.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(listBody)); got != "https://www.bilibili.com/video/av111\nhttps://www.bilibili.com/video/av333" {
		t.Fatalf("url list = %q", got)
	}
	for _, wantPath := range []string{"/live_user/v1/Master/info", "/x/space/wbi/arc/search", "/x/web-interface/view"} {
		if !slices.Contains(requestedPaths, wantPath) {
			t.Fatalf("requested paths %v missing %s", requestedPaths, wantPath)
		}
	}
}

func TestSpaceVideoFetcherReportsReadableNonJSONError(t *testing.T) {
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("<html>risk control</html>")),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}

	_, err := (&SpaceVideoFetcher{httpc: httpc, cfg: NewConfig()}).Fetch(context.Background(), "mid:42")
	if err == nil {
		t.Fatal("expected non-json error")
	}
	for _, want := range []string{"获取空间用户信息失败", "接口未返回合法JSON", "<html>risk control</html>"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestSpaceVideoFetcherFallsBackToLegacyArcSearchWhenWBIBlocked(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	cfg := NewConfig()
	cfg.WBISalt = "test-salt"
	var requestedPaths []string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedPaths = append(requestedPaths, req.URL.Path+"?pn="+req.URL.Query().Get("pn"))
			var body string
			switch req.URL.Path {
			case "/live_user/v1/Master/info":
				body = `{"data":{"info":{"uname":"UP主"}}}`
			case "/x/space/wbi/arc/search":
				body = `<html>risk control</html>`
			case "/x/space/arc/search":
				switch req.URL.Query().Get("pn") {
				case "1":
					body = `{"code":0,"data":{"page":{"count":51},"list":{"vlist":[{"aid":111,"first_cid":222,"title":"旧接口P1","length":"00:10","created":1717200000,"pic":"cover1.jpg","description":"简介1","author":"UP主","mid":42,"videos":1}]}}}`
				case "2":
					body = `{"code":0,"data":{"page":{"count":51},"list":{"vlist":[{"aid":333,"first_cid":444,"title":"旧接口P2","length":"00:20","created":1717200100,"pic":"cover2.jpg","description":"简介2","author":"UP主","mid":42,"videos":1}]}}}`
				default:
					t.Fatalf("unexpected legacy page %q", req.URL.Query().Get("pn"))
				}
			default:
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: cfg,
	}

	out := captureStdout(t, func() {
		info, err := (&SpaceVideoFetcher{httpc: httpc, cfg: cfg}).Fetch(context.Background(), "mid:42")
		if err != nil {
			t.Fatal(err)
		}
		if len(info.PagesInfo) != 2 {
			t.Fatalf("pages = %+v", info.PagesInfo)
		}
		if info.PagesInfo[0].Aid != "111" || info.PagesInfo[1].Aid != "333" {
			t.Fatalf("legacy pages = %+v", info.PagesInfo)
		}
	})
	if !strings.Contains(out, "空间 WBI 投稿列表失败，已回退旧接口") {
		t.Fatalf("fallback output = %q", out)
	}
	if slices.Contains(requestedPaths, "/x/space/wbi/arc/search?pn=2") {
		t.Fatalf("second page should keep using legacy endpoint: %v", requestedPaths)
	}
	for _, wantPath := range []string{"/x/space/wbi/arc/search?pn=1", "/x/space/arc/search?pn=1", "/x/space/arc/search?pn=2"} {
		if !slices.Contains(requestedPaths, wantPath) {
			t.Fatalf("requested paths %v missing %s", requestedPaths, wantPath)
		}
	}
}

func TestSpaceVideoFetcherReportsBothWBIAndLegacyArcErrors(t *testing.T) {
	cfg := NewConfig()
	cfg.WBISalt = "test-salt"
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.Path {
			case "/live_user/v1/Master/info":
				body = `{"data":{"info":{"uname":"UP主"}}}`
			case "/x/space/wbi/arc/search":
				body = `{"code":-352,"message":"风控校验失败"}`
			case "/x/space/arc/search":
				body = `{"code":-799,"message":"请求过于频繁，请稍后再试"}`
			default:
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: cfg,
	}

	_, err := (&SpaceVideoFetcher{httpc: httpc, cfg: cfg}).Fetch(context.Background(), "mid:42")
	if err == nil {
		t.Fatal("expected combined space arc error")
	}
	for _, want := range []string{"code=-352", "旧接口回退也失败", "code=-799"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestSpaceVideoFetcherFallsBackWhenMultiPageDetailFails(t *testing.T) {
	cfg := NewConfig()
	cfg.WBISalt = "test-salt"
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.Path {
			case "/live_user/v1/Master/info":
				body = `{"data":{"info":{"uname":"UP主"}}}`
			case "/x/space/wbi/arc/search":
				body = `{"data":{"page":{"count":1},"list":{"vlist":[{"aid":333,"first_cid":444,"title":"多P投稿","length":"01:17","created":1717200100,"pic":"cover2.jpg","description":"简介2","author":"UP主","mid":42,"videos":2}]}}}`
			case "/x/web-interface/view":
				body = `<html>blocked</html>`
			default:
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: cfg,
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	out := captureStdout(t, func() {
		info, err := (&SpaceVideoFetcher{httpc: httpc, cfg: cfg}).Fetch(context.Background(), "mid:42")
		if err != nil {
			t.Fatal(err)
		}
		if len(info.PagesInfo) != 1 {
			t.Fatalf("pages = %+v", info.PagesInfo)
		}
		page := info.PagesInfo[0]
		if page.Aid != "333" || page.Cid != "444" || page.Title != "多P投稿" || page.Desc != "简介2" {
			t.Fatalf("fallback page = %+v", page)
		}
	})
	if !strings.Contains(out, "分 P 详情获取失败") || !strings.Contains(out, "已获取该用户的全部投稿视频地址") {
		t.Fatalf("fallback output = %q", out)
	}
}

func TestParseDurationSeconds(t *testing.T) {
	cases := map[string]int{
		"66":      66,
		"01:06":   66,
		"1:02:03": 3723,
		"bad":     0,
		"01:bad":  0,
		"":        0,
	}
	for input, want := range cases {
		if got := parseDurationSeconds(input); got != want {
			t.Fatalf("parseDurationSeconds(%q) = %d, want %d", input, got, want)
		}
	}
}
