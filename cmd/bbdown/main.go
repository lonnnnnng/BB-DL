package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lonnnnnng/BB-DL/internal/bbdown"
)

var pageRetryDelay = 3 * time.Second

func main() {
	cfg := bbdown.NewConfig()
	httpc := bbdown.NewHTTPClient(cfg)

	if len(os.Args) < 2 {
		usage()
		return
	}
	if handled, err := handleRootMeta(os.Stdout, os.Args[1:]); handled {
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return
	}

	switch rootCommandName(os.Args[1]) {
	case "info":
		runInfo(cfg, httpc, os.Args[2:])
	case "login":
		if handled, err := handleLoginMeta(os.Stdout, os.Args[2:], "login", "WEB 二维码登录，扫码成功后保存 BBDown.data"); handled {
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			return
		}
		if err := bbdown.LoginWEB(context.Background(), httpc); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case "logintv":
		if handled, err := handleLoginMeta(os.Stdout, os.Args[2:], "logintv", "TV 二维码登录，扫码成功后保存 BBDownTV.data"); handled {
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			return
		}
		if err := bbdown.LoginTV(context.Background(), httpc); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case "doctor":
		if err := runDoctor(os.Stdout, os.Args[2:]); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case "serve":
		runServe(cfg, httpc, os.Args[2:])
	case "download", "down":
		runDownloadCommand(cfg, httpc, rootCommandName(os.Args[1]), rootDownloadArgs(os.Args[1:]))
	default:
		runDownload(cfg, httpc, os.Args[1:])
	}
}

func rootCommandName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func rootDownloadArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	switch rootCommandName(args[0]) {
	case "download", "down":
		// long 2026-06-28 00:08:00：文档和帮助已经把 download 当作主题；显式子命令也应去掉命令名本身，避免把 "download" 当作视频输入。
		return args[1:]
	default:
		return args
	}
}

type doctorOptions struct {
	FFmpegPath string
	MP4BoxPath string
	Aria2cPath string
	JSON       bool
	Help       bool
}

type doctorReport struct {
	Version     string                 `json:"version"`
	BuildTime   string                 `json:"buildTime"`
	Executable  string                 `json:"executable"`
	AppDir      string                 `json:"appDir"`
	WorkingDir  string                 `json:"workingDir"`
	Runtime     doctorRuntimeInfo      `json:"runtime"`
	Credentials []doctorCredentialInfo `json:"credentials"`
	Tools       []doctorToolInfo       `json:"tools"`
}

type doctorRuntimeInfo struct {
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GoVersion string `json:"goVersion"`
}

type doctorCredentialInfo struct {
	Name  string `json:"name"`
	File  string `json:"file"`
	Found bool   `json:"found"`
	Path  string `json:"path,omitempty"`
}

type doctorToolInfo struct {
	Name        string `json:"name"`
	Found       bool   `json:"found"`
	Path        string `json:"path,omitempty"`
	Version     string `json:"version,omitempty"`
	Input       string `json:"input,omitempty"`
	Note        string `json:"note"`
	Flag        string `json:"flag,omitempty"`
	InstallHint string `json:"installHint,omitempty"`
}

func runDoctor(out io.Writer, args []string) error {
	var opt doctorOptions
	fs := newFlagSetWithUsage("doctor", "BB-DL doctor [选项]")
	fs.SetOutput(out)
	fs.StringVar(&opt.FFmpegPath, "ffmpeg-path", "", "指定 ffmpeg 可执行文件路径或命令名")
	fs.StringVar(&opt.MP4BoxPath, "mp4box-path", "", "指定 MP4Box 可执行文件路径或命令名")
	fs.StringVar(&opt.Aria2cPath, "aria2c-path", "", "指定 aria2c 可执行文件路径或命令名")
	fs.BoolVar(&opt.JSON, "json", false, "以 JSON 格式输出诊断结果，便于脚本或反馈时使用")
	registerHelpFlags(fs, &opt.Help)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if opt.Help {
		fs.Usage()
		return nil
	}

	report := buildDoctorReport(opt)
	if opt.JSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	printDoctorReport(out, report)
	return nil
}

func buildDoctorReport(opt doctorOptions) doctorReport {
	wd, err := os.Getwd()
	if err != nil {
		wd = "获取失败: " + err.Error()
	}
	return doctorReport{
		Version:     bbdown.Version,
		BuildTime:   bbdown.BuildTime,
		Executable:  doctorExecutablePath(),
		AppDir:      bbdown.AppDir(),
		WorkingDir:  wd,
		Runtime:     doctorRuntimeInfo{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: runtime.Version()},
		Credentials: []doctorCredentialInfo{doctorCredential("WEB Cookie", "BBDown.data"), doctorCredential("TV Token", "BBDownTV.data"), doctorCredential("APP Token", "BBDownApp.data")},
		Tools: []doctorToolInfo{
			doctorTool("ffmpeg", opt.FFmpegPath, []string{"ffmpeg"}, "默认混流需要", "--ffmpeg-path", "ffmpeg"),
			doctorTool("MP4Box", opt.MP4BoxPath, []string{"mp4box", "MP4box", "MP4Box"}, "使用 --use-mp4box 或低版本 ffmpeg 处理杜比视界时需要", "--mp4box-path", "gpac"),
			doctorTool("aria2c", opt.Aria2cPath, []string{"aria2c"}, "使用 --use-aria2c 时需要", "--aria2c-path", "aria2"),
		},
	}
}

func printDoctorReport(out io.Writer, report doctorReport) {
	fmt.Fprintln(out, "BB-DL 诊断")
	fmt.Fprintf(out, "版本: %s\n", report.Version)
	fmt.Fprintf(out, "构建时间: %s\n", report.BuildTime)
	fmt.Fprintf(out, "可执行文件: %s\n", report.Executable)
	fmt.Fprintf(out, "平台: %s/%s\n", report.Runtime.GOOS, report.Runtime.GOARCH)
	fmt.Fprintf(out, "Go运行时: %s\n", report.Runtime.GoVersion)
	fmt.Fprintf(out, "程序目录: %s\n", report.AppDir)
	fmt.Fprintf(out, "当前目录: %s\n", report.WorkingDir)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "登录态文件:")
	for _, credential := range report.Credentials {
		printDoctorFile(out, report.AppDir, credential)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "外部工具:")
	for _, tool := range report.Tools {
		printDoctorTool(out, tool)
	}
}

func doctorCredential(label, name string) doctorCredentialInfo {
	if path, ok := findRuntimeFile(name); ok {
		return doctorCredentialInfo{Name: label, File: name, Found: true, Path: path}
	}
	return doctorCredentialInfo{Name: label, File: name}
}

func printDoctorFile(out io.Writer, appDir string, credential doctorCredentialInfo) {
	if credential.Found {
		fmt.Fprintf(out, "  %s: 已找到 %s\n", credential.Name, credential.Path)
		return
	}
	fmt.Fprintf(out, "  %s: 未找到（检查 %s 和当前目录）\n", credential.Name, appDir)
}

func findRuntimeFile(name string) (string, bool) {
	candidates := uniquePaths(filepath.Join(bbdown.AppDir(), name), filepath.Join(".", name))
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Size() > 0 {
			return absolutePathForDoctor(path), true
		}
	}
	return "", false
}

func doctorTool(label, explicit string, names []string, note, flag, brewPackage string) doctorToolInfo {
	path := bbdown.FindBinaryPath(explicit, names...)
	if path == "" {
		installHint := doctorInstallHint(label, flag, brewPackage)
		return doctorToolInfo{Name: label, Input: strings.TrimSpace(explicit), Note: note, Flag: flag, InstallHint: installHint}
	}
	absPath := absolutePathForDoctor(path)
	return doctorToolInfo{Name: label, Found: true, Path: absPath, Version: doctorToolVersion(label, absPath), Note: note, Flag: flag}
}

func printDoctorTool(out io.Writer, tool doctorToolInfo) {
	if !tool.Found {
		fmt.Fprintf(out, "  %s: 未找到（%s）\n", tool.Name, tool.Note)
		if tool.Input != "" {
			fmt.Fprintf(out, "    当前指定值不可用: %s\n", tool.Input)
		}
		if tool.Flag != "" {
			fmt.Fprintf(out, "    可使用 %s 指定完整路径\n", tool.Flag)
		}
		if tool.InstallHint != "" {
			fmt.Fprintf(out, "    %s\n", tool.InstallHint)
		}
		return
	}
	if tool.Version != "" {
		fmt.Fprintf(out, "  %s: 已找到 %s；版本: %s（%s）\n", tool.Name, tool.Path, tool.Version, tool.Note)
		return
	}
	fmt.Fprintf(out, "  %s: 已找到 %s（%s）\n", tool.Name, tool.Path, tool.Note)
}

func doctorInstallHint(label, flag, brewPackage string) string {
	if runtime.GOOS == "darwin" && brewPackage != "" {
		return "macOS 可执行 brew install " + brewPackage
	}
	if strings.TrimSpace(label) == "" {
		return ""
	}
	if flag != "" {
		return "请安装 " + label + "，或使用 " + flag + " 指定完整路径"
	}
	return "请安装 " + label
}

