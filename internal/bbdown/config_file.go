package bbdown

import (
	"os"
	"path/filepath"
	"strings"
)

func MergeConfigArgs(args []string, explicitConfigPath string) []string {
	configPath := explicitConfigPath
	if strings.TrimSpace(configPath) == "" {
		configPath = filepath.Join(AppDir(), "BBDown.config")
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return args
	}
	Log("加载配置文件: " + configPath)

	existing := make(map[string]bool)
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			existing[canonicalConfigKey(arg)] = true
		}
	}

	tokens := configTokens(string(content))
	var merged []string
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if !strings.HasPrefix(token, "-") {
			continue
		}
		key := canonicalConfigKey(token)
		if key == "" {
			continue
		}
		takesValue := configValueOptions[key]
		// long 2026-06-19 02:23:56：配置文件允许沿用原版的 --flag=value 写法；这类值已经在当前 token 内，继续读取下一项会把后续配置误吞掉。
		inlineValue := configTokenHasInlineValue(token)
		boolValue := false
		if !takesValue && !inlineValue && i+1 < len(tokens) && isConfigBoolLiteral(tokens[i+1]) {
			boolValue = true
		}
		if existing[key] {
			if (takesValue && !inlineValue) || boolValue {
				i++
			}
			continue
		}
		merged = append(merged, token)
		existing[key] = true
		if (takesValue && !inlineValue) || boolValue {
			if i+1 < len(tokens) {
				merged = append(merged, tokens[i+1])
				i++
			}
		}
	}
	return append(merged, args...)
}

func configTokens(content string) []string {
	var tokens []string
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "-") && strings.Contains(trimmed, " ") {
			idx := strings.Index(trimmed, " ")
			key := strings.TrimSpace(trimmed[:idx])
			val := strings.Trim(strings.TrimSpace(trimmed[idx+1:]), "\"")
			tokens = append(tokens, key, val)
			continue
		}
		token := strings.Trim(trimmed, "\"")
		tokens = append(tokens, token)
	}
	return tokens
}

func canonicalConfigKey(arg string) string {
	if idx := strings.Index(arg, "="); idx >= 0 {
		arg = arg[:idx]
	}
	arg = strings.TrimLeft(arg, "-")
	if canonical, ok := configOptionAliases[arg]; ok {
		return canonical
	}
	return arg
}

func configTokenHasInlineValue(arg string) bool {
	return strings.Contains(arg, "=")
}

func isConfigBoolLiteral(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false":
		return true
	default:
		return false
	}
}

var configOptionAliases = map[string]string{
	"tv":                       "use-tv-api",
	"app":                      "use-app-api",
	"intl":                     "use-intl-api",
	"e":                        "encoding-priority",
	"q":                        "dfn-priority",
	"info":                     "only-show-info",
	"aria2":                    "use-aria2c",
	"ia":                       "interactive",
	"hs":                       "hide-streams",
	"mt":                       "multi-thread",
	"dd":                       "download-danmaku",
	"ddf":                      "download-danmaku-formats",
	"F":                        "file-pattern",
	"M":                        "multi-file-pattern",
	"p":                        "select-page",
	"ua":                       "user-agent",
	"c":                        "cookie",
	"token":                    "access-token",
	"hevc":                     "only-hevc",
	"avc":                      "only-avc",
	"av1":                      "only-av1",
	"use-tv-api":               "use-tv-api",
	"use-app-api":              "use-app-api",
	"use-intl-api":             "use-intl-api",
	"use-mp4box":               "use-mp4box",
	"encoding-priority":        "encoding-priority",
	"dfn-priority":             "dfn-priority",
	"only-show-info":           "only-show-info",
	"show-all":                 "show-all",
	"use-aria2c":               "use-aria2c",
	"aria2c-args":              "aria2c-args",
	"aria2c-proxy":             "aria2c-proxy",
	"interactive":              "interactive",
	"hide-streams":             "hide-streams",
	"multi-thread":             "multi-thread",
	"simply-mux":               "simply-mux",
	"video-only":               "video-only",
	"audio-only":               "audio-only",
	"danmaku-only":             "danmaku-only",
	"cover-only":               "cover-only",
	"sub-only":                 "sub-only",
	"debug":                    "debug",
	"skip-mux":                 "skip-mux",
	"skip-subtitle":            "skip-subtitle",
	"skip-cover":               "skip-cover",
	"force-http":               "force-http",
	"download-danmaku":         "download-danmaku",
	"download-danmaku-formats": "download-danmaku-formats",
	"skip-ai":                  "skip-ai",
	"video-ascending":          "video-ascending",
	"audio-ascending":          "audio-ascending",
	"allow-pcdn":               "allow-pcdn",
	"force-replace-host":       "force-replace-host",
	"file-exists-action":       "file-exists-action",
	"save-archives-to-file":    "save-archives-to-file",
	"only-hevc":                "only-hevc",
	"only-avc":                 "only-avc",
	"only-av1":                 "only-av1",
	"add-dfn-subfix":           "add-dfn-subfix",
	"no-padding-page-num":      "no-padding-page-num",
	"bandwith-ascending":       "bandwith-ascending",
	"file-pattern":             "file-pattern",
	"multi-file-pattern":       "multi-file-pattern",
	"select-page":              "select-page",
	"video-index":              "video-index",
	"audio-index":              "audio-index",
	"language":                 "language",
	"user-agent":               "user-agent",
	"cookie":                   "cookie",
	"access-token":             "access-token",
	"work-dir":                 "work-dir",
	"ffmpeg-path":              "ffmpeg-path",
	"mp4box-path":              "mp4box-path",
	"aria2c-path":              "aria2c-path",
	"upos-host":                "upos-host",
	"delay-per-page":           "delay-per-page",
	"host":                     "host",
	"ep-host":                  "ep-host",
	"tv-host":                  "tv-host",
	"area":                     "area",
	"config-file":              "config-file",
	"version":                  "version",
}

var configValueOptions = map[string]bool{
	"encoding-priority":        true,
	"dfn-priority":             true,
	"aria2c-args":              true,
	"aria2c-proxy":             true,
	"download-danmaku-formats": true,
	"file-exists-action":       true,
	"file-pattern":             true,
	"multi-file-pattern":       true,
	"select-page":              true,
	"video-index":              true,
	"audio-index":              true,
	"language":                 true,
	"user-agent":               true,
	"cookie":                   true,
	"access-token":             true,
	"work-dir":                 true,
	"ffmpeg-path":              true,
	"mp4box-path":              true,
	"aria2c-path":              true,
	"upos-host":                true,
	"delay-per-page":           true,
	"host":                     true,
	"ep-host":                  true,
	"tv-host":                  true,
	"area":                     true,
	"config-file":              true,
}
