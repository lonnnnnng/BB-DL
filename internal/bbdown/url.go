package bbdown

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	reAV        = regexp.MustCompile(`av(\d+)`)
	reBV        = regexp.MustCompile(`[Bb][Vv]1(\w+)`)
	reEp        = regexp.MustCompile(`/ep(\d+)`)
	reSS        = regexp.MustCompile(`/ss(\d+)`)
	reUID       = regexp.MustCompile(`space\.bilibili\.com/(\d+)`)
	reGlobalEp  = regexp.MustCompile(`\.bilibili\.tv/\w+/play/\d+/(\d+)`)
	reBangumiMD = regexp.MustCompile(`bangumi/media/(md\d+)`)
	reMD        = regexp.MustCompile(`md(\d+)`)
	reState     = regexp.MustCompile(`window\.__INITIAL_STATE__=([\s\S].*?);\(function\(\)`)
	reQuery     = regexp.MustCompile(`(^|&)?(\w+)=([^&]+)(&|$)?`)
)

func GetQueryString(name, rawURL string) string {
	// long 2026-06-19 04:16:23：原版用正则直接扫 URL 字符串，业务上依赖拿到未 URL 解码的原始参数值，不能用 net/url 自动把 %2F、+ 等字符折叠掉。
	for _, match := range reQuery.FindAllStringSubmatch(rawURL, -1) {
		if len(match) >= 4 && match[2] == name {
			return match[3]
		}
	}
	return ""
}

func ResolveAvid(ctx context.Context, input string, httpc *HTTPClient, cfgOpt ...*Config) (string, error) {
	var cfg *Config
	if len(cfgOpt) > 0 {
		cfg = cfgOpt[0]
	}
	if cfg == nil {
		cfg = NewConfig()
	}
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "http") {
		if strings.Contains(trimmed, "b23.tv") {
			next, err := httpc.ResolveLocation(ctx, trimmed)
			if err != nil {
				return "", err
			}
			if next == trimmed {
				return "", fmt.Errorf("无限重定向")
			}
			trimmed = next
		}
		switch {
		case strings.Contains(trimmed, "video/av"):
			return fixResolvedNumericAvid(ctx, httpc, firstRegexGroup(reAV, trimmed))
		case strings.Contains(strings.ToLower(trimmed), "video/bv"):
			avid, err := decodeBVFromURL(trimmed)
			if err != nil {
				return "", err
			}
			return fixResolvedNumericAvid(ctx, httpc, avid)
		case strings.Contains(trimmed, "/cheese/"):
			return resolveCheese(ctx, trimmed, httpc)
		case strings.Contains(trimmed, "/ep"):
			return "ep:" + firstRegexGroup(reEp, trimmed), nil
		case strings.Contains(trimmed, "/ss"):
			ssid := firstRegexGroup(reSS, trimmed)
			epid, err := GetEpIDByBangumiSSID(ctx, httpc, cfg, ssid)
			if err != nil {
				return "", err
			}
			return "ep:" + epid, nil
		case strings.Contains(trimmed, "/medialist/") && strings.Contains(trimmed, "business_id=") && strings.Contains(trimmed, "business=space_collection"):
			return "listBizId:" + GetQueryString("business_id", trimmed), nil
		case strings.Contains(trimmed, "/medialist/") && strings.Contains(trimmed, "business_id=") && strings.Contains(trimmed, "business=space_series"):
			return "seriesBizId:" + GetQueryString("business_id", trimmed), nil
		case strings.Contains(trimmed, "/channel/collectiondetail?sid="):
			return "listBizId:" + GetQueryString("sid", trimmed), nil
		case strings.Contains(trimmed, "/channel/seriesdetail?sid="):
			return "seriesBizId:" + GetQueryString("sid", trimmed), nil
		case strings.Contains(trimmed, "/space.bilibili.com/") && strings.Contains(trimmed, "/lists/"):
			return resolveSpaceList(trimmed), nil
		case strings.Contains(trimmed, "/space.bilibili.com/") && strings.Contains(trimmed, "/favlist"):
			mid := firstRegexGroup(reUID, trimmed)
			return "favId:" + GetQueryString("fid", trimmed) + ":" + mid, nil
		case strings.Contains(trimmed, "/space.bilibili.com/"):
			return "mid:" + firstRegexGroup(reUID, trimmed), nil
		case strings.Contains(trimmed, "ep_id="):
			return "ep:" + GetQueryString("ep_id", trimmed), nil
		case reGlobalEp.MatchString(trimmed):
			return "ep:" + reGlobalEp.FindStringSubmatch(trimmed)[1], nil
		case reBangumiMD.MatchString(trimmed):
			mdID := reBangumiMD.FindStringSubmatch(trimmed)[1]
			epid, err := GetEpIDByMD(ctx, httpc, mdID)
			if err != nil {
				return "", err
			}
			return "ep:" + epid, nil
		default:
			body, err := httpc.Get(ctx, trimmed)
			if err != nil {
				return "", err
			}
			match := reState.FindSubmatch(body)
			if len(match) < 2 {
				return "", fmt.Errorf("输入有误")
			}
			state, err := ParseJ(match[1])
			if err != nil {
				return "", err
			}
			epList := state.Arr("epList")
			if len(epList) == 0 {
				return "", fmt.Errorf("输入有误")
			}
			return "ep:" + asJ(epList[0]).Str("id"), nil
		}
	}
	switch {
	case strings.HasPrefix(strings.ToLower(trimmed), "bv"):
		avid, err := decodeBV(trimmed)
		if err != nil {
			return "", err
		}
		return fixResolvedNumericAvid(ctx, httpc, avid)
	case strings.HasPrefix(strings.ToLower(trimmed), "av"):
		return fixResolvedNumericAvid(ctx, httpc, trimmed[2:])
	case strings.HasPrefix(trimmed, "cheese/"):
		return resolveCheese(ctx, "https://"+trimmed, httpc)
	case strings.HasPrefix(trimmed, "ep"):
		return "ep:" + trimmed[2:], nil
	case strings.HasPrefix(trimmed, "ss"):
		epid, err := GetEpIDByBangumiSSID(ctx, httpc, cfg, trimmed[2:])
		if err != nil {
			return "", err
		}
		return "ep:" + epid, nil
	case strings.HasPrefix(trimmed, "md"):
		epid, err := GetEpIDByMD(ctx, httpc, firstRegexGroup(reMD, trimmed))
		if err != nil {
			return "", err
		}
		return "ep:" + epid, nil
	default:
		return "", fmt.Errorf("输入有误")
	}
}