func doctorToolVersion(label, path string) string {
	arg := "-version"
	if strings.EqualFold(label, "aria2c") {
		arg = "--version"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, arg).CombinedOutput()
	if err != nil {
		return ""
	}
	return firstNonEmptyLine(string(out))
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func uniquePaths(paths ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func absolutePathForDoctor(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func doctorExecutablePath() string {
	path, err := os.Executable()
	if err != nil || strings.TrimSpace(path) == "" {
		return ""
	}
	return absolutePathForDoctor(path)
}

func runServe(cfg *bbdown.Config, httpc *bbdown.HTTPClient, args []string) {
	var listen string
	var help bool
	fs := newServeFlagSet(&listen, &help)
	fs.Parse(args)
	if help {
		fs.Usage()
		return
	}
	addr, err := bbdown.NormalizeListenAddr(listen)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	srv := bbdown.NewApiServer(cfg, httpc)
	bbdown.CheckUpdateAsync(httpc)
	if err := srv.Run(addr); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runInfo(cfg *bbdown.Config, httpc *bbdown.HTTPClient, args []string) {
	var opt bbdown.MyOption
	args = normalizeFlagOrder(normalizeBoolFlagValues(args))
	infoCmd := newInfoFlagSet("info", &opt, cfg)
	infoCmd.Parse(args)
	if handleMetaFlags(infoCmd, &opt) {
		return
	}

	mergedArgs := bbdown.MergeConfigArgs(args, opt.ConfigFile)
	mergedArgs = normalizeFlagOrder(normalizeBoolFlagValues(mergedArgs))
	if len(mergedArgs) > 0 {
		opt = bbdown.MyOption{}
		infoCmd = newInfoFlagSet("info", &opt, cfg)
		infoCmd.Parse(mergedArgs)
		if handleMetaFlags(infoCmd, &opt) {
			return
		}
	}
	normalizeOptions(&opt)
	encodingFirst := prefersEncodingPriorityFirst(mergedArgs)

	url := requireSingleURL(infoCmd)

	if !bbdown.IsServerChildProcess() {
		printStartupBanner()
	}
	opt.Url = url
	cfg.Cookie = opt.Cookie
	cfg.Token = strings.TrimPrefix(opt.AccessToken, "access_token=")
	cfg.Debug = opt.Debug
	cfg.Host = opt.Host
	cfg.EpHost = opt.EpHost
	cfg.TvHost = opt.TvHost
	cfg.Area = opt.Area
	httpc.SetUserAgent(opt.UserAgent)
	encodingPriority := bbdown.ParseEncodingPriority(opt.EncodingPriority)
	dfnPriority := bbdown.ParseDfnPriority(opt.DfnPriority)
	if err := applyWorkDir(&opt); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	logDebugSetup(cfg, &opt)
	if !bbdown.IsServerChildProcess() {
		bbdown.CheckUpdateAsync(httpc)
	}

	ctx := context.Background()
	aidOri, vInfo, apiType, err := bbdown.GetVideoInfo(ctx, cfg, httpc, &opt, "", opt.Url)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	_ = aidOri
	bbdown.PrintVideoInfo(vInfo, apiType, opt.ShowAll)
	apiType = bbdown.ApplySteinGateTVFallback(vInfo, &opt)
	if len(vInfo.PagesInfo) == 0 {
		return
	}
	page := vInfo.PagesInfo[0]
	firstEncoding := bbdown.FirstEncodingPriority(opt.EncodingPriority)
	tracks, err := bbdown.ExtractTracks(ctx, cfg, httpc, aidOri, page.Aid, page.Cid, page.Epid, opt.UseTvApi, opt.UseIntlApi, opt.UseAppApi, firstEncoding, "")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	tracks.VideoTracks = bbdown.SortTracksVideoWithPriorityOrder(tracks.VideoTracks, dfnPriority, encodingPriority, opt.VideoAscending, encodingFirst)
	tracks.AudioTracks = bbdown.SortTracksAudioWithPriority(tracks.AudioTracks, encodingPriority, opt.AudioAscending)
	bbdown.PrintTracks(tracks, page.Dur, opt.OnlyShowInfo)
}

func usage() {
	printRootUsage(os.Stdout)
}

func handleRootMeta(out io.Writer, args []string) (bool, error) {
	if len(args) == 0 {
		printRootUsage(out)
		return true, nil
	}
	switch rootCommandName(args[0]) {
	case "help":
		if len(args) == 1 {
			printRootUsage(out)
			return true, nil
		}
		if len(args) > 2 {
			return true, fmt.Errorf("help 只支持一个命令名: %s", strings.Join(args[1:], " "))
		}
		if isRootHelpFlag(args[1]) {
			printHelpCommandUsage(out)
			return true, nil
		}
		return true, printHelpTopic(out, args[1])
	case "version":
		if len(args) == 2 && isRootHelpFlag(args[1]) {
			printVersionCommandUsage(out)
			return true, nil
		}
		if len(args) > 1 {
			return true, fmt.Errorf("version 不支持额外参数: %s", strings.Join(args[1:], " "))
		}
		printVersionInfoTo(out)
		return true, nil
	default:
		return false, nil
	}
}

func printRootUsage(out io.Writer) {
	fmt.Fprintln(out, "BBDown 风格复刻版（Go）")
	fmt.Fprintln(out, "用法: BB-DL <url>、BB-DL download <url>、BB-DL help [命令]、BB-DL version、BB-DL info <url>、BB-DL login、BB-DL logintv、BB-DL doctor 或 BB-DL serve")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "可用命令:")
	fmt.Fprintln(out, "  download, down  显式下载命令，等价于直接传入 <url>")
	fmt.Fprintln(out, "  help [命令]     显示总览或指定命令帮助")
	fmt.Fprintln(out, "  version         显示当前版本和构建时间")
	fmt.Fprintln(out, "  info <url>      只展示视频信息和可用流，不下载")
	fmt.Fprintln(out, "  login           WEB 二维码登录，保存 BBDown.data")
	fmt.Fprintln(out, "  logintv         TV 二维码登录，保存 BBDownTV.data")
	fmt.Fprintln(out, "  doctor          诊断版本、登录态文件和外部工具路径")
	fmt.Fprintln(out, "  serve           启动 HTTP 任务服务")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "查看帮助:")
	fmt.Fprintln(out, "  BB-DL --help")
	fmt.Fprintln(out, "  BB-DL help download")
	fmt.Fprintln(out, "  BB-DL help login")
}

func printHelpCommandUsage(out io.Writer) {
	fmt.Fprintln(out, "用法: BB-DL help [命令]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "显示总览或指定命令帮助。")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "可用主题:")
	fmt.Fprintln(out, "  download, down   默认下载命令")
	fmt.Fprintln(out, "  info             只展示视频信息和可用流")
	fmt.Fprintln(out, "  login            WEB 二维码登录")
	fmt.Fprintln(out, "  logintv          TV 二维码登录")
	fmt.Fprintln(out, "  doctor           环境诊断")
	fmt.Fprintln(out, "  serve            HTTP 任务服务")
	fmt.Fprintln(out, "  version          版本和构建时间")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "示例:")
	fmt.Fprintln(out, "  BB-DL help")
	fmt.Fprintln(out, "  BB-DL help download")
}

func printHelpTopic(out io.Writer, topic string) error {
	topic = strings.ToLower(strings.TrimSpace(topic))
	switch topic {
	case "bbdown", "root":
		printRootUsage(out)
	case "help":
		printHelpCommandUsage(out)
	case "", "download", "down":
		var opt bbdown.MyOption
		fs := newDownloadFlagSet("bbdown", &opt, bbdown.NewConfig())
		fs.SetOutput(out)
		fs.Usage()
	case "info":
		var opt bbdown.MyOption
		fs := newInfoFlagSet("info", &opt, bbdown.NewConfig())
		fs.SetOutput(out)
		fs.Usage()
	case "login":
		printLoginUsage(out, "login", "WEB 二维码登录，扫码成功后保存 BBDown.data")
	case "logintv":
		printLoginUsage(out, "logintv", "TV 二维码登录，扫码成功后保存 BBDownTV.data")
	case "doctor":
		return runDoctor(out, []string{"--help"})
	case "serve":
		var listen string
		var help bool
		fs := newServeFlagSet(&listen, &help)
		fs.SetOutput(out)
		fs.Usage()
	case "version":
		printVersionCommandUsage(out)
	default:
		return fmt.Errorf("未知帮助主题: %s", topic)
	}
	return nil
}

func printVersionCommandUsage(out io.Writer) {
	fmt.Fprintln(out, "用法: BB-DL version")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "显示当前版本和构建时间。")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "等价命令:")
	fmt.Fprintln(out, "  BB-DL --version")
}

func isRootHelpFlag(value string) bool {
	// long 2026-06-27 23:35:00：`BB-DL help --help` 是用户查 help 命令本身的自然写法，不能把 `--help` 当成未知主题。
	switch strings.TrimSpace(value) {
	case "--help", "-h", "-?":
		return true
	default:
		return false
	}
}

func handleLoginMeta(out io.Writer, args []string, command, description string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if containsLoginHelpFlag(args) {
		printLoginUsage(out, command, description)
		return true, nil
	}
	return true, fmt.Errorf("%s 不支持额外参数: %s", command, strings.Join(args, " "))
}

