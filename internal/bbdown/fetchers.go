package bbdown

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var intlInitialStatePattern = regexp.MustCompile(`window\.__INITIAL_STATE__=([\s\S].*?);\(function\(\)`)

type Fetcher interface {
	Fetch(ctx context.Context, id string) (*VInfo, error)
}

type FetcherFactory struct {
	httpc *HTTPClient
	cfg   *Config
}

func NewFetcherFactory(httpc *HTTPClient, cfg *Config) *FetcherFactory {
	return &FetcherFactory{httpc: httpc, cfg: cfg}
}

func (f *FetcherFactory) CreateFetcher(aidOri string, useIntl bool) Fetcher {
	switch {
	case strings.HasPrefix(aidOri, "cheese:"):
		return &CheeseInfoFetcher{httpc: f.httpc}
	case strings.HasPrefix(aidOri, "ep:") && useIntl:
		return &IntlBangumiInfoFetcher{httpc: f.httpc, cfg: f.cfg}
	case strings.HasPrefix(aidOri, "ep:"):
		return &BangumiInfoFetcher{httpc: f.httpc, cfg: f.cfg}
	case strings.HasPrefix(aidOri, "mid:"):
		return &SpaceVideoFetcher{httpc: f.httpc, cfg: f.cfg}
	case strings.HasPrefix(aidOri, "listBizId:"):
		return &MediaListFetcher{httpc: f.httpc}
	case strings.HasPrefix(aidOri, "seriesBizId:"):
		return &SeriesListFetcher{httpc: f.httpc}
	case strings.HasPrefix(aidOri, "favId:"):
		return &FavListFetcher{httpc: f.httpc}
	default:
		return &NormalInfoFetcher{httpc: f.httpc}
	}
}

type NormalInfoFetcher struct{ httpc *HTTPClient }

func (f *NormalInfoFetcher) Fetch(ctx context.Context, id string) (*VInfo, error) {
	body, err := f.httpc.Get(ctx, "https://api.bilibili.com/x/web-interface/view?aid="+id)
	if err != nil {
		return nil, err
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil, err
	}
	data := root.Obj("data")
	owner := data.Obj("owner")
	pubTime := data.Int64("pubdate")
	bvid := data.Str("bvid")
	cid := data.Str("cid")
	pages := make([]Page, 0, len(data.Arr("pages")))
	for _, page := range arrItems(data.Arr("pages")) {
		dimension := page.Obj("dimension")
		pages = append(pages, Page{
			Index:     page.Int("page"),
			Aid:       id,
			Cid:       page.Str("cid"),
			Epid:      "",
			Title:     strings.TrimSpace(page.Str("part")),
			Dur:       page.Int("duration"),
			Res:       fmt.Sprintf("%sx%s", dimension.Str("width"), dimension.Str("height")),
			PubTime:   pubTime,
			OwnerName: owner.Str("name"),
			OwnerMid:  owner.Str("mid"),
		})
	}
	isSteinGate := data.Obj("rights").Int("is_stein_gate") == 1
	if isSteinGate {
		var err error
		pages, err = f.appendSteinGatePages(ctx, pages, id, bvid, cid, pubTime, owner)
		if err != nil {
			return nil, err
		}
	}
	isBangumi := strings.Contains(data.Str("redirect_url"), "bangumi")
	if isBangumi && len(pages) == 1 {
		if match := reEp.FindStringSubmatch(data.Str("redirect_url")); len(match) > 1 {
			// long: 普通视频接口遇到版权番剧重定向时，单 P 下载需要带上 epid 才能走番剧播放接口拿到正确资源。
			pages[0].Epid = match[1]
		}
	}
	info := &VInfo{
		Title:       strings.TrimSpace(data.Str("title")),
		Desc:        strings.TrimSpace(data.Str("desc")),
		Pic:         data.Str("pic"),
		PubTime:     pubTime,
		PagesInfo:   pages,
		IsBangumi:   isBangumi,
		IsSteinGate: isSteinGate,
	}
	return info, nil
}

