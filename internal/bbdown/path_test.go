package bbdown

import (
	"testing"
	"time"
)

// long 2026-06-18 13:23:41：保存路径日期变量跟随 time.Local，发布 CI 默认 UTC；
// 固定北京时间能验证 BBDown 旧命名行为，避免测试结果被运行器时区带偏。
func useBeijingLocalForPathTest(t *testing.T) {
	t.Helper()

	oldLocal := time.Local
	time.Local = time.FixedZone("CST", 8*60*60)
	t.Cleanup(func() {
		time.Local = oldLocal
	})
}

func TestFormatSavePathBVID(t *testing.T) {
	path := FormatSavePath("<bvid>-<aid>", "title", nil, nil, Page{Aid: "80433022", Index: 1}, 1, "WEB", 0)
	if path != "BV1GJ411x7h7-80433022.mp4" {
		t.Fatalf("path = %q", path)
	}
}

func TestFormatSavePathBVIDRejectsTrailingGarbageLikeOriginal(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("FormatSavePath should panic when <bvid> aid is not a strict integer")
		}
	}()

	FormatSavePath("<bvid>", "title", nil, nil, Page{Aid: "80433022abc", Index: 1}, 1, "WEB", 0)
}

func TestFormatSavePathCustomDateLayout(t *testing.T) {
	useBeijingLocalForPathTest(t)

	path := FormatSavePath("<publishDate:yyyy-MM-dd_HH-mm-ss>/<videoDate:yyyyMMdd>", "title", nil, nil, Page{Aid: "1", Index: 1, PubTime: 1717200000}, 1, "WEB", 1717286400)
	if path != "2024-06-02_08-00-00/20240601.mp4" {
		t.Fatalf("path = %q", path)
	}
}

func TestFormatSavePathCustomDateLayoutWithFractionAndZone(t *testing.T) {
	useBeijingLocalForPathTest(t)

	path := FormatSavePath("<publishDate:yyyy-MM-ddTHH-mm-ss.fffzzz>", "title", nil, nil, Page{Aid: "1", Index: 1}, 1, "WEB", 1717286400)
	if path != "2024-06-02T08-00-00.000+08:00.mp4" {
		t.Fatalf("path = %q", path)
	}
}

func TestFormatSavePathKeepsOriginalDotTitleBehavior(t *testing.T) {
	page := Page{Aid: "1", Index: 1, Title: ".page."}
	got := FormatSavePath("<videoTitle>/<pageTitle>", ".hidden.", nil, nil, page, 1, "WEB", 0)
	if got != ".hidden/.page.mp4" {
		t.Fatalf("dot title path = %q", got)
	}
}

func TestDownloadTitleFixesDotFoldersLikeOriginalDownloadPage(t *testing.T) {
	cases := map[string]string{
		"title":    "title",
		"title.":   "title._fix",
		".title":   "_.title",
		".title.":  "_.title._fix",
		"...title": "_...title",
	}
	for input, want := range cases {
		if got := DownloadTitle(input); got != want {
			t.Fatalf("DownloadTitle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatSavePathWithDownloadTitleMatchesOriginalDownloadContext(t *testing.T) {
	page := Page{Aid: "1", Index: 1, Title: "P1"}
	got := FormatSavePath("<videoTitle>", DownloadTitle(".hidden."), nil, nil, page, 1, "WEB", 0)
	if got != "_.hidden._fix.mp4" {
		t.Fatalf("download context path = %q", got)
	}
}

func TestFormatSavePathTrimsAllTrailingDotsLikeOriginal(t *testing.T) {
	page := Page{Aid: "1", Index: 1, Title: "page... "}
	got := FormatSavePath("<videoTitle>/<pageTitle>", " title... ", nil, nil, page, 1, "WEB", 0)
	if got != "title/page.mp4" {
		t.Fatalf("multi-dot title path = %q", got)
	}
}

func TestFormatSavePathReplacesOriginalInvalidChars(t *testing.T) {
	page := Page{Aid: "1", Index: 1, Title: "line\nbreak:part"}
	got := FormatSavePath("<videoTitle>/<pageTitle>", "bad\t<title>|", nil, nil, page, 1, "WEB", 0)
	if got != "bad__title__/line_break_part.mp4" {
		t.Fatalf("invalid char path = %q", got)
	}
}

func TestFormatSavePathKeepsWhitespacePatternLikeOriginal(t *testing.T) {
	got := FormatSavePath("   ", "标题", nil, nil, Page{Aid: "1", Index: 1}, 1, "WEB", 0)
	if got != "   .mp4" {
		t.Fatalf("whitespace pattern path = %q", got)
	}
}