func containsLoginHelpFlag(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-?", "-h", "--help":
			return true
		}
	}
	return false
}

func printLoginUsage(out io.Writer, command, description string) {
	fmt.Fprintf(out, "用法: BB-DL %s\n\n%s\n\n参数:\n", command, description)
	fmt.Fprintln(out, "  -?, -h, --help")
	fmt.Fprintln(out, "        显示当前命令帮助")
}

func printStartupBanner() {
	printVersionInfo()
}

func newInfoFlagSet(name string, opt *bbdown.MyOption, cfg *bbdown.Config) *flag.FlagSet {
	fs := newFlagSetWithUsage(name, "BB-DL info [选项] <url>")
	registerCommonFlags(fs, opt, cfg)
	registerPriorityFlags(fs, opt)
	return fs
}

func newDownloadFlagSet(name string, opt *bbdown.MyOption, cfg *bbdown.Config) *flag.FlagSet {
	fs := newFlagSetWithUsage(name, downloadUsageLine(name))
	registerCommonFlags(fs, opt, cfg)
	registerPriorityFlags(fs, opt)
	fs.BoolVar(&opt.UseMP4box, "use-mp4box", false, "使用 MP4Box 混流；默认使用 ffmpeg")
	fs.BoolVar(&opt.UseAria2c, "use-aria2c", false, "使用外部 aria2c 下载资源")
	fs.BoolVar(&opt.UseAria2c, "aria2", false, "同 --use-aria2c，使用外部 aria2c 下载资源")
	fs.StringVar(&opt.Aria2cArgs, "aria2c-args", "", "传给 aria2c 的额外参数，例如 \"-x 16 -s 16\"")
	fs.StringVar(&opt.Aria2cProxy, "aria2c-proxy", "", "旧参数兼容：转换为 aria2c 的 --all-proxy 参数")
	fs.BoolVar(&opt.Interactive, "interactive", false, "交互式按序号选择视频流和音频流")
	fs.BoolVar(&opt.Interactive, "ia", false, "同 --interactive，交互式选择音视频流")
	fs.StringVar(&opt.VideoIndex, "video-index", "", "按可用流列表序号选择视频流，0 表示第一条；留空自动选择")
	fs.StringVar(&opt.AudioIndex, "audio-index", "", "按可用流列表序号选择音频流，0 表示第一条；留空自动选择")
	fs.BoolVar(&opt.VideoOnly, "video-only", false, "只下载视频轨道，不下载音频")
	fs.BoolVar(&opt.AudioOnly, "audio-only", false, "只下载音频轨道，不下载视频")
	fs.BoolVar(&opt.DanmakuOnly, "danmaku-only", false, "只下载弹幕，不下载媒体")
	fs.BoolVar(&opt.CoverOnly, "cover-only", false, "只下载封面，不下载媒体")
	fs.BoolVar(&opt.SubOnly, "sub-only", false, "只下载字幕，不下载媒体")
	fs.BoolVar(&opt.SkipMux, "skip-mux", false, "跳过混流，保留下载后的音视频分轨文件")
	fs.BoolVar(&opt.SkipSubtitle, "skip-subtitle", false, "跳过字幕下载和字幕混流")
	fs.BoolVar(&opt.SkipCover, "skip-cover", false, "跳过封面下载")
	fs.BoolVar(&opt.DownloadDanmaku, "download-danmaku", false, "随媒体一起下载弹幕")
	fs.BoolVar(&opt.DownloadDanmaku, "dd", false, "同 --download-danmaku，随媒体一起下载弹幕")
	fs.StringVar(&opt.DownloadDanmakuFormats, "download-danmaku-formats", "", "弹幕保存格式，支持 xml、ass，可用逗号分隔")
	fs.StringVar(&opt.DownloadDanmakuFormats, "ddf", "", "同 --download-danmaku-formats，弹幕保存格式")
	fs.BoolVar(&opt.VideoAscending, "video-ascending", false, "视频流按低优先级到高优先级排序")
	fs.BoolVar(&opt.AudioAscending, "audio-ascending", false, "音频流按低码率到高码率排序")
	fs.BoolVar(&opt.ForceReplaceHost, "force-replace-host", true, "替换可控 UPOS/PCDN host；设为 false 可保留原始 host")
	fs.BoolVar(&opt.SaveArchivesToFile, "save-archives-to-file", false, "把已下载 Aid 记录到 BBDown.archives，并跳过已记录内容")
	fs.BoolVar(&opt.SimplyMux, "simply-mux", false, "简化混流，不写入 BBDown 额外 metadata")
	fs.StringVar(&opt.FileExistsAction, "file-exists-action", bbdown.FileExistsActionSkip, "同名文件处理方式：rename 追加流水号、skip 跳过、overwrite 覆盖")
	fs.StringVar(&opt.FFmpegPath, "ffmpeg-path", "", "指定 ffmpeg 可执行文件路径")
	fs.StringVar(&opt.Mp4boxPath, "mp4box-path", "", "指定 MP4Box 可执行文件路径")
	fs.StringVar(&opt.Aria2cPath, "aria2c-path", "", "指定 aria2c 可执行文件路径")
	fs.StringVar(&opt.UposHost, "upos-host", "", "指定替换用的 UPOS host")
	fs.StringVar(&opt.DelayPerPage, "delay-per-page", "0", "多 P 下载时每页之间的等待秒数")
	fs.BoolVar(&opt.OnlyHevc, "only-hevc", false, "旧参数兼容：优先选择 HEVC 编码")
	fs.BoolVar(&opt.OnlyHevc, "hevc", false, "同 --only-hevc，优先选择 HEVC 编码")
	fs.BoolVar(&opt.OnlyAvc, "only-avc", false, "旧参数兼容：优先选择 AVC 编码")
	fs.BoolVar(&opt.OnlyAvc, "avc", false, "同 --only-avc，优先选择 AVC 编码")
	fs.BoolVar(&opt.OnlyAv1, "only-av1", false, "旧参数兼容：优先选择 AV1 编码")
	fs.BoolVar(&opt.OnlyAv1, "av1", false, "同 --only-av1，优先选择 AV1 编码")
	fs.BoolVar(&opt.AddDfnSubfix, "add-dfn-subfix", false, "旧参数兼容：在输出文件名中追加清晰度后缀")
	fs.BoolVar(&opt.NoPaddingPageNum, "no-padding-page-num", false, "旧参数兼容：多 P 页码不补零")
	fs.BoolVar(&opt.BandwithAscending, "bandwith-ascending", false, "旧参数兼容：音视频按低码率优先排序")
	return fs
}

func downloadUsageLine(name string) string {
	switch rootCommandName(name) {
	case "download":
		return "BB-DL download [选项] <url>"
	case "down":
		return "BB-DL down [选项] <url>"
	default:
		return "BB-DL [选项] <url>"
	}
}

func registerCommonFlags(fs *flag.FlagSet, opt *bbdown.MyOption, cfg *bbdown.Config) {
	fs.BoolVar(&opt.UseTvApi, "use-tv-api", false, "使用 TV 播放接口解析")
	fs.BoolVar(&opt.UseTvApi, "tv", false, "同 --use-tv-api，使用 TV 播放接口解析")
	fs.BoolVar(&opt.UseAppApi, "use-app-api", false, "使用 APP 接口解析，优先 gRPC，失败后回退 REST")
	fs.BoolVar(&opt.UseAppApi, "app", false, "同 --use-app-api，使用 APP 接口解析")
	fs.BoolVar(&opt.UseIntlApi, "use-intl-api", false, "使用国际版接口解析")
	fs.BoolVar(&opt.UseIntlApi, "intl", false, "同 --use-intl-api，使用国际版接口解析")
	fs.BoolVar(&opt.OnlyShowInfo, "only-show-info", false, "只展示视频信息和可用流，不下载")
	fs.BoolVar(&opt.OnlyShowInfo, "info", false, "同 --only-show-info，只展示信息")
	fs.BoolVar(&opt.ShowAll, "show-all", false, "展示全部分 P 信息")
	fs.BoolVar(&opt.HideStreams, "hide-streams", false, "隐藏可用视频、音频和字幕流列表")
	fs.BoolVar(&opt.HideStreams, "hs", false, "同 --hide-streams，隐藏流列表")
	fs.BoolVar(&opt.MultiThread, "multi-thread", true, "启用 Range 多线程下载")
	fs.BoolVar(&opt.MultiThread, "mt", true, "同 --multi-thread，启用 Range 多线程下载")
	fs.BoolVar(&opt.Debug, "debug", false, "输出调试日志并保存接口 JSON")
	fs.BoolVar(&opt.ForceHttp, "force-http", true, "媒体下载地址尽量改用 http")
	fs.BoolVar(&opt.SkipAi, "skip-ai", true, "跳过 AI 字幕")
	fs.BoolVar(&opt.AllowPcdn, "allow-pcdn", false, "允许使用 PCDN 下载地址")
	fs.StringVar(&opt.FilePattern, "file-pattern", "", "单 P 输出文件名模板")
	fs.StringVar(&opt.FilePattern, "F", "", "同 --file-pattern，单 P 输出文件名模板")
	fs.StringVar(&opt.MultiFilePattern, "multi-file-pattern", "", "多 P 输出文件名模板")
	fs.StringVar(&opt.MultiFilePattern, "M", "", "同 --multi-file-pattern，多 P 输出文件名模板")
	fs.StringVar(&opt.SelectPage, "select-page", "", "选择分 P，支持 ALL、1,2、1-3、LAST/LATEST/NEW")
	fs.StringVar(&opt.SelectPage, "p", "", "同 --select-page，选择分 P")
	fs.StringVar(&opt.Language, "language", "", "写入混流音轨语言 metadata，例如 jpn、eng、chi")
	fs.StringVar(&opt.UserAgent, "user-agent", "", "自定义请求 User-Agent")
	fs.StringVar(&opt.UserAgent, "ua", "", "同 --user-agent，自定义请求 User-Agent")
	fs.StringVar(&opt.Cookie, "cookie", "", "自定义 B 站 Cookie；建议优先使用 login 保存登录态")
	fs.StringVar(&opt.Cookie, "c", "", "同 --cookie，自定义 B 站 Cookie")
	fs.StringVar(&opt.AccessToken, "access-token", "", "自定义 access token，用于 TV/APP/国际版等接口")
	fs.StringVar(&opt.AccessToken, "token", "", "同 --access-token，自定义 access token")
	fs.StringVar(&opt.WorkDir, "work-dir", "", "下载工作目录；默认使用当前目录")
	fs.StringVar(&opt.Host, "host", cfg.Host, "自定义 BiliPlus / 播放接口 host")
	fs.StringVar(&opt.EpHost, "ep-host", cfg.EpHost, "自定义番剧接口 host")
	fs.StringVar(&opt.TvHost, "tv-host", cfg.TvHost, "自定义 TV 接口 host")
	fs.StringVar(&opt.Area, "area", "", "区域参数，用于 BiliPlus / 代理解析")
	fs.StringVar(&opt.ConfigFile, "config-file", "", "指定配置文件，命令行显式参数会覆盖配置")
	fs.BoolVar(&opt.Version, "version", false, "显示当前版本号")
	registerHelpFlags(fs, &opt.Help)
}

