package bbdown

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var reBaseURL = regexp.MustCompile(`http.*:\d+`)

func WbiSign(api string, cfg *Config) string {
	sum := md5.Sum([]byte(api + cfg.WBISalt))
	return fmt.Sprintf("%s&w_rid=%s", api, hex.EncodeToString(sum[:]))
}

func GetVideoCodec(code string) string {
	switch code {
	case "13":
		return "AV1"
	case "12":
		return "HEVC"
	case "7":
		return "AVC"
	default:
		return "UNKNOWN"
	}
}

func GetMaxQn() string {
	keys := make([]string, 0, len(QualityMap))
	for k := range QualityMap {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys[0]
}

func sortStrings(items []string) {
	sort.Slice(items, func(i, j int) bool {
		ii, _ := strconv.Atoi(items[i])
		jj, _ := strconv.Atoi(items[j])
		return ii > jj
	})
}

func GetTimeStamp(seconds bool) string {
	now := timeNow()
	if seconds {
		return strconv.FormatInt(now.Unix(), 10)
	}
	return strconv.FormatInt(now.UnixMilli(), 10)
}

type nowTime interface {
	Unix() int64
	UnixMilli() int64
}

func timeNow() nowTime { return realNowTime{} }

type realNowTime struct{}

func (realNowTime) Unix() int64      { return time.Now().Unix() }
func (realNowTime) UnixMilli() int64 { return time.Now().UnixMilli() }

func GetSign(parms string, isBiliPlus bool) string {
	suffix := "59b43e04ad6965f34319062b478f83dd"
	if isBiliPlus {
		suffix = "acd495b248ec528c2eed1e862d393126"
	}
	sum := md5.Sum([]byte(parms + suffix))
	return hex.EncodeToString(sum[:])
}

func GetAppSign(params string) string {
	sum := md5.Sum([]byte(params + "560c52ccd288fed045859ed18bffd973"))
	return hex.EncodeToString(sum[:])
}

type ParsedTracks struct {
	WebJSONString    string
	VideoTracks      []Video
	AudioTracks      []Audio
	BackgroundAudios []Audio
	RoleAudioLists   []AudioMaterialInfo
	ExtraPoints      []ViewPoint
	Clips            []string
	Dfns             []string
}

func ExtractTracks(ctx context.Context, cfg *Config, httpc *HTTPClient, aidOri, aid, cid, epID string, tvApi, intlApi, appApi bool, encoding string, qn string) (*ParsedTracks, error) {
	if qn == "" {
		qn = "0"
	}
	res := &ParsedTracks{}
	body, err := getPlayJSON(ctx, cfg, httpc, aidOri, aid, cid, epID, tvApi, intlApi, appApi, encoding, qn)
	if err != nil {
		return nil, err
	}
	res.WebJSONString = body
	if cfg.Debug {
		if _, err := writeDebugPlayJSON(body); err != nil {
			return nil, err
		}
	}
	if strings.Contains(body, "\"dash\":{") || strings.Contains(body, "\"stream_list\"") {
		return parseDashLike(ctx, httpc, body, cfg, aidOri, aid, cid, epID, tvApi, intlApi, appApi, res, encoding, qn)
	}
	if strings.Contains(body, "\"durl\":[") {
		return parseFLVLike(ctx, httpc, cfg, aidOri, aid, cid, epID, tvApi, intlApi, appApi, res, encoding, qn)
	}
	return res, nil
}

func FetchPoints(ctx context.Context, httpc *HTTPClient, cfg *Config, cid, aid string) []ViewPoint {
	// long: 普通视频章节是独立接口返回的附加信息，拿不到时不能影响主视频下载，所以这里保持静默降级。
	if httpc == nil {
		return nil
	}
	api := fmt.Sprintf("https://api.bilibili.com/x/player/wbi/v2?cid=%s&aid=%s", cid, aid)
	body, err := httpc.Get(ctx, api)
	if err != nil {
		return nil
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil
	}
	data := root.Obj("data")
	if data == nil {
		return nil
	}
	raw := data.Arr("view_points")
	if len(raw) == 0 {
		return nil
	}
	points := make([]ViewPoint, 0, len(raw))
	for _, item := range raw {
		node := asJ(item)
		if node == nil {
			continue
		}
		points = append(points, ViewPoint{
			Title: node.Str("content"),
			Start: int(node.Int64("from")),
			End:   int(node.Int64("to")),
		})
	}
	return points
}

func writeDebugPlayJSON(body string) (string, error) {
	path := "debug_" + time.Now().Format("20060102150405000") + ".json"
	// long: 原版 --debug 会把播放接口原始 JSON 落盘，解析失败时这个文件是复现 B 站接口返回的关键证据。
	return path, os.WriteFile(path, []byte(body), 0o644)
}

func getPlayJSON(ctx context.Context, cfg *Config, httpc *HTTPClient, aidOri, aid, cid, epID string, tvApi, intlApi, appApi bool, encoding, qn string) (string, error) {
	if intlApi {
		return getIntlPlayJSON(ctx, cfg, httpc, aid, cid, epID, qn, "0")
	}
	cheese := strings.HasPrefix(aidOri, "cheese:")
	bangumi := cheese || strings.HasPrefix(aidOri, "ep:")
	if appApi {
		if body, err := getAppGrpcPlayJSON(ctx, cfg, httpc, aidOri, aid, cid, epID, qn, encoding); err == nil {
			return body, nil
		} else if cfg.Debug {
			Warnf("APP gRPC解析失败，回退到APP REST: %v", err)
		}
		return getAppPlayJSON(ctx, cfg, httpc, aidOri, aid, cid, epID, qn)
	}
	if tvApi {
		prefix := fmt.Sprintf("https://%s/x/tv/playurl?", cfg.TvHost)
		if bangumi {
			prefix = fmt.Sprintf("https://%s/pgc/player/api/playurltv?", cfg.TvHost)
		}
		apiBuilder := strings.Builder{}
		if cfg.Token != "" {
			apiBuilder.WriteString("access_key=")
			apiBuilder.WriteString(cfg.Token)
			apiBuilder.WriteString("&")
		}
		apiBuilder.WriteString("appkey=4409e2ce8ffd12b8&build=106500&cid=")
		apiBuilder.WriteString(cid)
		apiBuilder.WriteString("&device=android")
		if bangumi {
			apiBuilder.WriteString("&ep_id=")
			apiBuilder.WriteString(epID)
			apiBuilder.WriteString("&expire=0")
		}
		apiBuilder.WriteString("&fnval=4048&fnver=0&fourk=1&mid=0&mobi_app=android_tv_yst")
		apiBuilder.WriteString("&object_id=")
		apiBuilder.WriteString(aid)
		apiBuilder.WriteString("&platform=android&playurl_type=1&qn=")
		apiBuilder.WriteString(qn)
		apiBuilder.WriteString("&ts=")
		apiBuilder.WriteString(GetTimeStamp(true))
		body, err := httpc.Get(ctx, prefix+apiBuilder.String()+"&sign="+GetSign(apiBuilder.String(), false))
		return string(body), err
	}

	prefix := "https://api.bilibili.com/x/player/wbi/playurl?"
	if bangumi {
		prefix = fmt.Sprintf("https://%s/pgc/player/web/v2/playurl?", cfg.Host)
	}
	apiBuilder := strings.Builder{}
	apiBuilder.WriteString("support_multi_audio=true&from_client=BROWSER&avid=")
	apiBuilder.WriteString(aid)
	apiBuilder.WriteString("&cid=")
	apiBuilder.WriteString(cid)
	apiBuilder.WriteString("&fnval=4048&fnver=0&fourk=1")
	if cfg.Area != "" {
		apiBuilder.WriteString("&access_key=")
		apiBuilder.WriteString(cfg.Token)
		apiBuilder.WriteString("&area=")
		apiBuilder.WriteString(cfg.Area)
	}
	apiBuilder.WriteString("&otype=json&qn=")
	apiBuilder.WriteString(qn)
	if bangumi {
		apiBuilder.WriteString("&module=bangumi&ep_id=")
		apiBuilder.WriteString(epID)
		apiBuilder.WriteString("&session=")
	}
	if cfg.Cookie == "" {
		apiBuilder.WriteString("&try_look=1")
	}
	apiBuilder.WriteString("&wts=")
	apiBuilder.WriteString(GetTimeStamp(true))
	param := apiBuilder.String()
	if bangumi {
		body, err := httpc.Get(ctx, prefix+param)
		return string(body), err
	}
	body, err := httpc.Get(ctx, prefix+WbiSign(param, cfg))
	bodyText := string(body)
	if err != nil {
		return bodyText, err
	}
	if shouldFallbackToLegacyUGCPlayURL(bodyText) {
		legacyBody, legacyErr := getLegacyUGCPlayJSON(ctx, cfg, httpc, aid, cid, qn)
		if legacyErr == nil && hasPlayablePlayURL(legacyBody) {
			return legacyBody, nil
		}
		if cfg.Debug && legacyErr != nil {
			Warnf("WEB WBI播放地址为空，旧播放接口回退失败: %v", legacyErr)
		}
	}
	return bodyText, nil
}

func getLegacyUGCPlayJSON(ctx context.Context, cfg *Config, httpc *HTTPClient, aid, cid, qn string) (string, error) {
	prefix := fmt.Sprintf("https://%s/x/player/playurl?", cfg.Host)
	apiBuilder := strings.Builder{}
	apiBuilder.WriteString("avid=")
	apiBuilder.WriteString(aid)
	apiBuilder.WriteString("&cid=")
	apiBuilder.WriteString(cid)
	apiBuilder.WriteString("&fnval=4048&fnver=0&fourk=1&otype=json&qn=")
	apiBuilder.WriteString(qn)
	if cfg.Cookie == "" {
		apiBuilder.WriteString("&try_look=1")
	}
	body, err := httpc.Get(ctx, prefix+apiBuilder.String())
	return string(body), err
}

func hasPlayablePlayURL(body string) bool {
	return strings.Contains(body, "\"dash\":{") || strings.Contains(body, "\"durl\":[") || strings.Contains(body, "\"stream_list\"")
}

func shouldFallbackToLegacyUGCPlayURL(body string) bool {
	if hasPlayablePlayURL(body) {
		return false
	}
	root, top, err := parsePlayRoot(body)
	if err != nil || root.Int("code") != 0 || top == nil {
		return false
	}
	if top.Str("v_voucher") != "" {
		return true
	}
	// long: 一些早期 UGC 在 WBI 播放接口只返回空 data，旧接口仍能给出 FLV durl；只在 code=0 且没有任何可播放结构时补走旧接口。
	return top.Obj("dash") == nil && len(top.Arr("durl")) == 0 && !strings.Contains(body, "\"stream_list\"")
}

func getAppPlayJSON(ctx context.Context, cfg *Config, httpc *HTTPClient, aidOri, aid, cid, epID, qn string) (string, error) {
	cheese := strings.HasPrefix(aidOri, "cheese:")
	bangumi := cheese || strings.HasPrefix(aidOri, "ep:")
	prefix := "https://api.bilibili.com/x/player/playurl?"
	if bangumi {
		prefix = "https://api.bilibili.com/pgc/player/api/playurl?"
	}
	if cheese {
		prefix = "https://api.bilibili.com/pugv/player/web/playurl?"
	}
	apiBuilder := strings.Builder{}
	if cfg.Token != "" {
		apiBuilder.WriteString("access_key=")
		apiBuilder.WriteString(cfg.Token)
		apiBuilder.WriteString("&")
	}
	apiBuilder.WriteString("appkey=1d8b6e7d45233436&build=7320200&cid=")
	apiBuilder.WriteString(cid)
	apiBuilder.WriteString("&device=android&fnval=4048&fnver=0&fourk=1")
	if bangumi {
		apiBuilder.WriteString("&ep_id=")
		apiBuilder.WriteString(epID)
		apiBuilder.WriteString("&module=bangumi")
	}
	if cheese {
		apiBuilder.WriteString("&avid=")
		apiBuilder.WriteString(aid)
		apiBuilder.WriteString("&ep_id=")
		apiBuilder.WriteString(epID)
	} else if !bangumi {
		apiBuilder.WriteString("&avid=")
		apiBuilder.WriteString(aid)
	}
	apiBuilder.WriteString("&mobi_app=android&platform=android&qn=")
	apiBuilder.WriteString(qn)
	apiBuilder.WriteString("&ts=")
	apiBuilder.WriteString(GetTimeStamp(true))
	param := apiBuilder.String()
	body, err := httpc.Get(ctx, prefix+param+"&sign="+GetAppSign(param))
	return string(body), err
}

func getIntlPlayJSON(ctx context.Context, cfg *Config, httpc *HTTPClient, aid, cid, epID, qn, preferCode string) (string, error) {
	host := "api.biliintl.com"
	isBiliPlus := cfg.Host != "api.bilibili.com"
	if isBiliPlus {
		host = cfg.Host
	}
	api := fmt.Sprintf("https://%s/intl/gateway/v2/ogv/playurl?", host)
	paramBuilder := strings.Builder{}
	if cfg.Token != "" {
		paramBuilder.WriteString("access_key=")
		paramBuilder.WriteString(cfg.Token)
		paramBuilder.WriteString("&")
	}
	paramBuilder.WriteString("aid=")
	paramBuilder.WriteString(aid)
	if isBiliPlus {
		paramBuilder.WriteString("&appkey=7d089525d3611b1c&area=")
		if cfg.Area == "" {
			paramBuilder.WriteString("th")
		} else {
			paramBuilder.WriteString(cfg.Area)
		}
	}
	paramBuilder.WriteString("&cid=")
	paramBuilder.WriteString(cid)
	paramBuilder.WriteString("&ep_id=")
	paramBuilder.WriteString(epID)
	paramBuilder.WriteString("&platform=android&prefer_code_type=")
	paramBuilder.WriteString(preferCode)
	paramBuilder.WriteString("&qn=")
	paramBuilder.WriteString(qn)
	if isBiliPlus {
		paramBuilder.WriteString("&ts=")
		paramBuilder.WriteString(GetTimeStamp(true))
	}
	paramBuilder.WriteString("&s_locale=zh_SG")
	param := paramBuilder.String()
	if isBiliPlus {
		api += param + "&sign=" + GetSign(param, true)
	} else {
		api += param
	}
	body, err := httpc.Get(ctx, api)
	return string(body), err
}

func parseDashLike(ctx context.Context, httpc *HTTPClient, body string, cfg *Config, aidOri, aid, cid, epID string, tvApi, intlApi, appApi bool, res *ParsedTracks, encoding, qn string) (*ParsedTracks, error) {
	root, top, err := parsePlayRoot(body)
	if err != nil {
		return nil, err
	}
	if strings.Contains(body, "\"stream_list\"") {
		pDur := appendIntlStreamTracks(root, res)
		if intlApi {
			// long: 国际版接口的 prefer_code_type=0/1 会返回不同编码集合，原版固定补抓一次 code=1 后合并，避免只看到单一编码的视频流。
			reparsedBody, err := getIntlPlayJSON(ctx, cfg, httpc, aid, cid, epID, qn, "1")
			if err != nil {
				return nil, err
			}
			res.WebJSONString = reparsedBody
			nextRoot, _, err := parsePlayRoot(reparsedBody)
			if err != nil {
				return nil, err
			}
			appendIntlStreamTracks(nextRoot, res)
		}
		parseDubbingInfo(findDubbingInfo(root, top), pDur, aid, cid, res)
		return res, nil
	}

	bangumi := strings.HasPrefix(aidOri, "ep:")
	rootNode := top
	pDur := rootNode.Obj("dash").Int("duration")
	if pDur == 0 {
		pDur = rootNode.Int("timelength") / 1000
	}

	video := arrItems(rootNode.Obj("dash").Arr("video"))
	audio := collectDashAudio(rootNode, tvApi)
	if appApi && bangumi {
		parseDubbingInfo(findDubbingInfo(root, rootNode), pDur, aid, cid, res)
	}
	appendDashVideos(res, video, pDur, !tvApi && !appApi)
	if len(res.VideoTracks) > 0 && !appApi {
		// long: 原版会额外用最高 qn 补抓一次非 APP DASH 轨道，用于发现免二压或更完整的可选视频流；音频也以补抓响应为准。
		reparsedBody, err := getPlayJSON(ctx, cfg, httpc, aidOri, aid, cid, epID, tvApi, intlApi, appApi, encoding, GetMaxQn())
		if err != nil {
			return nil, err
		}
		res.WebJSONString = reparsedBody
		root, top, err = parsePlayRoot(reparsedBody)
		if err != nil {
			return nil, err
		}
		rootNode = top
		video = arrItems(rootNode.Obj("dash").Arr("video"))
		audio = collectDashAudio(rootNode, tvApi)
		appendDashVideos(res, video, pDur, !tvApi && !appApi)
	}
	for _, node := range audio {
		urls := append([]string{node.Str("base_url")}, toStrings(node.Arr("backup_url"))...)
		codecs := node.Str("codecs")
		switch codecs {
		case "mp4a.40.2", "mp4a.40.5":
			codecs = "M4A"
		case "ec-3":
			codecs = "E-AC-3"
		case "fLaC":
			codecs = "FLAC"
		}
		res.AudioTracks = appendUniqueAudio(res.AudioTracks, Audio{
			ID:        node.Str("id"),
			Dfn:       node.Str("id"),
			Bandwidth: node.Int64("bandwidth") / 1000,
			BaseURL:   firstGoodURL(urls),
			Codecs:    codecs,
			Dur:       pDur,
		})
	}
	if bangumi {
		res.ExtraPoints = buildBangumiChapterPoints(rootNode["clip_info_list"])
	}
	return res, nil
}

func parsePlayRoot(body string) (J, J, error) {
	var raw any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, nil, err
	}
	root := asJ(raw)
	nodeName := "data"
	if root.Obj("result") != nil {
		nodeName = "result"
	}
	if strings.Contains(body, "\"video_info\":{") {
		nodeName = "video_info"
	}
	top := root
	if nodeName == "result" {
		top = root.Obj("result")
	} else if nodeName == "video_info" {
		top = root.Obj("result").Obj("video_info")
	} else if root.Obj("data") != nil {
		top = root.Obj("data")
	}
	return root, top, nil
}

