package bbdown

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Subtitle struct {
	Lan  string
	URL  string
	Path string
}

func GetSubtitles(ctx context.Context, httpc *HTTPClient, cfg *Config, aid, cid, epID string, index int, intl bool) ([]Subtitle, error) {
	var subs []Subtitle
	if intl {
		subs = firstSubtitleResult(
			getIntlSubtitlesFromAPI1(ctx, httpc, cfg, aid, cid, epID),
			getIntlSubtitlesFromAPI2(ctx, httpc, cfg, aid, cid, epID, index),
		)
	} else {
		if cfg.Cookie != "" && cfg.WBISalt == "" {
			_, _ = CheckLogin(ctx, httpc, cfg)
		}
		if cfg.Cookie == "" {
			subs = firstSubtitleResult(getSubtitlesFromGrpc(ctx, httpc, cfg, aid, cid))
		} else {
			subs = firstSubtitleResult(
				getSubtitlesFromAPI2(ctx, httpc, cfg, aid, cid),
				getSubtitlesFromAPI1(ctx, httpc, aid, cid),
				getSubtitlesFromGrpc(ctx, httpc, cfg, aid, cid),
			)
		}
	}
	for i := range subs {
		if strings.HasPrefix(subs[i].URL, "//") {
			subs[i].URL = "https:" + subs[i].URL
		}
	}
	if subs == nil {
		return []Subtitle{}, nil
	}
	return subs, nil
}

func firstSubtitleResult(results ...[]Subtitle) []Subtitle {
	for _, subs := range results {
		if subs == nil {
			continue
		}
		// long: 原版字幕接口用 null 表示该路失败；空列表和带空 URL 的列表都属于该路已经给出结果，是否变成 null 由各接口解析函数按源仓库边界决定。
		return subs
	}
	return nil
}

func getSubtitlesFromAPI1(ctx context.Context, httpc *HTTPClient, aid, cid string) []Subtitle {
	api := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?aid=%s&cid=%s", url.QueryEscape(aid), url.QueryEscape(cid))
	body, err := httpc.Get(ctx, api)
	if err != nil {
		return nil
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil
	}
	list := root.Obj("data").Obj("subtitle").Arr("list")
	return parseSubtitleList(aid, cid, list, "lan", "subtitle_url")
}

func getSubtitlesFromAPI2(ctx context.Context, httpc *HTTPClient, cfg *Config, aid, cid string) []Subtitle {
	// long 2026-06-19 04:25:44：源仓库字幕 API2 虽然走 wbi/v2 路径，但请求参数只包含 cid 和 aid，不会附加 wts/sign。
	api := fmt.Sprintf("https://api.bilibili.com/x/player/wbi/v2?cid=%s&aid=%s", url.QueryEscape(cid), url.QueryEscape(aid))
	body, err := httpc.Get(ctx, api)
	if err != nil {
		return nil
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil
	}
	list := root.Obj("data").Obj("subtitle").Arr("subtitles")
	return parseSubtitleList(aid, cid, list, "lan", "subtitle_url")
}

func getIntlSubtitlesFromAPI1(ctx context.Context, httpc *HTTPClient, cfg *Config, aid, cid, epID string) []Subtitle {
	if epID == "" {
		return nil
	}
	host := cfg.EpHost
	if host == "api.bilibili.com" {
		host = "api.biliintl.com"
	}
	api := fmt.Sprintf("https://%s/intl/gateway/web/v2/subtitle?episode_id=%s", host, url.QueryEscape(epID))
	body, err := httpc.Get(ctx, api)
	if err != nil {
		return nil
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil
	}
	return parseIntlSubtitleList(aid, cid, root.Obj("data").Arr("subtitles"), "lang_key")
}

func getIntlSubtitlesFromAPI2(ctx context.Context, httpc *HTTPClient, cfg *Config, aid, cid, epID string, index int) []Subtitle {
	if epID == "" || index <= 0 {
		return nil
	}
	host := cfg.Host
	if host == "api.bilibili.com" {
		host = "api.bilibili.tv"
	}
	api := fmt.Sprintf("https://%s/intl/gateway/v2/ogv/view/app/season?ep_id=%s&platform=android&s_locale=zh_SG", host, url.QueryEscape(epID))
	if cfg.Token != "" {
		api += "&access_key=" + url.QueryEscape(cfg.Token)
	}
	body, err := httpc.Get(ctx, api)
	if err != nil {
		return nil
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil
	}
	episodes := root.Obj("result").Obj("modules").Arr("data")
	if len(episodes) == 0 {
		modules := root.Obj("result").Arr("modules")
		if len(modules) > 0 {
			episodes = asJ(modules[0]).Obj("data").Arr("episodes")
		}
	}
	if len(episodes) < index {
		return nil
	}
	return parseIntlSubtitleList(aid, cid, asJ(episodes[index-1]).Arr("subtitles"), "key")
}

