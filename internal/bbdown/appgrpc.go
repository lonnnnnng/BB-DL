package bbdown

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	appGrpcUGCAPI = "https://grpc.biliapi.net/bilibili.app.playurl.v1.PlayURL/PlayView"
	appGrpcPGCAPI = "https://app.bilibili.com/bilibili.pgc.gateway.player.v2.PlayURL/PlayView"
)

type appPlayViewReply struct {
	Timelength       int64
	Videos           []appVideo
	Audios           []appAudio
	Flac             *appAudio
	Dolby            *appAudio
	Clips            []ViewPoint
	BackgroundAudios []Audio
	RoleAudios       []appRoleAudio
}

type appVideo struct {
	Quality   string
	BaseURL   string
	BackupURL []string
	Bandwidth int64
	Codecid   string
	Size      int64
}

type appAudio struct {
	ID        string
	BaseURL   string
	BackupURL []string
	Bandwidth int64
	Codecs    string
}

type appRoleAudio struct {
	AudioID    string
	Title      string
	PersonName string
	Audio      []Audio
}

func getAppGrpcPlayJSON(ctx context.Context, cfg *Config, httpc *HTTPClient, aidOri, aid, cid, epID, qn, encodingName string) (string, error) {
	bangumi := stringsHasAnyPrefix(aidOri, "ep:", "cheese:")
	if bangumi && encodingName != "" && encodingName != "HEVC" {
		Warnf("APP的番剧不支持 HEVC 以外的编码")
	}
	payload, err := buildPlayViewPayload(aid, cid, epID, qn, encodingName, bangumi)
	if err != nil {
		return "", err
	}
	api := appGrpcUGCAPI
	host := "grpc.biliapi.net"
	if bangumi {
		// long: 原版 PGC gRPC 请求虽然打到 app.bilibili.com，但 AppHelper.GetHeader 仍固定发送 grpc.biliapi.net 作为 Host。
		api = appGrpcPGCAPI
	}
	body, err := appGrpcPost(ctx, httpc, api, payload, appGrpcHeaders(cfg.Token, host))
	if err != nil {
		return "", err
	}
	message, err := unpackGrpcMessage(body)
	if err != nil {
		return "", err
	}
	reply := parseAppPlayViewReply(message)
	return appReplyToDashJSON(reply)
}

func buildPlayViewPayload(aid, cid, epID, qn, encodingName string, forceHEVC bool) ([]byte, error) {
	targetID := aid
	if epID != "" {
		targetID = epID
	}
	epNum, err := strconv.ParseInt(targetID, 10, 64)
	if err != nil {
		return nil, err
	}
	cidNum, err := strconv.ParseInt(cid, 10, 64)
	if err != nil {
		return nil, err
	}
	// long: 原版 AppHelper.GetPayload 注释掉了传入 qn，始终请求 127，避免 APP gRPC 清晰度请求和 Web/TV 路径混用语义。
	qnNum := int64(127)
	var msg []byte
	msg = protowire.AppendTag(msg, 1, protowire.VarintType)
	msg = protowire.AppendVarint(msg, uint64(epNum))
	msg = protowire.AppendTag(msg, 2, protowire.VarintType)
	msg = protowire.AppendVarint(msg, uint64(cidNum))
	msg = protowire.AppendTag(msg, 3, protowire.VarintType)
	msg = protowire.AppendVarint(msg, uint64(qnNum))
	msg = protowire.AppendTag(msg, 4, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 0)
	msg = protowire.AppendTag(msg, 5, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 4048)
	msg = protowire.AppendTag(msg, 6, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 0)
	msg = protowire.AppendTag(msg, 7, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 2)
	msg = protowire.AppendTag(msg, 8, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 1)
	msg = appendProtoString(msg, 9, "main.ugc-video-detail.0.0")
	msg = appendProtoString(msg, 10, "main.my-history.0.0")
	msg = protowire.AppendTag(msg, 12, protowire.VarintType)
	// long: APP 番剧/课程接口只接受 HEVC 偏好，原版会忽略用户指定的 AVC/AV1，避免 PGC gRPC 返回和普通 UGC 路径混用导致解析结果不稳定。
	if forceHEVC {
		msg = protowire.AppendVarint(msg, uint64(appCodeType("HEVC")))
	} else {
		msg = protowire.AppendVarint(msg, uint64(appCodeType(encodingName)))
	}
	return packGrpcMessage(msg)
}

func appCodeType(name string) int {
	switch name {
	case "AVC":
		return 1
	case "HEVC":
		return 2
	case "AV1":
		return 3
	default:
		return 2
	}
}