func appendDashVideos(res *ParsedTracks, video []J, pDur int, includeWebDetails bool) {
	for _, node := range video {
		urls := append([]string{node.Str("base_url")}, toStrings(node.Arr("backup_url"))...)
		v := Video{
			ID:        node.Str("id"),
			Dfn:       QualityMap[node.Str("id")],
			Bandwidth: node.Int64("bandwidth") / 1000,
			BaseURL:   firstGoodURL(urls),
			Codecs:    GetVideoCodec(node.Str("codecid")),
			Dur:       pDur,
			Size:      float64(node.Int64("size")),
		}
		if includeWebDetails {
			v.Res = fmt.Sprintf("%sx%s", node.Str("width"), node.Str("height"))
			v.Fps = node.Str("frame_rate")
		}
		res.VideoTracks = appendUniqueVideo(res.VideoTracks, v)
	}
}

func collectDashAudio(rootNode J, tvApi bool) []J {
	dash := rootNode.Obj("dash")
	rawAudio := dash.Arr("audio")
	if rawAudio == nil {
		return nil
	}
	audio := arrItems(rawAudio)
	if tvApi {
		return audio
	}
	if dolbyAudio := arrItems(dash.Obj("dolby").Arr("audio")); len(dolbyAudio) > 0 {
		// long: WEB/APP DASH 会把杜比音频放在 dash.dolby.audio，缺少这一步会导致用户看不到 E-AC-3 音轨。
		audio = append(audio, dolbyAudio...)
	}
	if flacAudio := dash.Obj("flac").Obj("audio"); flacAudio != nil {
		// long: Hi-Res 无损音频是单个 dash.flac.audio 对象，和普通音频一起参与后续排序和下载选择。
		audio = append(audio, flacAudio)
	}
	return audio
}

