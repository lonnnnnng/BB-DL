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

func TestFirstSubtitleResultKeepsSuccessfulEmptyList(t *testing.T) {
	empty := []Subtitle{}
	fallback := []Subtitle{{Lan: "zh-CN", URL: "https://example.test/sub.json"}}

	got := firstSubtitleResult(nil, empty, fallback)
	if got == nil {
		t.Fatal("successful empty subtitle list should not be treated as API failure")
	}
	if len(got) != 0 {
		t.Fatalf("subtitle fallback should stop at empty success, got %+v", got)
	}
}

func TestFirstSubtitleResultFallsBackOnNilResult(t *testing.T) {
	fallback := []Subtitle{{Lan: "en-US", URL: "https://example.test/en.json"}}

	got := firstSubtitleResult(nil, fallback)
	if len(got) != 1 || got[0].Lan != "en-US" {
		t.Fatalf("nil result should fall back, got %+v", got)
	}
}

func TestParseSubtitleListTreatsEmptyURLAsAPIFailureLikeOriginal(t *testing.T) {
	list := []any{
		map[string]any{"lan": "zh-CN", "subtitle_url": ""},
	}

	if got := parseSubtitleList("123", "456", list, "lan", "subtitle_url"); got != nil {
		t.Fatalf("regular subtitle API with empty url should fail like original, got %+v", got)
	}
}

func TestParseIntlSubtitleListKeepsSingleEmptyURLLikeOriginal(t *testing.T) {
	list := []any{
		map[string]any{"lang_key": "en", "url": ""},
	}

	got := parseIntlSubtitleList("123", "456", list, "lang_key")
	if len(got) != 1 || got[0].Lan != "en" || got[0].URL != "" || got[0].Path != "123/123.456.en.ass" {
		t.Fatalf("intl subtitle single empty url should keep original odd result, got %+v", got)
	}
}

func TestParseIntlSubtitleListFailsAfterPreviousEmptyURLLikeOriginal(t *testing.T) {
	list := []any{
		map[string]any{"key": "en", "url": ""},
		map[string]any{"key": "zh", "url": "https:\\/\\/subs.example\\/zh.json"},
	}

	if got := parseIntlSubtitleList("123", "456", list, "key"); got != nil {
		t.Fatalf("intl subtitle API should fail once a previous empty url exists, got %+v", got)
	}
}

func TestSubtitleCodeNormalizesLanguageDashCaseLikeOriginal(t *testing.T) {
	code, title := SubtitleCode("ai-zh")
	if code != "chi" || title != "中文（简体, AI识别）" {
		t.Fatalf("SubtitleCode(ai-zh) = %q, %q", code, title)
	}

	code, title = SubtitleCode("zh-hans")
	if code != "chi" || title != "中文（简体）" {
		t.Fatalf("SubtitleCode(zh-hans) = %q, %q", code, title)
	}
}

func TestNormalizeSubtitleLanUsesFirstRegexMatchLikeOriginal(t *testing.T) {
	if got, want := normalizeSubtitleLan("zh-hans-cn"), "zh-Hans-cn"; got != want {
		t.Fatalf("normalizeSubtitleLan multi segment = %q, want %q", got, want)
	}
	if got, want := normalizeSubtitleLan("aa-aa"), "aa-Aa"; got != want {
		t.Fatalf("normalizeSubtitleLan repeated first match = %q, want %q", got, want)
	}
}

func TestSubtitleCodeTableMatchesOriginalDisplayName(t *testing.T) {
	code, title := SubtitleCode("ab")
	if code != "abk" || title != "\u0410\u04B3\u04D9\u044B\u043D\u04AD\u049B\u0430\u0440\u0440\u0430" {
		t.Fatalf("SubtitleCode(ab) = %q, %q", code, title)
	}
}

