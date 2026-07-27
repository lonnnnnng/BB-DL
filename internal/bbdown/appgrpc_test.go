package bbdown

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestPackAndUnpackGrpcMessage(t *testing.T) {
	payload := []byte("hello grpc")
	packed, err := packGrpcMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	unpacked, err := unpackGrpcMessage(packed)
	if err != nil {
		t.Fatal(err)
	}
	if string(unpacked) != string(payload) {
		t.Fatalf("unpacked = %q, want %q", string(unpacked), string(payload))
	}
}

func TestAppCodeTypeUsesPreferredEncoding(t *testing.T) {
	cases := map[string]int{
		"AVC":  1,
		"HEVC": 2,
		"AV1":  3,
		"":     2,
	}
	for input, want := range cases {
		if got := appCodeType(input); got != want {
			t.Fatalf("appCodeType(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestBuildPlayViewPayloadForcesHEVCForBangumi(t *testing.T) {
	ugcPayload, err := buildPlayViewPayload("123", "456", "", "80", "AVC", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := playViewPayloadCodeType(t, ugcPayload); got != 1 {
		t.Fatalf("ugc payload code type = %d, want AVC code 1", got)
	}

	bangumiPayload, err := buildPlayViewPayload("123", "456", "789", "80", "AVC", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := playViewPayloadCodeType(t, bangumiPayload); got != 2 {
		t.Fatalf("bangumi payload code type = %d, want forced HEVC code 2", got)
	}
}

func TestBuildPlayViewPayloadUsesOriginalFixedQuality(t *testing.T) {
	payload, err := buildPlayViewPayload("123", "456", "", "80", "HEVC", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := playViewPayloadVarintField(t, payload, 3); got != 127 {
		t.Fatalf("payload qn = %d, want original fixed 127", got)
	}
}

func TestAppGrpcPostReadsNonSuccessBodyLikeOriginal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/grpc" {
			t.Fatalf("Content-Type = %q", got)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("grpc-body"))
	}))
	defer server.Close()

	body, err := appGrpcPost(context.Background(), &HTTPClient{client: server.Client(), config: NewConfig()}, server.URL, []byte("payload"), map[string]string{
		"content-type": "application/grpc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "grpc-body" {
		t.Fatalf("body = %q", body)
	}
}

func TestAppGrpcPostHonorsHostHeaderLikeOriginal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "grpc.biliapi.net" {
			t.Fatalf("Host = %q", r.Host)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	body, err := appGrpcPost(context.Background(), &HTTPClient{client: server.Client(), config: NewConfig()}, server.URL, []byte("payload"), map[string]string{
		"Host": "grpc.biliapi.net",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
}

func TestGetAppGrpcPlayJSONUsesOriginalHostForBangumi(t *testing.T) {
	response, err := packGrpcMessage(nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bilibili.pgc.gateway.player.v2.PlayURL/PlayView" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Host != "grpc.biliapi.net" {
			t.Fatalf("Host = %q", r.Host)
		}
		_, _ = w.Write(response)
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := NewConfig()
	httpc := &HTTPClient{
		client: &http.Client{Transport: rewriteHostTransport{target: target, base: server.Client().Transport}},
		config: cfg,
	}
	if _, err := getAppGrpcPlayJSON(context.Background(), cfg, httpc, "ep:123", "0", "456", "123", "80", "AVC"); err != nil {
		t.Fatal(err)
	}
}

func TestAppReplyToDashJSON(t *testing.T) {
	body, err := appReplyToDashJSON(appPlayViewReply{
		Timelength: 1000,
		Videos: []appVideo{{
			Quality:   "32",
			BaseURL:   "https://example.test/video.m4s",
			BackupURL: []string{"https://example.test/video-bak.m4s"},
			Bandwidth: 100000,
			Codecid:   "7",
			Size:      123,
		}},
		Audios: []appAudio{{
			ID:        "30280",
			BaseURL:   "https://example.test/audio.m4s",
			Bandwidth: 200000,
			Codecs:    "M4A",
		}},
		RoleAudios: []appRoleAudio{{
			AudioID:    "7",
			PersonName: "long",
			Audio: []Audio{{
				ID:        "30281",
				BaseURL:   "https://example.test/role.m4a",
				Bandwidth: 96,
				Codecs:    "M4A",
			}},
		}},
		Clips: []ViewPoint{{Title: "跳过片头", Start: 1, End: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"dash"`, `"video"`, `"audio"`, `"clip_info_list"`, `"dubbing_info"`, `"toastText":"跳过片头"`, `"id":32`, `"codecid":7`, `"bandwidth":984`, `"id":30280`, `"id":30281`, `"audio_id":"7"`, `"title":"7"`, `"person_name":"long"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("json %s missing %s", body, want)
		}
	}
	for _, forbidden := range []string{`"stream_list"`, `"dash_audio"`, `"id":"32"`, `"codecid":"7"`, `"id":"30280"`, `"id":"30281"`, `"bandwidth":100000`, `"personName"`, `"path"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("app grpc json should not contain %s, got %s", forbidden, body)
		}
	}
	if strings.Contains(body, `"stream_list"`) || strings.Contains(body, `"dash_audio"`) {
		t.Fatalf("app grpc json should use original dash shape, got %s", body)
	}
	tracks, err := parseDashLike(context.Background(), nil, body, NewConfig(), "ep:1", "100", "200", "1", false, false, true, &ParsedTracks{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks.ExtraPoints) != 2 || tracks.ExtraPoints[0].Title != "正片" || tracks.ExtraPoints[1].Title != "跳过片头" {
		t.Fatalf("chapter points = %+v", tracks.ExtraPoints)
	}
	if len(tracks.VideoTracks) != 1 || tracks.VideoTracks[0].Bandwidth != 0 {
		t.Fatalf("video tracks = %+v, want 984 bps converted to 0 kbps like original parser", tracks.VideoTracks)
	}
	if len(tracks.RoleAudioLists) != 1 || tracks.RoleAudioLists[0].Path != "100/100.200.7.m4a" || tracks.RoleAudioLists[0].Title != "7" {
		t.Fatalf("role audio list = %+v", tracks.RoleAudioLists)
	}
}

func TestParseBusinessClipsKeepsZeroLengthClipLikeOriginal(t *testing.T) {
	var clip []byte
	clip = protowire.AppendTag(clip, 2, protowire.VarintType)
	clip = protowire.AppendVarint(clip, 10)
	clip = protowire.AppendTag(clip, 3, protowire.VarintType)
	clip = protowire.AppendVarint(clip, 10)
	clip = appendProtoString(clip, 5, "即将跳过片尾")

	var business []byte
	business = protowire.AppendTag(business, 6, protowire.BytesType)
	business = protowire.AppendBytes(business, clip)

	points := parseBusinessClips(business)
	if len(points) != 1 || points[0].Start != 10 || points[0].End != 10 || points[0].Title != "即将跳过片尾" {
		t.Fatalf("business clips = %+v", points)
	}
	body, err := appReplyToDashJSON(appPlayViewReply{Clips: points})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"start":10`) || !strings.Contains(body, `"end":10`) || !strings.Contains(body, `"toastText":"即将跳过片尾"`) {
		t.Fatalf("dash json should keep original zero-length clip, got %s", body)
	}
}

func TestParseDubbingInfoRequiresBackgroundAudioLikeOriginal(t *testing.T) {
	roleAudio := appDashItemProto("30281", "https://example.test/role.m4a", 96000)
	var material []byte
	material = appendProtoString(material, 1, "7")
	material = appendProtoString(material, 3, "粤语版")
	material = protowire.AppendTag(material, 7, protowire.BytesType)
	material = protowire.AppendBytes(material, roleAudio)

	var role []byte
	role = protowire.AppendTag(role, 4, protowire.BytesType)
	role = protowire.AppendBytes(role, material)

	var onlyRole []byte
	onlyRole = protowire.AppendTag(onlyRole, 2, protowire.BytesType)
	onlyRole = protowire.AppendBytes(onlyRole, role)
	var withoutBackground appPlayViewReply
	withoutBackground.parseDubbingInfo(onlyRole)
	if len(withoutBackground.RoleAudios) != 0 {
		t.Fatalf("role audios without background = %+v, want ignored like original", withoutBackground.RoleAudios)
	}

	var withBackgroundData []byte
	withBackgroundData = protowire.AppendTag(withBackgroundData, 1, protowire.BytesType)
	withBackgroundData = protowire.AppendBytes(withBackgroundData, nil)
	withBackgroundData = append(withBackgroundData, onlyRole...)
	var withBackground appPlayViewReply
	withBackground.parseDubbingInfo(withBackgroundData)
	if len(withBackground.RoleAudios) != 1 {
		t.Fatalf("role audios with background field = %+v", withBackground.RoleAudios)
	}
	body, err := appReplyToDashJSON(withBackground)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"audio_id":"7"`, `"title":"7"`, `"person_name":"粤语版"`, `"id":30281`} {
		if !strings.Contains(body, want) {
			t.Fatalf("dubbing json %s missing %s", body, want)
		}
	}
}

func appDashItemProto(id, baseURL string, bandwidth uint64) []byte {
	var msg []byte
	idNum, _ := strconv.ParseUint(id, 10, 64)
	msg = protowire.AppendTag(msg, 1, protowire.VarintType)
	msg = protowire.AppendVarint(msg, idNum)
	msg = appendProtoString(msg, 2, baseURL)
	msg = protowire.AppendTag(msg, 4, protowire.VarintType)
	msg = protowire.AppendVarint(msg, bandwidth)
	return msg
}

func playViewPayloadCodeType(t *testing.T, payload []byte) uint64 {
	return playViewPayloadVarintField(t, payload, 12)
}

func playViewPayloadVarintField(t *testing.T, payload []byte, target protowire.Number) uint64 {
	t.Helper()
	body, err := unpackGrpcMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			t.Fatalf("consume tag failed: %v", protowire.ParseError(n))
		}
		body = body[n:]
		if num == target {
			value, n := protowire.ConsumeVarint(body)
			if n < 0 {
				t.Fatalf("consume field %d failed: %v", target, protowire.ParseError(n))
			}
			return value
		}
		n = protowire.ConsumeFieldValue(num, typ, body)
		if n < 0 {
			t.Fatalf("consume field %d failed: %v", num, protowire.ParseError(n))
		}
		body = body[n:]
	}
	t.Fatalf("payload missing varint field %d", target)
	return 0
}