func appGrpcPost(ctx context.Context, httpc *HTTPClient, rawURL string, body []byte, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		if strings.EqualFold(k, "Host") {
			// long: Go 的 Header.Set("Host") 不会改变实际发出的 Host，原版把 Host 放在请求头里时需要映射到 req.Host 才能复现。
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := httpc.Client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func appGrpcHeaders(token, host string) map[string]string {
	return map[string]string{
		"Host":                   host,
		"content-type":           "application/grpc",
		"user-agent":             "Dalvik/2.1.0 (Linux; U; Android 11; M2012K11AC Build/RKQ1.200826.002) 7.32.0 os/android model/M2012K11AC mobi_app/android build/7320200 channel/xiaomi_cn_tv.danmaku.bili_zm20200902 innerVer/7320200 osVer/11 network/2 grpc-java-cronet/1.36.1",
		"te":                     "trailers",
		"x-bili-fawkes-req-bin":  encodeProtoBase64(protoMessageString(1, "android64", 2, "prod", 3, "dedf8669")),
		"x-bili-metadata-bin":    encodeProtoBase64(buildMetadataBin(token)),
		"authorization":          "identify_v1 " + token,
		"x-bili-device-bin":      encodeProtoBase64(buildDeviceBin()),
		"x-bili-network-bin":     encodeProtoBase64(buildNetworkBin()),
		"x-bili-restriction-bin": "",
		"x-bili-locale-bin":      encodeProtoBase64(buildLocaleBin()),
		"x-bili-exps-bin":        "",
		"grpc-encoding":          "gzip",
		"grpc-accept-encoding":   "identity,gzip",
		"grpc-timeout":           "17996161u",
	}
}

func packGrpcMessage(input []byte) ([]byte, error) {
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(input); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	out := make([]byte, 5, compressed.Len()+5)
	out[0] = 1
	binary.BigEndian.PutUint32(out[1:5], uint32(compressed.Len()))
	out = append(out, compressed.Bytes()...)
	return out, nil
}

func unpackGrpcMessage(data []byte) ([]byte, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("grpc response too short")
	}
	size := int(binary.BigEndian.Uint32(data[1:5]))
	if len(data) < 5+size {
		return nil, fmt.Errorf("grpc response truncated")
	}
	payload := data[5 : 5+size]
	if data[0] != 1 {
		return payload, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func parseAppPlayViewReply(data []byte) appPlayViewReply {
	var out appPlayViewReply
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		switch num {
		case 1:
			out.parseVideoInfo(value)
		case 3:
			out.Clips = parseBusinessClips(value)
		case 7:
			out.parsePlayExtInfo(value)
		}
	})
	return out
}

func (r *appPlayViewReply) parseVideoInfo(data []byte) {
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		switch num {
		case 3:
			r.Timelength = int64(readVarint(value))
		case 5:
			if video, ok := parseStreamItem(value); ok {
				r.Videos = append(r.Videos, video)
			}
		case 6:
			if audio, ok := parseDashItem(value, "M4A"); ok {
				r.Audios = append(r.Audios, audio)
			}
		case 7:
			if audio, ok := parseDolbyItem(value, "E-AC-3"); ok {
				r.Dolby = &audio
			}
		case 9:
			if audio, ok := parseDolbyItem(value, "FLAC"); ok {
				r.Flac = &audio
			}
		}
	})
}

func parseStreamItem(data []byte) (appVideo, bool) {
	var quality string
	var video appVideo
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		switch num {
		case 1:
			quality = parseStreamInfoQuality(value)
		case 2:
			video = parseDashVideo(value)
		}
	})
	video.Quality = quality
	return video, video.BaseURL != ""
}

func parseStreamInfoQuality(data []byte) string {
	var quality string
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		if num == 1 {
			quality = fmt.Sprintf("%d", readVarint(value))
		}
	})
	return quality
}

func parseDashVideo(data []byte) appVideo {
	var item appVideo
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		switch num {
		case 1:
			item.BaseURL = string(value)
		case 2:
			item.BackupURL = append(item.BackupURL, string(value))
		case 3:
			item.Bandwidth = int64(readVarint(value))
		case 4:
			item.Codecid = fmt.Sprintf("%d", readVarint(value))
		case 6:
			item.Size = int64(readVarint(value))
		}
	})
	return item
}