func TestConvertSubtitleJSONToSRTKeepsContentTextLikeOriginal(t *testing.T) {
	got, err := ConvertSubtitleJSONToSRT([]byte(`{"body":[{"from":1.2,"to":3.4,"content":"A &amp; B"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n00:00:01,200 --> 00:00:03,400\nA &amp; B\n\n"
	if got != want {
		t.Fatalf("srt = %q, want %q", got, want)
	}
}

func TestConvertSubtitleJSONToSRTHandlesMissingFromAndContentLikeOriginal(t *testing.T) {
	got, err := ConvertSubtitleJSONToSRT([]byte(`{"body":[{"to":2.5},{"from":3,"to":4.25}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n00:00:00,000 --> 00:00:02,500\n\n2\n00:00:03,000 --> 00:00:04,250\n\n"
	if got != want {
		t.Fatalf("srt = %q, want %q", got, want)
	}
}

func TestConvertSubtitleJSONToSRTWrapsHoursAfterOneDayLikeOriginal(t *testing.T) {
	got, err := ConvertSubtitleJSONToSRT([]byte(`{"body":[{"from":90061.25,"to":90062.5,"content":"late"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n01:01:01,250 --> 01:01:02,500\nlate\n\n"
	if got != want {
		t.Fatalf("srt = %q, want %q", got, want)
	}
}

func TestGetSubtitlesFromAPI2UsesOriginalUnsignedQuery(t *testing.T) {
	var rawQuery string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/x/player/wbi/v2" {
				t.Fatalf("unexpected path: %s", req.URL.String())
			}
			rawQuery = req.URL.RawQuery
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"data":{"subtitle":{"subtitles":[{"lan":"zh-CN","subtitle_url":"https://subs.example/zh.json"}]}}}`)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}
	cfg := NewConfig()
	cfg.WBISalt = "non-empty-salt"

	subs := getSubtitlesFromAPI2(context.Background(), httpc, cfg, "123", "456")
	if rawQuery != "cid=456&aid=123" {
		t.Fatalf("raw query = %q, want cid=456&aid=123", rawQuery)
	}
	if len(subs) != 1 || subs[0].Lan != "zh-CN" || subs[0].URL != "https://subs.example/zh.json" {
		t.Fatalf("subtitles = %+v", subs)
	}
}

func TestSaveSubtitleConvertsOnlyLowercaseSRTLikeOriginal(t *testing.T) {
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"body":[{"from":1,"to":2,"content":"hello"}]}`)),
				Request:    req,
			}, nil
		})},
		config: NewConfig(),
	}
	dir := t.TempDir()

	lowerPath := filepath.Join(dir, "lower.srt")
	if err := SaveSubtitle(context.Background(), httpc, Subtitle{URL: "https://subs.example/lower.json"}, lowerPath); err != nil {
		t.Fatal(err)
	}
	lowerBody, err := os.ReadFile(lowerPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(lowerBody), "1\n00:00:01,000 --> 00:00:02,000\nhello\n\n"; got != want {
		t.Fatalf("lowercase srt body = %q, want %q", got, want)
	}

	upperPath := filepath.Join(dir, "upper.SRT")
	if err := SaveSubtitle(context.Background(), httpc, Subtitle{URL: "https://subs.example/upper.json"}, upperPath); err != nil {
		t.Fatal(err)
	}
	upperBody, err := os.ReadFile(upperPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(upperBody), `{"body":[{"from":1,"to":2,"content":"hello"}]}`; got != want {
		t.Fatalf("uppercase SRT body = %q, want raw json %q", got, want)
	}
}

func TestGetIntlSubtitlesUsesBiliPlusHostsAndToken(t *testing.T) {
	var calls []string
	httpc := &HTTPClient{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls = append(calls, req.URL.Host+req.URL.Path+"?"+req.URL.RawQuery)
			var body string
			switch req.URL.Host {
			case "epplus.example":
				if req.URL.Path != "/intl/gateway/web/v2/subtitle" || req.URL.Query().Get("episode_id") != "789" {
					t.Fatalf("unexpected EP host subtitle request: %s", req.URL.String())
				}
				body = `{"data":{"subtitles":[{"lang_key":"en","url":"https://subs.example/en.ass"}]}}`
			case "biliplus.example":
				if req.URL.Path != "/intl/gateway/v2/ogv/view/app/season" || req.URL.Query().Get("access_key") != "intl-token" {
					t.Fatalf("unexpected BiliPlus season subtitle request: %s", req.URL.String())
				}
				body = `{"result":{"modules":[{"data":{"episodes":[{"subtitles":[{"key":"zh","url":"https:\/\/subs.example\/zh.json"}]}]}}]}}`
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
	cfg := NewConfig()
	cfg.Host = "biliplus.example"
	cfg.EpHost = "epplus.example"
	cfg.Token = "intl-token"

	subs, err := GetSubtitles(context.Background(), httpc, cfg, "123", "456", "789", 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].Lan != "en" || subs[0].Path != "123/123.456.en.ass" {
		t.Fatalf("subtitles = %+v", subs)
	}
	if !slices.ContainsFunc(calls, func(call string) bool {
		return strings.HasPrefix(call, "epplus.example/intl/gateway/web/v2/subtitle?")
	}) {
		t.Fatalf("calls %v missing EP host subtitle API", calls)
	}
	if !slices.ContainsFunc(calls, func(call string) bool {
		return strings.HasPrefix(call, "biliplus.example/intl/gateway/v2/ogv/view/app/season?") && strings.Contains(call, "access_key=intl-token")
	}) {
		t.Fatalf("calls %v missing BiliPlus host season API", calls)
	}
}
