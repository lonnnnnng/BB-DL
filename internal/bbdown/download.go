package bbdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

var infoPlaceholder = regexp.MustCompile(`<([\w:\-.]+?)>`)
var pcdnHostPattern = regexp.MustCompile(`://.*:\d+/`)
var akamaiHostPattern = regexp.MustCompile(`://.*akamaized\.net/`)
var uposHostPattern = regexp.MustCompile(`://[^/]+/`)
var libavutilVersionPattern = regexp.MustCompile(`libavutil\s+(\d+)\.\s*(\d+)\.`)

const BackupHost = "upos-sz-mirrorcoso1.bilivideo.com"

var errRangeNotSupported = errors.New("range request is not supported")

type downloadPart struct {
	index int
	start int64
	end   int64
	path  string
}

func GetSelectedPages(opt *MyOption, vInfo *VInfo, input string) []string {
	selectPage := strings.ToUpper(strings.Trim(strings.TrimSpace(opt.SelectPage), ","))
	if selectPage == "" {
		if vInfo.Index != "" {
			Log("程序已自动选择你输入的集数, 如果要下载其他集数请自行指定分P(如可使用-p ALL代表全部)")
			return []string{vInfo.Index}
		}
		if p := GetQueryString("p", input); p != "" {
			Log("程序已自动选择你输入的集数, 如果要下载其他集数请自行指定分P(如可使用-p ALL代表全部)")
			return []string{p}
		}
		return nil
	}
	if selectPage == "ALL" {
		return nil
	}
	lastPage := fmt.Sprintf("%d", len(vInfo.PagesInfo))
	for _, key := range []string{"LAST", "NEW", "LATEST"} {
		selectPage = strings.ReplaceAll(selectPage, key, lastPage)
	}
	var selected []string
	if strings.Contains(selectPage, "-") {
		parts := strings.Split(selectPage, "-")
		if len(parts) < 2 {
			Errorf("解析分P参数时失败了~")
			return nil
		}
		// long: C# 的 int.Parse 会接受范围端点两侧空白，范围模式要保留这个宽松解析；逗号列表仍保持原版不 trim 单项的行为。
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			Errorf("解析分P参数时失败了~")
			return nil
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			Errorf("解析分P参数时失败了~")
			return nil
		}
		for i := start; i <= end; i++ {
			selected = append(selected, fmt.Sprintf("%d", i))
		}
		return selected
	}
	for _, item := range strings.Split(selectPage, ",") {
		// long 2026-06-19 04:16:23：源仓库分割逗号列表后不会再 trim 单个片段，空片段和片段内空格会原样进入后续匹配。
		selected = append(selected, item)
	}
	return selected
}

func FormatSavePath(savePathFormat, title string, videoTrack *Video, audioTrack *Audio, p Page, pagesCount int, apiType string, pubTime int64) string {
	// long: 原版只把空字符串视为未设置模板，用户显式传入空格也会作为自定义文件名参与生成。
	if savePathFormat == "" {
		savePathFormat = "<videoTitle>"
		if pagesCount > 1 {
			savePathFormat = "<videoTitle>/[P<pageNumberWithZero>]<pageTitle>"
		}
	}
	result := strings.ReplaceAll(savePathFormat, "\\", "/")
	result = infoPlaceholder.ReplaceAllStringFunc(result, func(m string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(m, "<"), ">")
		defaultDateFormat := "2006-01-02_15-04-05"
		for _, prefix := range []string{"publishDate:", "videoDate:"} {
			if strings.HasPrefix(key, prefix) {
				defaultDateFormat = normalizeDateLayout(strings.TrimPrefix(key, prefix))
				key = strings.TrimSuffix(prefix, ":")
				break
			}
		}
		switch key {
		case "videoTitle":
			return sanitizeFilename(title)
		case "pageNumber":
			return fmt.Sprintf("%d", p.Index)
		case "pageNumberWithZero":
			return fmt.Sprintf("%0*d", len(fmt.Sprintf("%d", pagesCount)), p.Index)
		case "pageTitle":
			return sanitizeFilename(p.Title)
		case "bvid":
			// long: 原版 Page.bvid 直接 long.Parse(aid)，非法 aid 会让保存路径生成失败，不能把 "123abc" 宽松当作 av123。
			n, err := parseInt64(p.Aid)
			if err != nil {
				panic(err)
			}
			bv, err := EncodeBV(n)
			if err != nil {
				panic(err)
			}
			return bv
		case "aid":
			return p.Aid
		case "cid":
			return p.Cid
		case "ownerName":
			return sanitizeFilename(p.OwnerName)
		case "ownerMid":
			return p.OwnerMid
		case "dfn":
			if videoTrack != nil {
				return videoTrack.Dfn
			}
		case "res":
			if videoTrack != nil {
				return videoTrack.Res
			}
		case "fps":
			if videoTrack != nil {
				return videoTrack.Fps
			}
		case "videoCodecs":
			if videoTrack != nil {
				return videoTrack.Codecs
			}
		case "videoBandwidth":
			if videoTrack != nil {
				return fmt.Sprintf("%d", videoTrack.Bandwidth)
			}
		case "audioCodecs":
			if audioTrack != nil {
				return audioTrack.Codecs
			}
		case "audioBandwidth":
			if audioTrack != nil {
				return fmt.Sprintf("%d", audioTrack.Bandwidth)
			}
		case "publishDate":
			return FormatTimeStamp(pubTime, defaultDateFormat)
		case "videoDate":
			return FormatTimeStamp(p.PubTime, defaultDateFormat)
		case "apiType":
			return apiType
		}
		return m
	})
	if !strings.HasSuffix(result, ".mp4") {
		result += ".mp4"
	}
	return result
}

