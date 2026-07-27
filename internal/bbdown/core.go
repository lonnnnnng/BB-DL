package bbdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func AppDir() string {
	exe, err := os.Executable()
	if err == nil && exe != "" {
		return filepath.Dir(exe)
	}
	wd, err := os.Getwd()
	if err == nil {
		return wd
	}
	return "."
}

func ResolveWorkDir(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	expanded := expandPercentEnv(os.ExpandEnv(raw))
	if strings.HasPrefix(expanded, "~") {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			switch {
			case expanded == "~":
				expanded = home
			case strings.HasPrefix(expanded, "~/"):
				expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~/"))
			}
		}
	}
	return filepath.Abs(expanded)
}

func expandPercentEnv(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); {
		if raw[i] != '%' {
			b.WriteByte(raw[i])
			i++
			continue
		}
		end := strings.IndexByte(raw[i+1:], '%')
		if end < 0 {
			b.WriteByte(raw[i])
			i++
			continue
		}
		name := raw[i+1 : i+1+end]
		if name == "" {
			b.WriteString("%%")
			i += 2
			continue
		}
		if value, ok := os.LookupEnv(name); ok {
			b.WriteString(value)
		} else {
			b.WriteString(raw[i : i+end+2])
		}
		i += end + 2
	}
	return b.String()
}

func FormatTimeStamp(ts int64, format string) string {
	if ts == 0 {
		return "null"
	}
	return time.Unix(ts, 0).Local().Format(format)
}

func FormatTime(seconds int, absolute bool) string {
	d := time.Duration(seconds) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if absolute {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	if h == 0 {
		return fmt.Sprintf("%02dm%02ds", m, s)
	}
	return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
}

func FormatFileSize(fileSize float64) string {
	switch {
	case fileSize < 0:
		// long 2026-06-19 04:16:23：原版 FormatFileSize 对负数直接抛 ArgumentOutOfRangeException，下载/展示层不应把异常大小伪装成 0 bytes。
		panic("fileSize must be non-negative")
	case fileSize >= 1024*1024*1024:
		return fmt.Sprintf("%.2f GB", fileSize/(1024*1024*1024))
	case fileSize >= 1024*1024:
		return fmt.Sprintf("%.2f MB", fileSize/(1024*1024))
	case fileSize >= 1024:
		return fmt.Sprintf("%.2f KB", fileSize/1024)
	default:
		// long: 原版小于 1KB 时直接插值 double，不会把 1023.5 bytes 四舍五入成 1024 bytes。
		return strconv.FormatFloat(fileSize, 'f', -1, 64) + " bytes"
	}
}

func RSubString(sub string) string {
	sub = sub[strings.LastIndex(sub, "/")+1:]
	if idx := strings.LastIndex(sub, "."); idx >= 0 {
		return sub[:idx]
	}
	return sub
}

func getMixinKey(orig string) string {
	indexes := []int{46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49, 33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13}
	var b strings.Builder
	b.Grow(32)
	for _, idx := range indexes {
		if idx < len(orig) {
			b.WriteByte(orig[idx])
		}
	}
	return b.String()
}

func CheckLogin(ctx context.Context, httpc *HTTPClient, cfg *Config) (bool, error) {
	body, err := httpc.Get(ctx, "https://api.bilibili.com/x/web-interface/nav")
	if err != nil {
		return false, err
	}
	root, err := ParseJ(body)
	if err != nil {
		return false, err
	}
	data := root.Obj("data")
	wbi := data.Obj("wbi_img")
	cfg.WBISalt = getMixinKey(RSubString(wbi.Str("img_url")) + RSubString(wbi.Str("sub_url")))
	return data.Bool("isLogin"), nil
}

func LoadCredentials(cfg *Config, opt *MyOption) {
	load := func(paths ...string) (string, bool) {
		for _, p := range paths {
			if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
				return strings.TrimSpace(string(b)), true
			}
		}
		return "", false
	}
	if cfg.Cookie == "" {
		if cookie, ok := load(filepath.Join(AppDir(), "BBDown.data"), filepath.Join(".", "BBDown.data")); ok {
			Log("加载本地cookie...")
			cfg.Cookie = cookie
		}
	}
	if cfg.Token == "" {
		var token string
		var ok bool
		if opt.UseTvApi {
			token, ok = load(filepath.Join(AppDir(), "BBDownTV.data"), filepath.Join(".", "BBDownTV.data"))
		} else if opt.UseAppApi {
			token, ok = load(filepath.Join(AppDir(), "BBDownApp.data"), filepath.Join(".", "BBDownApp.data"))
		}
		if ok {
			Log("加载本地token...")
			cfg.Token = strings.TrimPrefix(token, "access_token=")
		}
	}
}