func registerPriorityFlags(fs *flag.FlagSet, opt *bbdown.MyOption) {
	fs.StringVar(&opt.EncodingPriority, "encoding-priority", "", "编码优先级，逗号分隔；支持 hevc、avc、av1")
	fs.StringVar(&opt.EncodingPriority, "e", "", "同 --encoding-priority，编码优先级")
	fs.StringVar(&opt.DfnPriority, "dfn-priority", "", "清晰度优先级，逗号分隔；例如 \"1080P 高清,720P 高清\"")
	fs.StringVar(&opt.DfnPriority, "q", "", "同 --dfn-priority，清晰度优先级")
}

func newFlagSetWithUsage(name, usageLine string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "用法: %s\n\n参数:\n", usageLine)
		fs.PrintDefaults()
	}
	return fs
}

func registerHelpFlags(fs *flag.FlagSet, target *bool) {
	fs.BoolVar(target, "?", false, "显示当前命令帮助")
	fs.BoolVar(target, "h", false, "同 --help，显示当前命令帮助")
	fs.BoolVar(target, "help", false, "显示当前命令帮助")
}

func normalizeOptions(opt *bbdown.MyOption) {
	bbdown.NormalizeOptions(opt)
}

func savePathPattern(opt *bbdown.MyOption, pagesCount int) string {
	if pagesCount > 1 && opt.MultiFilePattern != "" {
		return opt.MultiFilePattern
	}
	return opt.FilePattern
}

func savePathPatternForVideo(opt *bbdown.MyOption, vInfo *bbdown.VInfo, totalPagesCount int) string {
	// long: 原版按视频原始总页数决定默认单 P/多 P 路径，用户只选择一个分 P 时也不能退回单 P 命名。
	if totalPagesCount > 1 || (vInfo != nil && vInfo.IsBangumi && !vInfo.IsBangumiEnd) {
		if opt.MultiFilePattern != "" {
			return opt.MultiFilePattern
		}
		return "<videoTitle>/[P<pageNumberWithZero>]<pageTitle>"
	}
	return opt.FilePattern
}

func selectedPagesText(selected []string) string {
	if selected == nil {
		return "ALL"
	}
	return strings.Join(selected, ",")
}

func subtitleMuxSuffix(subtitles []bbdown.Subtitle) string {
	if len(subtitles) == 0 {
		return ""
	}
	return "和字幕"
}

func originalChapterPoints(fetchPoints, extraPoints []bbdown.ViewPoint) []bbdown.ViewPoint {
	// long: 原版先查普通章节接口，只有接口没有章节时才沿用播放解析里附带的番剧/APP 章节，避免两路章节同时存在时改写用户最终文件里的章节来源。
	if len(fetchPoints) > 0 {
		return fetchPoints
	}
	return extraPoints
}

func sidecarWorkBeforeExistingMediaSkip(opt *bbdown.MyOption) bool {
	if opt == nil {
		return false
	}
	// long: 原版在判断最终媒体文件已存在前，会先执行用户显式要的弹幕/封面 only 任务，以及普通下载默认附带的字幕处理；这里把这些 sidecar 产物放在跳过主媒体之前，避免现成 mp4 把用户真正请求的资源短路掉。
	if opt.CoverOnly || opt.DanmakuOnly || opt.DownloadDanmaku {
		return true
	}
	return !opt.SkipSubtitle && !opt.DanmakuOnly && !opt.CoverOnly
}

func logDebugSavePath(cfg *bbdown.Config, savePathFormat, savePath string) {
	if cfg == nil || !cfg.Debug {
		return
	}
	bbdown.Logf("Format Before: %s", savePathFormat)
	bbdown.Logf("Format After: %s", savePath)
}

func logDebugSetup(cfg *bbdown.Config, opt *bbdown.MyOption) {
	if cfg == nil || !cfg.Debug {
		return
	}
	bbdown.Logf("AppDirectory: %s", bbdown.AppDir())
	body, err := json.Marshal(opt)
	if err != nil {
		bbdown.Logf("运行参数：%+v", opt)
		return
	}
	bbdown.Logf("运行参数：%s", string(body))
}

func finalizeDanmakuFiles(danmakuPath, danmakuAssPath string, formats map[bbdown.DanmakuFormat]bool, opt *bbdown.MyOption) error {
	items, err := bbdown.ParseDanmakuXMLFile(danmakuPath)
	if err != nil {
		bbdown.Log("弹幕Xml解析失败, 删除Xml...")
		removeIfExists(danmakuPath)
		return nil
	}
	if len(items) == 0 {
		bbdown.Log("当前视频没有弹幕, 删除Xml...")
		removeIfExists(danmakuPath)
		return nil
	}
	if formats[bbdown.DanmakuASS] {
		if bbdown.ShouldSkipExistingFile(danmakuAssPath, opt) {
			bbdown.Logf("%s已存在, 跳过下载...", danmakuAssPath)
		} else {
			// long 2026-07-27 01:41:55：ASS 由 XML 在本地转换生成，不经过 DownloadResource；这里单独执行同名策略，避免 skip 模式仍截断已有弹幕文件。
			bbdown.Log("正在保存弹幕Ass文件...")
			if err := bbdown.SaveDanmakuASS(items, danmakuAssPath); err != nil {
				return err
			}
		}
	}
	if !formats[bbdown.DanmakuXML] {
		removeIfExists(danmakuPath)
	}
	return nil
}

func applyOnlyModeToTracks(opt *bbdown.MyOption, tracks *bbdown.ParsedTracks) {
	if opt == nil || tracks == nil || len(tracks.Clips) > 0 {
		return
	}
	if len(tracks.VideoTracks) == 0 {
		bbdown.Warnf("没有找到符合要求的视频流")
		if opt.VideoOnly {
			tracks.AudioTracks = nil
		}
	}
	if len(tracks.AudioTracks) == 0 {
		bbdown.Warnf("没有找到符合要求的音频流")
		if opt.AudioOnly {
			tracks.VideoTracks = nil
		}
	}
	if opt.AudioOnly {
		tracks.VideoTracks = nil
	}
	if opt.VideoOnly {
		tracks.AudioTracks = nil
		tracks.BackgroundAudios = nil
		tracks.RoleAudioLists = nil
	}
}

func shouldSkipMissingOnlyMode(opt *bbdown.MyOption, tracks *bbdown.ParsedTracks) bool {
	if opt == nil || tracks == nil || len(tracks.Clips) > 0 {
		return false
	}
	if opt.VideoOnly && len(tracks.VideoTracks) == 0 {
		return true
	}
	if opt.AudioOnly && len(tracks.AudioTracks) == 0 {
		return true
	}
	return false
}

// long: 单个分 P 下载遇到临时网络或混流异常时只重试当前页，避免把已完成页重复下载或提前写入下载归档。
func shouldRetryPageDownload(err error, retryCount *int) bool {
	*retryCount = *retryCount + 1
	if *retryCount > 2 {
		return false
	}
	bbdown.Errorf("%v", err)
	bbdown.Warnf("下载出现异常, 3秒后将进行自动重试...")
	time.Sleep(pageRetryDelay)
	return true
}

