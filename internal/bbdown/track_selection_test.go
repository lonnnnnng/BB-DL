package bbdown

import (
	"strings"
	"testing"
)

func TestValidateTrackIndexSyntaxRejectsInvalidValuesEarly(t *testing.T) {
	if err := ValidateTrackIndexSyntax(&MyOption{VideoIndex: "abc"}); err == nil || !strings.Contains(err.Error(), "视频流序号无效") {
		t.Fatalf("invalid video index should fail clearly, got %v", err)
	}
	if err := ValidateTrackIndexSyntax(&MyOption{AudioIndex: "-1"}); err == nil || !strings.Contains(err.Error(), "音频流序号不能小于 0") {
		t.Fatalf("negative audio index should fail clearly, got %v", err)
	}
}

func TestValidateTrackIndexSyntaxIgnoresUnusedIndexes(t *testing.T) {
	tests := []MyOption{
		{OnlyShowInfo: true, VideoIndex: "abc", AudioIndex: "-1"},
		{AudioOnly: true, VideoIndex: "abc", AudioIndex: "0"},
		{VideoOnly: true, VideoIndex: "0", AudioIndex: "abc"},
	}
	for _, opt := range tests {
		if err := ValidateTrackIndexSyntax(&opt); err != nil {
			t.Fatalf("unused stream index should be ignored for %+v: %v", opt, err)
		}
	}
}

func TestNormalizeTrackIndexOptionsClearsUnusedIndexes(t *testing.T) {
	tests := []struct {
		name      string
		opt       MyOption
		wantVideo string
		wantAudio string
	}{
		{name: "only show info", opt: MyOption{OnlyShowInfo: true, VideoIndex: "bad", AudioIndex: "bad"}, wantVideo: "", wantAudio: ""},
		{name: "audio only", opt: MyOption{AudioOnly: true, VideoIndex: "bad", AudioIndex: "1"}, wantVideo: "", wantAudio: "1"},
		{name: "video only", opt: MyOption{VideoOnly: true, VideoIndex: "2", AudioIndex: "bad"}, wantVideo: "2", wantAudio: ""},
		{name: "audio and video only before compatibility", opt: MyOption{AudioOnly: true, VideoOnly: true, VideoIndex: "2", AudioIndex: "1"}, wantVideo: "2", wantAudio: "1"},
		{name: "normal download", opt: MyOption{VideoIndex: "2", AudioIndex: "1"}, wantVideo: "2", wantAudio: "1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			NormalizeTrackIndexOptions(&tc.opt)
			if tc.opt.VideoIndex != tc.wantVideo || tc.opt.AudioIndex != tc.wantAudio {
				t.Fatalf("indexes = video %q audio %q, want %q %q", tc.opt.VideoIndex, tc.opt.AudioIndex, tc.wantVideo, tc.wantAudio)
			}
		})
	}
}

func TestSelectedTrackIndexesRejectsNegativeIndex(t *testing.T) {
	tracks := &ParsedTracks{
		VideoTracks: []Video{{Dfn: "360P"}},
		AudioTracks: []Audio{{ID: "0"}},
	}
	if _, _, err := SelectedTrackIndexes(&MyOption{VideoIndex: "-1"}, tracks); err == nil || !strings.Contains(err.Error(), "视频流序号不能小于 0") {
		t.Fatalf("negative selected video index should fail clearly, got %v", err)
	}
}