func (f *NormalInfoFetcher) appendSteinGatePages(ctx context.Context, pages []Page, aid, bvid, cid string, pubTime int64, owner J) ([]Page, error) {
	body, err := f.httpc.Get(ctx, fmt.Sprintf("https://api.bilibili.com/x/player.so?bvid=%s&id=cid:%s", url.QueryEscape(bvid), url.QueryEscape(cid)))
	if err != nil {
		return nil, err
	}
	graphVersion, ok := steinGateGraphVersion(body)
	if !ok {
		// long: 线上互动视频的老 player.so 偶尔只返回错误页，新 player/v2 仍会给剧情图版本；这里兜底后才能继续展开分支分 P。
		graphVersion, ok = f.fetchSteinGateGraphVersionV2(ctx, aid, cid)
		if !ok {
			return nil, fmt.Errorf("互动视频获取分P信息失败")
		}
	}
	edgeBody, err := f.httpc.Get(ctx, fmt.Sprintf("https://api.bilibili.com/x/stein/edgeinfo_v2?graph_version=%s&bvid=%s", url.QueryEscape(graphVersion), url.QueryEscape(bvid)))
	if err != nil {
		return nil, err
	}
	root, err := ParseJ(edgeBody)
	if err != nil {
		return nil, err
	}
	index := 2
	for _, question := range arrItems(root.Obj("data").Obj("edges").Arr("questions")) {
		for _, choice := range arrItems(question.Arr("choices")) {
			pages = append(pages, Page{
				Index:     index,
				Aid:       aid,
				Cid:       choice.Str("cid"),
				Title:     strings.TrimSpace(choice.Str("option")),
				PubTime:   pubTime,
				OwnerName: owner.Str("name"),
				OwnerMid:  owner.Str("mid"),
			})
			index++
		}
	}
	return pages, nil
}

func (f *NormalInfoFetcher) fetchSteinGateGraphVersionV2(ctx context.Context, aid, cid string) (string, bool) {
	body, err := f.httpc.Get(ctx, fmt.Sprintf("https://api.bilibili.com/x/player/v2?aid=%s&cid=%s", url.QueryEscape(aid), url.QueryEscape(cid)))
	if err != nil {
		return "", false
	}
	root, err := ParseJ(body)
	if err != nil {
		return "", false
	}
	version := root.Obj("data").Obj("interaction").Str("graph_version")
	return version, version != ""
}

func steinGateGraphVersion(body []byte) (string, bool) {
	decoder := xml.NewDecoder(strings.NewReader("<root>" + string(body) + "</root>"))
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", false
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "interaction" {
			continue
		}
		var raw string
		if err := decoder.DecodeElement(&raw, &start); err != nil {
			return "", false
		}
		state, err := ParseJ([]byte(raw))
		if err != nil {
			return "", false
		}
		version := state.Str("graph_version")
		return version, version != ""
	}
}

type BangumiInfoFetcher struct {
	httpc *HTTPClient
	cfg   *Config
}

