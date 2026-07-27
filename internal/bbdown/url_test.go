package bbdown

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGetQueryStringKeepsOriginalRawValue(t *testing.T) {
	rawURL := "https://www.bilibili.com/medialist/play/watchlater?business_id=abc%2Fdef+ghi&business=space_collection&business_id=second"
	if got := GetQueryString("business_id", rawURL); got != "abc%2Fdef+ghi" {
		t.Fatalf("GetQueryString raw value = %q, want abc%%2Fdef+ghi", got)
	}
}

func TestGetQueryStringMatchesOriginalRegexBoundaries(t *testing.T) {
	rawURL := "https://www.bilibili.com/video/BV1xx?p=&foo-bar=1&bar=2"
	if got := GetQueryString("p", rawURL); got != "" {
		t.Fatalf("empty p = %q, want empty string", got)
	}
	if got := GetQueryString("foo-bar", rawURL); got != "" {
		t.Fatalf("hyphen key = %q, want empty string", got)
	}
	if got := GetQueryString("ep_id", "https://www.bilibili.com/bangumi/play/ss1#ep_id=456&from=player"); got != "456" {
		t.Fatalf("fragment ep_id = %q, want 456", got)
	}
}

func TestResolveAvidConvertsBangumiSSID(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		if req.URL.Path != "/pgc/view/web/season" || req.URL.Query().Get("season_id") != "123" {
			t.Fatalf("unexpected request: %s", req.URL.String())
		}
		return `{"result":{"episodes":[{"id":456}]} }`
	})

	got, err := ResolveAvid(context.Background(), "ss123", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ep:456" {
		t.Fatalf("ResolveAvid ss = %q, want ep:456", got)
	}
}

func TestResolveAvidKeepsEmptyAVGroupLikeOriginal(t *testing.T) {
	var sawEmptyAvid bool
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		if req.URL.Path != "/video/av/" {
			t.Fatalf("unexpected request for empty av group: %s", req.URL.String())
		}
		sawEmptyAvid = true
		return `{}`
	})

	got, err := ResolveAvid(context.Background(), "https://www.bilibili.com/video/av", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if !sawEmptyAvid {
		t.Fatal("ResolveAvid should probe /video/av/ for empty av group like original")
	}
	if got != "" {
		t.Fatalf("ResolveAvid empty av = %q, want empty string", got)
	}
}

func TestResolveAvidInvalidBVURLUsesConverterErrorLikeOriginal(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		t.Fatalf("ResolveAvid should not request network for invalid BV URL: %s", req.URL.String())
		return `{}`
	})

	got, err := ResolveAvid(context.Background(), "https://www.bilibili.com/video/BV", httpc)
	if err == nil {
		t.Fatalf("ResolveAvid invalid BV = %q, want BV converter error", got)
	}
	if !strings.Contains(err.Error(), "must be 9 chars") {
		t.Fatalf("ResolveAvid invalid BV error = %v, want converter length error", err)
	}
}

func TestResolveAvidRequestsEmptySSIDLikeOriginal(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		if req.URL.Path != "/pgc/view/web/season" || req.URL.Query().Get("season_id") != "" {
			t.Fatalf("unexpected request for empty ss group: %s", req.URL.String())
		}
		return `{"result":{"episodes":[{"id":456}]} }`
	})

	got, err := ResolveAvid(context.Background(), "https://www.bilibili.com/bangumi/play/ss", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ep:456" {
		t.Fatalf("ResolveAvid empty ss = %q, want ep:456", got)
	}
}

func TestResolveAvidCheeseRequestsEmptySSIDLikeOriginal(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		if req.URL.Path != "/pugv/view/web/season" || req.URL.Query().Get("season_id") != "" {
			t.Fatalf("unexpected request for empty cheese ss group: %s", req.URL.String())
		}
		return `{"data":{"episodes":[{"id":789}]}}`
	})

	got, err := ResolveAvid(context.Background(), "cheese/ss", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "cheese:789" {
		t.Fatalf("ResolveAvid empty cheese ss = %q, want cheese:789", got)
	}
}