func DownloadTitle(title string) string {
	// long 2026-06-20 11:00:01：原版 DownloadPageAsync 在保存路径和混流 metadata 前修正首尾点号标题，避免点号目录名触发平台兼容问题。
	if strings.HasSuffix(title, ".") {
		title += "_fix"
	}
	if strings.HasPrefix(title, ".") {
		title = "_" + title
	}
	return title
}

func normalizeDateLayout(layout string) string {
	if strings.TrimSpace(layout) == "" {
		return "2006-01-02_15-04-05"
	}
	replacer := strings.NewReplacer(
		"fffffff", "0000000",
		"FFFFFFF", "0000000",
		"ffffff", "000000",
		"FFFFFF", "000000",
		"fffff", "00000",
		"FFFFF", "00000",
		"ffff", "0000",
		"FFFF", "0000",
		"fff", "000",
		"FFF", "000",
		"ff", "00",
		"FF", "00",
		"f", "0",
		"F", "0",
		"yyyy", "2006",
		"YYYY", "2006",
		"yy", "06",
		"YY", "06",
		"MM", "01",
		"dd", "02",
		"DD", "02",
		"HH", "15",
		"hh", "03",
		"mm", "04",
		"ss", "05",
		"zzz", "-07:00",
		"zz", "-07",
		"z", "-07",
	)
	return replacer.Replace(layout)
}

func parseInt64(raw string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
}

func sanitizeFilename(s string) string {
	return sanitizeFilenameWithReplacement(s, "_", true)
}

func sanitizeFilenameWithReplacement(s, replacement string, trimEdge bool) string {
	var b strings.Builder
	for _, r := range s {
		// long 2026-06-19 02:42:49：原版不同调用点会选择不同替换符；这里复用同一套非法字符表，避免空间投稿文件名和普通下载模板各自漂移。
		if r < 32 || strings.ContainsRune(`"<>|:*?\/`, r) {
			b.WriteString(replacement)
			continue
		}
		b.WriteRune(r)
	}
	cleaned := b.String()
	if trimEdge {
		// long 2026-06-19 03:56:49：保存路径变量沿用原版 Trim().TrimEnd('.').Trim()，一次去掉所有尾随点号，避免生成 Windows 难处理的边缘文件名。
		cleaned = strings.TrimSpace(cleaned)
		cleaned = strings.TrimRight(cleaned, ".")
		cleaned = strings.TrimSpace(cleaned)
	}
	return cleaned
}

func CoverExt(coverURL string) string {
	cleanURL := strings.SplitN(coverURL, "?", 2)[0]
	ext := strings.ToLower(filepath.Ext(cleanURL))
	if ext != "" {
		return ext
	}
	return ".jpg"
}

func PrepareDownloadURL(rawURL string, opt *MyOption, cfg *Config) string {
	if rawURL == "" {
		return rawURL
	}
	if isMediaDownloadURL(rawURL) {
		uposHost := opt.UposHost
		if opt.ForceReplaceHost && uposHost == "" {
			uposHost = BackupHost
		}
		if uposHost != "" {
			rawURL = uposHostPattern.ReplaceAllString(rawURL, "://"+uposHost+"/")
		} else if !opt.AllowPcdn {
			if pcdnHostPattern.MatchString(rawURL) {
				rawURL = pcdnHostPattern.ReplaceAllString(rawURL, "://"+BackupHost+"/")
			}
		}
		if uposHost == "" && cfg.Area != "" && strings.Contains(rawURL, "akamaized.net") {
			rawURL = akamaiHostPattern.ReplaceAllString(rawURL, "://"+BackupHost+"/")
		}
	}
	if opt.ForceHttp && !(opt.MultiThread && strings.Contains(rawURL, "-cmcc-")) && !strings.Contains(rawURL, ".mcdn.bilivideo.cn:") {
		rawURL = strings.ReplaceAll(rawURL, "https:", "http:")
	}
	return rawURL
}