func (f *BangumiInfoFetcher) Fetch(ctx context.Context, id string) (*VInfo, error) {
	epid := strings.TrimPrefix(id, "ep:")
	api := fmt.Sprintf("https://%s/pgc/view/web/season?ep_id=%s", f.cfg.EpHost, epid)
	body, err := f.httpc.Get(ctx, api)
	if err != nil {
		return nil, err
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil, err
	}
	result := root.Obj("result")
	episodes := arrItems(result.Arr("episodes"))
	title := strings.TrimSpace(result.Str("title"))
	if !containsEpisode(episodes, epid) {
		for _, section := range arrItems(result.Arr("section")) {
			sectionEpisodes := arrItems(section.Arr("episodes"))
			if containsEpisode(sectionEpisodes, epid) {
				title += "[" + section.Str("title") + "]"
				episodes = sectionEpisodes
				break
			}
		}
	}
	pages := make([]Page, 0, len(episodes))
	index := ""
	i := 1
	for _, page := range episodes {
		if page.Str("badge") == "预告" {
			continue
		}
		dimension := page.Obj("dimension")
		p := Page{
			Index:   i,
			Aid:     page.Str("aid"),
			Cid:     page.Str("cid"),
			Epid:    page.Str("id"),
			Title:   strings.TrimSpace(page.Str("title") + " " + page.Str("long_title")),
			Dur:     0,
			Res:     fmt.Sprintf("%sx%s", dimension.Str("width"), dimension.Str("height")),
			PubTime: page.Int64("pub_time"),
		}
		if p.Epid == epid {
			index = fmt.Sprintf("%d", p.Index)
		}
		pages = append(pages, p)
		i++
	}
	pubTime := parsePubTime(result.Obj("publish").Str("pub_time"))
	return &VInfo{
		Title:     title,
		Desc:      strings.TrimSpace(result.Str("evaluate")),
		Pic:       result.Str("cover"),
		PubTime:   pubTime,
		PagesInfo: pages,
		IsBangumi: true,
		IsCheese:  true,
		Index:     index,
	}, nil
}

func containsEpisode(episodes []J, epid string) bool {
	for _, page := range episodes {
		if page.Str("id") == epid || strings.Contains(page.Str("link"), "/ep"+epid) {
			return true
		}
	}
	return false
}

type CheeseInfoFetcher struct{ httpc *HTTPClient }

func (f *CheeseInfoFetcher) Fetch(ctx context.Context, id string) (*VInfo, error) {
	epid := strings.TrimPrefix(id, "cheese:")
	body, err := f.httpc.Get(ctx, "https://api.bilibili.com/pugv/view/web/season?ep_id="+epid)
	if err != nil {
		return nil, err
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil, err
	}
	data := root.Obj("data")
	up := data.Obj("up_info")
	index := ""
	pages := make([]Page, 0, len(data.Arr("episodes")))
	for _, page := range arrItems(data.Arr("episodes")) {
		p := Page{
			Index:     page.Int("index"),
			Aid:       page.Str("aid"),
			Cid:       page.Str("cid"),
			Epid:      page.Str("id"),
			Title:     strings.TrimSpace(page.Str("title")),
			Dur:       page.Int("duration"),
			PubTime:   page.Int64("release_date"),
			OwnerName: up.Str("uname"),
			OwnerMid:  up.Str("mid"),
		}
		if p.Epid == epid {
			index = fmt.Sprintf("%d", p.Index)
		}
		pages = append(pages, p)
	}
	pubTime := int64(0)
	if len(pages) > 0 {
		pubTime = pages[0].PubTime
	}
	return &VInfo{
		Title:     strings.TrimSpace(data.Str("title")),
		Desc:      strings.TrimSpace(data.Str("subtitle")),
		Pic:       data.Str("cover"),
		PubTime:   pubTime,
		PagesInfo: pages,
		IsBangumi: true,
		IsCheese:  true,
		Index:     index,
	}, nil
}

type IntlBangumiInfoFetcher struct {
	httpc *HTTPClient
	cfg   *Config
}