func ensureFLVMergeBinary(opt *bbdown.MyOption, clipCount int) error {
	if clipCount <= 1 {
		return nil
	}
	return bbdown.ResolveFFmpegBinary(opt)
}

func recordArchiveIfNeeded(opt *bbdown.MyOption, aid string) {
	if opt != nil && opt.SaveArchivesToFile {
		_ = bbdown.SaveAidToArchive(aid)
	}
}

func finishPageLikeOriginal(opt *bbdown.MyOption, aid string) {
	// long 2026-06-19 08:50:12：原版在 DownloadPageAsync 正常返回后由外层统一写归档；
	// 合并失败、only 模式缺流或解析不到轨道这类“记录日志后 return”的分支也会进入归档文件。
	recordArchiveIfNeeded(opt, aid)
}

func runDownload(cfg *bbdown.Config, httpc *bbdown.HTTPClient, args []string) {
	runDownloadCommand(cfg, httpc, "bbdown", args)
}

func runDownloadCommand(cfg *bbdown.Config, httpc *bbdown.HTTPClient, commandName string, args []string) {
	var opt bbdown.MyOption
	args = normalizeFlagOrder(normalizeBoolFlagValues(args))
	infoCmd := newDownloadFlagSet(commandName, &opt, cfg)
	infoCmd.Parse(args)
	if handleMetaFlags(infoCmd, &opt) {
		return
	}
	mergedArgs := bbdown.MergeConfigArgs(args, opt.ConfigFile)
	mergedArgs = normalizeFlagOrder(normalizeBoolFlagValues(mergedArgs))
	if len(mergedArgs) > 0 {
		opt = bbdown.MyOption{}
		infoCmd = newDownloadFlagSet(commandName, &opt, cfg)
		infoCmd.Parse(mergedArgs)
		if handleMetaFlags(infoCmd, &opt) {
			return
		}
	}
	normalizeOptions(&opt)
	bbdown.NormalizeTrackIndexOptions(&opt)
	encodingFirst := prefersEncodingPriorityFirst(mergedArgs)
	url := requireSingleURL(infoCmd)
	if err := validateDownloadStartupOptions(&opt); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if !bbdown.IsServerChildProcess() {
		printStartupBanner()
	}
	if err := bbdown.ResolveRequiredBinaries(&opt); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	opt.Url = url
	cfg.Cookie = opt.Cookie
	cfg.Token = strings.TrimPrefix(opt.AccessToken, "access_token=")
	cfg.Debug = opt.Debug
	cfg.Host = opt.Host
	cfg.EpHost = opt.EpHost
	cfg.TvHost = opt.TvHost
	cfg.Area = opt.Area
	httpc.SetUserAgent(opt.UserAgent)
	encodingPriority := bbdown.ParseEncodingPriority(opt.EncodingPriority)
	firstEncoding := bbdown.FirstEncodingPriority(opt.EncodingPriority)
	dfnPriority := bbdown.ParseDfnPriority(opt.DfnPriority)
	if err := applyWorkDir(&opt); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	logDebugSetup(cfg, &opt)
	if !bbdown.IsServerChildProcess() {
		bbdown.CheckUpdateAsync(httpc)
	}
	ctx := context.Background()
	aidOri, vInfo, apiType, err := bbdown.GetVideoInfo(ctx, cfg, httpc, &opt, "", opt.Url)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bbdown.PrintVideoInfo(vInfo, apiType, opt.ShowAll)
	apiType = bbdown.ApplySteinGateTVFallback(vInfo, &opt)
	pages := vInfo.PagesInfo
	totalPagesCount := len(pages)
	selected := bbdown.GetSelectedPages(&opt, vInfo, opt.Url)
	bbdown.Logf("共计 %d 个分P, 已选择：%s", totalPagesCount, selectedPagesText(selected))
	if selected != nil {
		var filtered []bbdown.Page
		for _, p := range pages {
			for _, s := range selected {
				if fmt.Sprintf("%d", p.Index) == s {
					filtered = append(filtered, p)
				}
			}
		}
		pages = filtered
	}
	savePathFormat := savePathPatternForVideo(&opt, vInfo, totalPagesCount)
	delaySeconds, _ := strconv.Atoi(strings.TrimSpace(opt.DelayPerPage))
	for i, p := range pages {
		if len(pages) > 1 && delaySeconds > 0 {
			bbdown.Logf("停顿%d秒...", delaySeconds)
			time.Sleep(time.Duration(delaySeconds) * time.Second)
		}
		bbdown.Logf("开始解析P%d: %s... (%d of %d)", p.Index, p.Aid, i+1, len(pages))
		downloadTitle := bbdown.DownloadTitle(vInfo.Title)
		if opt.SaveArchivesToFile && bbdown.CheckAidFromArchive(p.Aid) {
			bbdown.Logf("aid: %s已下载过, 跳过下载...", p.Aid)
			continue
		}
		selectedTrack := false
		videoIndex := 0
		audioIndex := 0
		retryCount := 0
		pageSavePath := ""
	downloadPage:
		if opt.SubOnly && !opt.SkipSubtitle && !opt.DanmakuOnly && !opt.CoverOnly {
			if pageSavePath == "" {
				formattedPath := bbdown.FormatSavePath(savePathFormat, downloadTitle, nil, nil, p, len(pages), apiType, vInfo.PubTime)
				pageSavePath, err = resolvePageSavePath(formattedPath, &opt)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			}
			savePath := pageSavePath
			subs, err := downloadSubtitles(ctx, httpc, cfg, &opt, p, savePath)
			if err != nil {
				if shouldRetryPageDownload(err, &retryCount) {
					goto downloadPage
				}
				fmt.Println(err)
				os.Exit(1)
			}
			for _, sub := range subs {
				fmt.Println(sub.Path)
			}
			finishPageLikeOriginal(&opt, p.Aid)
			continue
		}
		chapterPoints := bbdown.FetchPoints(ctx, httpc, cfg, p.Cid, p.Aid)
		tracks, err := bbdown.ExtractTracks(ctx, cfg, httpc, aidOri, p.Aid, p.Cid, p.Epid, opt.UseTvApi, opt.UseIntlApi, opt.UseAppApi, firstEncoding, "")
		if err != nil {
			if shouldRetryPageDownload(err, &retryCount) {
				goto downloadPage
			}
			fmt.Println(err)
			os.Exit(1)
		}
		chapterPoints = originalChapterPoints(chapterPoints, tracks.ExtraPoints)
		if opt.Interactive && !selectedTrack && len(tracks.Clips) > 0 && len(tracks.Dfns) > 0 {
			selectedQn := selectFLVQualityManually(tracks.Dfns)
			if selectedQn != "" {
				tracks, err = bbdown.ExtractTracks(ctx, cfg, httpc, aidOri, p.Aid, p.Cid, p.Epid, opt.UseTvApi, opt.UseIntlApi, opt.UseAppApi, firstEncoding, selectedQn)
				if err != nil {
					if shouldRetryPageDownload(err, &retryCount) {
						goto downloadPage
					}
					fmt.Println(err)
					os.Exit(1)
				}
				chapterPoints = originalChapterPoints(chapterPoints, tracks.ExtraPoints)
			}
			selectedTrack = true
		}
		applyOnlyModeToTracks(&opt, tracks)
		tracks.VideoTracks = bbdown.SortTracksVideoWithPriorityOrder(tracks.VideoTracks, dfnPriority, encodingPriority, opt.VideoAscending, encodingFirst)
		tracks.AudioTracks = bbdown.SortTracksAudioWithPriority(tracks.AudioTracks, encodingPriority, opt.AudioAscending)
		tracks.BackgroundAudios = bbdown.SortTracksAudioWithPriority(tracks.BackgroundAudios, encodingPriority, opt.AudioAscending)
		for i := range tracks.RoleAudioLists {
			tracks.RoleAudioLists[i].Audio = bbdown.SortTracksAudioWithPriority(tracks.RoleAudioLists[i].Audio, encodingPriority, opt.AudioAscending)
		}
		if !opt.HideStreams {
			bbdown.PrintTracks(tracks, p.Dur, opt.OnlyShowInfo)
			printManualSelectionHint(&opt, tracks)
		}
		if opt.OnlyShowInfo {
			finishPageLikeOriginal(&opt, p.Aid)
			continue
		}
		explicitVideoIndex, explicitAudioIndex, err := bbdown.SelectedTrackIndexes(&opt, tracks)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		videoIndex = explicitVideoIndex
		audioIndex = explicitAudioIndex
		if opt.Interactive && !selectedTrack {
			videoIndex, audioIndex = selectTracksManually(tracks)
			selectedTrack = true
		}
		if len(tracks.VideoTracks) == 0 && len(tracks.AudioTracks) == 0 && len(tracks.Clips) == 0 {
			if shouldSkipMissingOnlyMode(&opt, tracks) {
				finishPageLikeOriginal(&opt, p.Aid)
				continue
			}
			bbdown.Errorf("解析此分P失败(建议--debug查看详细信息)")
			finishPageLikeOriginal(&opt, p.Aid)
			continue
		}
		var video *bbdown.Video
		if len(tracks.VideoTracks) > 0 {
			video = videoTrackAtOriginalIndex(tracks.VideoTracks, videoIndex)
		}
		var audio *bbdown.Audio
		if len(tracks.AudioTracks) > 0 {
			audio = audioTrackAtOriginalIndex(tracks.AudioTracks, audioIndex)
		}
		var backgroundAudio *bbdown.Audio
		if len(tracks.BackgroundAudios) > 0 {
			backgroundAudio = audioTrackAtOriginalIndex(tracks.BackgroundAudios, audioIndex)
		}
		if pageSavePath == "" {
			formattedPath := bbdown.FormatSavePath(savePathFormat, downloadTitle, video, audio, p, len(pages), apiType, vInfo.PubTime)
			pageSavePath, err = resolvePageSavePath(formattedPath, &opt)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
		savePath := pageSavePath
		logDebugSavePath(cfg, savePathFormat, savePath)
		finalPath := savePath
		if opt.AudioOnly {
			finalPath = strings.TrimSuffix(savePath, ".mp4") + ".m4a"
		}
		delayExistingMediaSkip := sidecarWorkBeforeExistingMediaSkip(&opt)
		if !delayExistingMediaSkip && !opt.SkipMux && shouldSkipExistingFinal(&opt, finalPath) {
			bbdown.Logf("%s已存在, 跳过下载...", finalPath)
			finishPageLikeOriginal(&opt, p.Aid)
			continue
		}
		var subtitles []bbdown.Subtitle
		if !opt.SkipSubtitle && !opt.DanmakuOnly && !opt.CoverOnly {
			var err error
			subtitles, err = downloadSubtitles(ctx, httpc, cfg, &opt, p, savePath)
			if err != nil {
				if shouldRetryPageDownload(err, &retryCount) {
					goto downloadPage
				}
				fmt.Println(err)
				os.Exit(1)
			}
			if opt.SubOnly {
				for _, sub := range subtitles {
					fmt.Println(sub.Path)
				}
				finishPageLikeOriginal(&opt, p.Aid)
				continue
			}
		}
		if opt.DownloadDanmaku || opt.DanmakuOnly {
			danmakuPath := strings.TrimSuffix(savePath, ".mp4") + ".xml"
			danmakuAssPath := strings.TrimSuffix(savePath, ".mp4") + ".ass"
			formats := bbdown.ParseDanmakuFormats(opt.DownloadDanmakuFormats)
			bbdown.Log("正在下载弹幕Xml文件")
			if err := bbdown.DownloadResource(ctx, httpc, cfg, fmt.Sprintf("https://comment.bilibili.com/%s.xml", p.Cid), danmakuPath, &opt); err != nil {
				if shouldRetryPageDownload(err, &retryCount) {
					goto downloadPage
				}
				fmt.Println(err)
				os.Exit(1)
			}
			bbdown.EmitServerTransferEvent(danmakuPath)
			if err := finalizeDanmakuFiles(danmakuPath, danmakuAssPath, formats, &opt); err != nil {
				if shouldRetryPageDownload(err, &retryCount) {
					goto downloadPage
				}
				fmt.Println(err)
				os.Exit(1)
			}
			if opt.DanmakuOnly {
				finishPageLikeOriginal(&opt, p.Aid)
				continue
			}
		}
		if opt.CoverOnly {
			coverURL := vInfo.Pic
			if coverURL == "" {
				coverURL = p.Cover
			}
			coverPath := strings.TrimSuffix(savePath, ".mp4") + bbdown.CoverExt(coverURL)
			if err := bbdown.DownloadResource(ctx, httpc, cfg, coverURL, coverPath, &opt); err != nil {
				if shouldRetryPageDownload(err, &retryCount) {
					goto downloadPage
				}
				fmt.Println(err)
				os.Exit(1)
			}
			bbdown.EmitServerTransferEvent(coverPath)
			fmt.Println(coverPath)
			finishPageLikeOriginal(&opt, p.Aid)
			continue
		}
		if delayExistingMediaSkip && !opt.SkipMux && shouldSkipExistingFinal(&opt, finalPath) {
			bbdown.Logf("%s已存在, 跳过下载...", finalPath)
			finishPageLikeOriginal(&opt, p.Aid)
			continue
		}
		bbdown.Log("已选择的流:")
		bbdown.PrintSelectedTracks(video, audio, p.Dur)
		bbdown.HandleSelectedTrackHosts(&opt, cfg, video, audio)
		coverPath := ""
		if !opt.SkipMux && !opt.SkipCover {
			coverPath = downloadCoverForMux(ctx, httpc, cfg, &opt, vInfo, p, savePath)
		}
		if bbdown.IsDolbyVisionTrack(video) && !opt.UseMP4box && !bbdown.CheckFFmpegDOVI(opt.FFmpegPath) {
			bbdown.Warnf("检测到杜比视界清晰度且您的ffmpeg版本小于5.0,将使用mp4box混流...")
			opt.UseMP4box = true
			if err := bbdown.ResolveRequiredBinaries(&opt); err != nil {
				if shouldRetryPageDownload(err, &retryCount) {
					goto downloadPage
				}
				fmt.Println(err)
				os.Exit(1)
			}
		}
		var audioMaterials []bbdown.AudioMaterial
		muxOptions := func(videoPath, audioPath string, isHevc bool) bbdown.MuxOptions {
			return bbdown.MuxOptions{
				UseMP4Box:      opt.UseMP4box,
				FFmpegPath:     opt.FFmpegPath,
				MP4BoxPath:     opt.Mp4boxPath,
				BVID:           bvidFromAid(p.Aid),
				VideoPath:      videoPath,
				AudioPath:      audioPath,
				CoverPath:      coverPath,
				OutPath:        savePath,
				Title:          downloadTitle,
				Description:    vInfo.Desc,
				Author:         p.OwnerName,
				EpisodeID:      episodeTitle(vInfo, p, len(pages)),
				AudioLanguage:  opt.Language,
				Subtitles:      subtitles,
				AudioMaterials: audioMaterials,
				Chapters:       chapterPoints,
				PubTime:        p.PubTime,
				SimplyMux:      opt.SimplyMux,
				AudioOnly:      opt.AudioOnly,
				VideoOnly:      opt.VideoOnly,
				IsHevc:         isHevc,
			}
		}
		if len(tracks.Clips) == 0 {
			videoPath := strings.TrimSuffix(savePath, ".mp4") + ".video.mp4"
			audioPath := strings.TrimSuffix(savePath, ".mp4") + ".audio.m4a"
			if !opt.AudioOnly {
				if video == nil {
					err := fmt.Errorf("没有找到可下载的视频流")
					if shouldRetryPageDownload(err, &retryCount) {
						goto downloadPage
					}
					fmt.Println("没有找到可下载的视频流")
					os.Exit(1)
				}
				bbdown.Logf("开始下载P%d视频...", p.Index)
				if err := bbdown.DownloadResource(ctx, httpc, cfg, video.BaseURL, videoPath, &opt); err != nil {
					if shouldRetryPageDownload(err, &retryCount) {
						goto downloadPage
					}
					fmt.Println(err)
					os.Exit(1)
				}
				bbdown.EmitServerTransferEvent(videoPath)
			} else {
				videoPath = ""
			}
			if audio != nil && !opt.VideoOnly {
				bbdown.Logf("开始下载P%d音频...", p.Index)
				if err := bbdown.DownloadResource(ctx, httpc, cfg, audio.BaseURL, audioPath, &opt); err != nil {
					if shouldRetryPageDownload(err, &retryCount) {
						goto downloadPage
					}
					fmt.Println(err)
					os.Exit(1)
				}
				bbdown.EmitServerTransferEvent(audioPath)
			} else {
				audioPath = ""
			}
			if backgroundAudio != nil {
				backgroundPath := filepath.Join(p.Aid, fmt.Sprintf("%s.%s.P%d.back_ground.m4a", p.Aid, p.Cid, p.Index))
				bbdown.Logf("开始下载P%d背景配音...", p.Index)
				if err := bbdown.DownloadResource(ctx, httpc, cfg, backgroundAudio.BaseURL, backgroundPath, &opt); err != nil {
					if shouldRetryPageDownload(err, &retryCount) {
						goto downloadPage
					}
					fmt.Println(err)
					os.Exit(1)
				}
				bbdown.EmitServerTransferEvent(backgroundPath)
				audioMaterials = append(audioMaterials, bbdown.AudioMaterial{Title: "背景音频", Path: backgroundPath})
			}
			for _, role := range tracks.RoleAudioLists {
				roleAudio := audioTrackAtOriginalIndex(role.Audio, audioIndex)
				if roleAudio == nil {
					continue
				}
				rolePath := role.Path
				if strings.TrimSpace(rolePath) == "" {
					rolePath = filepath.Join(p.Aid, fmt.Sprintf("%s.%s.%s.m4a", p.Aid, p.Cid, roleAudio.ID))
				}
				bbdown.Logf("开始下载P%d配音[%s]...", p.Index, role.Title)
				if err := bbdown.DownloadResource(ctx, httpc, cfg, roleAudio.BaseURL, rolePath, &opt); err != nil {
					if shouldRetryPageDownload(err, &retryCount) {
						goto downloadPage
					}
					fmt.Println(err)
					os.Exit(1)
				}
				bbdown.EmitServerTransferEvent(rolePath)
				audioMaterials = append(audioMaterials, bbdown.AudioMaterial{Title: role.Title, PersonName: role.PersonName, Path: rolePath})
			}
			bbdown.Logf("下载P%d完毕", p.Index)
			if opt.SkipMux {
				fmt.Println(videoPath)
				if audioPath != "" {
					fmt.Println(audioPath)
				}
				finishPageLikeOriginal(&opt, p.Aid)
				continue
			}
			if opt.AudioOnly {
				savePath = strings.TrimSuffix(savePath, ".mp4") + ".m4a"
			}
			bbdown.Logf("开始合并音视频%s...", subtitleMuxSuffix(subtitles))
			if err := bbdown.PrepareDownloadDestination(savePath, &opt); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			options := muxOptions(videoPath, audioPath, video != nil && video.Codecs == "HEVC")
			options.OutPath = savePath
			if err := bbdown.MuxAVWithOptions(options); err != nil {
				if shouldRetryPageDownload(err, &retryCount) {
					goto downloadPage
				}
				fmt.Println(err)
				os.Exit(1)
			}
			if !fileExistsNonEmpty(savePath) {
				bbdown.Errorf("合并失败")
				finishPageLikeOriginal(&opt, p.Aid)
				continue
			}
			bbdown.Log("清理临时文件...")
			removeIfExists(videoPath)
			removeIfExists(audioPath)
			removeAudioMaterialFiles(audioMaterials)
			removeSubtitleFiles(subtitles)
			removeIfExists(coverPath)
		} else {
			var clipFiles []string
			tmpMerged := strings.TrimSuffix(savePath, ".mp4") + ".flv-merged.mp4"
			// long 2026-06-27 04:47:00：FLV 的 skip-mux 仍会把多个分段合成 .flv-merged.mp4；这里在下载分段前确认 ffmpeg，避免下载完成后才暴露本地合并依赖缺失。单分段按原版直接移动文件，不需要 ffmpeg。
			if err := ensureFLVMergeBinary(&opt, len(tracks.Clips)); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			for i, link := range tracks.Clips {
				clipPath := strings.TrimSuffix(savePath, ".mp4") + fmt.Sprintf(".clip%05d.mp4", i)
				bbdown.Logf("开始下载P%d视频, 片段(%0*d/%d)...", p.Index, len(fmt.Sprintf("%d", len(tracks.Clips))), i+1, len(tracks.Clips))
				if err := bbdown.DownloadResource(ctx, httpc, cfg, link, clipPath, &opt); err != nil {
					if shouldRetryPageDownload(err, &retryCount) {
						goto downloadPage
					}
					fmt.Println(err)
					os.Exit(1)
				}
				bbdown.EmitServerTransferEvent(clipPath)
				clipFiles = append(clipFiles, clipPath)
			}
			bbdown.Logf("下载P%d完毕", p.Index)
			bbdown.Log("开始合并分段...")
			if err := bbdown.MergeFLV(clipFiles, tmpMerged, opt.FFmpegPath); err != nil {
				if shouldRetryPageDownload(err, &retryCount) {
					goto downloadPage
				}
				fmt.Println(err)
				os.Exit(1)
			}
			if opt.SkipMux {
				fmt.Println(tmpMerged)
				finishPageLikeOriginal(&opt, p.Aid)
				continue
			}
			bbdown.Logf("开始混流视频%s...", subtitleMuxSuffix(subtitles))
			if err := bbdown.PrepareDownloadDestination(savePath, &opt); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if err := bbdown.MuxAVWithOptions(muxOptions(tmpMerged, "", false)); err != nil {
				if shouldRetryPageDownload(err, &retryCount) {
					goto downloadPage
				}
				fmt.Println(err)
				os.Exit(1)
			}
			if !fileExistsNonEmpty(savePath) {
				bbdown.Errorf("合并失败")
				finishPageLikeOriginal(&opt, p.Aid)
				continue
			}
			bbdown.Log("清理临时文件...")
			removeIfExists(tmpMerged)
			for _, file := range clipFiles {
				removeIfExists(file)
			}
			removeSubtitleFiles(subtitles)
			removeIfExists(coverPath)
		}
		fmt.Println(savePath)
		finishPageLikeOriginal(&opt, p.Aid)
	}
	bbdown.Log("任务完成")
}

func validateDownloadStartupOptions(opt *bbdown.MyOption) error {
	if err := bbdown.ValidateFileExistsAction(opt.FileExistsAction); err != nil {
		return err
	}
	return bbdown.ValidateTrackIndexSyntax(opt)
}

func applyWorkDir(opt *bbdown.MyOption) error {
	dir, err := bbdown.ResolveWorkDir(opt.WorkDir)
	if err != nil || dir == "" {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	opt.WorkDir = dir
	if opt.Debug {
		bbdown.Logf("切换工作目录至：%s", dir)
	}
	return nil
}

func handleMetaFlags(fs *flag.FlagSet, opt *bbdown.MyOption) bool {
	if opt.Version {
		printVersionInfo()
		return true
	}
	if opt.Help {
		fs.Usage()
		return true
	}
	return false
}

func printVersionInfo() {
	printVersionInfoTo(os.Stdout)
}

func printVersionInfoTo(out io.Writer) {
	fmt.Fprintf(out, "版本: %s\n", bbdown.Version)
	fmt.Fprintf(out, "构建时间: %s\n", bbdown.BuildTime)
}

func newServeFlagSet(listen *string, help *bool) *flag.FlagSet {
	fs := newFlagSetWithUsage("serve", "BB-DL serve [选项]")
	fs.StringVar(listen, "listen", "http://0.0.0.0:23333", "服务监听地址，支持 http://host:port 或 host:port")
	fs.StringVar(listen, "l", "http://0.0.0.0:23333", "同 --listen，服务监听地址")
	registerHelpFlags(fs, help)
	return fs
}

func requireSingleURL(fs *flag.FlagSet) string {
	url, err := singleURLArg(fs)
	if err != nil {
		fmt.Println(err)
		fmt.Printf("请使用 %s 查看帮助\n", helpCommandForFlagSet(fs))
		os.Exit(1)
	}
	return url
}

func singleURLArg(fs *flag.FlagSet) (string, error) {
	if fs.NArg() < 1 {
		return "", errors.New("缺少视频地址或 BV/av/ep 等输入")
	}
	if fs.NArg() > 1 {
		return "", fmt.Errorf("无法识别多余参数：%s", fs.Arg(1))
	}
	return fs.Arg(0), nil
}

func helpCommandForFlagSet(fs *flag.FlagSet) string {
	if fs == nil {
		return "BB-DL --help"
	}
	switch rootCommandName(fs.Name()) {
	case "info":
		return "BB-DL info --help"
	case "download":
		return "BB-DL download --help"
	case "down":
		return "BB-DL down --help"
	}
	return "BB-DL --help"
}

func normalizeFlagOrder(args []string) []string {
	if len(args) == 0 {
		return args
	}
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !isOptionToken(arg) {
			positionals = append(positionals, arg)
			continue
		}
		flags = append(flags, arg)
		if optionTakesValue(arg) && !strings.Contains(arg, "=") && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, positionals...)
}

func isOptionToken(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-"
}

func optionTakesValue(arg string) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	switch name {
	case "access-token", "aria2c-args", "aria2c-path", "aria2c-proxy", "area", "audio-index", "c", "config-file", "cookie", "ddf", "delay-per-page", "dfn-priority", "download-danmaku-formats", "e", "encoding-priority", "ep-host", "F", "ffmpeg-path", "file-pattern", "host", "language", "M", "mp4box-path", "multi-file-pattern", "p", "q", "select-page", "token", "tv-host", "ua", "upos-host", "user-agent", "video-index", "work-dir":
		return true
	default:
		return false
	}
}

