package bbdown

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	ansiReset      = "\033[0m"
	ansiDarkGray   = "\033[90m"
	ansiCyan       = "\033[36m"
	ansiGreen      = "\033[32m"
	ansiDarkYellow = "\033[33m"
	ansiRed        = "\033[31m"

	logContinuationIndent = "                            "
)

func timestamp() string {
	return time.Now().Format("[2006-01-02 15:04:05.000]")
}

func Log(args ...any) {
	writeLogLine("", true, strings.TrimSuffix(fmt.Sprintln(args...), "\n"))
}

func Logf(format string, args ...any) {
	writeLogLine("", true, fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...any) {
	writeLogLine(ansiDarkYellow, true, fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...any) {
	writeLogLine(ansiRed, true, fmt.Sprintf(format, args...))
}

func LogColor(text any, timePrefix bool) {
	writeLogLine(ansiCyan, timePrefix, fmt.Sprint(text))
}

func writeLogLine(color string, timePrefix bool, text string) {
	useColor := shouldColorOutput()
	if timePrefix {
		writeMaybeColored(timestamp()+" - ", ansiDarkGray, useColor)
	} else {
		fmt.Print(logContinuationIndent)
	}
	writeMaybeColored(text, color, useColor)
	fmt.Println()
}

func writeMaybeColored(text, color string, enabled bool) {
	if enabled && color != "" {
		fmt.Print(color, text, ansiReset)
		return
	}
	fmt.Print(text)
}

func shouldColorOutput() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	switch os.Getenv("BBDOWN_FORCE_COLOR") {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	}
	return stdoutIsTerminal()
}
