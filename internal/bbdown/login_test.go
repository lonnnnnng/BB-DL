package bbdown

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRenderConsoleQRCode(t *testing.T) {
	rendered, err := renderConsoleQRCode("https://example.test/login?qrcode_key=test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "\x1b[40m") || !strings.Contains(rendered, "\x1b[47m") {
		t.Fatalf("console QR should contain black and white terminal cells")
	}
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) < 21 {
		t.Fatalf("console QR has too few lines: %d", len(lines))
	}
}

func TestLoginWEBWritesCookieWithoutLoggingSecret(t *testing.T) {
	dir := enterTempLoginDir(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/web/generate":
			_, _ = w.Write([]byte(`{"data":{"url":"https://passport.example/qrcode?qrcode_key=web-qr-key"}}`))
		case "/web/poll":
			if r.URL.Query().Get("qrcode_key") != "web-qr-key" {
				t.Errorf("qrcode_key = %q, want web-qr-key", r.URL.Query().Get("qrcode_key"))
			}
			_, _ = w.Write([]byte(`{"data":{"code":0,"url":"https://www.bilibili.com/?SESSDATA=web-secret,part&bili_jct=jct-secret"}}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	resetLoginTestHooks(t, dir, server.URL)

	httpc := NewHTTPClient(NewConfig())
	var err error
	out := captureStdout(t, func() {
		err = LoginWEB(context.Background(), httpc)
	})
	if err != nil {
		t.Fatal(err)
	}

	payload, err := os.ReadFile(filepath.Join(dir, "BBDown.data"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), "SESSDATA=web-secret%2Cpart;bili_jct=jct-secret"; got != want {
		t.Fatalf("BBDown.data = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "qrcode.png")); !os.IsNotExist(err) {
		t.Fatalf("qrcode.png should be removed after login success, stat err=%v", err)
	}
	for _, leaked := range []string{"web-secret", "jct-secret", "SESSDATA=", "bili_jct="} {
		if strings.Contains(out, leaked) {
			t.Fatalf("login output leaked %q: %s", leaked, out)
		}
	}
	if !strings.Contains(out, "登录成功, 已保存 BBDown.data") {
		t.Fatalf("login output should mention saved credential file: %s", out)
	}
}

func TestLoginTVWritesTokenWithoutLoggingSecret(t *testing.T) {
	dir := enterTempLoginDir(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		switch r.URL.Path {
		case "/tv/auth_code":
			_, _ = w.Write([]byte(`{"data":{"url":"https://passport.example/tv?qrcode_key=tv-qr-key","auth_code":"tv-auth-code"}}`))
		case "/tv/poll":
			if r.Form.Get("auth_code") != "tv-auth-code" {
				t.Errorf("auth_code = %q, want tv-auth-code", r.Form.Get("auth_code"))
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"tv-secret-token"}}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	resetLoginTestHooks(t, dir, server.URL)

	httpc := NewHTTPClient(NewConfig())
	var err error
	out := captureStdout(t, func() {
		err = LoginTV(context.Background(), httpc)
	})
	if err != nil {
		t.Fatal(err)
	}

	payload, err := os.ReadFile(filepath.Join(dir, "BBDownTV.data"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), "access_token=tv-secret-token"; got != want {
		t.Fatalf("BBDownTV.data = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "qrcode.png")); !os.IsNotExist(err) {
		t.Fatalf("qrcode.png should be removed after TV login success, stat err=%v", err)
	}
	for _, leaked := range []string{"tv-secret-token", "access_token=", "AccessToken"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("TV login output leaked %q: %s", leaked, out)
		}
	}
	if !strings.Contains(out, "登录成功, 已保存 BBDownTV.data") {
		t.Fatalf("TV login output should mention saved credential file: %s", out)
	}
}

func TestLoginTVReportsWaitingStates(t *testing.T) {
	dir := enterTempLoginDir(t)
	var pollCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		switch r.URL.Path {
		case "/tv/auth_code":
			_, _ = w.Write([]byte(`{"data":{"url":"https://passport.example/tv?qrcode_key=tv-qr-key","auth_code":"tv-auth-code"}}`))
		case "/tv/poll":
			if r.Form.Get("auth_code") != "tv-auth-code" {
				t.Errorf("auth_code = %q, want tv-auth-code", r.Form.Get("auth_code"))
			}
			switch pollCount.Add(1) {
			case 1:
				_, _ = w.Write([]byte(`{"code":86039,"message":"waiting scan"}`))
			case 2:
				_, _ = w.Write([]byte(`{"code":86090,"message":"waiting confirm"}`))
			default:
				_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"tv-secret-token"}}`))
			}
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	resetLoginTestHooks(t, dir, server.URL)

	httpc := NewHTTPClient(NewConfig())
	var err error
	out := captureStdout(t, func() {
		err = LoginTV(context.Background(), httpc)
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"等待扫码...", "扫码成功, 请确认...", "登录成功, 已保存 BBDownTV.data"} {
		if !strings.Contains(out, want) {
			t.Fatalf("TV login output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "tv-secret-token") || strings.Contains(out, "access_token=") {
		t.Fatalf("TV login output leaked token: %s", out)
	}
}

func TestLoginTVAcceptsTokenOnNonZeroCode(t *testing.T) {
	dir := enterTempLoginDir(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		switch r.URL.Path {
		case "/tv/auth_code":
			_, _ = w.Write([]byte(`{"data":{"url":"https://passport.example/tv?qrcode_key=tv-qr-key","auth_code":"tv-auth-code"}}`))
		case "/tv/poll":
			_, _ = w.Write([]byte(`{"code":86090,"message":"confirmed","data":{"access_token":"tv-secret-token"}}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	resetLoginTestHooks(t, dir, server.URL)

	httpc := NewHTTPClient(NewConfig())
	var err error
	out := captureStdout(t, func() {
		err = LoginTV(context.Background(), httpc)
	})
	if err != nil {
		t.Fatal(err)
	}

	payload, err := os.ReadFile(filepath.Join(dir, "BBDownTV.data"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), "access_token=tv-secret-token"; got != want {
		t.Fatalf("BBDownTV.data = %q, want %q", got, want)
	}
	if strings.Contains(out, "tv-secret-token") || strings.Contains(out, "access_token=") {
		t.Fatalf("TV login output leaked token: %s", out)
	}
}

func enterTempLoginDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	return dir
}

func resetLoginTestHooks(t *testing.T, dir, baseURL string) {
	t.Helper()
	oldWebGenerateURL := webQRCodeGenerateURL
	oldWebPollURL := webQRCodePollURL
	oldTVAuthCodeURL := tvQRCodeAuthCodeURL
	oldTVPollURL := tvQRCodePollURL
	oldPollInterval := loginPollInterval
	oldStatusLogEvery := loginStatusLogEvery
	oldAppDir := loginAppDir

	webQRCodeGenerateURL = baseURL + "/web/generate"
	webQRCodePollURL = baseURL + "/web/poll"
	tvQRCodeAuthCodeURL = baseURL + "/tv/auth_code"
	tvQRCodePollURL = baseURL + "/tv/poll"
	loginPollInterval = time.Nanosecond
	loginStatusLogEvery = time.Hour
	loginAppDir = func() string { return dir }

	t.Cleanup(func() {
		webQRCodeGenerateURL = oldWebGenerateURL
		webQRCodePollURL = oldWebPollURL
		tvQRCodeAuthCodeURL = oldTVAuthCodeURL
		tvQRCodePollURL = oldTVPollURL
		loginPollInterval = oldPollInterval
		loginStatusLogEvery = oldStatusLogEvery
		loginAppDir = oldAppDir
	})
}