func HandleSelectedTrackHosts(opt *MyOption, cfg *Config, video *Video, audio *Audio) {
	if opt == nil {
		return
	}
	uposHost := opt.UposHost
	if opt.ForceReplaceHost && uposHost == "" {
		uposHost = BackupHost
	}
	if uposHost == "" {
		if !opt.AllowPcdn {
			if video != nil && pcdnHostPattern.MatchString(video.BaseURL) {
				Warnf("检测到视频流为PCDN, 尝试强制替换为%s……", BackupHost)
				video.BaseURL = pcdnHostPattern.ReplaceAllString(video.BaseURL, "://"+BackupHost+"/")
			}
			if audio != nil && pcdnHostPattern.MatchString(audio.BaseURL) {
				Warnf("检测到音频流为PCDN, 尝试强制替换为%s……", BackupHost)
				audio.BaseURL = pcdnHostPattern.ReplaceAllString(audio.BaseURL, "://"+BackupHost+"/")
			}
		}
		if cfg != nil && cfg.Area != "" {
			if video != nil && strings.Contains(video.BaseURL, "akamaized.net") {
				Warnf("检测到视频流为外国源, 尝试强制替换为%s……", BackupHost)
				video.BaseURL = akamaiHostPattern.ReplaceAllString(video.BaseURL, "://"+BackupHost+"/")
			}
			if audio != nil && strings.Contains(audio.BaseURL, "akamaized.net") {
				Warnf("检测到音频流为外国源, 尝试强制替换为%s……", BackupHost)
				audio.BaseURL = akamaiHostPattern.ReplaceAllString(audio.BaseURL, "://"+BackupHost+"/")
			}
		}
		return
	}
	if video != nil {
		Warnf("尝试将视频流强制替换为%s……", uposHost)
		video.BaseURL = uposHostPattern.ReplaceAllString(video.BaseURL, "://"+uposHost+"/")
	}
	if audio != nil {
		Warnf("尝试将音频流强制替换为%s……", uposHost)
		audio.BaseURL = uposHostPattern.ReplaceAllString(audio.BaseURL, "://"+uposHost+"/")
	}
}

func isMediaDownloadURL(rawURL string) bool {
	return strings.Contains(rawURL, "bilivideo.com") || strings.Contains(rawURL, "bilivideo.cn") || strings.Contains(rawURL, "akamaized.net")
}

func DownloadResource(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL, path string, opt *MyOption) error {
	// long 2026-06-19 08:56:48：原版 DownloadFileAsync 遇到空资源地址会直接跳过，不能因为用户启用 aria2c 就启动外部下载器去处理空 URL。
	if rawURL == "" {
		return nil
	}
	if ShouldSkipExistingFile(path, opt) {
		Logf("%s已存在, 跳过下载...", path)
		return nil
	}
	if err := PrepareDownloadDestination(path, opt); err != nil {
		return err
	}
	var progress *downloadProgress
	if !opt.UseAria2c {
		progress = newConsoleDownloadProgress(path)
	}
	var err error
	defer func() {
		if err != nil {
			progress.Abort()
			return
		}
		progress.Finish()
	}()
	rawURL = PrepareDownloadURL(rawURL, opt, cfg)
	if opt.MultiThread && isMediaDownloadURL(rawURL) && strings.Contains(rawURL, "-cmcc-") {
		// long: 原版遇到 cmcc CDN 会在共享 DownloadConfig 上关闭 ForceHttp，后续同页音频或配音也不再被强制改成 http。
		Warnf("检测到cmcc域名cdn, 已经禁用多线程")
		opt.ForceHttp = false
	}
	if opt.UseAria2c {
		err = DownloadByAria2c(rawURL, path, opt.Aria2cPath, opt.Aria2cArgs, cfg.Cookie)
		return err
	}
	// long: 原版只把音视频媒体流交给多线程分片下载；弹幕、字幕、封面等普通资源可能没有 Content-Length，必须走单线程流式读取到 EOF。
	if opt.MultiThread && isMediaDownloadURL(rawURL) && !strings.Contains(rawURL, "-cmcc-") {
		err = downloadMediaMultiThreadWithProgress(ctx, httpc, cfg, rawURL, path, 20*1024*1024, 8, progress)
		return err
	}
	err = downloadFileWithProgress(ctx, httpc, cfg, rawURL, path, false, progress)
	return err
}

func DownloadByAria2c(rawURL, path, aria2cPath, extraArgs, cookie string) error {
	if aria2cPath == "" {
		aria2cPath = "aria2c"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	args := buildAria2cArgs(rawURL, path, extraArgs, cookie)
	cmd := exec.Command(aria2cPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return err
		}
		// long 2026-06-19 03:50:56：原版 aria2c 分支不读取进程退出码，只以后续产物和 .aria2 控制文件判断下载是否可靠。
	}
	if _, err := os.Stat(path + ".aria2"); err == nil {
		return fmt.Errorf("aria2下载可能存在错误")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("aria2下载可能存在错误")
	}
	return nil
}

func buildAria2cArgs(rawURL, path, extraArgs, cookie string) []string {
	args := []string{
		"--auto-file-renaming=false",
		"--download-result=hide",
		"--allow-overwrite=true",
		"--console-log-level=warn",
		"-x16", "-s16", "-j16", "-k5M",
	}
	if shouldSendBilibiliReferer(rawURL) {
		// long: aria2c 分支把 header 作为外部进程参数暴露，顺序按原版 Referer、User-Agent、Cookie 保持，方便日志和脚本比对。
		args = append(args, "--header=Referer: https://www.bilibili.com")
	}
	args = append(args, "--header=User-Agent: Mozilla/5.0")
	args = append(args, "--header=Cookie: "+cookie)
	if strings.TrimSpace(extraArgs) != "" {
		args = append(args, splitCommandArgs(extraArgs)...)
	}
	args = append(args, rawURL, "-d", filepath.Dir(path), "-o", filepath.Base(path))
	return args
}