func TestResolveAvidKeepsEmptySpaceUIDLikeOriginal(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		t.Fatalf("ResolveAvid should not request network for empty space uid: %s", req.URL.String())
		return `{}`
	})

	got, err := ResolveAvid(context.Background(), "https://space.bilibili.com/", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "mid:" {
		t.Fatalf("ResolveAvid empty space uid = %q, want mid:", got)
	}

	got, err = ResolveAvid(context.Background(), "https://space.bilibili.com/favlist?fid=12", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "favId:12:" {
		t.Fatalf("ResolveAvid empty favlist uid = %q, want favId:12:", got)
	}
}

func TestResolveAvidKeepsEmptyEpGroupLikeOriginal(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		t.Fatalf("ResolveAvid should not request network for direct empty ep URL: %s", req.URL.String())
		return `{}`
	})

	got, err := ResolveAvid(context.Background(), "https://www.bilibili.com/bangumi/play/ep", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ep:" {
		t.Fatalf("ResolveAvid empty ep = %q, want ep:", got)
	}
}

func TestResolveAvidCheeseKeepsEmptyEpGroupLikeOriginal(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		t.Fatalf("ResolveAvid should not request network for direct empty cheese ep: %s", req.URL.String())
		return `{}`
	})

	got, err := ResolveAvid(context.Background(), "cheese/ep", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "cheese:" {
		t.Fatalf("ResolveAvid empty cheese ep = %q, want cheese:", got)
	}
}

func TestResolveAvidConvertsBangumiMD(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		if req.URL.Path != "/pgc/review/user" || req.URL.Query().Get("media_id") != "md789" {
			t.Fatalf("unexpected request: %s", req.URL.String())
		}
		return `{"result":{"media":{"new_ep":{"id":654}}}}`
	})

	got, err := ResolveAvid(context.Background(), "https://www.bilibili.com/bangumi/media/md789", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ep:654" {
		t.Fatalf("ResolveAvid md = %q, want ep:654", got)
	}
}

func TestResolveAvidRequestsEmptyMDLikeOriginal(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		if req.URL.Path != "/pgc/review/user" || req.URL.Query().Get("media_id") != "" {
			t.Fatalf("unexpected request for empty md group: %s", req.URL.String())
		}
		return `{"result":{"media":{"new_ep":{"id":654}}}}`
	})

	got, err := ResolveAvid(context.Background(), "md", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ep:654" {
		t.Fatalf("ResolveAvid empty md = %q, want ep:654", got)
	}
}

func TestResolveAvidFixesCopyrightRedirectLikeOriginal(t *testing.T) {
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/video/av123/":
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"https://www.bilibili.com/bangumi/play/ep456"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			case "/bangumi/play/ep456":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			default:
				t.Fatalf("unexpected request: %s", req.URL.String())
				return nil, nil
			}
		})},
		config: NewConfig(),
	}

	got, err := ResolveAvid(context.Background(), "av123", httpc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ep:456" {
		t.Fatalf("ResolveAvid redirected av = %q, want ep:456", got)
	}
}

func TestResolveAvidReturnsBangumiConversionError(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		return `{"result":{"episodes":[]}}`
	})

	_, err := ResolveAvid(context.Background(), "ss123", httpc)
	if err == nil || !strings.Contains(err.Error(), "未找到番剧集数") {
		t.Fatalf("ResolveAvid ss error = %v, want missing episodes error", err)
	}
}

func TestResolveAvidReturnsMediaConversionError(t *testing.T) {
	httpc := newURLTestHTTPClient(func(req *http.Request) string {
		return `{"result":{"media":{}}}`
	})

	got, err := ResolveAvid(context.Background(), "md789", httpc)
	if err == nil {
		t.Fatalf("ResolveAvid md = %q, want conversion error", got)
	}
}

func newURLTestHTTPClient(respond func(*http.Request) string) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := respond(req)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}
}