func findDubbingInfo(root, top J) J {
	candidates := []J{
		root.Obj("dubbing_info"),
		root.Obj("data").Obj("dubbing_info"),
		root.Obj("result").Obj("dubbing_info"),
		top.Obj("dubbing_info"),
	}
	for _, candidate := range candidates {
		if candidate != nil {
			// long: APP/gRPC、REST 和代理返回对 dubbing_info 的包裹层级不完全一致；统一查找可避免漏掉背景音和角色配音。
			return candidate
		}
	}
	return nil
}

func appendIntlStreamTracks(root J, res *ParsedTracks) int {
	videoInfo := root.Obj("data").Obj("video_info")
	pDur := videoInfo.Int("timelength") / 1000
	for _, stream := range arrItems(videoInfo.Arr("stream_list")) {
		dashVideo := stream.Obj("dash_video")
		if dashVideo == nil || dashVideo.Str("base_url") == "" {
			continue
		}
		quality := stream.Obj("stream_info").Str("quality")
		urls := append([]string{dashVideo.Str("base_url")}, toStrings(dashVideo.Arr("backup_url"))...)
		res.VideoTracks = appendUniqueVideoWithURL(res.VideoTracks, Video{
			ID:        quality,
			Dfn:       QualityMap[quality],
			Bandwidth: dashVideo.Int64("bandwidth") / 1000,
			BaseURL:   firstGoodURL(urls),
			Codecs:    GetVideoCodec(dashVideo.Str("codecid")),
			Dur:       pDur,
			Size:      float64(dashVideo.Int64("size")),
		})
	}
	for _, node := range arrItems(videoInfo.Arr("dash_audio")) {
		urls := append([]string{node.Str("base_url")}, toStrings(node.Arr("backup_url"))...)
		res.AudioTracks = appendUniqueAudio(res.AudioTracks, Audio{
			ID:        node.Str("id"),
			Dfn:       node.Str("id"),
			Bandwidth: node.Int64("bandwidth") / 1000,
			BaseURL:   firstGoodURL(urls),
			Codecs:    "M4A",
			Dur:       pDur,
		})
	}
	return pDur
}

