package bbdown

const Version = "1.0.12"

var BuildTime = "unknown"

func DefaultMyOption() MyOption {
	return MyOption{
		MultiThread:      true,
		ForceHttp:        true,
		SkipAi:           true,
		ForceReplaceHost: true,
		FileExistsAction: FileExistsActionSkip,
		DelayPerPage:     "0",
		Host:             "api.bilibili.com",
		EpHost:           "api.bilibili.com",
		TvHost:           "api.snm0516.aisee.tv",
	}
}

type MyOption struct {
	Url                    string
	UseTvApi               bool
	UseAppApi              bool
	UseIntlApi             bool
	UseMP4box              bool
	EncodingPriority       string
	DfnPriority            string
	OnlyShowInfo           bool
	ShowAll                bool
	UseAria2c              bool
	Aria2cProxy            string
	Interactive            bool
	HideStreams            bool
	MultiThread            bool
	SimplyMux              bool
	VideoOnly              bool
	AudioOnly              bool
	DanmakuOnly            bool
	CoverOnly              bool
	SubOnly                bool
	Debug                  bool
	SkipMux                bool
	SkipSubtitle           bool
	SkipCover              bool
	ForceHttp              bool
	DownloadDanmaku        bool
	DownloadDanmakuFormats string
	SkipAi                 bool
	VideoAscending         bool
	AudioAscending         bool
	AllowPcdn              bool
	ForceReplaceHost       bool
	SaveArchivesToFile     bool
	OnlyHevc               bool
	OnlyAvc                bool
	OnlyAv1                bool
	AddDfnSubfix           bool
	NoPaddingPageNum       bool
	BandwithAscending      bool
	FileExistsAction       string
	FilePattern            string
	MultiFilePattern       string
	SelectPage             string
	VideoIndex             string
	AudioIndex             string
	Language               string
	UserAgent              string
	Cookie                 string
	AccessToken            string
	Aria2cArgs             string
	WorkDir                string
	FFmpegPath             string
	Mp4boxPath             string
	Aria2cPath             string
	UposHost               string
	DelayPerPage           string
	Host                   string
	EpHost                 string
	TvHost                 string
	Area                   string
	ConfigFile             string
	Version                bool
	Help                   bool
}

type Page struct {
	Index     int
	Aid       string
	Cid       string
	Epid      string
	Title     string
	Dur       int
	Res       string
	PubTime   int64
	Cover     string
	Desc      string
	OwnerName string
	OwnerMid  string
}

type Video struct {
	ID        string
	Dfn       string
	BaseURL   string
	Res       string
	Fps       string
	Codecs    string
	Bandwidth int64
	Dur       int
	Size      float64
}

type Audio struct {
	ID        string
	Dfn       string
	BaseURL   string
	Codecs    string
	Bandwidth int64
	Dur       int
}

type VInfo struct {
	Title        string
	Desc         string
	Pic          string
	PubTime      int64
	IsBangumi    bool
	IsCheese     bool
	IsBangumiEnd bool
	Index        string
	PagesInfo    []Page
	IsSteinGate  bool
}

type ParsedResult struct {
	WebJSONString    string
	VideoTracks      []Video
	AudioTracks      []Audio
	BackgroundAudios []Audio
	RoleAudioLists   []AudioMaterialInfo
	ExtraPoints      []ViewPoint
	Clips            []string
	Dfns             []string
}

type ViewPoint struct {
	Title string
	Start int
	End   int
}

type AudioMaterialInfo struct {
	Title      string
	PersonName string
	Path       string
	Audio      []Audio
}

type AudioMaterial struct {
	Title      string
	PersonName string
	Path       string
}