func (f *IntlBangumiInfoFetcher) Fetch(ctx context.Context, id string) (*VInfo, error) {
	epid := strings.TrimPrefix(id, "ep:")
	host := f.cfg.Host
	if host == "api.bilibili.com" {
		host = "api.bilibili.tv"
	}
	api := fmt.Sprintf("https://%s/intl/gateway/v2/ogv/view/app/season?ep_id=%s&platform=android&s_locale=zh_SG&mobi_app=bstar_a", host, epid)
	if f.cfg.Token != "" {
		// long: 国际版 APP 番剧接口只通过 access_key 识别登录态，缺失时会把会员或地区内容按匿名结果返回。
		api += "&access_key=" + url.QueryEscape(f.cfg.Token)
	}
	body, err := f.httpc.Get(ctx, api)
	if err != nil {
		return nil, err
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil, err
	}
	result := root.Obj("result")
	index := ""
	title := strings.TrimSpace(result.Str("title"))
	desc := strings.TrimSpace(result.Str("evaluate"))
	cover := result.Str("cover")
	if cover == "" {
		title, desc, cover = f.fetchIntlSeasonWebMetadata(ctx, result.Str("season_id"), title, desc, cover)
	}
	episodes := arrItems(result.Arr("episodes"))
	for _, module := range arrItems(result.Arr("modules")) {
		moduleEpisodes := arrItems(module.Obj("data").Arr("episodes"))
		if containsIntlModuleEpisode(module, moduleEpisodes, epid) {
			// long: 国际版会把番外、PV 等章节藏在 modules 里；命中目标 ep 后必须整组切换，Index 才能和原版保持一致。
			episodes = moduleEpisodes
			break
		}
	}
	pages := make([]Page, 0, len(episodes))
	i := 1
	for _, page := range episodes {
		if page.Str("badge") == "预告" {
			continue
		}
		dimension := page.Obj("dimension")
		p := Page{
			Index:   i,
			Aid:     page.Str("aid"),
			Cid:     page.Str("cid"),
			Epid:    page.Str("id"),
			Title:   strings.TrimSpace(page.Str("title") + " " + page.Str("long_title")),
			Res:     fmt.Sprintf("%sx%s", dimension.Str("width"), dimension.Str("height")),
			PubTime: page.Int64("pub_time"),
		}
		if p.Epid == epid {
			index = fmt.Sprintf("%d", p.Index)
		}
		pages = append(pages, p)
		i++
	}
	return &VInfo{
		Title:     title,
		Desc:      desc,
		Pic:       cover,
		PubTime:   parsePubTime(result.Obj("publish").Str("pub_time")),
		PagesInfo: pages,
		IsBangumi: true,
		IsCheese:  true,
		Index:     index,
	}, nil
}

func (f *IntlBangumiInfoFetcher) fetchIntlSeasonWebMetadata(ctx context.Context, seasonID, title, desc, cover string) (string, string, string) {
	if seasonID == "" {
		return title, desc, cover
	}
	web, err := f.httpc.Get(ctx, "https://bangumi.bilibili.com/anime/"+url.PathEscape(seasonID))
	if err != nil || len(web) == 0 {
		return title, desc, cover
	}
	matches := intlInitialStatePattern.FindSubmatch(web)
	if len(matches) < 2 {
		return title, desc, cover
	}
	state, err := ParseJ(matches[1])
	if err != nil {
		return title, desc, cover
	}
	media := state.Obj("mediaInfo")
	if v := strings.TrimSpace(media.Str("title")); v != "" {
		title = v
	}
	if v := strings.TrimSpace(media.Str("evaluate")); v != "" {
		desc = v
	}
	if v := media.Str("cover"); v != "" {
		// long: 国际版接口偶尔不给封面，原版会回退到番剧页的初始状态，避免最终文件元信息缺少封面。
		cover = v
	}
	return title, desc, cover
}

func containsIntlModuleEpisode(module J, episodes []J, epid string) bool {
	if containsEpisode(episodes, epid) {
		return true
	}
	raw, err := json.Marshal(module)
	if err != nil {
		return false
	}
	text := string(raw)
	return strings.Contains(text, "/"+epid) || strings.Contains(text, "/ep"+epid) || strings.Contains(text, "ep_id="+epid)
}

type MediaListFetcher struct{ httpc *HTTPClient }
type SeriesListFetcher struct{ httpc *HTTPClient }
type FavListFetcher struct{ httpc *HTTPClient }
type SpaceVideoFetcher struct {
	httpc *HTTPClient
	cfg   *Config
}