func parseDubbingInfo(dubbing J, pDur int, aid, cid string, res *ParsedTracks) {
	if dubbing == nil || res == nil {
		return
	}
	for _, node := range arrItems(dubbing.Arr("background_audio")) {
		urls := append([]string{node.Str("base_url")}, toStrings(node.Arr("backup_url"))...)
		res.BackgroundAudios = append(res.BackgroundAudios, Audio{
			ID:        node.Str("id"),
			Dfn:       node.Str("id"),
			Bandwidth: node.Int64("bandwidth") / 1000,
			BaseURL:   firstGoodURL(urls),
			Codecs:    node.Str("codecs"),
			Dur:       pDur,
		})
	}
	for _, role := range arrItems(dubbing.Arr("role_audio_list")) {
		var audios []Audio
		for _, node := range arrItems(role.Arr("audio")) {
			audioID := node.Str("id")
			urls := append([]string{node.Str("base_url")}, toStrings(node.Arr("backup_url"))...)
			audios = append(audios, Audio{
				ID:        audioID,
				Dfn:       audioID,
				Bandwidth: node.Int64("bandwidth") / 1000,
				BaseURL:   firstGoodURL(urls),
				Codecs:    node.Str("codecs"),
				Dur:       pDur,
			})
		}
		personName := role.Str("person_name")
		if personName == "" {
			personName = role.Str("personName")
		}
		rolePath := role.Str("path")
		if rolePath == "" {
			audioID := role.Str("audio_id")
			rolePath = fmt.Sprintf("%s/%s.%s.%s.m4a", aid, aid, cid, audioID)
		}
		res.RoleAudioLists = append(res.RoleAudioLists, AudioMaterialInfo{
			Title:      role.Str("title"),
			PersonName: personName,
			Path:       rolePath,
			Audio:      audios,
		})
	}
}

