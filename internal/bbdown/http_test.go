package bbdown

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPostFormDecodesCompressedResponses(t *testing.T) {
	cases := map[string]func(io.Writer) (io.WriteCloser, error){
		"gzip": func(w io.Writer) (io.WriteCloser, error) {
			return gzip.NewWriter(w), nil
		},
		"deflate": func(w io.Writer) (io.WriteCloser, error) {
			return flate.NewWriter(w, flate.DefaultCompression)
		},
	}

	for encoding, newWriter := range cases {
		t.Run(encoding, func(t *testing.T) {
			var sawAcceptEncoding bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Accept-Encoding"); strings.Contains(got, "gzip") && strings.Contains(got, "deflate") {
					sawAcceptEncoding = true
				}
				if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
					t.Fatalf("Content-Type = %q", got)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if got := r.Form.Get("auth_code"); got != "qr-token" {
					t.Fatalf("auth_code = %q", got)
				}

				var buf bytes.Buffer
				zw, err := newWriter(&buf)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := zw.Write([]byte(`{"code":0,"message":"ok"}`)); err != nil {
					t.Fatal(err)
				}
				if err := zw.Close(); err != nil {
					t.Fatal(err)
				}
				w.Header().Set("Content-Encoding", encoding)
				_, _ = w.Write(buf.Bytes())
			}))
			defer server.Close()

			httpc := &HTTPClient{client: server.Client(), config: NewConfig(), userAgent: "test-agent"}
			body, err := httpc.PostForm(context.Background(), server.URL, url.Values{"auth_code": {"qr-token"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != `{"code":0,"message":"ok"}` {
				t.Fatalf("decoded body = %q", body)
			}
			if !sawAcceptEncoding {
				t.Fatal("PostForm should advertise gzip and deflate support")
			}
		})
	}
}

func TestResolveLocationUsesOriginalHeaders(t *testing.T) {
	var sawHEAD bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("method = %s, want HEAD", r.Method)
		}
		sawHEAD = true
		if got := r.Header.Get("User-Agent"); got != "test-agent" {
			t.Fatalf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Accept-Encoding"); !strings.Contains(got, "gzip") || !strings.Contains(got, "deflate") {
			t.Fatalf("Accept-Encoding = %q, want gzip and deflate", got)
		}
		if got := r.Header.Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("Cache-Control = %q, want no-cache", got)
		}
	}))
	defer server.Close()

	httpc := &HTTPClient{client: server.Client(), config: NewConfig(), userAgent: "test-agent"}
	if _, err := httpc.ResolveLocation(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	if !sawHEAD {
		t.Fatal("ResolveLocation did not issue HEAD request")
	}
}

func TestGetUsesOriginalWebSourceHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "test-agent" {
			t.Fatalf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Accept-Encoding"); !strings.Contains(got, "gzip") || !strings.Contains(got, "deflate") {
			t.Fatalf("Accept-Encoding = %q, want gzip and deflate", got)
		}
		if got := r.Header.Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("Cache-Control = %q, want no-cache", got)
		}
		switch r.Host {
		case "api.bilibili.com":
			if got := r.Header.Get("Referer"); got != "https://www.bilibili.com/" {
				t.Fatalf("Referer = %q", got)
			}
			if got := r.Header.Get("Cookie"); got != "fake-cookie" {
				t.Fatalf("Cookie = %q", got)
			}
		case "api.bilibili.tv":
			if got := r.Header.Get("sec-ch-ua"); got != `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"` {
				t.Fatalf("sec-ch-ua = %q", got)
			}
		default:
			t.Fatalf("unexpected host %q", r.Host)
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := NewConfig()
	cfg.Cookie = "fake-cookie"
	httpc := &HTTPClient{client: &http.Client{Transport: rewriteHostTransport{target: target, base: server.Client().Transport}}, config: cfg, userAgent: "test-agent"}

	if _, err := httpc.Get(context.Background(), "https://api.bilibili.com/x/web-interface/view?bvid=BV1qt4y1X7TW"); err != nil {
		t.Fatal(err)
	}
	if _, err := httpc.Get(context.Background(), "https://api.bilibili.tv/intl/gateway/web/v2/subtitle?s_locale=zh_SG"); err != nil {
		t.Fatal(err)
	}
}
