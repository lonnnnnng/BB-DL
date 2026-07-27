package bbdown

import (
	"strings"
	"testing"
)

func TestLogColorIndentsContinuationLineLikeOriginal(t *testing.T) {
	out := captureStdout(t, func() {
		LogColor("0. [480P 清晰]", false)
	})
	want := logContinuationIndent + "0. [480P 清晰]\n"
	if out != want {
		t.Fatalf("LogColor continuation output = %q, want %q", out, want)
	}
}

func TestLoggerUsesColorsByTypeWhenForced(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("BBDOWN_FORCE_COLOR", "1")
	out := captureStdout(t, func() {
		Log("info")
		Warnf("warn")
		Errorf("err")
		LogColor("title", true)
	})
	for _, want := range []string{
		ansiDarkGray + "[",
		ansiDarkYellow + "warn" + ansiReset,
		ansiRed + "err" + ansiReset,
		ansiCyan + "title" + ansiReset,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("colored output missing %q: %q", want, out)
		}
	}
}

func TestLoggerNoColorDisablesForcedColor(t *testing.T) {
	t.Setenv("BBDOWN_FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "1")
	out := captureStdout(t, func() {
		Warnf("warn")
	})
	if strings.Contains(out, "\033[") {
		t.Fatalf("NO_COLOR should suppress ANSI escapes: %q", out)
	}
}