func parseFLVLike(ctx context.Context, httpc *HTTPClient, cfg *Config, aidOri, aid, cid, epID string, tvApi, intlApi, appApi bool, res *ParsedTracks, encoding, qn string) (*ParsedTracks, error) {
	// long: 原版 FLV 路径默认重新请求最高 qn 再解析，避免初始 qn=0 时只拿到低清或不完整分段。
	reparsedBody, err := getPlayJSON(ctx, cfg, httpc, aidOri, aid, cid, epID, tvApi, intlApi, appApi, encoding, GetMaxQn())
	if err != nil {
		return nil, err
	}
	res.WebJSONString = reparsedBody
	_, root, err := parsePlayRoot(reparsedBody)
	if err != nil {
		return nil, err
	}
	quality := root.Str("quality")
	codecid := root.Str("video_codecid")
	var size float64
	var length float64
	for _, node := range arrItems(root.Arr("durl")) {
		res.Clips = append(res.Clips, node.Str("url"))
		size += float64(node.Int64("size"))
		length += float64(node.Int64("length"))
	}
	res.VideoTracks = appendUniqueVideo(res.VideoTracks, Video{
		ID:     quality,
		Dfn:    QualityMap[quality],
		Codecs: GetVideoCodec(codecid),
		Dur:    int(length) / 1000,
		Size:   size,
	})
	if qnExtras := root.Arr("qn_extras"); len(qnExtras) > 0 {
		for _, item := range arrItems(qnExtras) {
			if qn := item.Str("qn"); qn != "" {
				res.Dfns = append(res.Dfns, qn)
			}
		}
	} else if accept := root.Arr("accept_quality"); len(accept) > 0 {
		for _, q := range accept {
			if s := fmt.Sprint(q); s != "" {
				res.Dfns = append(res.Dfns, s)
			}
		}
	}
	if strings.HasPrefix(aidOri, "ep:") {
		res.ExtraPoints = buildBangumiChapterPoints(root["clip_info_list"])
	}
	return res, nil
}