func (f *MediaListFetcher) Fetch(ctx context.Context, id string) (*VInfo, error) {
	bizID := strings.TrimPrefix(id, "listBizId:")
	info, err := fetchMediaList(ctx, f.httpc, bizID, 8, false)
	if err == nil {
		return info, nil
	}
	if !isMediaListInfoUnavailable(err) {
		return nil, err
	}
	// long: B 站有时会把系列链接误识别成合集，原版在合集信息为空时会按系列再解析一次。
	return fetchMediaList(ctx, f.httpc, bizID, 5, true)
}

func (f *SeriesListFetcher) Fetch(ctx context.Context, id string) (*VInfo, error) {
	return fetchMediaList(ctx, f.httpc, strings.TrimPrefix(id, "seriesBizId:"), 5, true)
}

func (f *FavListFetcher) Fetch(ctx context.Context, id string) (*VInfo, error) {
	id = strings.TrimPrefix(id, "favId:")
	parts := strings.Split(id, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("收藏夹参数无效")
	}
	favID, mid := parts[0], parts[1]
	if favID == "" {
		api := "https://api.bilibili.com/x/v3/fav/folder/created/list-all?up_mid=" + mid
		body, err := f.httpc.Get(ctx, api)
		if err != nil {
			return nil, err
		}
		root, err := ParseJ(body)
		if err != nil {
			return nil, err
		}
		list := root.Obj("data").Arr("list")
		if len(list) == 0 {
			return nil, fmt.Errorf("未找到默认收藏夹")
		}
		favID = asJ(list[0]).Str("id")
	}

	pageSize := 20
	api := fmt.Sprintf("https://api.bilibili.com/x/v3/fav/resource/list?media_id=%s&pn=1&ps=%d&order=mtime&type=2&tid=0&platform=web", favID, pageSize)
	body, err := f.httpc.Get(ctx, api)
	if err != nil {
		return nil, err
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil, err
	}
	data := root.Obj("data")
	infoNode := data.Obj("info")
	totalCount := infoNode.Int("media_count")
	totalPage := (totalCount + pageSize - 1) / pageSize
	medias := arrItems(data.Arr("medias"))
	for page := 2; page <= totalPage; page++ {
		api = fmt.Sprintf("https://api.bilibili.com/x/v3/fav/resource/list?media_id=%s&pn=%d&ps=%d&order=mtime&type=2&tid=0&platform=web", favID, page, pageSize)
		body, err := f.httpc.Get(ctx, api)
		if err != nil {
			return nil, err
		}
		root, err := ParseJ(body)
		if err != nil {
			return nil, err
		}
		medias = append(medias, arrItems(root.Obj("data").Arr("medias"))...)
	}

	pages := make([]Page, 0, len(medias))
	index := 1
	for _, m := range medias {
		if m.Int("attr") != 0 {
			continue
		}
		pageCount := m.Int("page")
		if pageCount > 1 {
			tmpInfo, err := (&NormalInfoFetcher{httpc: f.httpc}).Fetch(ctx, m.Str("id"))
			if err == nil {
				for _, item := range tmpInfo.PagesInfo {
					item.Index = index
					item.Title = fmt.Sprintf("%s_P%d_%s", m.Str("title"), item.Index, item.Title)
					item.Cover = tmpInfo.Pic
					item.Desc = m.Str("intro")
					if appendUniquePage(&pages, item) {
						index++
					}
				}
				continue
			}
		}
		ugc := m.Obj("ugc")
		upper := m.Obj("upper")
		if appendUniquePage(&pages, Page{
			Index:     index,
			Aid:       m.Str("id"),
			Cid:       ugc.Str("first_cid"),
			Title:     m.Str("title"),
			Dur:       m.Int("duration"),
			PubTime:   m.Int64("pubtime"),
			Cover:     m.Str("cover"),
			Desc:      m.Str("intro"),
			OwnerName: upper.Str("name"),
			OwnerMid:  upper.Str("mid"),
		}) {
			index++
		}
	}
	return &VInfo{
		Title:     strings.TrimSpace(infoNode.Str("title")),
		Desc:      strings.TrimSpace(infoNode.Str("intro")),
		PubTime:   infoNode.Int64("ctime"),
		PagesInfo: pages,
	}, nil
}