func splitCommandArgs(input string) []string {
	var args []string
	var b strings.Builder
	var quote rune
	inToken := false
	// long 2026-06-19 03:15:40：aria2c 附加参数沿用原版的命令行字符串语义，代理和 header 值里可能带空格，不能按普通空白字符直接切碎。
	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' && i+1 < len(runes) {
			next := runes[i+1]
			if quote != 0 {
				if next == quote || next == '\\' {
					b.WriteRune(next)
					i++
					inToken = true
					continue
				}
			} else if unicode.IsSpace(next) || next == '"' || next == '\'' || next == '\\' {
				b.WriteRune(next)
				i++
				inToken = true
				continue
			}
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				inToken = true
				continue
			}
			b.WriteRune(r)
			inToken = true
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			inToken = true
			continue
		}
		if unicode.IsSpace(r) {
			if inToken {
				args = append(args, b.String())
				b.Reset()
				inToken = false
			}
			continue
		}
		b.WriteRune(r)
		inToken = true
	}
	if inToken {
		args = append(args, b.String())
	}
	return args
}

func DownloadFile(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL, path string, forceHTTP bool) error {
	return downloadFileWithProgress(ctx, httpc, cfg, rawURL, path, forceHTTP, nil)
}

func downloadFileWithProgress(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL, path string, forceHTTP bool, progress *downloadProgress) error {
	if rawURL == "" {
		return nil
	}
	if forceHTTP && !strings.Contains(rawURL, ".mcdn.bilivideo.cn:") {
		rawURL = strings.ReplaceAll(rawURL, "https:", "http:")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := singleThreadTempPath(path)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		lastErr = downloadFileOnce(ctx, httpc, cfg, rawURL, tmpPath, path, progress)
		if lastErr == nil {
			return os.Rename(tmpPath, path)
		}
	}
	return lastErr
}

func downloadFileOnce(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL, tmpPath, reportPath string, progress *downloadProgress) error {
	var downloaded int64
	var ifRange string
	if info, err := os.Stat(tmpPath); err == nil {
		downloaded = info.Size()
		ifRange = info.ModTime().UTC().Format(http.TimeFormat)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	applyDownloadHeaders(req, cfg)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
	if ifRange != "" {
		// long 2026-06-19 03:22:12：单线程续传沿用原版 If-Range 语义，避免 CDN 资源变更后把旧临时文件和新响应拼在一起。
		req.Header.Set("If-Range", ifRange)
	}
	resp, err := httpc.Client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if resp.StatusCode == http.StatusOK {
		downloaded = 0
		if err := out.Truncate(0); err != nil {
			return err
		}
	}
	if _, err := out.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	reader, cleanup, err := decodedBodyReader(resp)
	if err != nil {
		return err
	}
	defer cleanup()
	if resp.ContentLength >= 0 {
		progress.SetTotal(downloaded+resp.ContentLength, downloaded)
	} else {
		progress.SetTotal(-1, downloaded)
	}
	writer, flushTransfer := transferWriter(out, reportPath, progress)
	if _, err = io.Copy(writer, reader); err != nil {
		flushTransfer()
		return err
	}
	flushTransfer()
	if resp.ContentLength >= 0 {
		info, statErr := out.Stat()
		if statErr != nil {
			return statErr
		}
		if info.Size() != downloaded+resp.ContentLength {
			return fmt.Errorf("Retry...")
		}
	}
	return nil
}

func DownloadFileMultiThread(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL, path string, chunkSize int64, workers int) error {
	return downloadFileMultiThreadWithProgress(ctx, httpc, cfg, rawURL, path, chunkSize, workers, nil)
}

func downloadFileMultiThreadWithProgress(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL, path string, chunkSize int64, workers int, progress *downloadProgress) error {
	if chunkSize <= 0 {
		chunkSize = 20 * 1024 * 1024
	}
	if workers <= 0 {
		workers = 8
	}
	length, err := remoteContentLength(ctx, httpc, cfg, rawURL)
	if err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil && info.Size() == length {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var parts []downloadPart
	for start, idx := int64(0), 0; start < length; idx++ {
		end := start + chunkSize
		if end >= length {
			end = -1
		}
		parts = append(parts, downloadPart{
			index: idx,
			start: start,
			end:   end,
			path:  multiThreadPartPath(path, idx),
		})
		if end < 0 {
			break
		}
		start = end + 1
	}
	progress.SetTotal(length, existingPartBytes(parts, length))

	jobs := make(chan downloadPart)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				var lastErr error
				for attempt := 0; attempt < 3; attempt++ {
					lastErr = downloadRangePart(ctx, httpc, cfg, rawURL, item.path, item.start, item.end, path, progress)
					if lastErr == nil {
						break
					}
				}
				if lastErr == nil {
					continue
				}
				if errors.Is(lastErr, errRangeNotSupported) {
					lastErr = fmt.Errorf("服务器可能并不支持多线程下载, 请使用 --multi-thread false 关闭多线程")
				} else {
					lastErr = fmt.Errorf("Failed to download clip %d", item.index)
				}
				select {
				case errCh <- lastErr:
				default:
				}
				return
			}
		}()
	}

	for _, item := range parts {
		select {
		case err := <-errCh:
			close(jobs)
			wg.Wait()
			return err
		case jobs <- item:
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
	}
	return nil
}