func parseDashItem(data []byte, codecs string) (appAudio, bool) {
	item := appAudio{Codecs: codecs}
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		switch num {
		case 1:
			item.ID = fmt.Sprintf("%d", readVarint(value))
		case 2:
			item.BaseURL = string(value)
		case 3:
			item.BackupURL = append(item.BackupURL, string(value))
		case 4:
			item.Bandwidth = int64(readVarint(value))
		}
	})
	return item, item.BaseURL != ""
}

func parseDolbyItem(data []byte, codecs string) (appAudio, bool) {
	var audio appAudio
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		if num == 2 {
			audio, _ = parseDashItem(value, codecs)
		}
	})
	return audio, audio.BaseURL != ""
}

func parseBusinessClips(data []byte) []ViewPoint {
	var points []ViewPoint
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		if num == 6 {
			points = append(points, parseClipInfo(value))
		}
	})
	return points
}

func parseClipInfo(data []byte) ViewPoint {
	var point ViewPoint
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		switch num {
		case 2:
			point.Start = int(readVarint(value))
		case 3:
			point.End = int(readVarint(value))
		case 5:
			point.Title = string(value)
		}
	})
	return point
}

func (r *appPlayViewReply) parsePlayExtInfo(data []byte) {
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		if num == 1 {
			r.parseDubbingInfo(value)
		}
	})
}

func (r *appPlayViewReply) parseDubbingInfo(data []byte) {
	var backgroundAudios []Audio
	var roleAudios []appRoleAudio
	backgroundSeen := false
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		switch num {
		case 1:
			backgroundSeen = true
			backgroundAudios = append(backgroundAudios, parseAudioMaterial(value).Audio...)
		case 2:
			roleAudios = append(roleAudios, parseRoleAudio(value)...)
		}
	})
	if !backgroundSeen {
		return
	}
	r.BackgroundAudios = append(r.BackgroundAudios, backgroundAudios...)
	r.RoleAudios = append(r.RoleAudios, roleAudios...)
}

func parseRoleAudio(data []byte) []appRoleAudio {
	var roles []appRoleAudio
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		if num == 4 {
			roles = append(roles, parseAudioMaterial(value))
		}
	})
	return roles
}

func parseAudioMaterial(data []byte) appRoleAudio {
	var material appRoleAudio
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		switch num {
		case 1:
			material.AudioID = string(value)
		case 2:
			material.Title = string(value)
		case 3:
			if material.PersonName == "" {
				material.PersonName = string(value)
			}
		case 5:
			material.PersonName = string(value)
		case 7:
			if audio, ok := parseDashItem(value, "M4A"); ok {
				material.Audio = append(material.Audio, Audio{
					ID:        audio.ID,
					Dfn:       audio.ID,
					BaseURL:   audio.BaseURL,
					Codecs:    audio.Codecs,
					Bandwidth: audio.Bandwidth / 1000,
				})
			}
		}
	})
	return material
}

func appReplyToDashJSON(reply appPlayViewReply) (string, error) {
	videoItems := make([]any, 0, len(reply.Videos))
	for _, item := range reply.Videos {
		videoItems = append(videoItems, J{
			"id":         appJSONInt(item.Quality),
			"base_url":   item.BaseURL,
			"backup_url": item.BackupURL,
			"bandwidth":  appVideoBandwidth(item.Size, reply.Timelength),
			"codecid":    appJSONInt(item.Codecid),
		})
	}
	audioItems := make([]any, 0, len(reply.Audios)+2)
	for _, item := range reply.Audios {
		audioItems = append(audioItems, audioToJ(item))
	}
	if reply.Flac != nil {
		audioItems = append(audioItems, audioToJ(*reply.Flac))
	}
	if reply.Dolby != nil {
		audioItems = append(audioItems, audioToJ(*reply.Dolby))
	}
	clips := make([]any, 0, len(reply.Clips))
	for _, clip := range reply.Clips {
		clips = append(clips, J{"start": clip.Start, "end": clip.End, "toastText": clip.Title})
	}
	data := J{
		"code":    0,
		"message": "0",
		"ttl":     1,
		"data": J{
			"timelength": reply.Timelength,
			"dash": J{
				"video": videoItems,
				"audio": audioItems,
			},
			"clip_info_list": clips,
		},
		"dubbing_info": J{
			"background_audio": audioMaterialsFromAudios(reply.BackgroundAudios),
			"role_audio_list":  audioMaterialsToJ(reply.RoleAudios),
		},
	}
	out, err := json.Marshal(data)
	return string(out), err
}

func audioToJ(item appAudio) J {
	return J{
		"id":         appJSONInt(item.ID),
		"base_url":   item.BaseURL,
		"backup_url": item.BackupURL,
		"bandwidth":  item.Bandwidth,
		"codecs":     item.Codecs,
	}
}

