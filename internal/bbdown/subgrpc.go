package bbdown

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"google.golang.org/protobuf/encoding/protowire"
)

const appGrpcDmViewAPI = "https://app.biliapi.net/bilibili.community.service.dm.v1.DM/DmView"

func getSubtitlesFromGrpc(ctx context.Context, httpc *HTTPClient, cfg *Config, aid, cid string) []Subtitle {
	payload, err := buildDmViewPayload(aid, cid)
	if err != nil {
		return nil
	}
	body, err := appGrpcPost(ctx, httpc, appGrpcDmViewAPI, payload, appGrpcHeaders(cfg.Token, "app.biliapi.net"))
	if err != nil {
		return nil
	}
	message, err := unpackGrpcMessage(body)
	if err != nil {
		return nil
	}
	return parseDmViewSubtitles(aid, cid, message)
}

func buildDmViewPayload(aid, cid string) ([]byte, error) {
	aidNum, err := strconv.ParseInt(aid, 10, 64)
	if err != nil {
		return nil, err
	}
	cidNum, err := strconv.ParseInt(cid, 10, 64)
	if err != nil {
		return nil, err
	}
	var msg []byte
	msg = protowire.AppendTag(msg, 1, protowire.VarintType)
	msg = protowire.AppendVarint(msg, uint64(aidNum))
	msg = protowire.AppendTag(msg, 2, protowire.VarintType)
	msg = protowire.AppendVarint(msg, uint64(cidNum))
	msg = protowire.AppendTag(msg, 3, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 1)
	// long: DmView 是 App 播放页弹幕/字幕入口，spmid 固定成视频详情页来源，才能贴近原版客户端请求并拿到未登录场景下仍可见的字幕列表。
	msg = appendProtoString(msg, 4, "main.ugc-video-detail.0.0")
	return packGrpcMessage(msg)
}

func parseDmViewSubtitles(aid, cid string, data []byte) []Subtitle {
	subs := []Subtitle{}
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		if num != 3 {
			return
		}
		subs = append(subs, parseDmViewVideoSubtitle(aid, cid, value)...)
	})
	if hasEmptySubtitleURL(subs) {
		// long: App gRPC 字幕对应源仓库 GetSubtitlesFromApi3Async，整批解析完发现空 URL 就把该接口视为失败，交给上层尝试下一路。
		return nil
	}
	return subs
}

func parseDmViewVideoSubtitle(aid, cid string, data []byte) []Subtitle {
	subs := []Subtitle{}
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		if num != 3 {
			return
		}
		sub, ok := parseDmViewSubtitleItem(aid, cid, value)
		if ok {
			subs = append(subs, sub)
		}
	})
	return subs
}

func parseDmViewSubtitleItem(aid, cid string, data []byte) (Subtitle, bool) {
	var lan, subURL string
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		switch num {
		case 3:
			lan = string(value)
		case 5:
			subURL = string(value)
		}
	})
	if lan == "" && subURL == "" {
		return Subtitle{}, false
	}
	return Subtitle{
		Lan:  lan,
		URL:  subURL,
		Path: filepath.Join(aid, fmt.Sprintf("%s.%s.%s.srt", aid, cid, lan)),
	}, true
}