func DownloadMediaMultiThread(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL, path string, chunkSize int64, workers int) error {
	return downloadMediaMultiThreadWithProgress(ctx, httpc, cfg, rawURL, path, chunkSize, workers, nil)
}

func downloadMediaMultiThreadWithProgress(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL, path string, chunkSize int64, workers int, progress *downloadProgress) error {
	if err := downloadFileMultiThreadWithProgress(ctx, httpc, cfg, rawURL, path, chunkSize, workers, progress); err != nil {
		return err
	}
	files, err := multiThreadPartFiles(path)
	if err != nil {
		return err
	}
	// long: 原版的 MultiThreadDownloadFileAsync 只落分片，DownloadTrackAsync 再按 .vclip/.aclip 排序合并到最终轨道文件。
	if err := CombineFiles(files, path); err != nil {
		return err
	}
	for _, file := range multiThreadCleanupFiles(path) {
		_ = os.Remove(file)
	}
	return nil
}

func existingPartBytes(parts []downloadPart, totalLength int64) int64 {
	var total int64
	for _, item := range parts {
		info, err := os.Stat(item.path)
		if err != nil || info.IsDir() {
			continue
		}
		size := info.Size()
		expected := partExpectedSize(item, totalLength)
		if expected >= 0 && size > expected {
			size = expected
		}
		if size > 0 {
			total += size
		}
	}
	return total
}

func partExpectedSize(item downloadPart, totalLength int64) int64 {
	if item.end >= 0 {
		return item.end - item.start + 1
	}
	if totalLength > item.start {
		return totalLength - item.start
	}
	return -1
}

func multiThreadPartFiles(path string) ([]string, error) {
	suffix := ".aclip"
	if strings.HasSuffix(filepath.Ext(path), ".mp4") {
		suffix = ".vclip"
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// long 2026-07-27 00:55:09：同一下载目录可能同时存在多个任务的断点分片，合并当前轨道时只能读取与目标基名完全匹配的分片。
		if isExactDownloadPart(entry.Name(), base, suffix) {
			matches = append(matches, filepath.Join(filepath.Dir(path), entry.Name()))
		}
	}
	sort.Strings(matches)
	return matches, nil
}

func multiThreadCleanupFiles(path string) []string {
	matches, err := multiThreadPartFiles(path)
	if err != nil {
		return nil
	}
	return matches
}

func multiThreadPartPath(path string, index int) string {
	ext := filepath.Ext(path)
	suffix := ".aclip"
	if strings.HasSuffix(ext, ".mp4") {
		suffix = ".vclip"
	}
	base := strings.TrimSuffix(filepath.Base(path), ext)
	// long: 原版按“00000_目标文件名.vclip/aclip”保存多线程临时片，保留这个命名能让中断续传和人工排查时看到一致的分片类型。
	return filepath.Join(filepath.Dir(path), fmt.Sprintf("%05d_%s%s", index, base, suffix))
}

func remoteContentLength(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	applyDownloadHeaders(req, cfg)
	resp, err := httpc.Client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return 0, fmt.Errorf("get content length failed: %s", resp.Status)
	}
	if resp.ContentLength <= 0 {
		return 0, fmt.Errorf("missing content length")
	}
	return resp.ContentLength, nil
}