func normalizeBoolFlagValues(args []string) []string {
	if len(args) == 0 {
		return args
	}
	boolFlags := map[string]bool{
		"add-dfn-subfix":        true,
		"allow-pcdn":            true,
		"app":                   true,
		"aria2":                 true,
		"audio-ascending":       true,
		"audio-only":            true,
		"av1":                   true,
		"avc":                   true,
		"bandwith-ascending":    true,
		"cover-only":            true,
		"danmaku-only":          true,
		"dd":                    true,
		"debug":                 true,
		"download-danmaku":      true,
		"force-http":            true,
		"force-replace-host":    true,
		"hevc":                  true,
		"hide-streams":          true,
		"hs":                    true,
		"ia":                    true,
		"info":                  true,
		"interactive":           true,
		"intl":                  true,
		"mt":                    true,
		"multi-thread":          true,
		"no-padding-page-num":   true,
		"only-av1":              true,
		"only-avc":              true,
		"only-hevc":             true,
		"only-show-info":        true,
		"save-archives-to-file": true,
		"show-all":              true,
		"simply-mux":            true,
		"skip-ai":               true,
		"skip-cover":            true,
		"skip-mux":              true,
		"skip-subtitle":         true,
		"sub-only":              true,
		"tv":                    true,
		"use-app-api":           true,
		"use-aria2c":            true,
		"use-intl-api":          true,
		"use-mp4box":            true,
		"use-tv-api":            true,
		"video-ascending":       true,
		"video-only":            true,
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, hasName := flagName(arg)
		if hasName && boolFlags[name] && i+1 < len(args) && isBoolLiteral(args[i+1]) && !strings.Contains(arg, "=") {
			out = append(out, arg+"="+strings.ToLower(args[i+1]))
			i++
			continue
		}
		out = append(out, arg)
	}
	return out
}

