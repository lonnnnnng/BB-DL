package bbdown

import (
	"fmt"
	"strconv"
	"strings"
)

func SelectedTrackIndexes(opt *MyOption, tracks *ParsedTracks) (int, int, error) {
	videoIndex := 0
	audioIndex := 0
	if opt == nil || tracks == nil {
		return videoIndex, audioIndex, nil
	}
	if strings.TrimSpace(opt.VideoIndex) != "" && !opt.AudioOnly {
		index, err := parseExplicitTrackIndex(opt.VideoIndex, len(tracks.VideoTracks), "视频流")
		if err != nil {
			return 0, 0, err
		}
		videoIndex = index
	}
	if strings.TrimSpace(opt.AudioIndex) != "" && !opt.VideoOnly {
		index, err := parseExplicitTrackIndex(opt.AudioIndex, len(tracks.AudioTracks), "音频流")
		if err != nil {
			return 0, 0, err
		}
		audioIndex = index
	}
	return videoIndex, audioIndex, nil
}

func ValidateTrackIndexSyntax(opt *MyOption) error {
	if opt == nil || opt.OnlyShowInfo {
		return nil
	}
	if strings.TrimSpace(opt.VideoIndex) != "" && !opt.AudioOnly {
		if _, err := parseExplicitTrackIndexSyntax(opt.VideoIndex, "视频流"); err != nil {
			return err
		}
	}
	if strings.TrimSpace(opt.AudioIndex) != "" && !opt.VideoOnly {
		if _, err := parseExplicitTrackIndexSyntax(opt.AudioIndex, "音频流"); err != nil {
			return err
		}
	}
	return nil
}

func NormalizeTrackIndexOptions(opt *MyOption) {
	if opt == nil {
		return
	}
	if opt.OnlyShowInfo {
		// long 2026-06-27 23:55:00：仅查看任务只负责列出可用流，不会下载任何轨道；保留旧序号只会污染服务子进程命令和桌面任务模板。
		opt.VideoIndex = ""
		opt.AudioIndex = ""
		return
	}
	if opt.AudioOnly && opt.VideoOnly {
		// long 2026-06-28 00:03:00：互斥 only 参数会在兼容归一化后退回普通下载；在这里提前清序号会把普通下载真正要用的坏参数藏起来。
		return
	}
	if opt.AudioOnly {
		opt.VideoIndex = ""
	}
	if opt.VideoOnly {
		opt.AudioIndex = ""
	}
}

func parseExplicitTrackIndex(value string, count int, label string) (int, error) {
	index, err := parseExplicitTrackIndexSyntax(value, label)
	if err != nil {
		return 0, err
	}
	// long 2026-06-27 03:02:32：显式选流通常来自桌面任务或脚本，越界时直接失败，避免下载到用户没有选择的第 0 条轨道。
	if index >= count {
		if count <= 0 {
			return 0, fmt.Errorf("当前没有可选择的%s", label)
		}
		return 0, fmt.Errorf("%s序号超出范围: %d，可用范围 0-%d", label, index, count-1)
	}
	return index, nil
}

func parseExplicitTrackIndexSyntax(value string, label string) (int, error) {
	trimmed := strings.TrimSpace(value)
	index, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%s序号无效: %q", label, value)
	}
	if index < 0 {
		return 0, fmt.Errorf("%s序号不能小于 0: %d", label, index)
	}
	return index, nil
}