func downloadRangePart(ctx context.Context, httpc *HTTPClient, cfg *Config, rawURL, path string, start, end int64, reportPath string, progress *downloadProgress) error {
	var downloaded int64
	var ifRange string
	if info, err := os.Stat(path); err == nil {
		downloaded = info.Size()
		ifRange = info.ModTime().UTC().Format(http.TimeFormat)
	}
	expectedSize := int64(-1)
	if end >= 0 {
		expectedSize = end - start + 1
		if downloaded == expectedSize {
			return nil
		}
		if downloaded > expectedSize {
			if err := os.Truncate(path, 0); err != nil {
				return err
			}
			downloaded = 0
			ifRange = ""
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	applyDownloadHeaders(req, cfg)
	rangeStart := start + downloaded
	rangeValue := fmt.Sprintf("bytes=%d-%d", rangeStart, end)
	if end < 0 {
		// long: 最后一段下载沿用原版的开放 Range，交给 CDN 返回剩余内容，避免本地长度探测和服务端实际可读长度轻微不一致时截断尾部。
		rangeValue = fmt.Sprintf("bytes=%d-", rangeStart)
	}
	req.Header.Set("Range", rangeValue)
	if ifRange != "" {
		req.Header.Set("If-Range", ifRange)
	}
	resp, err := httpc.Client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return errRangeNotSupported
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := out.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	writer, flushTransfer := transferWriter(out, reportPath, progress)
	if _, err = io.Copy(writer, resp.Body); err != nil {
		flushTransfer()
		return err
	}
	flushTransfer()
	if expectedSize >= 0 {
		info, statErr := out.Stat()
		if statErr != nil {
			return statErr
		}
		if info.Size() != expectedSize {
			return fmt.Errorf("Retry...")
		}
	}
	return err
}

func applyDownloadHeaders(req *http.Request, cfg *Config) {
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if shouldSendBilibiliReferer(req.URL.String()) {
		req.Header.Set("Referer", "https://www.bilibili.com")
	}
	if cfg.Cookie != "" {
		req.Header.Set("Cookie", cfg.Cookie)
	}
}

func shouldSendBilibiliReferer(rawURL string) bool {
	// long: APP/TV 播放接口生成的下载地址带有 platform 标记，原版对这类地址不写 Referer，避免覆盖客户端侧鉴权语义。
	return !strings.Contains(rawURL, "platform=android_tv_yst") && !strings.Contains(rawURL, "platform=android")
}

func MuxAV(videoPath, audioPath, outPath, ffmpegPath string) error {
	return MuxAVWithSubtitles(videoPath, audioPath, outPath, ffmpegPath, "", nil)
}

func MuxAVWithSubtitles(videoPath, audioPath, outPath, ffmpegPath, audioLanguage string, subs []Subtitle) error {
	return MuxAVWithOptions(MuxOptions{
		VideoPath:     videoPath,
		AudioPath:     audioPath,
		OutPath:       outPath,
		FFmpegPath:    ffmpegPath,
		AudioLanguage: audioLanguage,
		Subtitles:     subs,
	})
}

func MergeFLV(files []string, outPath, ffmpegPath string) error {
	if len(files) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if len(files) == 1 {
		return os.Rename(files[0], outPath)
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	tsFiles := make([]string, 0, len(files))
	for _, file := range files {
		tmpFile := filepath.Join(filepath.Dir(file), strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))+".ts")
		// long: FLV 分段直接字节拼接在部分视频上会破坏容器边界，先转成 MPEG-TS 后再拼接，和原版 BBDown 的容器处理方式保持一致。
		cmd := exec.Command(ffmpegPath, "-loglevel", "warning", "-y", "-i", file, "-map", "0", "-c", "copy", "-f", "mpegts", "-bsf:v", "h264_mp4toannexb", tmpFile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := ignoreStartedProcessExitError(cmd.Run()); err != nil {
			return err
		}
		_ = os.Remove(file)
		tsFiles = append(tsFiles, tmpFile)
	}
	if err := CombineFiles(tsFiles, outPath); err != nil {
		return err
	}
	for _, file := range tsFiles {
		_ = os.Remove(file)
	}
	return nil
}

type MuxOptions struct {
	UseMP4Box      bool
	FFmpegPath     string
	MP4BoxPath     string
	BVID           string
	VideoPath      string
	AudioPath      string
	CoverPath      string
	OutPath        string
	Title          string
	Description    string
	Author         string
	EpisodeID      string
	AudioLanguage  string
	Subtitles      []Subtitle
	AudioMaterials []AudioMaterial
	Chapters       []ViewPoint
	PubTime        int64
	SimplyMux      bool
	AudioOnly      bool
	VideoOnly      bool
	IsHevc         bool
}

func MuxAVWithOptions(opt MuxOptions) error {
	if opt.UseMP4Box {
		return muxByMP4Box(opt)
	}
	return muxByFFmpeg(opt)
}

func IsDolbyVisionTrack(video *Video) bool {
	if video == nil {
		return false
	}
	return video.ID == "126" || video.Dfn == QualityMap["126"]
}

func CheckFFmpegDOVI(ffmpegPath string) bool {
	if strings.TrimSpace(ffmpegPath) == "" {
		ffmpegPath = "ffmpeg"
	}
	cmd := exec.Command(ffmpegPath, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return ffmpegVersionSupportsDOVI(string(out))
}

func ffmpegVersionSupportsDOVI(info string) bool {
	match := libavutilVersionPattern.FindStringSubmatch(info)
	if len(match) < 3 {
		return false
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return false
	}
	if _, err := strconv.Atoi(match[2]); err != nil {
		return false
	}
	// long 2026-06-20 12:32:00：原版条件误把 minor 写成 major，导致所有 libavutil 57.x 都判定支持杜比视界；这里保留这个兼容怪癖。
	return major > 57 || (major == 57 && major >= 17)
}

func muxByFFmpeg(opt MuxOptions) error {
	ffmpegPath := opt.FFmpegPath
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if opt.AudioOnly && opt.AudioPath != "" {
		opt.VideoPath = ""
	}
	if opt.VideoOnly {
		opt.AudioPath = ""
	}
	if err := os.MkdirAll(filepath.Dir(opt.OutPath), 0o755); err != nil {
		return err
	}
	args := []string{"-loglevel", "warning", "-y"}
	inputIndex := 0
	videoIndex := -1
	audioIndex := -1
	coverIndex := -1
	chapterIndex := -1
	var audioMaterialIndexes []int
	var includedAudioMaterials []AudioMaterial
	var includedSubtitles []Subtitle
	var subIndexes []int
	if opt.VideoPath != "" {
		args = append(args, "-i", opt.VideoPath)
		videoIndex = inputIndex
		inputIndex++
	}
	if opt.AudioPath != "" {
		args = append(args, "-i", opt.AudioPath)
		audioIndex = inputIndex
		inputIndex++
	}
	for _, material := range opt.AudioMaterials {
		if strings.TrimSpace(material.Path) == "" {
			continue
		}
		args = append(args, "-i", material.Path)
		audioMaterialIndexes = append(audioMaterialIndexes, inputIndex)
		includedAudioMaterials = append(includedAudioMaterials, material)
		inputIndex++
	}
	if opt.CoverPath != "" {
		// long: 原版只判断封面路径字符串是否为空；文件缺失时应交给 ffmpeg 报错，不能在 Go 层静默跳过封面输入。
		args = append(args, "-i", opt.CoverPath)
		coverIndex = inputIndex
		inputIndex++
	}
	for _, sub := range opt.Subtitles {
		if !fileExists(sub.Path) {
			continue
		}
		args = append(args, "-i", sub.Path)
		subIndexes = append(subIndexes, inputIndex)
		includedSubtitles = append(includedSubtitles, sub)
		inputIndex++
	}
	chapterFile := ""
	if len(opt.Chapters) > 0 {
		base := opt.VideoPath
		if base == "" {
			base = opt.AudioPath
		}
		if base == "" {
			base = opt.OutPath
		}
		chapterFile = filepath.Join(filepath.Dir(base), "chapters")
		if err := os.WriteFile(chapterFile, []byte(formatFFmpegChapters(opt.Chapters)), 0o644); err != nil {
			return err
		}
		args = append(args, "-i", chapterFile, "-map_chapters", fmt.Sprintf("%d", inputIndex))
		chapterIndex = inputIndex
		inputIndex++
	}
	if videoIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("%d:v:0", videoIndex))
	}
	if audioIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("%d:a:0", audioIndex))
	}
	for _, idx := range audioMaterialIndexes {
		args = append(args, "-map", fmt.Sprintf("%d:a:0", idx))
	}
	if coverIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("%d:v:0", coverIndex))
	}
	for _, idx := range subIndexes {
		args = append(args, "-map", fmt.Sprintf("%d:s:0", idx))
	}
	args = append(args, "-c:v", "copy", "-c:a", "copy")
	if len(subIndexes) > 0 {
		// long: mp4 容器不能直接保存 SRT/ASS 文本轨，ffmpeg 这里转换为 mov_text，播放器才能识别内嵌字幕。
		args = append(args, "-c:s", "mov_text")
	}
	if coverIndex >= 0 {
		coverTrack := 0
		if videoIndex >= 0 {
			coverTrack = 1
		}
		args = append(args, fmt.Sprintf("-disposition:v:%d", coverTrack), "attached_pic")
	}
	if opt.AudioLanguage != "" && audioIndex >= 0 {
		args = append(args, "-metadata:s:a:0", "language="+opt.AudioLanguage)
	}
	if len(audioMaterialIndexes) > 0 && audioIndex >= 0 {
		// long: 额外配音轨混入后，播放器里需要区分主音轨、背景音频和角色配音，原版用这些 metadata 保留音轨业务含义。
		args = append(args, "-metadata:s:a:0", "title=原音频")
	}
	for i, material := range includedAudioMaterials {
		audioTrack := i + 1
		if strings.TrimSpace(material.Title) != "" {
			args = append(args, fmt.Sprintf("-metadata:s:a:%d", audioTrack), "title="+material.Title)
		}
		if strings.TrimSpace(material.PersonName) != "" {
			args = append(args, fmt.Sprintf("-metadata:s:a:%d", audioTrack), "artist="+material.PersonName)
		}
	}
	for i, sub := range includedSubtitles {
		lang, title := SubtitleCode(sub.Lan)
		args = append(args, "-metadata:s:s:"+fmt.Sprintf("%d", i), "language="+lang, "-metadata:s:s:"+fmt.Sprintf("%d", i), "title="+title)
	}
	if !opt.SimplyMux {
		args = appendMuxMetadata(args, opt)
	}
	if runtime.GOOS == "darwin" && opt.IsHevc {
		args = append(args, "-tag:v:0", "hvc1")
	}
	args = append(args, "-movflags", "faststart", "-strict", "unofficial", "-strict", "-2", "-f", "mp4", opt.OutPath)
	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := ignoreStartedProcessExitError(cmd.Run())
	if chapterIndex >= 0 {
		_ = os.Remove(chapterFile)
	}
	return err
}