func GetEpidBySSID(ctx context.Context, httpc *HTTPClient, ssid string) (string, error) {
	body, err := httpc.Get(ctx, "https://api.bilibili.com/pugv/view/web/season?season_id="+ssid)
	if err != nil {
		return "", err
	}
	root, err := ParseJ(body)
	if err != nil {
		return "", err
	}
	episodes := root.Obj("data").Arr("episodes")
	if len(episodes) == 0 {
		return "", fmt.Errorf("未找到课程集数")
	}
	return asJ(episodes[0]).Str("id"), nil
}

func GetEpIDByBangumiSSID(ctx context.Context, httpc *HTTPClient, cfg *Config, ssid string) (string, error) {
	body, err := httpc.Get(ctx, fmt.Sprintf("https://%s/pgc/view/web/season?season_id=%s", cfg.EpHost, ssid))
	if err != nil {
		return "", err
	}
	root, err := ParseJ(body)
	if err != nil {
		return "", err
	}
	episodes := root.Obj("result").Arr("episodes")
	if len(episodes) == 0 {
		return "", fmt.Errorf("未找到番剧集数")
	}
	return asJ(episodes[0]).Str("id"), nil
}

func GetEpIDByMD(ctx context.Context, httpc *HTTPClient, mdID string) (string, error) {
	body, err := httpc.Get(ctx, "https://api.bilibili.com/pgc/review/user?media_id="+mdID)
	if err != nil {
		return "", err
	}
	root, err := ParseJ(body)
	if err != nil {
		return "", err
	}
	epid := root.Obj("result").Obj("media").Obj("new_ep").Str("id")
	if epid == "" {
		return "", fmt.Errorf("未找到媒体最新集数")
	}
	return epid, nil
}

func FixAvid(ctx context.Context, httpc *HTTPClient, avid string) (string, error) {
	if strings.IndexFunc(avid, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
		return avid, nil
	}
	loc, err := httpc.ResolveLocation(ctx, "https://www.bilibili.com/video/av"+avid+"/")
	if err != nil {
		return avid, nil
	}
	if strings.Contains(loc, "/ep") {
		// long: 原版只判断 Location 是否包含 /ep，再读取正则分组；分组未匹配时值为空，也会返回 ep: 而不是中断。
		return "ep:" + firstRegexGroup(reEp, loc), nil
	}
	return avid, nil
}

func GetVideoInfo(ctx context.Context, cfg *Config, httpc *HTTPClient, opt *MyOption, aidOri, input string) (string, *VInfo, string, error) {
	LoadCredentials(cfg, opt)
	if !opt.UseIntlApi && !opt.UseTvApi && cfg.Area == "" {
		Log("检测账号登录...")
		ok, err := CheckLogin(ctx, httpc, cfg)
		if err == nil && !ok {
			Warnf("你尚未登录B站账号, 解析可能受到限制")
		}
	}

	var err error
	Log("获取aid...")
	aidOri, err = ResolveAvid(ctx, input, httpc, cfg)
	if err != nil {
		return "", nil, "", err
	}
	aidOri, _ = FixAvid(ctx, httpc, aidOri)
	Logf("获取aid结束: %s", aidOri)
	if aidOri == "" {
		return "", nil, "", fmt.Errorf("输入有误")
	}

	Log("获取视频信息...")
	factory := NewFetcherFactory(httpc, cfg)
	fetcher := factory.CreateFetcher(aidOri, opt.UseIntlApi)
	vInfo, err := fetcher.Fetch(ctx, aidOri)
	if err != nil {
		if strings.Contains(err.Error(), "无法解析") || strings.Contains(err.Error(), "未找到") {
			if strings.HasPrefix(aidOri, "ep:") && !strings.HasPrefix(aidOri, "cheese:") {
				Warnf("未找到此 EP/SS 对应番剧信息, 正在尝试按课程查找。")
				aidOri = strings.Replace(aidOri, "ep:", "cheese:", 1)
				Log("新的 aid: " + aidOri)
				Log("获取视频信息...")
				fetcher = factory.CreateFetcher(aidOri, opt.UseIntlApi)
				vInfo, err = fetcher.Fetch(ctx, aidOri)
			}
		}
	}
	if err != nil {
		return "", nil, "", err
	}

	return aidOri, vInfo, apiTypeFromOptions(opt), nil
}