func (f *SpaceVideoFetcher) Fetch(ctx context.Context, id string) (*VInfo, error) {
	mid := strings.TrimPrefix(id, "mid:")
	userInfoAPI := "https://api.live.bilibili.com/live_user/v1/Master/info?uid=" + mid
	body, err := f.httpc.Get(ctx, userInfoAPI)
	if err != nil {
		return nil, err
	}
	root, err := parseJSONForAPI("获取空间用户信息失败", body)
	if err != nil {
		return nil, err
	}
	userName := sanitizeFilenameWithReplacement(root.Obj("data").Obj("info").Str("uname"), ".", false)
	if userName == "" {
		userName = "用户" + mid
	}
	pageSize := 50
	pageNumber := 1
	totalPage := 1
	forceLegacyArcSearch := false
	var urls []string
	var pages []Page
	index := 1
	for {
		data, usedLegacy, err := f.fetchSpaceArcPage(ctx, mid, pageNumber, pageSize, forceLegacyArcSearch)
		if err != nil {
			return nil, err
		}
		if pageNumber == 1 {
			forceLegacyArcSearch = usedLegacy
			count := data.Obj("page").Int("count")
			totalPage = (count + pageSize - 1) / pageSize
		}
		for _, item := range arrItems(data.Obj("list").Arr("vlist")) {
			aid := item.Str("aid")
			urls = append(urls, "https://www.bilibili.com/video/av"+aid)
			itemPages, err := f.pagesFromSpaceVideo(ctx, item, index)
			if err != nil {
				return nil, err
			}
			for _, page := range itemPages {
				if appendUniquePage(&pages, page) {
					index++
				}
			}
		}
		if pageNumber >= totalPage || totalPage == 0 {
			break
		}
		pageNumber++
	}
	fileName := fmt.Sprintf("%s的投稿视频.txt", userName)
	if err := os.WriteFile(fileName, []byte(strings.Join(urls, "\n")), 0o644); err != nil {
		return nil, err
	}
	// long: 原版到这里会停止并提示用户用脚本批量调用；Go 版直接返回多页列表，让现有下载链路复用清晰度选择、归档和分 P 延迟。
	Logf("已获取该用户的全部投稿视频地址并保存至：%s", fileName)
	return &VInfo{
		Title:     userName + "的投稿视频",
		PagesInfo: pages,
	}, nil
}

func (f *SpaceVideoFetcher) fetchSpaceArcPage(ctx context.Context, mid string, pageNumber, pageSize int, forceLegacy bool) (J, bool, error) {
	if forceLegacy {
		data, err := f.fetchLegacySpaceArcPage(ctx, mid, pageNumber, pageSize)
		return data, true, err
	}

	data, err := f.fetchWbiSpaceArcPage(ctx, mid, pageNumber, pageSize)
	if err != nil {
		legacyData, legacyErr := f.fetchLegacySpaceArcPage(ctx, mid, pageNumber, pageSize)
		if legacyErr == nil {
			Warnf("空间 WBI 投稿列表失败，已回退旧接口: %v", err)
			return legacyData, true, nil
		}
		return nil, false, fmt.Errorf("%v; 旧接口回退也失败: %w", err, legacyErr)
	}

	if pageNumber == 1 {
		vlistLen := len(data.Obj("list").Arr("vlist"))
		count := data.Obj("page").Int("count")
		if vlistLen == pageSize && count <= vlistLen {
			legacyData, legacyErr := f.fetchLegacySpaceArcPage(ctx, mid, pageNumber, pageSize)
			if legacyErr == nil && legacyData.Obj("page").Int("count") > count {
				// long: 线上 WBI 空间投稿接口在未登录环境可能只暴露首屏数量；旧接口若能返回更大的总数，就用旧接口完成后续分页导出。
				Warnf("空间 WBI 投稿总数疑似被截断，已回退旧接口")
				return legacyData, true, nil
			}
		}
	}
	return data, false, nil
}