func fixResolvedNumericAvid(ctx context.Context, httpc *HTTPClient, avid string) (string, error) {
	// long 2026-06-19 04:06:18：源仓库 GetAvIdAsync 最后会对纯数字 aid 做版权重定向修正，入口解析阶段就要把 av 页面重定向到番剧 ep 的场景改成 ep:xxx。
	return FixAvid(ctx, httpc, avid)
}

func resolveSpaceList(raw string) string {
	u, _ := url.Parse(raw)
	sid := strings.TrimSuffix(strings.TrimPrefix(u.Path[strings.LastIndex(u.Path, "/")+1:], "/"), "/")
	switch strings.ToLower(u.Query().Get("type")) {
	case "series":
		return "seriesBizId:" + sid
	default:
		return "listBizId:" + sid
	}
}

func decodeBVFromURL(raw string) (string, error) {
	// long: 源仓库 video/bv 分支直接把 BVRegex 的分组交给转换器；分组为空时也由 BV 解码器产出长度错误。
	aid, err := DecodeBV(firstRegexGroup(reBV, raw))
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(aid, 10), nil
}

func decodeBV(bv string) (string, error) {
	bv = strings.TrimSpace(bv)
	if strings.HasPrefix(strings.ToLower(bv), "bv1") {
		bv = bv[3:]
	} else if strings.HasPrefix(strings.ToLower(bv), "bv") {
		bv = bv[2:]
	}
	aid, err := DecodeBV(bv)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(aid, 10), nil
}

func resolveCheese(ctx context.Context, raw string, httpc *HTTPClient) (string, error) {
	if strings.Contains(raw, "/ep") {
		return "cheese:" + firstRegexGroup(reEp, raw), nil
	}
	if strings.Contains(raw, "/ss") {
		epid, err := GetEpidBySSID(ctx, httpc, firstRegexGroup(reSS, raw))
		if err != nil {
			return "", err
		}
		return "cheese:" + epid, nil
	}
	return "", fmt.Errorf("无法解析课程链接")
}

func firstRegexGroup(re *regexp.Regexp, raw string) string {
	match := re.FindStringSubmatch(raw)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}
