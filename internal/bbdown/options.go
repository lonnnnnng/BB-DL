package bbdown

import (
	"fmt"
	"strings"
)

func NormalizeOptions(opt *MyOption) {
	normalizeOptionsForCompatibility(opt, true)
}

func normalizeOptionsForCompatibility(opt *MyOption, emitWarnings bool) {
	if opt == nil {
		return
	}
	opt.FileExistsAction = NormalizeFileExistsAction(opt.FileExistsAction)
	warn := func(format string, args ...any) {
		if emitWarnings {
			Warnf(format, args...)
		}
	}
	if opt.Aria2cProxy != "" {
		warn("--aria2c-proxy 已被弃用, 请使用 --aria2c-args 来设置aria2c代理, 本次执行已添加该代理")
		proxyArg := fmt.Sprintf("--all-proxy=%q", opt.Aria2cProxy)
		// long 2026-06-18 23:16:28：服务模式父进程和子进程都会走兼容归一化，代理参数必须保持幂等，避免同一个代理被追加两次。
		if !strings.Contains(opt.Aria2cArgs, proxyArg) {
			if strings.TrimSpace(opt.Aria2cArgs) != "" {
				opt.Aria2cArgs += " "
			}
			opt.Aria2cArgs += proxyArg
		}
	}
	if opt.AddDfnSubfix {
		warn("--add-dfn-subfix 已被弃用, 建议使用 --file-pattern/-F 或 --multi-file-pattern/-M 来自定义输出文件名格式")
		if opt.FilePattern == "" && opt.MultiFilePattern == "" {
			opt.FilePattern = "<videoTitle>[<dfn>]"
			opt.MultiFilePattern = "<videoTitle>/[P<pageNumberWithZero>]<pageTitle>[<dfn>]"
			warn("已切换至 -F %q -M %q", opt.FilePattern, opt.MultiFilePattern)
		}
	}
	if opt.OnlyHevc {
		warn("--only-hevc/-hevc 已被弃用, 请使用 --encoding-priority 来设置编码优先级, 本次执行已将hevc设置为最高优先级")
		opt.EncodingPriority = "hevc"
	}
	if opt.OnlyAvc {
		warn("--only-avc/-avc 已被弃用, 请使用 --encoding-priority 来设置编码优先级, 本次执行已将avc设置为最高优先级")
		opt.EncodingPriority = "avc"
	}
	if opt.OnlyAv1 {
		warn("--only-av1/-av1 已被弃用, 请使用 --encoding-priority 来设置编码优先级, 本次执行已将av1设置为最高优先级")
		opt.EncodingPriority = "av1"
	}
	if opt.BandwithAscending {
		warn("--bandwith-ascending 已被弃用, 建议使用 --video-ascending 与 --audio-ascending 来指定视频或音频是否升序, 本次执行已将视频与音频均设为升序")
		opt.VideoAscending = true
		opt.AudioAscending = true
	}
	if opt.NoPaddingPageNum && opt.FilePattern == "" && opt.MultiFilePattern == "" {
		warn("--no-padding-page-num 已被弃用, 建议使用 --file-pattern/-F 或 --multi-file-pattern/-M 来自定义输出文件名格式")
		opt.MultiFilePattern = "<videoTitle>/[P<pageNumber>]<pageTitle>"
		warn("已切换至 -M %q", opt.MultiFilePattern)
	} else if opt.NoPaddingPageNum && opt.AddDfnSubfix && opt.MultiFilePattern == "<videoTitle>/[P<pageNumberWithZero>]<pageTitle>[<dfn>]" {
		warn("--no-padding-page-num 已被弃用, 建议使用 --file-pattern/-F 或 --multi-file-pattern/-M 来自定义输出文件名格式")
		opt.MultiFilePattern = "<videoTitle>/[P<pageNumber>]<pageTitle>[<dfn>]"
		warn("已切换至 -M %q", opt.MultiFilePattern)
	}
	if opt.Interactive {
		opt.HideStreams = false
	}
	if opt.AudioOnly && opt.VideoOnly {
		opt.AudioOnly = false
		opt.VideoOnly = false
	}
	if opt.SkipSubtitle {
		opt.SubOnly = false
	}
}
