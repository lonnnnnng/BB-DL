package bbdown

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestBuildDmViewPayload(t *testing.T) {
	packed, err := buildDmViewPayload("123", "456")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := unpackGrpcMessage(packed)
	if err != nil {
		t.Fatal(err)
	}
	var spmid string
	consumeFields(payload, func(num protowire.Number, typ protowire.Type, value []byte) {
		if num == 4 && typ == protowire.BytesType {
			spmid = string(value)
		}
	})
	if readProtoVarintField(payload, 1) != 123 || readProtoVarintField(payload, 2) != 456 || readProtoVarintField(payload, 3) != 1 {
		t.Fatalf("unexpected DmView payload fields")
	}
	if spmid != "main.ugc-video-detail.0.0" {
		t.Fatalf("spmid = %q", spmid)
	}
}

func TestParseDmViewSubtitles(t *testing.T) {
	item := appendProtoString(nil, 3, "zh-CN")
	item = appendProtoString(item, 5, "//subtitle.example.test/123.json")
	var videoSubtitle []byte
	videoSubtitle = protowire.AppendTag(videoSubtitle, 3, protowire.BytesType)
	videoSubtitle = protowire.AppendBytes(videoSubtitle, item)
	var reply []byte
	reply = protowire.AppendTag(reply, 3, protowire.BytesType)
	reply = protowire.AppendBytes(reply, videoSubtitle)

	subs := parseDmViewSubtitles("123", "456", reply)
	if len(subs) != 1 {
		t.Fatalf("len(subs) = %d", len(subs))
	}
	if subs[0].Lan != "zh-CN" || subs[0].URL != "//subtitle.example.test/123.json" || subs[0].Path != "123/123.456.zh-CN.srt" {
		t.Fatalf("subtitle = %+v", subs[0])
	}
}

func TestParseDmViewSubtitlesReturnsEmptyOnNoSubtitle(t *testing.T) {
	subs := parseDmViewSubtitles("123", "456", nil)
	if subs == nil {
		t.Fatal("empty DmView subtitle response should be a successful empty list")
	}
	if len(subs) != 0 {
		t.Fatalf("len(subs) = %d, want 0", len(subs))
	}
}

func TestParseDmViewSubtitlesTreatsEmptyURLAsAPIFailureLikeOriginal(t *testing.T) {
	item := appendProtoString(nil, 3, "zh-CN")
	var videoSubtitle []byte
	videoSubtitle = protowire.AppendTag(videoSubtitle, 3, protowire.BytesType)
	videoSubtitle = protowire.AppendBytes(videoSubtitle, item)
	var reply []byte
	reply = protowire.AppendTag(reply, 3, protowire.BytesType)
	reply = protowire.AppendBytes(reply, videoSubtitle)

	if got := parseDmViewSubtitles("123", "456", reply); got != nil {
		t.Fatalf("DmView subtitle with empty url should fail like original, got %+v", got)
	}
}

func readProtoVarintField(data []byte, want protowire.Number) uint64 {
	var out uint64
	consumeFields(data, func(num protowire.Number, typ protowire.Type, value []byte) {
		if num == want && typ == protowire.VarintType {
			out = readVarint(value)
		}
	})
	return out
}
