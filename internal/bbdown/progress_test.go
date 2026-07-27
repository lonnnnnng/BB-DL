package bbdown

import (
	"bytes"
	"strings"
	"testing"
)

func TestDownloadProgressRendersPercentAndSpeed(t *testing.T) {
	var out bytes.Buffer
	progress := newDownloadProgress(&out, "demo.mp4", 10, 0, true)
	progress.Add(5)
	progress.Finish()
	body := out.String()
	for _, want := range []string{"下载进度 demo.mp4", "100.0%", "10 bytes"} {
		if !strings.Contains(body, want) {
			t.Fatalf("progress output missing %q: %q", want, body)
		}
	}
	if !strings.HasSuffix(body, "\n") {
		t.Fatalf("finished progress should end with newline: %q", body)
	}
}

func TestDownloadProgressFinishWithoutActivityIsSilent(t *testing.T) {
	var out bytes.Buffer
	progress := newDownloadProgress(&out, "demo.mp4", -1, 0, true)
	progress.Finish()
	if out.String() != "" {
		t.Fatalf("inactive progress should not print fake zero-byte line: %q", out.String())
	}
}