func muxByMP4Box(opt MuxOptions) error {
	mp4boxPath := opt.MP4BoxPath
	if mp4boxPath == "" {
		mp4boxPath = "mp4box"
	}
	if err := os.MkdirAll(filepath.Dir(opt.OutPath), 0o755); err != nil {
		return err
	}
	args := []string{"-inter", "500", "-noprog"}
	nowID := 0
	if opt.AudioOnly && opt.AudioPath != "" {
		opt.VideoPath = ""
	}
	if opt.VideoOnly {
		opt.AudioPath = ""
	}
	if opt.VideoPath != "" {
		trackID := "1"
		if opt.AudioOnly && opt.AudioPath == "" {
			trackID = "2"
		}
		args = append(args, "-add", fmt.Sprintf("%s#trackID=%s:name=", opt.VideoPath, trackID))
		nowID++
	}
	if opt.AudioPath != "" {
		lang := opt.AudioLanguage
		if lang == "" {
			lang = "und"
		}
		args = append(args, "-add", fmt.Sprintf("%s:lang=%s", opt.AudioPath, lang))
		nowID++
	}
	chapterFile := ""
	if len(opt.Chapters) > 0 {
		base := opt.VideoPath
		if base == "" {
			base = opt.AudioPath
		}
		if base == "" {
			base = opt.OutPath
		}
		chapterFile = filepath.Join(filepath.Dir(base), "chapters")
		if err := os.WriteFile(chapterFile, []byte(formatMP4BoxChapters(opt.Chapters)), 0o644); err != nil {
			return err
		}
		args = append(args, "-chap", chapterFile)
	}
	for _, sub := range opt.Subtitles {
		if !fileExists(sub.Path) {
			continue
		}
		lang, title := SubtitleCode(sub.Lan)
		nowID++
		args = append(args, "-add", fmt.Sprintf("%s#trackID=1:name=:hdlr=sbtl:lang=%s", sub.Path, lang))
		// long: MP4Box 的 udta 目标是输出文件中的轨道编号，必须先计入视频/音频轨，否则字幕名称会写到错误轨道。
		args = append(args, "-udta", fmt.Sprintf("%d:type=name:str=%s", nowID, title))
	}
	tags := formatMP4BoxTags(opt)
	if tags != "" {
		args = append(args, "-itags", tags)
	}
	args = append(args, "-new", "--", opt.OutPath)
	cmd := exec.Command(mp4boxPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := ignoreStartedProcessExitError(cmd.Run())
	if chapterFile != "" {
		_ = os.Remove(chapterFile)
	}
	return err
}

func ignoreStartedProcessExitError(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// long: 源仓库 RunExe 启动外部工具后没有读取 ExitCode，后续由输出文件是否存在且非空判断混流结果。
		return nil
	}
	return err
}

