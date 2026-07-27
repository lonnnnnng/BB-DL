package bbdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDanmakuFormats(t *testing.T) {
	defaults := ParseDanmakuFormats("")
	if !defaults[DanmakuXML] || !defaults[DanmakuASS] {
		t.Fatalf("default formats = %#v, want xml and ass", defaults)
	}

	assOnly := ParseDanmakuFormats("ass")
	if assOnly[DanmakuXML] || !assOnly[DanmakuASS] {
		t.Fatalf("ass formats = %#v, want ass only", assOnly)
	}

	xmlOnly := ParseDanmakuFormats("xml")
	if !xmlOnly[DanmakuXML] || xmlOnly[DanmakuASS] {
		t.Fatalf("xml formats = %#v, want xml only", xmlOnly)
	}

	fullWidthComma := ParseDanmakuFormats("xml，ass")
	if !fullWidthComma[DanmakuXML] || !fullWidthComma[DanmakuASS] {
		t.Fatalf("fullWidthComma formats = %#v, want xml and ass", fullWidthComma)
	}

	spacedUpper := ParseDanmakuFormats(" XML ， ASS ,, ")
	if !spacedUpper[DanmakuXML] || !spacedUpper[DanmakuASS] {
		t.Fatalf("spacedUpper formats = %#v, want xml and ass", spacedUpper)
	}

	invalid := ParseDanmakuFormats("ass,bad")
	if !invalid[DanmakuXML] || !invalid[DanmakuASS] {
		t.Fatalf("invalid formats = %#v, want fallback xml and ass", invalid)
	}

	emptyExplicitList := ParseDanmakuFormats(" ，,, ")
	if len(emptyExplicitList) != 0 {
		t.Fatalf("explicit empty format list = %#v, want no formats like original", emptyExplicitList)
	}

	whitespaceOnly := ParseDanmakuFormats("   ")
	if len(whitespaceOnly) != 0 {
		t.Fatalf("whitespace-only format list = %#v, want no formats like original", whitespaceOnly)
	}
}

func TestParseDanmakuXMLFileMatchesOriginalMetadataBoundaries(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "danmaku.xml")
	xmlBody := `<i>` +
		`<d p="1.2,1,25,16777215">short metadata should be skipped</d>` +
		`<d p="2.5,5,25,not-color,1710000000,0,hash,row">&amp;lt;tag&amp;gt;</d>` +
		`</i>`
	if err := os.WriteFile(xmlPath, []byte(xmlBody), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := ParseDanmakuXMLFile(xmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want only complete metadata item", items)
	}
	item := items[0]
	if item.Second != 2.5 || item.Mode != 2 || item.Color != "" || item.Content != "&lt;tag&gt;" {
		t.Fatalf("item = %+v, want original metadata and single XML entity decode", item)
	}
}

func TestSaveDanmakuASSKeepsRawContentLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "danmaku.ass")
	err := SaveDanmakuASS([]DanmakuItem{{
		Second:  1.2,
		Mode:    1,
		Color:   "FFFFFF",
		Content: "A {tag}\nB",
	}}, outPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "}A {tag}\nB\n") {
		t.Fatalf("ASS should keep raw danmaku content like original, got:\n%s", text)
	}
	if strings.Contains(text, `\{tag\}`) || strings.Contains(text, `\N`) {
		t.Fatalf("ASS content should not be escaped like a safer variant, got:\n%s", text)
	}
}

func TestSaveDanmakuASSWritesEmptyColorEffectLikeOriginal(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "danmaku.ass")
	err := SaveDanmakuASS([]DanmakuItem{{
		Second:  1.2,
		Mode:    1,
		Color:   "",
		Content: "bad color",
	}}, outPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `\c&&}bad color`) {
		t.Fatalf("ASS should keep original empty color effect, got:\n%s", text)
	}
}

func TestSaveDanmakuASSUsesUTF16LengthLikeCSharp(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "danmaku.ass")
	err := SaveDanmakuASS([]DanmakuItem{{
		Second:  1.2,
		Mode:    1,
		Color:   "FFFFFF",
		Content: "🙂",
	}}, outPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `\move(1920, 0, -80, 0)`) {
		t.Fatalf("ASS should use UTF-16 length like C#, got:\n%s", text)
	}
}
