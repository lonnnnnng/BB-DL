package bbdown

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

var (
	webQRCodeGenerateURL = "https://passport.bilibili.com/x/passport-login/web/qrcode/generate?source=main-fe-header"
	webQRCodePollURL     = "https://passport.bilibili.com/x/passport-login/web/qrcode/poll"
	tvQRCodeAuthCodeURL  = "https://passport.snm0516.aisee.tv/x/passport-tv-login/qrcode/auth_code"
	tvQRCodePollURL      = "https://passport.bilibili.com/x/passport-tv-login/qrcode/poll"
	loginPollInterval    = time.Second
	loginStatusLogEvery  = 30 * time.Second
	loginAppDir          = AppDir
)

func LoginWEB(ctx context.Context, httpc *HTTPClient) error {
	Log("获取登录地址...")
	body, err := httpc.Get(ctx, webQRCodeGenerateURL)
	if err != nil {
		return err
	}
	root, err := ParseJ(body)
	if err != nil {
		return err
	}
	loginURL := root.Obj("data").Str("url")
	qrKey := GetQueryString("qrcode_key", loginURL)
	if qrKey == "" {
		return fmt.Errorf("未拿到 qrcode_key")
	}
	if err := writeQRCode("qrcode.png", loginURL); err != nil {
		return err
	}
	Log("生成二维码成功: qrcode.png, 请打开并扫描, 或扫描打印的二维码")
	_ = printConsoleQRCode(loginURL)

	waitingConfirm := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(loginPollInterval):
		}
		statusBody, err := httpc.Get(ctx, webQRCodePollURL+"?qrcode_key="+url.QueryEscape(qrKey)+"&source=main-fe-header")
		if err != nil {
			return err
		}
		statusRoot, err := ParseJ(statusBody)
		if err != nil {
			return err
		}
		data := statusRoot.Obj("data")
		code := data.Int("code")
		switch code {
		case 86038:
			LogColor("二维码已过期, 请重新执行登录指令.", true)
			return nil
		case 86101:
			continue
		case 86090:
			if !waitingConfirm {
				Log("扫码成功, 请确认...")
				waitingConfirm = true
			}
		default:
			cbURL := data.Str("url")
			idx := strings.Index(cbURL, "?")
			if idx < 0 || idx+1 >= len(cbURL) {
				return fmt.Errorf("登录成功回调缺少 cookie 参数")
			}
			payload := strings.ReplaceAll(cbURL[idx+1:], "&", ";")
			payload = strings.ReplaceAll(payload, ",", "%2C")
			// long: 登录态只写入本地凭据文件，终端日志不暴露 SESSDATA/bili_jct，便于后续扫码回归时保留安全边界。
			if err := os.WriteFile(filepath.Join(loginAppDir(), "BBDown.data"), []byte(payload), 0o644); err != nil {
				return err
			}
			Log("登录成功, 已保存 BBDown.data")
			_ = os.Remove("qrcode.png")
			return nil
		}
	}
}