func prefersEncodingPriorityFirst(args []string) bool {
	encodingIndex := -1
	dfnIndex := -1
	for i, arg := range args {
		name, ok := flagName(arg)
		if !ok {
			continue
		}
		switch name {
		case "encoding-priority":
			if encodingIndex < 0 {
				encodingIndex = i
			}
		case "dfn-priority":
			if dfnIndex < 0 {
				dfnIndex = i
			}
		}
	}
	// long 2026-06-19 04:01:17：原版直接在 Environment.CommandLine 中查长参数名，短别名 -e/-q 虽能赋值，但不会参与“编码优先还是清晰度优先”的顺序判断。
	return encodingIndex >= 0 && dfnIndex >= 0 && encodingIndex < dfnIndex
}

func flagName(arg string) (string, bool) {
	if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
		return "", false
	}
	name := strings.TrimLeft(arg, "-")
	if idx := strings.Index(name, "="); idx >= 0 {
		name = name[:idx]
	}
	return name, name != ""
}

func isBoolLiteral(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false":
		return true
	default:
		return false
	}
}

func selectTracksManually(tracks *bbdown.ParsedTracks) (int, int) {
	reader := bufio.NewReader(os.Stdin)
	videoIndex := 0
	audioIndex := 0
	if len(tracks.VideoTracks) > 0 {
		fmt.Print("请选择最想要的视频流(输入序号): ")
		if _, err := fmt.Fscan(reader, &videoIndex); err != nil {
			videoIndex = 0
		}
		videoIndex = normalizeInteractiveIndexLikeOriginal(videoIndex, len(tracks.VideoTracks))
	}
	if len(tracks.AudioTracks) > 0 {
		fmt.Print("请选择最想要的音频流(输入序号): ")
		if _, err := fmt.Fscan(reader, &audioIndex); err != nil {
			audioIndex = 0
		}
		audioIndex = normalizeInteractiveIndexLikeOriginal(audioIndex, len(tracks.AudioTracks))
	}
	return videoIndex, audioIndex
}