func (f *SpaceVideoFetcher) fetchWbiSpaceArcPage(ctx context.Context, mid string, pageNumber, pageSize int) (J, error) {
	api := WbiSign(fmt.Sprintf("mid=%s&order=pubdate&pn=%d&ps=%d&tid=0&wts=%s", mid, pageNumber, pageSize, GetTimeStamp(true)), f.cfg)
	body, err := f.httpc.Get(ctx, "https://api.bilibili.com/x/space/wbi/arc/search?"+api)
	if err != nil {
		return nil, err
	}
	root, err := parseJSONForAPI("获取空间投稿列表失败", body)
	if err != nil {
		return nil, err
	}
	return root.Obj("data"), nil
}

func (f *SpaceVideoFetcher) fetchLegacySpaceArcPage(ctx context.Context, mid string, pageNumber, pageSize int) (J, error) {
	api := fmt.Sprintf("https://api.bilibili.com/x/space/arc/search?mid=%s&pn=%d&ps=%d&order=pubdate&jsonp=jsonp", mid, pageNumber, pageSize)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			// long: 旧空间投稿接口比 WBI 更少触发 HTML 风控，但连续翻页时可能返回 -799；短退避后通常可以继续导出后续页。
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		body, err := f.httpc.Get(ctx, api)
		if err != nil {
			return nil, err
		}
		root, err := parseJSONForAPI("获取空间投稿列表失败", body)
		if err == nil {
			return root.Obj("data"), nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "code=-799") {
			break
		}
	}
	return nil, lastErr
}

func (f *SpaceVideoFetcher) pagesFromSpaceVideo(ctx context.Context, item J, startIndex int) ([]Page, error) {
	aid := item.Str("aid")
	pageCount := item.Int("videos")
	if pageCount > 1 {
		info, err := (&NormalInfoFetcher{httpc: f.httpc}).Fetch(ctx, aid)
		if err == nil && len(info.PagesInfo) > 0 {
			pages := make([]Page, 0, len(info.PagesInfo))
			for _, page := range info.PagesInfo {
				page.Index = startIndex + len(pages)
				page.Title = fmt.Sprintf("%s_P%d_%s", item.Str("title"), len(pages)+1, page.Title)
				if page.Cover == "" {
					page.Cover = firstNonEmptyString(info.Pic, item.Str("pic"))
				}
				if page.Desc == "" {
					page.Desc = firstNonEmptyString(info.Desc, item.Str("description"))
				}
				pages = append(pages, page)
			}
			return pages, nil
		}
		if err != nil {
			Warnf("投稿 %s 分 P 详情获取失败，将按空间列表中的首 P 信息处理: %v", aid, err)
		}
	}
	return []Page{{
		Index:     startIndex,
		Aid:       aid,
		Cid:       firstNonEmptyString(item.Str("first_cid"), item.Str("cid")),
		Title:     strings.TrimSpace(item.Str("title")),
		Dur:       parseDurationSeconds(item.Str("length")),
		PubTime:   item.Int64("created"),
		Cover:     item.Str("pic"),
		Desc:      item.Str("description"),
		OwnerName: item.Str("author"),
		OwnerMid:  item.Str("mid"),
	}}, nil
}

func parseJSONForAPI(prefix string, body []byte) (J, error) {
	root, err := ParseJ(body)
	if err == nil {
		if _, ok := root["code"]; ok && root.Int("code") != 0 {
			return nil, fmt.Errorf("%s: 接口返回错误 code=%s message=%q", prefix, root.Str("code"), root.Str("message"))
		}
		return root, nil
	}
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 120 {
		snippet = snippet[:120]
	}
	return nil, fmt.Errorf("%s: 接口未返回合法JSON: %v; 响应片段: %q", prefix, err, snippet)
}