func buildBangumiChapterPoints(raw any) []ViewPoint {
	var clips []J
	switch v := raw.(type) {
	case []any:
		clips = arrItems(v)
	case map[string]any:
		if items, ok := v["items"].([]any); ok {
			clips = arrItems(items)
		}
	}
	if len(clips) == 0 {
		return nil
	}
	points := make([]ViewPoint, 0, len(clips))
	for _, clip := range clips {
		points = append(points, ViewPoint{
			Title: strings.ReplaceAll(clip.Str("toastText"), "即将跳过", ""),
			Start: clip.Int("start"),
			End:   clip.Int("end"),
		})
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].Start < points[j].Start
	})
	chapters := make([]ViewPoint, 0, len(points)*2)
	lastEnd := 0
	for _, point := range points {
		// long: 原版会把 B 站的“即将跳过片头/片尾”点补成完整章节，混流后播放器才能显示“正片 -> 片头 -> 正片 -> 片尾”的时间线。
		if lastEnd < point.Start {
			chapters = append(chapters, ViewPoint{Title: "正片", Start: lastEnd, End: point.Start})
		}
		chapters = append(chapters, point)
		lastEnd = point.End
	}
	return chapters
}

func firstGoodURL(urls []string) string {
	for _, u := range urls {
		if !reBaseURL.MatchString(u) {
			return u
		}
	}
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func toStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func appendUniqueVideo(list []Video, item Video) []Video {
	for _, v := range list {
		if sameVideoTrack(v, item) {
			return list
		}
	}
	return append(list, item)
}

func appendUniqueVideoWithURL(list []Video, item Video) []Video {
	for _, v := range list {
		if sameVideoTrack(v, item) && v.BaseURL == item.BaseURL && v.Size == item.Size {
			return list
		}
	}
	return append(list, item)
}

func appendUniqueAudio(list []Audio, item Audio) []Audio {
	for _, v := range list {
		if sameAudioTrack(v, item) {
			return list
		}
	}
	return append(list, item)
}

func sameVideoTrack(a, b Video) bool {
	// long 2026-06-21 23:54:38：原版 Video.Equals 不比较 baseUrl 和 size；补抓最高 qn 时同一轨道可能换 CDN URL，但用户可见清晰度列表不能因此重复。
	return a.ID == b.ID &&
		a.Dfn == b.Dfn &&
		a.Res == b.Res &&
		a.Fps == b.Fps &&
		a.Codecs == b.Codecs &&
		a.Bandwidth == b.Bandwidth &&
		a.Dur == b.Dur
}

func sameAudioTrack(a, b Audio) bool {
	// long 2026-06-21 23:54:38：原版 Audio.Equals 同样忽略 baseUrl，同一音轨的不同 CDN 地址只保留一条可选项。
	return a.ID == b.ID &&
		a.Dfn == b.Dfn &&
		a.Codecs == b.Codecs &&
		a.Bandwidth == b.Bandwidth &&
		a.Dur == b.Dur
}