func audioMaterialsFromAudios(items []Audio) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, J{
			"id":         appJSONInt(item.ID),
			"base_url":   item.BaseURL,
			"backup_url": []any{},
			"bandwidth":  item.Bandwidth * 1000,
			"codecs":     item.Codecs,
		})
	}
	return out
}

func appVideoBandwidth(size, timelength int64) int64 {
	seconds := timelength / 1000
	if seconds == 0 {
		return 0
	}
	return size * 8 / seconds
}

func audioMaterialsToJ(items []appRoleAudio) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		audios := make([]any, 0, len(item.Audio))
		for _, audio := range item.Audio {
			audios = append(audios, J{
				"id":         appJSONInt(audio.ID),
				"base_url":   audio.BaseURL,
				"backup_url": []any{},
				"bandwidth":  audio.Bandwidth * 1000,
				"codecs":     audio.Codecs,
			})
		}
		title := item.Title
		if title == "" {
			title = item.AudioID
		}
		out = append(out, J{"audio_id": item.AudioID, "title": title, "person_name": item.PersonName, "audio": audios})
	}
	return out
}

func appJSONInt(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func consumeFields(data []byte, fn func(protowire.Number, protowire.Type, []byte)) {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return
		}
		data = data[n:]
		var value []byte
		var consumed int
		switch typ {
		case protowire.VarintType:
			_, consumed = protowire.ConsumeVarint(data)
			if consumed < 0 {
				return
			}
			value = data[:consumed]
		case protowire.BytesType:
			var bytesValue []byte
			bytesValue, consumed = protowire.ConsumeBytes(data)
			if consumed < 0 {
				return
			}
			value = bytesValue
		case protowire.Fixed32Type:
			_, consumed = protowire.ConsumeFixed32(data)
			if consumed < 0 {
				return
			}
			value = data[:consumed]
		case protowire.Fixed64Type:
			_, consumed = protowire.ConsumeFixed64(data)
			if consumed < 0 {
				return
			}
			value = data[:consumed]
		default:
			return
		}
		fn(num, typ, value)
		data = data[consumed:]
	}
}

func readVarint(data []byte) uint64 {
	value, _ := protowire.ConsumeVarint(data)
	return value
}

func appendProtoString(msg []byte, fieldNumber protowire.Number, value string) []byte {
	msg = protowire.AppendTag(msg, fieldNumber, protowire.BytesType)
	msg = protowire.AppendString(msg, value)
	return msg
}

func protoMessageString(field1 protowire.Number, value1 string, field2 protowire.Number, value2 string, field3 protowire.Number, value3 string) []byte {
	var msg []byte
	msg = appendProtoString(msg, field1, value1)
	msg = appendProtoString(msg, field2, value2)
	msg = appendProtoString(msg, field3, value3)
	return msg
}

func buildMetadataBin(token string) []byte {
	var msg []byte
	msg = appendProtoString(msg, 1, token)
	msg = appendProtoString(msg, 2, "android")
	msg = protowire.AppendTag(msg, 4, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 7320200)
	msg = appendProtoString(msg, 5, "xiaomi_cn_tv.danmaku.bili_zm20200902")
	msg = appendProtoString(msg, 6, "")
	msg = appendProtoString(msg, 7, "android")
	return msg
}

func buildDeviceBin() []byte {
	var msg []byte
	msg = protowire.AppendTag(msg, 1, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 1)
	msg = protowire.AppendTag(msg, 2, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 7320200)
	msg = appendProtoString(msg, 3, "")
	msg = appendProtoString(msg, 4, "android")
	msg = appendProtoString(msg, 5, "android")
	msg = appendProtoString(msg, 7, "xiaomi_cn_tv.danmaku.bili_zm20200902")
	msg = appendProtoString(msg, 8, "M2012K11AC")
	msg = appendProtoString(msg, 9, "Build/RKQ1.200826.002")
	msg = appendProtoString(msg, 10, "11")
	return msg
}

func buildNetworkBin() []byte {
	var msg []byte
	msg = protowire.AppendTag(msg, 1, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 1)
	msg = appendProtoString(msg, 3, "46007")
	return msg
}

func buildLocaleBin() []byte {
	locale := protoMessageString(1, "zh", 3, "CN", 2, "")
	var msg []byte
	msg = protowire.AppendTag(msg, 1, protowire.BytesType)
	msg = protowire.AppendBytes(msg, locale)
	return msg
}

func encodeProtoBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func stringsHasAnyPrefix(value string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