func parseSubtitleList(aid, cid string, list []any, lanKey, urlKey string) []Subtitle {
	subs := make([]Subtitle, 0, len(list))
	for _, item := range arrItems(list) {
		lan := item.Str(lanKey)
		subURL := strings.ReplaceAll(item.Str(urlKey), `\\/`, "/")
		subs = append(subs, Subtitle{
			Lan:  lan,
			URL:  subURL,
			Path: filepath.Join(aid, fmt.Sprintf("%s.%s.%s.srt", aid, cid, lan)),
		})
	}
	if hasEmptySubtitleURL(subs) {
		return nil
	}
	return subs
}

func parseIntlSubtitleList(aid, cid string, list []any, lanKey string) []Subtitle {
	subs := make([]Subtitle, 0, len(list))
	for _, item := range arrItems(list) {
		lan := item.Str(lanKey)
		subURL := strings.ReplaceAll(item.Str("url"), `\\/`, "/")
		ext := ".ass"
		if strings.Contains(subURL, ".json") {
			ext = ".srt"
		}
		if hasEmptySubtitleURL(subs) {
			// long: 源仓库 Intl API 的空 URL 检查写在添加当前字幕之前；这里保留这个边界，让单条空 URL 仍按原版作为结果返回。
			return nil
		}
		subs = append(subs, Subtitle{
			Lan:  lan,
			URL:  subURL,
			Path: filepath.Join(aid, fmt.Sprintf("%s.%s.%s%s", aid, cid, lan, ext)),
		})
	}
	return subs
}

func hasEmptySubtitleURL(subs []Subtitle) bool {
	for _, sub := range subs {
		if sub.URL == "" {
			return true
		}
	}
	return false
}

func SaveSubtitle(ctx context.Context, httpc *HTTPClient, sub Subtitle, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	body, err := httpc.Get(ctx, sub.URL)
	if err != nil {
		return err
	}
	content := body
	// long 2026-06-19 04:30:04：源仓库用 path.EndsWith(".srt") 做大小写敏感判断，只有小写 .srt 才会触发 JSON 转 SRT。
	if strings.HasSuffix(outPath, ".srt") {
		converted, err := ConvertSubtitleJSONToSRT(body)
		if err != nil {
			return err
		}
		content = []byte(converted)
	}
	return os.WriteFile(outPath, content, 0o644)
}

type subtitleJSON struct {
	Body []subtitleLine `json:"body"`
}

type subtitleLine struct {
	From    *float64 `json:"from"`
	To      float64  `json:"to"`
	Content *string  `json:"content"`
}