func printManualSelectionHint(opt *bbdown.MyOption, tracks *bbdown.ParsedTracks) {
	if opt == nil || tracks == nil || opt.Interactive || opt.OnlyShowInfo {
		return
	}
	if strings.TrimSpace(opt.VideoIndex) != "" || strings.TrimSpace(opt.AudioIndex) != "" {
		return
	}
	if len(tracks.VideoTracks) <= 1 && len(tracks.AudioTracks) <= 1 {
		return
	}
	bbdown.Log("检测到多个音/视频流；如需按序号选择，请添加 --video-index / --audio-index，或使用 --interactive / -ia，也可以用 --dfn-priority / --encoding-priority 调整自动选择。")
}

func normalizeInteractiveIndexLikeOriginal(index, count int) int {
	if index > count || index < 0 {
		return 0
	}
	return index
}

func videoTrackAtOriginalIndex(items []bbdown.Video, index int) *bbdown.Video {
	if index < 0 || index >= len(items) {
		return nil
	}
	return &items[index]
}

func audioTrackAtOriginalIndex(items []bbdown.Audio, index int) *bbdown.Audio {
	if index < 0 || index >= len(items) {
		return nil
	}
	return &items[index]
}

func selectFLVQualityManually(dfns []string) string {
	if len(dfns) == 0 {
		return ""
	}
	for i, qn := range dfns {
		label := bbdown.QualityMap[qn]
		if label == "" {
			label = qn
		}
		bbdown.LogColor(fmt.Sprintf("%d.%s", i, label), true)
	}
	fmt.Print("请选择最想要的清晰度(输入序号): ")
	reader := bufio.NewReader(os.Stdin)
	index := 0
	if _, err := fmt.Fscan(reader, &index); err != nil {
		index = 0
	}
	return flvQualityByIndex(dfns, index)
}

func flvQualityByIndex(dfns []string, index int) string {
	if len(dfns) == 0 {
		return ""
	}
	index = normalizeInteractiveIndexLikeOriginal(index, len(dfns))
	if index >= len(dfns) {
		return ""
	}
	return dfns[index]
}

func fileExistsNonEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func resolvePageSavePath(path string, opt *bbdown.MyOption) (string, error) {
	resolved, err := bbdown.ResolveFileExistsPath(path, opt.FileExistsAction)
	if err != nil {
		return "", err
	}
	if resolved != path {
		bbdown.Logf("检测到同名文件，输出路径已调整为: %s", resolved)
	}
	return resolved, nil
}

func shouldSkipExistingFinal(opt *bbdown.MyOption, path string) bool {
	return bbdown.ShouldSkipExistingFile(path, opt)
}

func downloadCoverForMux(ctx context.Context, httpc *bbdown.HTTPClient, cfg *bbdown.Config, opt *bbdown.MyOption, vInfo *bbdown.VInfo, p bbdown.Page, savePath string) string {
	coverURL := vInfo.Pic
	if coverURL == "" {
		coverURL = p.Cover
	}
	if strings.TrimSpace(coverURL) == "" {
		return ""
	}
	coverPath := strings.TrimSuffix(savePath, ".mp4") + bbdown.CoverExt(coverURL)
	if fileExists(coverPath) && !bbdown.IsFileOverwriteEnabled(opt) {
		return coverPath
	}
	if err := bbdown.DownloadResource(ctx, httpc, cfg, coverURL, coverPath, opt); err != nil {
		bbdown.Warnf("封面下载失败，跳过封面混流: %v", err)
		return ""
	}
	bbdown.EmitServerTransferEvent(coverPath)
	if fileExists(coverPath) {
		return coverPath
	}
	return ""
}

func bvidFromAid(aid string) string {
	n, err := strconv.ParseInt(aid, 10, 64)
	if err != nil {
		panic(err)
	}
	bvid, err := bbdown.EncodeBV(n)
	if err != nil {
		// long: 混流 metadata 使用的 BV 来自原版 p.bvid，aid 越界时应暴露数据异常，不能静默写入空视频 URL。
		panic(err)
	}
	return bvid
}

func episodeTitle(vInfo *bbdown.VInfo, p bbdown.Page, pagesCount int) string {
	if pagesCount > 1 || (vInfo.IsBangumi && !vInfo.IsBangumiEnd) {
		return p.Title
	}
	return ""
}

func removeIfExists(path string) {
	if strings.TrimSpace(path) != "" {
		_ = os.Remove(path)
	}
}

func removeSubtitleFiles(subs []bbdown.Subtitle) {
	for _, sub := range subs {
		removeIfExists(sub.Path)
	}
}

func removeAudioMaterialFiles(items []bbdown.AudioMaterial) {
	for _, item := range items {
		removeIfExists(item.Path)
	}
}

func logDebugSubtitleFetch(cfg *bbdown.Config) {
	if cfg != nil && cfg.Debug {
		bbdown.Log("获取字幕...")
	}
}

func logDebugSubtitleDownload(cfg *bbdown.Config, rawURL string) {
	if cfg != nil && cfg.Debug {
		bbdown.Logf("下载：%s", rawURL)
	}
}

func downloadSubtitles(ctx context.Context, httpc *bbdown.HTTPClient, cfg *bbdown.Config, opt *bbdown.MyOption, p bbdown.Page, savePath string) ([]bbdown.Subtitle, error) {
	logDebugSubtitleFetch(cfg)
	subs, err := bbdown.GetSubtitles(ctx, httpc, cfg, p.Aid, p.Cid, p.Epid, p.Index, opt.UseIntlApi)
	if err != nil {
		return nil, err
	}
	if opt.SkipAi && len(subs) > 0 {
		bbdown.Log("跳过下载AI字幕")
		filtered := subs[:0]
		for _, sub := range subs {
			if !strings.HasPrefix(sub.Lan, "ai-") {
				filtered = append(filtered, sub)
			}
		}
		subs = filtered
	}
	saved := make([]bbdown.Subtitle, 0, len(subs))
	for _, sub := range subs {
		_, desc := bbdown.SubtitleCode(sub.Lan)
		outPath := bbdown.SubtitleOutputPath(savePath, sub)
		if bbdown.ShouldSkipExistingFile(outPath, opt) {
			bbdown.Logf("%s已存在, 跳过下载...", outPath)
			sub.Path = outPath
			saved = append(saved, sub)
			continue
		}
		bbdown.Logf("下载字幕 %s => %s...", sub.Lan, desc)
		logDebugSubtitleDownload(cfg, sub.URL)
		if err := bbdown.SaveSubtitle(ctx, httpc, sub, outPath); err != nil {
			return nil, err
		}
		if info, err := os.Stat(outPath); err == nil && info.Size() > 0 {
			bbdown.EmitServerTransferEvent(outPath)
			sub.Path = outPath
			saved = append(saved, sub)
		}
	}
	return saved, nil
}
