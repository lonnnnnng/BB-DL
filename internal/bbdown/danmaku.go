package bbdown

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const (
	danmakuWidth     = 1920
	danmakuHeight    = 1080
	danmakuFontSize  = 40
	danmakuMoveTime  = 8.0
	danmakuStillTime = 4.0
	danmakuProtect   = 50
)

type DanmakuFormat int

const (
	DanmakuXML DanmakuFormat = iota
	DanmakuASS
)

type DanmakuItem struct {
	Second  float64
	Mode    int
	Color   string
	Content string
}

func ParseDanmakuFormats(raw string) map[DanmakuFormat]bool {
	formats := map[DanmakuFormat]bool{DanmakuXML: true, DanmakuASS: true}
	if raw == "" {
		return formats
	}
	formats = map[DanmakuFormat]bool{}
	for _, item := range strings.Split(strings.ReplaceAll(raw, "，", ","), ",") {
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "xml":
			formats[DanmakuXML] = true
		case "ass":
			formats[DanmakuASS] = true
		case "":
			continue
		default:
			Errorf("包含不支持的下载弹幕格式：%s", raw)
			return map[DanmakuFormat]bool{DanmakuXML: true, DanmakuASS: true}
		}
	}
	// long: 原版只在参数为空字符串时使用默认 xml+ass；用户显式传入空白或 ",," 这类空列表时会得到空格式集合，后续连已下载的 XML 也会删除。
	return formats
}

type danmakuXMLFile struct {
	Items []danmakuXMLItem `xml:"d"`
}

type danmakuXMLItem struct {
	P    string `xml:"p,attr"`
	Text string `xml:",chardata"`
}

func ParseDanmakuXMLFile(path string) ([]DanmakuItem, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload danmakuXMLFile
	if err := xml.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	items := make([]DanmakuItem, 0, len(payload.Items))
	for _, node := range payload.Items {
		attrs := strings.Split(node.P, ",")
		// long 2026-06-19 04:21:03：原版只处理完整的 8 段弹幕元数据，半截 p 属性会被当作损坏弹幕跳过。
		if len(attrs) < 8 {
			continue
		}
		second, err := strconv.ParseFloat(attrs[0], 64)
		if err != nil {
			continue
		}
		mode := 1
		switch attrs[1] {
		case "4":
			mode = 3
		case "5":
			mode = 2
		}
		color := ""
		if colorValue, err := strconv.Atoi(attrs[3]); err == nil {
			color = fmt.Sprintf("%06X", colorValue)
		}
		items = append(items, DanmakuItem{
			Second:  second,
			Mode:    mode,
			Color:   color,
			Content: node.Text,
		})
	}
	return items, nil
}

func SaveDanmakuASS(items []DanmakuItem, outputPath string) error {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Second < items[j].Second })
	var b strings.Builder
	b.WriteString("[Script Info]\n")
	b.WriteString("Script Updated By: BBDown(https://github.com/nilaoda/BBDown)\n")
	b.WriteString("ScriptType: v4.00+\n")
	b.WriteString(fmt.Sprintf("PlayResX: %d\n", danmakuWidth))
	b.WriteString(fmt.Sprintf("PlayResY: %d\n", danmakuHeight))
	b.WriteString(fmt.Sprintf("Aspect Ratio: %d:%d\n", danmakuWidth, danmakuHeight))
	b.WriteString("Collisions: Normal\nWrapStyle: 2\nScaledBorderAndShadow: yes\nYCbCr Matrix: TV.601\n")
	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	b.WriteString(fmt.Sprintf("Style: BBDOWN_Style, 黑体, %d, &H00FFFFFF, &H00FFFFFF, &H00000000, &H00000000, 0, 0, 0, 0, 100, 100, 0.00, 0.00, 1, 2, 0, 7, 0, 0, 0, 0\n", danmakuFontSize))
	b.WriteString("[Events]\n")
	b.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	controller := newDanmakuPositionController()
	for _, item := range items {
		contentLen := csharpStringLength(item.Content)
		height := controller.update(item.Mode, item.Second, contentLen)
		if height < 0 {
			continue
		}
		effect := ""
		switch item.Mode {
		case 3:
			effect = fmt.Sprintf(`\an8\pos(%d, %d)`, danmakuWidth/2, danmakuHeight-danmakuFontSize-height)
		case 2:
			effect = fmt.Sprintf(`\an8\pos(%d, %d)`, danmakuWidth/2, height)
		default:
			effect = fmt.Sprintf(`\move(%d, %d, %d, %d)`, danmakuWidth, height, -contentLen*danmakuFontSize, height)
		}
		// long 2026-06-19 04:21:03：颜色解析失败时原版 Color 保持空字符串，ASS 仍会写出 \c&&，这里保留这个边界输出。
		if item.Color != "FFFFFF" {
			effect += `\c&` + item.Color + `&`
		}
		// long: 原版直接把弹幕正文写入 ASS Dialogue，保留这个行为才能让导出的弹幕文件逐字节贴近 BBDown。
		b.WriteString(fmt.Sprintf("Dialogue: 2,%s,%s,BBDOWN_Style,,0000,0000,0000,,{%s}%s\n", formatASSTime(item.Second), formatASSTime(item.Second+danmakuDuration(item.Mode)), effect, item.Content))
	}
	return os.WriteFile(outputPath, []byte(b.String()), 0o644)
}

type danmakuPositionController struct {
	move   []float64
	top    []float64
	bottom []float64
}

func newDanmakuPositionController() *danmakuPositionController {
	maxLine := danmakuHeight * danmakuProtect / danmakuFontSize / 100
	return &danmakuPositionController{
		move:   make([]float64, maxLine),
		top:    make([]float64, maxLine),
		bottom: make([]float64, maxLine),
	}
}

func (c *danmakuPositionController) update(mode int, second float64, length int) int {
	queue := c.move
	displayTime := danmakuMoveTime * float64(length+5) * danmakuFontSize / float64(danmakuWidth+(length*int(danmakuMoveTime)))
	if mode == 2 {
		queue = c.top
		displayTime = danmakuStillTime
	} else if mode == 3 {
		queue = c.bottom
		displayTime = danmakuStillTime
	}
	for i := range queue {
		if second >= queue[i] {
			queue[i] = second + displayTime
			return i * danmakuFontSize
		}
	}
	return -1
}

func danmakuDuration(mode int) float64 {
	if mode == 2 || mode == 3 {
		return danmakuStillTime
	}
	return danmakuMoveTime
}

func csharpStringLength(s string) int {
	// long: 原版 C# 的 string.Length 按 UTF-16 码元计数，emoji 等补充平面字符会算作 2；ASS 位置计算必须复用这个长度才会和 BBDown 的弹幕轨迹一致。
	return len(utf16.Encode([]rune(s)))
}

func formatASSTime(second float64) string {
	if second < 0 {
		second = 0
	}
	hour := int(second) / 3600
	minute := (int(second) - hour*3600) / 60
	sec := second - float64(hour*3600+minute*60)
	return fmt.Sprintf("%d:%02d:%05.2f", hour, minute, sec)
}