func PrintVideoInfo(vInfo *VInfo, apiType string, showAll bool) {
	LogColor("视频标题: "+vInfo.Title, true)
	if vInfo.PubTime != 0 {
		Log("发布时间: " + FormatTimeStamp(vInfo.PubTime, "2006-01-02 15:04:05 -0700"))
	}
	if len(vInfo.PagesInfo) > 0 {
		if bvid, err := EncodeBV(mustInt64(vInfo.PagesInfo[0].Aid)); err == nil && apiType != "INTL" {
			Log("视频URL: https://www.bilibili.com/video/" + bvid + "/")
		}
		if mid := firstNonEmptyOwnerMid(vInfo.PagesInfo); mid != "" {
			Log("UP主页: https://space.bilibili.com/" + mid)
		}
	}

	more := false
	for _, p := range vInfo.PagesInfo {
		if !showAll {
			if more && p.Index != len(vInfo.PagesInfo) {
				continue
			}
			if !more && p.Index > 5 {
				Log("......")
				more = true
				continue
			}
		}
		Log(fmt.Sprintf("P%d: [%s] [%s] [%s]", p.Index, p.Cid, p.Title, FormatTime(p.Dur, false)))
	}
}

func ApplySteinGateTVFallback(vInfo *VInfo, opt *MyOption) string {
	if vInfo != nil && opt != nil && vInfo.IsSteinGate && opt.UseTvApi {
		Log("视频为互动视频，暂时不支持tv下载，修改为默认下载")
		opt.UseTvApi = false
		return "WEB"
	}
	return apiTypeFromOptions(opt)
}

func apiTypeFromOptions(opt *MyOption) string {
	switch {
	case opt != nil && opt.UseTvApi:
		return "TV"
	case opt != nil && opt.UseAppApi:
		return "APP"
	case opt != nil && opt.UseIntlApi:
		return "INTL"
	default:
		return "WEB"
	}
}

func PrintTracks(tracks *ParsedTracks, pageDur int, onlyShowInfo bool) {
	if len(tracks.BackgroundAudios) > 0 && len(tracks.RoleAudioLists) > 0 {
		Log(fmt.Sprintf("共计%d条背景音频流.", len(tracks.BackgroundAudios)))
		for i, a := range tracks.BackgroundAudios {
			LogColor(formatAudioTrackLine(fmt.Sprintf("%d.", i), &a, pageDur), false)
		}
		roleAudioCount := 0
		if len(tracks.RoleAudioLists[0].Audio) > 0 {
			roleAudioCount = len(tracks.RoleAudioLists[0].Audio)
		}
		Log(fmt.Sprintf("共计%d条配音, 每条包含%d条配音流.", len(tracks.RoleAudioLists), roleAudioCount))
		for i, a := range tracks.RoleAudioLists[0].Audio {
			LogColor(formatAudioTrackLine(fmt.Sprintf("%d.", i), &a, pageDur), false)
		}
	}
	if len(tracks.VideoTracks) > 0 {
		Log(fmt.Sprintf("共计%d条视频流.", len(tracks.VideoTracks)))
		for i, v := range tracks.VideoTracks {
			LogColor(formatVideoTrackLine(fmt.Sprintf("%d.", i), &v, pageDur), false)
			if onlyShowInfo {
				fmt.Println(v.BaseURL)
			}
		}
	}
	if len(tracks.AudioTracks) > 0 {
		Log(fmt.Sprintf("共计%d条音频流.", len(tracks.AudioTracks)))
		for i, a := range tracks.AudioTracks {
			LogColor(formatAudioTrackLine(fmt.Sprintf("%d.", i), &a, pageDur), false)
			if onlyShowInfo {
				fmt.Println(a.BaseURL)
			}
		}
	}
}

func PrintSelectedTracks(video *Video, audio *Audio, pageDur int) {
	if video != nil {
		LogColor(formatVideoTrackLine("[视频]", video, pageDur), false)
	}
	if audio != nil {
		LogColor(formatAudioTrackLine("[音频]", audio, pageDur), false)
	}
}

func formatVideoTrackLine(prefix string, video *Video, pageDur int) string {
	if video == nil {
		return ""
	}
	pDur := pageDur
	if pDur == 0 {
		pDur = video.Dur
	}
	size := video.Size
	if size <= 0 {
		size = float64(int64(pDur) * video.Bandwidth * 1024 / 8)
	}
	return strings.ReplaceAll(fmt.Sprintf("%s [%s] [%s] [%s] [%s] [%d kbps] [~%s]", prefix, video.Dfn, video.Res, video.Codecs, video.Fps, video.Bandwidth, FormatFileSize(size)), "[] ", "")
}

func formatAudioTrackLine(prefix string, audio *Audio, pageDur int) string {
	if audio == nil {
		return ""
	}
	pDur := pageDur
	if pDur == 0 {
		pDur = audio.Dur
	}
	size := float64(int64(pDur) * audio.Bandwidth * 1024 / 8)
	return fmt.Sprintf("%s [%s] [%d kbps] [~%s]", prefix, audio.Codecs, audio.Bandwidth, FormatFileSize(size))
}

