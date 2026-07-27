package bbdown

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type downloadProgress struct {
	out            io.Writer
	label          string
	total          int64
	current        int64
	lastRenderAt   time.Time
	lastSpeedAt    time.Time
	lastSpeedBytes int64
	lineActive     bool
	enabled        bool
	mu             sync.Mutex
}

func newConsoleDownloadProgress(path string) *downloadProgress {
	if IsServerChildProcess() || !stdoutIsTerminal() {
		return nil
	}
	return newDownloadProgress(os.Stdout, filepath.Base(path), -1, 0, true)
}

func newDownloadProgress(out io.Writer, label string, total, initial int64, enabled bool) *downloadProgress {
	if out == nil || !enabled {
		return nil
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "resource"
	}
	now := time.Now()
	return &downloadProgress{
		out:            out,
		label:          label,
		total:          total,
		current:        initial,
		lastSpeedAt:    now,
		lastSpeedBytes: initial,
		enabled:        true,
	}
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func (p *downloadProgress) SetTotal(total, initial int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.total = total
	if initial > p.current {
		p.current = initial
		p.lastSpeedBytes = initial
	}
	p.renderLocked(false)
}

func (p *downloadProgress) Add(delta int64) {
	if p == nil || delta <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current += delta
	p.renderLocked(false)
}

func (p *downloadProgress) Flush() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.renderLocked(false)
}

func (p *downloadProgress) Finish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.lineActive && p.current == 0 && p.total <= 0 {
		return
	}
	if p.total > 0 && p.current < p.total {
		p.current = p.total
	}
	p.renderLocked(true)
	if p.lineActive {
		fmt.Fprintln(p.out)
		p.lineActive = false
	}
}

func (p *downloadProgress) Abort() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lineActive {
		fmt.Fprint(p.out, "\r\033[K")
		fmt.Fprintln(p.out)
		p.lineActive = false
	}
}

func (p *downloadProgress) renderLocked(final bool) {
	if !p.enabled {
		return
	}
	now := time.Now()
	if !final && !p.lastRenderAt.IsZero() && now.Sub(p.lastRenderAt) < 200*time.Millisecond {
		return
	}
	speed := float64(0)
	if elapsed := now.Sub(p.lastSpeedAt).Seconds(); elapsed > 0 {
		speed = float64(p.current-p.lastSpeedBytes) / elapsed
	}
	p.lastSpeedAt = now
	p.lastSpeedBytes = p.current
	p.lastRenderAt = now

	// long 2026-06-20 16:09:00：普通 CLI 没有服务父进程帮忙消费隐藏事件，直接用资源写入量刷新单行进度，避免下载大文件时终端长时间没有反馈。
	prefix := "下载进度"
	if shouldColorOutput() {
		prefix = ansiGreen + prefix + ansiReset
	}
	if p.total > 0 {
		percent := float64(p.current) / float64(p.total) * 100
		if percent > 100 {
			percent = 100
		}
		fmt.Fprintf(p.out, "\r\033[K%s %s: %s / %s (%.1f%%) %s/s", prefix, p.label, FormatFileSize(float64(p.current)), FormatFileSize(float64(p.total)), percent, FormatFileSize(speed))
	} else {
		fmt.Fprintf(p.out, "\r\033[K%s %s: %s (%s/s)", prefix, p.label, FormatFileSize(float64(p.current)), FormatFileSize(speed))
	}
	p.lineActive = true
}