func ConvertSubtitleJSONToSRT(body []byte) (string, error) {
	var payload subtitleJSON
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	var b strings.Builder
	for i, line := range payload.Body {
		from := 0.0
		if line.From != nil {
			from = *line.From
		}
		// long: B站 JSON 字幕用秒数表达时间轴，SRT 播放器需要固定毫秒格式；content 字段缺失时沿用原版的空字幕块语义，不额外伪造正文行。
		b.WriteString(fmt.Sprintf("%d\n%s --> %s\n", i+1, formatSRTTime(from), formatSRTTime(line.To)))
		if line.Content != nil {
			b.WriteString(*line.Content)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func formatSRTTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	d := time.Duration(seconds * float64(time.Second))
	// long: 原版字幕转换使用 TimeSpan 的 hh 组件格式，超过 24 小时会回到当天小时，而不是输出总小时数。
	hours := int(d/time.Hour) % 24
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, int((d%time.Hour)/time.Minute), int((d%time.Minute)/time.Second), int((d%time.Second)/time.Millisecond))
}

var subtitleCodeTable = map[string][2]string{
	"ai-Zh":   {"chi", "中文（简体, AI识别）"},
	"ai-En":   {"eng", "English(generated by ai)"},
	"zh-CN":   {"chi", "中文（简体）"},
	"zh-HK":   {"chi", "中文（香港繁體）"},
	"zh-Hans": {"chi", "中文（简体）"},
	"zh-TW":   {"chi", "中文（台灣繁體）"},
	"zh-Hant": {"chi", "中文（繁體）"},
	"en-US":   {"eng", "English(USA)"},
	"ja":      {"jpn", "日本語"},
	"ko":      {"kor", "한국어"},
	"zh-SG":   {"chi", "中文（新加坡）"},
	"ab":      {"abk", "Аҳәынҭқарра"},
	"aa":      {"aar", "Qafár af"},
	"af":      {"afr", "Afrikaans"},
	"sq":      {"alb", "Gjuha shqipe"},
	"ase":     {"ase", "American Sign Language"},
	"am":      {"amh", "አማርኛ"},
	"arc":     {"arc", "ܐܪܡܝܐ"},
	"hy":      {"arm", "հայերեն"},
	"as":      {"asm", "অসমীয়া"},
	"ay":      {"aym", "Aymar aru"},
	"az":      {"aze", "Azərbaycan"},
	"bn":      {"ben", "বাংলা ভাষার"},
	"ba":      {"bak", "Башҡорттеле"},
	"eu":      {"baq", "Euskara"},
	"be":      {"bel", "беларуская мова biełaruskaja mova"},
	"bh":      {"bih", "Bihar"},
	"bi":      {"bis", "Bislama"},
	"bs":      {"bos", "босански"},
	"br":      {"bre", "Breton"},
	"bg":      {"bul", "български"},
	"yue":     {"chi", "粵語"},
	"yue-HK":  {"chi", "粵語（中國香港）"},
	"ca":      {"cat", "català"},
	"chr":     {"chr", "ᏣᎳᎩ ᎦᏬᏂᎯᏍᏗ"},
	"cho":     {"cho", "Chahta'"},
	"co":      {"cos", "lingua corsa"},
	"hr":      {"hrv", "Hrvatska"},
	"cs":      {"cze", "čeština"},
	"da":      {"dan", "Dansk"},
	"nl":      {"dut", "Nederlands"},
	"nl-BE":   {"dut", "Nederlands(Belgisch)"},
	"nl-NL":   {"dut", "Nederlands(Nederlands)"},
	"dz":      {"dzo", "རྫོང་ཁ།"},
	"en":      {"eng", "English"},
	"en-CA":   {"eng", "English(Canada)"},
	"en-IE":   {"eng", "English(Ireland)"},
	"en-GB":   {"eng", "English(UK)"},
	"eo":      {"epo", "Esperanto"},
	"et":      {"est", "Eestlane"},
	"fo":      {"fao", "føroyskt"},
	"fj":      {"fij", "Vakaviti"},
	"fil":     {"phi", "Pilipino"},
	"fi":      {"fin", "Suomi"},
	"fr":      {"fre", "Français"},
	"fr-BE":   {"fre", "Français(Belgique)"},
	"fr-CA":   {"fre", "Français(Canada)"},
	"fr-FR":   {"fre", "Français(La France)"},
	"fr-CH":   {"fre", "Français(Suisse)"},
	"ff":      {"ful", "Fulani"},
	"gl":      {"glg", "galego"},
	"ka":      {"geo", "ქართული ენა"},
	"de":      {"ger", "Deutsch"},
	"de-AT":   {"ger", "Deutsch(Österreich)"},
	"de-DE":   {"ger", "Deutsch(Deutschland)"},
	"de-CH":   {"ger", "Deutsch(Schweiz)"},
	"el":      {"gre", "Ελληνικά"},
	"kl":      {"kal", "Kalaallisut"},
	"gn":      {"grn", "avañe'ẽ"},
	"gu":      {"guj", "ગુજરાતી"},
	"hak":     {"hak", "Hak-kâ-fa"},
	"hak-TW":  {"hak", "Hak-kâ-fa"},
	"ha":      {"hau", "هَوُسَ"},
	"iw":      {"heb", "שפה עברית"},
	"hi":      {"hin", "हिन्दी"},
	"hi-Latn": {"hin", "हिंदी(फोनेटिक)"},
	"hu":      {"hun", "Magyar"},
	"is":      {"ice", "icelandic"},
	"ig":      {"ibo", "Asụsụ Igbo"},
	"id":      {"ind", "Indonesia"},
	"ia":      {"ina", "Interlingua"},
	"iu":      {"iku", "ᐃᓄᒃᑎᑐᑦ"},
	"ik":      {"ipk", "Inupiat"},
	"ga":      {"gle", "Gaeilge na hÉireann"},
	"it":      {"ita", "Italiano"},
	"jv":      {"jav", "ꦧꦱꦗꦮ"},
	"kn":      {"kan", "ಕನ್ನಡ"},
	"ks":      {"kas", "कॉशुर"},
	"kk":      {"kaz", "Қазақ тілі"},
	"km":      {"khm", "ភាសាខ្មែរ"},
	"rw":      {"kin", "Ikinyarwanda"},
	"tlh":     {"tlh", "tlhIngan Hol"},
	"ku":      {"kur", "Kurdî"},
	"ky":      {"kir", "кыргыз тили"},
	"lo":      {"lao", "ພາສາລາວ"},
	"la":      {"lat", "latīna"},
	"lv":      {"lav", "latviešu valoda"},
	"ln":      {"lin", "Lingála"},
	"lt":      {"lit", "lietuvių kalba"},
	"lb":      {"ltz", "Lëtzebuergesch"},
	"mk":      {"mac", "Македонски јазик"},
	"mg":      {"mlg", "maa.laa.gaas"},
	"ms":      {"may", "Melayu"},
	"ml":      {"mal", "മലയാളം"},
	"mt":      {"mlt", "Lingwa Maltija"},
	"mi":      {"mao", "Māori"},
	"mr":      {"mar", "मराठी Marāṭhī"},
	"mas":     {"mas", "Maasai"},
	"nan":     {"nan", "閩南語"},
	"nan-TW":  {"nan", "閩南語(台灣)"},
	"lus":     {"lus", "Mizo ṭawng"},
	"mo":      {"mol", "Limba moldovenească"},
	"mn":      {"mon", "монгол хэл"},
	"my":      {"bur", "မြန်မာဘာသာ"},
	"na":      {"nau", "Dorerin Naoero"},
	"nv":      {"nav", "Diné bizaad"},
	"ne":      {"nep", "नेपाली Nepālī"},
	"no":      {"nor", "norsk språk"},
	"fa":      {"per", "فارسی"},
	"fa-AF":   {"per", "فارسی"},
	"fa-IR":   {"per", "فارسی"},
	"pl":      {"pol", "Polski"},
	"pt":      {"por", "Português"},
	"pt-BR":   {"por", "Português(brasil)"},
	"pt-PT":   {"por", "Português(portugal)"},
	"ro":      {"rum", "Română"},
	"ru":      {"rus", "Русский"},
	"ru-Latn": {"rus", "Русский(фонетический)"},
	"sr":      {"srp", "Српски"},
	"sr-Cyrl": {"srp", "Српски(ћирилица)"},
	"sr-Latn": {"srp", "Српски(латиница)"},
	"sh":      {"scr", "srpskohrvatski"},
	"sk":      {"slo", "slovenský"},
	"es":      {"spa", "Español"},
	"es-419":  {"spa", "Español(Latinoamérica)"},
	"es-MX":   {"spa", "Español(México)"},
	"es-ES":   {"spa", "Español(España)"},
	"es-US":   {"spa", "Español(Estados Unidos)"},
	"sv":      {"swe", "Svenska"},
	"tl":      {"tgl", "Tagalog"},
	"th":      {"tha", "ไทย"},
	"tr":      {"tur", "Türkçe"},
	"uk":      {"ukr", "Українська"},
	"ur":      {"urd", "Urdu"},
	"vi":      {"vie", "Tiếng Việt"},
}

func SubtitleCode(key string) (string, string) {
	key = normalizeSubtitleLan(key)
	// long: 原版把 B 站字幕语言码翻译成 ISO-639-2 与可读标题，混流时播放器依赖这里识别字幕轨语言。
	if value, ok := subtitleCodeTable[key]; ok {
		return value[0], value[1]
	}
	return "und", "Undetermined"
}

func normalizeSubtitleLan(key string) string {
	for i := 0; i+1 < len(key); i++ {
		if key[i] == '-' && key[i+1] >= 'a' && key[i+1] <= 'z' {
			// long: 源仓库只读取 Regex.Match 的第一个命中，再把同一个片段全量替换，不能把后续不同片段也逐个首字母大写。
			match := key[i : i+2]
			return strings.ReplaceAll(key, match, strings.ToUpper(match))
		}
	}
	return key
}

func SubtitleOutputPath(savePath string, sub Subtitle) string {
	ext := ".srt"
	if !strings.Contains(sub.URL, ".json") && strings.HasSuffix(strings.ToLower(sub.Path), ".ass") {
		ext = ".ass"
	}
	return strings.TrimSuffix(savePath, ".mp4") + "." + sanitizeFilename(sub.Lan) + ext
}
