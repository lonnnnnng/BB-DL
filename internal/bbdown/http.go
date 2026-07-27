package bbdown

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPClient struct {
	client    *http.Client
	config    *Config
	userAgent string
}

func NewHTTPClient(cfg *Config) *HTTPClient {
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		DisableCompression:  false,
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 32,
	}
	return &HTTPClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   2 * time.Minute,
		},
		config:    cfg,
		userAgent: randomUserAgent(),
	}
}

func randomUserAgent() string {
	platforms := []string{
		"Windows NT 10.0; Win64",
		"Macintosh; Intel Mac OS X 10_15",
		"X11; Linux x86_64",
	}
	browsers := []string{
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Gecko/20100101 Firefox/130.0",
	}
	return "Mozilla/5.0 (" + platforms[rand.IntN(len(platforms))] + ") " + browsers[rand.IntN(len(browsers))]
}

func (h *HTTPClient) SetUserAgent(ua string) {
	if strings.TrimSpace(ua) != "" {
		h.userAgent = ua
	}
}

func (h *HTTPClient) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", h.userAgent)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Cache-Control", "no-cache")
	if strings.Contains(url, "/ep") || strings.Contains(url, "/ss") {
		req.Header.Set("Cookie", h.config.Cookie+";CURRENT_FNVAL=4048;")
	} else if h.config.Cookie != "" {
		req.Header.Set("Cookie", h.config.Cookie)
	}
	if strings.Contains(url, "api.bilibili.com") {
		req.Header.Set("Referer", "https://www.bilibili.com/")
	}
	if strings.Contains(url, "api.bilibili.tv") {
		// long: 国际版接口原版会带 Chrome 客户端提示头，避免部分边缘接口把 Go 默认请求当成非浏览器流量。
		req.Header.Set("sec-ch-ua", `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	reader, cleanup, err := decodedBodyReader(resp)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return io.ReadAll(reader)
}

func (h *HTTPClient) ResolveLocation(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", h.userAgent)
	// long: 原版重定向探测请求也声明压缩能力并禁用缓存，避免同一 URL 在 HEAD 与 GET 阶段触发不同的 CDN/缓存策略。
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return resp.Request.URL.String(), nil
}

func (h *HTTPClient) PostForm(ctx context.Context, rawURL string, values url.Values, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", h.userAgent)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// long: TV 登录等表单接口也可能返回压缩内容，和原版 HttpClientHandler 自动解压保持一致，避免扫码轮询拿到 gzip 原文。
	reader, cleanup, err := decodedBodyReader(resp)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return io.ReadAll(reader)
}

func decodedBodyReader(resp *http.Response) (io.Reader, func(), error) {
	encoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
	switch {
	case strings.Contains(encoding, "gzip"):
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, func() {}, err
		}
		return reader, func() { _ = reader.Close() }, nil
	case strings.Contains(encoding, "deflate"):
		flateReader := flate.NewReader(resp.Body)
		return flateReader, func() { _ = flateReader.Close() }, nil
	default:
		return resp.Body, func() {}, nil
	}
}