func LoginTV(ctx context.Context, httpc *HTTPClient) error {
	Log("获取登录地址...")
	values := GetTVLoginValues()
	body, err := httpc.PostForm(ctx, tvQRCodeAuthCodeURL, values, nil)
	if err != nil {
		return err
	}
	root, err := ParseJ(body)
	if err != nil {
		return err
	}
	data := root.Obj("data")
	qrURL := data.Str("url")
	authCode := data.Str("auth_code")
	if qrURL == "" || authCode == "" {
		return fmt.Errorf("未拿到 TV 登录二维码")
	}
	if err := writeQRCode("qrcode.png", qrURL); err != nil {
		return err
	}
	Log("生成二维码成功: qrcode.png, 请打开并扫描, 或扫描打印的二维码")
	_ = printConsoleQRCode(qrURL)
	values.Set("auth_code", authCode)
	values.Set("ts", GetTimeStamp(true))
	values.Del("sign")
	values.Set("sign", GetSign(values.Encode(), false))
	waitingScan := false
	waitingConfirm := false
	startedAt := time.Now()
	lastStatusLogAt := startedAt
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(loginPollInterval):
		}
		statusBody, err := httpc.PostForm(ctx, tvQRCodePollURL, values, nil)
		if err != nil {
			return err
		}
		var raw map[string]any
		if err := json.Unmarshal(statusBody, &raw); err != nil {
			return err
		}
		code := fmt.Sprint(raw["code"])
		message := loginResponseMessage(raw)
		if accessToken := loginAccessToken(raw); accessToken != "" {
			// long: 原版 TV 登录对非等待状态直接读取 access_token；这里先认 token 再看状态码，避免接口成功码变化时漏掉已确认登录。
			if err := os.WriteFile(filepath.Join(loginAppDir(), "BBDownTV.data"), []byte("access_token="+accessToken), 0o644); err != nil {
				return err
			}
			Log("登录成功, 已保存 BBDownTV.data")
			_ = os.Remove("qrcode.png")
			return nil
		}
		switch code {
		case "0":
			return fmt.Errorf("TV 登录成功响应缺少 access_token")
		case "86038":
			LogColor("二维码已过期, 请重新执行登录指令.", true)
			return nil
		case "86039", "86101":
			if !waitingScan {
				Log("等待扫码...")
				waitingScan = true
				lastStatusLogAt = time.Now()
			} else if time.Since(lastStatusLogAt) >= loginStatusLogEvery {
				// long: TV 登录问题常见于客户端未真正认领二维码，周期性输出状态码可以定位卡点，同时不暴露二维码内容或 token。
				Log(fmt.Sprintf("TV 登录仍在等待扫码: code=%s message=%s elapsed=%s", code, message, loginElapsed(startedAt)))
				lastStatusLogAt = time.Now()
			}
			continue
		case "86090":
			if !waitingConfirm {
				Log("扫码成功, 请确认...")
				waitingConfirm = true
				lastStatusLogAt = time.Now()
			} else if time.Since(lastStatusLogAt) >= loginStatusLogEvery {
				Log(fmt.Sprintf("TV 登录等待手机端确认: code=%s message=%s elapsed=%s", code, message, loginElapsed(startedAt)))
				lastStatusLogAt = time.Now()
			}
			continue
		default:
			return fmt.Errorf("TV 登录失败: code=%s message=%s", code, message)
		}
	}
}

func loginAccessToken(raw map[string]any) string {
	data, _ := raw["data"].(map[string]any)
	accessToken := fmt.Sprint(data["access_token"])
	if accessToken == "" || accessToken == "<nil>" {
		return ""
	}
	return accessToken
}

func loginResponseMessage(raw map[string]any) string {
	message := fmt.Sprint(raw["message"])
	if message == "<nil>" {
		return ""
	}
	return message
}

func loginElapsed(startedAt time.Time) string {
	elapsed := time.Since(startedAt).Round(time.Second)
	if elapsed < 0 {
		return "0s"
	}
	return elapsed.String()
}

func writeQRCode(path, text string) error {
	return qrcode.WriteFile(text, qrcode.Medium, 256, path)
}

func printConsoleQRCode(text string) error {
	rendered, err := renderConsoleQRCode(text)
	if err != nil {
		return err
	}
	fmt.Print(rendered)
	return nil
}

func renderConsoleQRCode(text string) (string, error) {
	code, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, row := range code.Bitmap() {
		for _, dark := range row {
			if dark {
				b.WriteString("\x1b[40m  \x1b[0m")
			} else {
				b.WriteString("\x1b[47m  \x1b[0m")
			}
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func GetTVLoginValues() url.Values {
	values := url.Values{}
	ts := GetTimeStamp(true)
	deviceID := GetRandomString(20)
	buvid := GetRandomString(37)
	fingerprint := time.Now().Format("20060102150405000") + GetRandomString(45)
	values.Set("appkey", "4409e2ce8ffd12b8")
	values.Set("auth_code", "")
	values.Set("bili_local_id", deviceID)
	values.Set("build", "102801")
	values.Set("buvid", buvid)
	values.Set("channel", "master")
	values.Set("device", "OnePlus")
	values.Set("device_id", deviceID)
	values.Set("device_name", "OnePlus7TPro")
	values.Set("device_platform", "Android10OnePlusHD1910")
	values.Set("fingerprint", fingerprint)
	values.Set("guid", buvid)
	values.Set("local_fingerprint", fingerprint)
	values.Set("local_id", buvid)
	values.Set("mobi_app", "android_tv_yst")
	values.Set("networkstate", "wifi")
	values.Set("platform", "android")
	values.Set("sys_ver", "29")
	values.Set("ts", ts)
	values.Set("sign", GetSign(values.Encode(), false))
	return values
}

func GetRandomString(length int) string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_0123456789"
	var b strings.Builder
	b.Grow(length)
	for i := 0; i < length; i++ {
		b.WriteByte(chars[rand.Intn(len(chars))])
	}
	return b.String()
}