func parseDurationSeconds(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	parts := strings.Split(raw, ":")
	total := 0
	for _, part := range parts {
		var value int
		if _, err := fmt.Sscan(strings.TrimSpace(part), &value); err != nil {
			return 0
		}
		total = total*60 + value
	}
	return total
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fetchMediaList(ctx context.Context, httpc *HTTPClient, bizID string, listType int, desc bool) (*VInfo, error) {
	api := fmt.Sprintf("https://api.bilibili.com/x/v1/medialist/info?type=%d&biz_id=%s&tid=0", listType, bizID)
	body, err := httpc.Get(ctx, api)
	if err != nil {
		return nil, err
	}
	root, err := ParseJ(body)
	if err != nil {
		return nil, err
	}
	data := root.Obj("data")
	if data == nil {
		return nil, newMediaListAPIError("获取合集信息失败", root)
	}
	info := &VInfo{
		Title:   strings.TrimSpace(data.Str("title")),
		Desc:    strings.TrimSpace(data.Str("intro")),
		PubTime: data.Int64("ctime"),
	}
	var pages []Page
	hasMore := true
	oid := ""
	index := 1
	for hasMore {
		listAPI := fmt.Sprintf("https://api.bilibili.com/x/v2/medialist/resource/list?type=%d&oid=%s&otype=2&biz_id=%s&with_current=true&mobi_app=web&ps=20&direction=false&sort_field=1&tid=0&desc=%t", listType, oid, bizID, desc)
		listBody, err := httpc.Get(ctx, listAPI)
		if err != nil {
			return nil, err
		}
		listRoot, err := ParseJ(listBody)
		if err != nil {
			return nil, err
		}
		data = listRoot.Obj("data")
		if data == nil {
			return nil, newMediaListAPIError("获取合集视频列表失败", listRoot)
		}
		hasMore = data.Bool("has_more")
		for _, media := range arrItems(data.Arr("media_list")) {
			if media.Int("attr") != 0 {
				continue
			}
			pageCount := media.Int("page")
			upper := media.Obj("upper")
			for _, page := range arrItems(media.Arr("pages")) {
				dimension := page.Obj("dimension")
				title := media.Str("title")
				if pageCount > 1 {
					title = fmt.Sprintf("%s_P%s_%s", media.Str("title"), page.Str("page"), page.Str("title"))
				}
				p := Page{
					Index:     index,
					Aid:       media.Str("id"),
					Cid:       page.Str("id"),
					Title:     title,
					Dur:       page.Int("duration"),
					Res:       fmt.Sprintf("%sx%s", dimension.Str("width"), dimension.Str("height")),
					PubTime:   media.Int64("pubtime"),
					Cover:     media.Str("cover"),
					Desc:      media.Str("intro"),
					OwnerName: upper.Str("name"),
					OwnerMid:  upper.Str("mid"),
				}
				if appendUniquePage(&pages, p) {
					index++
				}
			}
			oid = media.Str("id")
		}
	}
	info.PagesInfo = pages
	return info, nil
}

func isMediaListInfoUnavailable(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "获取合集信息失败")
}

func newMediaListAPIError(prefix string, root J) error {
	code := root.Int("code")
	message := root.Str("message")
	if message == "" {
		message = "未知错误"
	}
	return fmt.Errorf("%s(code=%d): %s", prefix, code, message)
}

func appendUniquePage(pages *[]Page, page Page) bool {
	for _, existing := range *pages {
		if existing.Aid == page.Aid && existing.Cid == page.Cid && existing.Epid == page.Epid {
			return false
		}
	}
	*pages = append(*pages, page)
	return true
}

func parsePubTime(raw string) int64 {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local)
	if err != nil {
		return 0
	}
	return t.Unix()
}
