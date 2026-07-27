package bbdown

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLatestVersionFromReleaseURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/lonnnnnng/BB-DL/releases/tag/1.0.0":            "1.0.0",
		"https://github.com/lonnnnnng/BB-DL/releases/tag/1.0.0?expanded=1": "1.0.0",
		"https://github.com/lonnnnnng/BB-DL/releases/latest":               "",
	}
	for input, want := range cases {
		if got := LatestVersionFromReleaseURL(input); got != want {
			t.Fatalf("LatestVersionFromReleaseURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCheckUpdateLogsNewVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			http.Redirect(w, r, "/lonnnnnng/BB-DL/releases/tag/9.9.9", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		err := CheckUpdate(context.Background(), NewHTTPClient(NewConfig()), server.URL+"/latest")
		if err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "发现新版本：9.9.9") {
		t.Fatalf("update output = %q", out)
	}
}

func TestCheckUpdateSkipsOlderRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			http.Redirect(w, r, "/lonnnnnng/BB-DL/releases/tag/0.9.9", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		err := CheckUpdate(context.Background(), NewHTTPClient(NewConfig()), server.URL+"/latest")
		if err != nil {
			t.Fatal(err)
		}
	})
	if out != "" {
		t.Fatalf("older release should not log update, got %q", out)
	}
}

func TestCheckUpdateSkipsCurrentVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			http.Redirect(w, r, "/lonnnnnng/BB-DL/releases/tag/"+Version, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		err := CheckUpdate(context.Background(), NewHTTPClient(NewConfig()), server.URL+"/latest")
		if err != nil {
			t.Fatal(err)
		}
	})
	if out != "" {
		t.Fatalf("current version should not log update, got %q", out)
	}
}

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{latest: "1.6.5", current: "1.6.4", want: true},
		{latest: "v1.7.0", current: "1.6.4", want: true},
		{latest: "1.10.0", current: "1.9.9", want: true},
		{latest: "1.6.4", current: "1.6.4", want: false},
		{latest: "1.6.3", current: "1.6.4", want: false},
		{latest: "1.6.4", current: "1.6.4.1", want: false},
		{latest: "", current: "1.6.4", want: false},
	}
	for _, tc := range cases {
		if got := IsNewerVersion(tc.latest, tc.current); got != tc.want {
			t.Fatalf("IsNewerVersion(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