func appendMuxMetadata(args []string, opt MuxOptions) []string {
	title := escapeMuxMetadataText(opt.Title)
	description := escapeMuxMetadataText(opt.Description)
	episodeID := escapeMuxMetadataText(opt.EpisodeID)
	if opt.EpisodeID != "" {
		title = episodeID
	}
	if title != "" {
		args = append(args, "-metadata", "title="+title)
	}
	if opt.BVID != "" {
		args = append(args, "-metadata", "comment=https://www.bilibili.com/video/"+opt.BVID+"/")
	}
	if description != "" {
		args = append(args, "-metadata", "description="+description)
	}
	if opt.Author != "" {
		args = append(args, "-metadata", "artist="+opt.Author)
	}
	if opt.EpisodeID != "" && opt.Title != "" {
		args = append(args, "-metadata", "album="+escapeMuxMetadataText(opt.Title))
	}
	if opt.PubTime != 0 {
		args = append(args, "-metadata", "creation_time="+time.Unix(opt.PubTime, 0).UTC().Format("2006-01-02T15:04:05.000000Z"))
	}
	return args
}

func escapeMuxMetadataText(raw string) string {
	if raw == "" {
		return raw
	}
	// long 2026-06-19 03:27:55：原版把标题、简介和分集名写入命令行前会替换引号并翻倍反斜杠；这里保持最终 metadata 文本一致。
	return strings.NewReplacer(`"`, `'`, `\`, `\\`).Replace(raw)
}

func formatFFmpegChapters(points []ViewPoint) string {
	var b strings.Builder
	b.WriteString(";FFMETADATA\n")
	for _, p := range points {
		b.WriteString("[CHAPTER]\nTIMEBASE=1/1000\n")
		b.WriteString(fmt.Sprintf("START=%d\nEND=%d\n", p.Start*1000, p.End*1000))
		b.WriteString("title=" + p.Title + "\n\n")
	}
	return b.String()
}

func formatMP4BoxChapters(points []ViewPoint) string {
	var b strings.Builder
	for _, p := range points {
		b.WriteString(fmt.Sprintf("%s %s\n", FormatTime(p.Start, true), p.Title))
	}
	return b.String()
}

func formatMP4BoxTags(opt MuxOptions) string {
	tags := []string{"tool="}
	if opt.CoverPath != "" {
		// long: MP4Box 分支同样按原版保留非空封面路径，让外部工具负责报告缺失文件或格式错误。
		tags = append(tags, "cover="+opt.CoverPath)
	}
	if opt.EpisodeID != "" {
		tags = append(tags, "album="+escapeMuxMetadataText(opt.Title), "title="+escapeMuxMetadataText(opt.EpisodeID))
	} else {
		tags = append(tags, "title="+escapeMuxMetadataText(opt.Title))
	}
	tags = append(tags,
		"sdesc="+escapeMuxMetadataText(opt.Description),
		"comment=https://www.bilibili.com/video/"+opt.BVID+"/",
		"artist="+opt.Author,
	)
	return strings.Join(tags, ":")
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func CombineFiles(files []string, outPath string) error {
	if len(files) == 0 {
		return nil
	}
	if len(files) == 1 {
		// long: 原版单分片合并直接 MoveTo，避免对大文件做一次无意义复制。
		_ = os.Remove(outPath)
		return os.Rename(files[0], outPath)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	for _, file := range files {
		in, err := os.Open(file)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = in.Close()
			return err
		}
		_ = in.Close()
	}
	return nil
}

func (h *HTTPClient) Client() *http.Client {
	return h.client
}
