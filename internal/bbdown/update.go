package bbdown

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// long 2026-06-20 15:30:00：Go 复刻版使用独立版本线，更新检查必须跟随 BB-DL 的 Release，避免重新从 1.0.0 发版后被原版 BBDown 的历史版本误提示。
const latestReleaseURL = "https://github.com/lonnnnnng/BB-DL/releases/latest"

func CheckUpdateAsync(httpc *HTTPClient) {
	if httpc == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = CheckUpdate(ctx, httpc, latestReleaseURL)
	}()
}

func CheckUpdate(ctx context.Context, httpc *HTTPClient, url string) error {
	if httpc == nil {
		return nil
	}
	redirectURL, err := httpc.ResolveLocation(ctx, url)
	if err != nil {
		return err
	}
	latestVer := LatestVersionFromReleaseURL(redirectURL)
	if !IsNewerVersion(latestVer, Version) {
		return nil
	}
	LogColor("发现新版本："+latestVer, true)
	return nil
}

func LatestVersionFromReleaseURL(url string) string {
	const marker = "/releases/tag/"
	idx := strings.Index(url, marker)
	if idx < 0 {
		return ""
	}
	ver := strings.TrimSpace(url[idx+len(marker):])
	if slash := strings.Index(ver, "/"); slash >= 0 {
		ver = ver[:slash]
	}
	if query := strings.Index(ver, "?"); query >= 0 {
		ver = ver[:query]
	}
	if hash := strings.Index(ver, "#"); hash >= 0 {
		ver = ver[:hash]
	}
	return ver
}

func IsNewerVersion(latest, current string) bool {
	latestParts, latestOK := parseVersionParts(latest)
	currentParts, currentOK := parseVersionParts(current)
	if !latestOK || !currentOK {
		return strings.TrimSpace(latest) != "" && strings.TrimSpace(latest) != strings.TrimSpace(current)
	}
	// long 2026-06-18 14:22:10：Go 版可能先于原版发布补丁版，更新提示只能在远端版本真正更高时出现，避免把原版旧 release 当成新版本。
	maxLen := len(latestParts)
	if len(currentParts) > maxLen {
		maxLen = len(currentParts)
	}
	for i := 0; i < maxLen; i++ {
		var left, right int
		if i < len(latestParts) {
			left = latestParts[i]
		}
		if i < len(currentParts) {
			right = currentParts[i]
		}
		if left > right {
			return true
		}
		if left < right {
			return false
		}
	}
	return false
}

func parseVersionParts(version string) ([]int, bool) {
	version = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V"))
	if version == "" {
		return nil, false
	}
	fields := strings.FieldsFunc(version, func(r rune) bool {
		return !unicode.IsDigit(r)
	})
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		value, err := strconv.Atoi(field)
		if err != nil {
			return nil, false
		}
		parts = append(parts, value)
	}
	return parts, len(parts) > 0
}