func SortTracksVideo(items []Video, ascending bool) []Video {
	return SortTracksVideoWithPriority(items, nil, nil, ascending)
}

func SortTracksVideoWithPriority(items []Video, dfnPriority map[string]int, encodingPriority map[string]int, ascending bool) []Video {
	return SortTracksVideoWithPriorityOrder(items, dfnPriority, encodingPriority, ascending, false)
}

func SortTracksVideoWithPriorityOrder(items []Video, dfnPriority map[string]int, encodingPriority map[string]int, ascending bool, encodingFirst bool) []Video {
	out := append([]Video(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		leftDfn := priorityValue(dfnPriority, out[i].Dfn)
		rightDfn := priorityValue(dfnPriority, out[j].Dfn)
		leftCodec := priorityValue(encodingPriority, normalizePriorityKey(out[i].Codecs))
		rightCodec := priorityValue(encodingPriority, normalizePriorityKey(out[j].Codecs))
		// long: 原版允许用户通过参数顺序决定“编码优先”或“清晰度优先”，这里把排序主键显式化，避免选流结果和 CLI 输入意图相反。
		if encodingFirst {
			if leftCodec != rightCodec {
				return leftCodec < rightCodec
			}
			if leftDfn != rightDfn {
				return leftDfn < rightDfn
			}
		} else {
			if leftDfn != rightDfn {
				return leftDfn < rightDfn
			}
			if leftCodec != rightCodec {
				return leftCodec < rightCodec
			}
		}
		leftID := mustInt64(out[i].ID)
		rightID := mustInt64(out[j].ID)
		if leftID != rightID {
			return leftID > rightID
		}
		return compareBandwidth(out[i].Bandwidth, out[j].Bandwidth, ascending)
	})
	return out
}

func SortTracksAudio(items []Audio, ascending bool) []Audio {
	return SortTracksAudioWithPriority(items, nil, ascending)
}

func SortTracksAudioWithPriority(items []Audio, encodingPriority map[string]int, ascending bool) []Audio {
	out := append([]Audio(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		leftCodec := priorityValue(encodingPriority, normalizePriorityKey(out[i].Codecs))
		rightCodec := priorityValue(encodingPriority, normalizePriorityKey(out[j].Codecs))
		if leftCodec != rightCodec {
			return leftCodec < rightCodec
		}
		return compareBandwidth(out[i].Bandwidth, out[j].Bandwidth, ascending)
	})
	return out
}

func ParseEncodingPriority(raw string) map[string]int {
	items := encodingPriorityItems(raw)
	return parsePriorityItems(items)
}

func FirstEncodingPriority(raw string) string {
	items := encodingPriorityItems(raw)
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func ParseDfnPriority(raw string) map[string]int {
	return parsePriorityList(strings.NewReplacer("，", ",").Replace(strings.ToUpper(raw)))
}

func encodingPriorityItems(raw string) []string {
	normalized := strings.NewReplacer("，", ",", "-", "").Replace(strings.ToUpper(raw))
	items := splitPriorityItems(normalized)
	if raw != "" && len(items) == 0 {
		// long: 原版显式设置编码优先级后会读取 First()，空白或逗号组成的空列表应暴露参数错误，不能悄悄回退默认编码选择。
		panic("encoding priority contains no valid entries")
	}
	return items
}

func parsePriorityList(raw string) map[string]int {
	return parsePriorityItems(splitPriorityItems(raw))
}

func splitPriorityItems(raw string) []string {
	items := []string{}
	for _, item := range strings.Split(raw, ",") {
		key := strings.TrimSpace(item)
		if key != "" {
			items = append(items, key)
		}
	}
	return items
}

func parsePriorityItems(items []string) map[string]int {
	out := map[string]int{}
	for _, key := range items {
		if _, exists := out[key]; !exists {
			out[key] = len(out)
		}
	}
	return out
}

func priorityValue(priority map[string]int, key string) int {
	if len(priority) == 0 {
		return 100
	}
	if v, ok := priority[strings.ToUpper(key)]; ok {
		return v
	}
	return 100
}

func normalizePriorityKey(key string) string {
	return strings.NewReplacer("-", "").Replace(strings.ToUpper(strings.TrimSpace(key)))
}

func compareBandwidth(left, right int64, ascending bool) bool {
	if ascending {
		return left < right
	}
	return left > right
}

func firstNonEmptyOwnerMid(pages []Page) string {
	for _, p := range pages {
		if p.OwnerMid != "" {
			return p.OwnerMid
		}
	}
	return ""
}

func mustInt64(s string) int64 {
	n, err := parseInt64(s)
	if err != nil {
		panic(err)
	}
	return n
}
