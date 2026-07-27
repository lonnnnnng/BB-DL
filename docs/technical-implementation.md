# BB-DL 技术实现路线

更新时间：2026-07-27 10:09:10 北京时间

本文记录 Go 复刻 BBDown 的当前实现结构、主流程和后续技术路线。目标不是重新设计一个下载器，而是在 Go 单二进制形态下尽量对齐原版 BBDown 的输入、参数、解析结果、下载行为和服务接口。

## 设计目标

| 目标 | 当前策略 |
| --- | --- |
| 命令行兼容 | 参数命名、短参数、旧参数警告和配置合并尽量保持原版习惯 |
| 单二进制交付 | Go 负责解析、下载调度和服务模式；混流仍调用 `ffmpeg` / `MP4Box` |
| 高波动接口可维护 | WEB、TV、APP、INTL、BiliPlus 分通道实现，回归样本单独维护 |
| 行为优先于抽象 | 先复刻原版可见行为，再逐步收敛内部模块边界 |
| 可回归 | 单测覆盖签名、URL 解析、轨道解析、文件命名、服务进度；真实样本写入 `docs/regression-samples.md` |

## 当前实现分层

| 层级 | 主要职责 | 代表代码 |
| --- | --- | --- |
| CLI 层 | 参数注册、配置合并、命令分发、交互选择、主下载循环 | `cmd/bbdown/main.go` |
| 领域模型层 | `MyOption`、`VInfo`、`Page`、`Video`、`Audio`、`ParsedTracks` 等内部数据结构 | `internal/bbdown/models.go` |
| 信息抓取层 | 把 URL / ID 转成视频、番剧、课程、收藏夹、合集、系列、空间投稿等统一页面列表 | `internal/bbdown/url.go`、`internal/bbdown/fetchers.go` |
| 播放流层 | WEB / TV / APP / INTL / BiliPlus 播放地址请求、签名和 DASH / FLV 解析 | `internal/bbdown/parser.go`、`internal/bbdown/appgrpc.go`、`internal/bbdown/subgrpc.go` |
| 资源处理层 | 多线程下载、aria2c、UPOS/PCDN 替换、字幕、弹幕、封面、混流 | `internal/bbdown/download.go`、`internal/bbdown/subtitle.go`、`internal/bbdown/danmaku.go` |
| 运行文件层 | 配置文件、登录文件、归档文件、外部二进制查找 | `internal/bbdown/config_file.go`、`internal/bbdown/login.go`、`internal/bbdown/archive.go`、`internal/bbdown/binaries.go` |
| 服务层 | HTTP API、任务队列、子进程调度、进度和传输统计、回调 | `internal/bbdown/serve.go` |
| 桌面层 | Wails 2 Go 后端 + React/TypeScript 前端、helper 调度、任务队列、事件流、二维码、速度/耗时和完成文件管理 | `main.go`、`app.go`、`task_manager.go`、`frontend/src/` |
| 发布层 | CLI 跨平台打包、桌面版目标系统原生打包和 GitHub Release 自动发布 | `scripts/build-release.sh`、`scripts/build-desktop.sh`、`.github/workflows/release.yml` |

## 模块地图

| 路径 | 职责 |
| --- | --- |
| `cmd/bbdown/main.go` | CLI 入口、参数注册、配置合并、下载主流程、交互选择 |
| `internal/bbdown/models.go` | 版本、运行参数、视频/音频/页面等核心模型 |
| `internal/bbdown/config.go` | 运行期全局认证、host、area、WBI salt |
| `internal/bbdown/config_file.go` | `BBDown.config` 解析和命令行优先合并 |
| `internal/bbdown/http.go` | HTTP 客户端、UA、Cookie、Referer、GET/POST 压缩响应处理 |
| `internal/bbdown/url.go` | AV/BV/EP/SS/MD/课程/空间/合集/短链解析 |
| `internal/bbdown/fetchers.go` | 普通视频、番剧、课程、收藏夹、合集、系列、空间投稿信息抓取 |
| `internal/bbdown/parser.go` | WEB/TV/APP/INTL 播放地址请求、签名、DASH/FLV 分流 |
| `internal/bbdown/download.go` | 保存路径、资源下载、多线程/aria2c、UPOS/PCDN/force-http、混流辅助 |
| `internal/bbdown/subtitle.go` | 字幕抓取、AI 字幕过滤、SRT 保存 |
| `internal/bbdown/danmaku.go` | 弹幕 XML 解析和 ASS 生成 |
| `internal/bbdown/login.go` | WEB/TV 二维码登录、认证文件读写和登录成功日志脱敏 |
| `internal/bbdown/binaries.go` | `ffmpeg`、`MP4Box`、`aria2c` 查找 |
| `internal/bbdown/update.go` | `BB-DL` GitHub latest release 跳转解析和语义版本比较，避免 Go 复刻版独立版本线被原版 BBDown 历史 release 干扰 |
| `internal/bbdown/serve.go` | HTTP 服务模式、任务队列、子进程调度、进度、传输统计和失败错误摘要 |
| `scripts/build-release.sh` | 跨平台 release 打包 |
| `.github/workflows/release.yml` | tag 触发测试、打包、GitHub Release 发布 |
| `main.go` / `app.go` | Wails 桌面入口和前端可调用绑定，负责窗口生命周期、退出确认、目录选择与任务 API 门面 |
| `task_manager.go` | 桌面任务队列、helper 子进程、日志/传输事件、自动续队和 Wails 事件广播 |
| `desktop_core.go` / `desktop_tools.go` | 桌面 DTO、参数构建、历史/偏好持久化、进度协议、工具探测和系统文件操作 |
| `frontend/src/` | React 19 + TypeScript 工作台，包含创建区、任务队列、详情、日志、文件和设置抽屉 |
| `cmd/bbdown-desktop/main.go` | 旧 Fyne 桌面入口，仅保留作历史行为参考和兼容回归，不进入当前 Release 构建 |
| `scripts/build-desktop.sh` | 当前平台 Wails 打包，macOS 输出 `.app`，Windows/Linux 输出桌面程序与 helper；系统 WebView 依赖由目标 runner 原生提供 |

## 数据流与边界

核心数据从外到内的转换顺序：

```text
用户输入 URL / ID / 参数
  -> MyOption
  -> ResolveAvid 标准化内部 ID
  -> VInfo + []Page
  -> ParsedTracks
  -> 文件路径和临时路径
  -> 下载产物 / 混流产物 / 服务任务状态
```

关键边界：

| 边界 | 规则 |
| --- | --- |
| 参数边界 | 所有 CLI、配置文件和服务 JSON 最终归一到 `MyOption`，避免多套下载配置 |
| 信息边界 | `Fetcher` 只负责产出 `VInfo` 和 `Page`，不选择清晰度和下载资源 |
| 播放流边界 | `ExtractTracks` 只产出轨道和章节等播放材料，排序和裁剪由调用层按参数处理 |
| 下载边界 | `DownloadResource` 不理解业务类型，只处理 URL、目标文件、多线程、aria2c 和 host 策略 |
| 混流边界 | `MuxAVWithOptions` 接收明确文件路径和 metadata，避免从接口 JSON 重新推导业务含义 |
| 服务边界 | `serve` 复用当前二进制子进程，不复制下载逻辑；父进程只解析日志和文件状态 |

## 主调用链

默认下载流程：

```text
main
  -> runDownload
    -> MergeConfigArgs / normalizeOptions
    -> ResolveRequiredBinaries
    -> ResolveAvid
    -> GetVideoInfo
      -> FetcherFactory.CreateFetcher
      -> NormalInfoFetcher / BangumiInfoFetcher / CheeseInfoFetcher / ...
    -> GetSelectedPages
    -> ExtractTracks
      -> getPlayJSON
        -> WEB WBI / legacy WEB fallback / TV / APP gRPC+REST / INTL / BiliPlus
      -> parseDashLike or parseFLVLike
    -> SortTracksVideoWithPriorityOrder / SortTracksAudioWithPriority
    -> downloadSubtitles / DownloadResource / SaveDanmakuASS / downloadCoverForMux
    -> MuxAVWithOptions or MergeFLV
    -> SaveAidToArchive
```

`info` 子命令复用解析链路，但只展示视频信息和可用流，不进入资源下载和混流。

`serve` 模式流程：

```text
serve
  -> NewApiServer.Run
    -> POST /add-task
      -> ResolveAvid 去重
      -> spawnCurrentBinaryAsChild
      -> 子进程执行默认下载命令
      -> 父进程解析 stdout/stderr 推进 Progress
      -> 优先消费子进程下载增量/完成事件，文件轮询兜底估算 DownloadSpeed / TotalDownloadedBytes
      -> 任务完成后移动 running -> finished
      -> 可选 CallBackWebHook
```

服务模式当前选择复用 CLI 子进程，优点是下载行为与默认命令天然一致；代价是进度和传输统计仍通过日志、内部事件和文件轮询估算，精度低于把下载循环完全搬入服务进程。父进程在预测 `SavePaths`、选轨和启动子进程前，会先静默套用与 CLI 相同的旧参数兼容归一化，确保 `--add-dfn-subfix`、`--no-padding-page-num`、`--only-hevc`、`--bandwith-ascending` 等参数不会出现“父进程预测旧路径、子进程生成新路径”的分叉。当前子进程会在 HTTP 下载写入过程中输出仅父进程消费的隐藏增量传输事件，并在每个资源下载完成后继续输出完成事件，覆盖弹幕 XML、封面、视频、音频、背景/角色音频、FLV 分段和字幕输出；父进程解析后按事件时间刷新 `DownloadSpeed`，并按路径去重累计 `TotalDownloadedBytes`，不会把事件行转发到用户日志。文件轮询仍作为兜底：服务端会在子进程启动前记录预测产物和下载候选文件的大小基线，再按 `.video.mp4`、`.audio.m4a`、`.clip*.mp4`、`.vclip`、`.aclip` 等下载文件相对基线的增长量累计；如果已经观察到下载增量/完成事件或下载文件，最终 mux 输出不会再被计入网络下载量，避免重复统计本地写盘。资源 only 和 AudioOnly/VideoOnly 单轨任务成功结束时，也只用最终输出文件相对基线的增长量校正短任务漏计，避免把任务启动前已存在的跳过文件算成本次下载。DASH `--skip-mux` 服务任务的 `SavePaths` 会指向 CLI 实际保留的 `.video.mp4` / `.audio.m4a`，FLV `--skip-mux` 服务任务的 `SavePaths` 会指向 `.flv-merged.mp4`；`TotalDownloadedBytes` 仍按下载增长量估算，不把本地混流、FLV 合并后的容器体积或既有输出文件当作网络下载量。

桌面版沿用同一套隐藏传输事件，但不启动 HTTP 服务；`TaskManager` 直接运行 helper 子进程，优先消费 `__BBDOWN_GO_SERVER_TRANSFER__` 事件并按资源路径累计增长量，再通过 Wails 事件刷新当前速度、平均速度、耗时和任务列表。如果使用 `aria2c` 或其它外部下载路径导致没有隐藏事件，后端会按保存目录启动前快照轮询下载相关文件的增长量作为兜底，统计范围限定在媒体、封面、字幕、弹幕和临时分片扩展名，并在后续出现隐藏事件时停止叠加兜底字节，避免重复计数或把同目录说明文件变化算作下载。任务完成后，后端对保存目录做启动前后快照对比，生成“完成文件”列表；列表只保留可直接打开、定位或复制路径的产物，过滤临时分片、aria2 控制文件、二维码和 `BBDown.*` 运行文件。打开保存目录会先创建目标目录再交给系统文件管理器，避免首次使用时目录尚未生成导致系统调用失败。这个边界保证桌面版不复制解析/下载逻辑，也能在下载过程中给用户持续反馈。

桌面布局由 React 组件按职责拆成 `AppHeader`、`CreatePanel`、`TaskQueue`、`TaskDetails`、`SettingsDrawer`、确认对话框和底部状态栏。新建任务区使用原生 `details` 渐进展示下载设置与额外参数；任务队列把开始/停止等主操作与删除/批量清理等危险操作分开，并在执行删除前显示确认对话框；任务详情独立维护状态、百分比、字节、速度和耗时，不复用全局操作反馈。任务项固定显示标题、语义状态、字节/速度和细进度条，浅色/深色主题通过 CSS 变量统一控制。

桌面日志采用 React 只读等宽日志列表。stdout/stderr 到达后立即写入后端完整日志和进度解析器，普通行通过 `task:log` 增量发送；任务真正启动时用 `task:log-reset` 清理旧执行日志，停止/失败/成功的终态行在任务终态事件前发送，避免前端漏掉最后一行。前端提供筛选和跟随末尾，只渲染匹配结果的最近 4000 行；历史裁剪和复制日志仍使用后端完整文本。

Wails 状态同步采用“单任务事件 + 必要时全队列重置”。`task:added`、`task:updated`、`task:finished` 用于局部更新；开始、启动失败或结束会额外广播 `tasks:reset`，因为 `CanStart`、`CanRetry` 等操作权限同时取决于全局是否已有运行任务。批量重试只追加失败/停止任务副本，自动启动统一调用 `StartNextPending`，确保先执行原本已等待的任务。启动前预检失败也会完成日志、终态、队列摘要和状态广播，再按自动队列继续下一条。

WEB/TV 登录使用独立 `LoginSessionDTO` 和 helper 子进程，不分配下载任务 ID、不写入 `tasks.json`，也不改变任务汇总或自动队列。前端通过 `login:updated`、`login:qrcode`、`login:finished` 在专用对话框中展示扫码状态；同一时间只允许一个登录会话，关闭进行中的对话框会停止该登录进程。启动时会过滤旧 Fyne/Wails 开发版历史中的 `Channel="账号"` 任务并回写历史，避免迁移后继续污染下载列表。

桌面客户端状态持久化写入用户配置目录的 `tasks.json`，只保存下载任务参数、状态、日志文本、完成文件列表、字节数、进度和时间戳；账号登录是临时会话，不进入该文件。任务创建、启动、停止、完成、删除等关键状态会立即落盘；运行中的任务通过 5 秒节流刷新历史，避免每行日志都触发磁盘写入，同时减少异常退出时丢失最近日志和进度的窗口。历史保存阶段会裁剪为最近 200 个任务，并把单任务日志限制为最后 256 KiB；裁剪只影响落盘历史，不修改当前窗口内存中的实时日志。`exec.Cmd`、下载基线、速度窗口和 transfer 去重表都属于当前进程运行态，不进入历史文件；运行中或停止中的任务在保存时会降级成“已停止”，重启后可以查看日志、打开文件或重新执行，但不会误导用户继续控制已经退出的 helper 进程。历史文件可能来自早期开发版，缺少完整 `Args`；启动任务和复制命令前会用 `rebuildDesktopTaskArgs` 从 URL、模式、接口、分 P、清晰度、编码、音视频序号、工具路径和额外参数重建 CLI 参数，避免旧任务空参数调用 helper。

桌面排障操作不引入新的下载逻辑：复制命令只把任务 `Args` 重新拼成可复现命令，helper 优先选择任务运行目录或应用包内的绝对路径，找不到时再回退到当前平台 helper 名称；macOS/Linux 使用单引号 shell quoting，Windows 使用双引号和 `CommandLineToArgvW` 兼容的反斜杠规则，避免带空格路径、文件模板元字符或尾部反斜杠在复制后被终端误解析。复制日志由后端返回完整文本，再通过 Wails Clipboard API 写入系统剪贴板。这样失败任务可以在终端复现，日志也能直接反馈，同时不会额外保存 cookie/token 以外的新状态。

桌面端的任务预检和表单回填会解析任务 `Args` 中的布尔开关，用于判断是否需要 FFmpeg / MP4Box / aria2c，以及是否恢复下载弹幕、跳过字幕、跳过封面、跳过混流和 aria2c 复选框。解析规则与 CLI 保持一致，支持 `--flag`、`--flag=true/false` 和 `--flag true/false`，并对 `--aria2` / `--use-aria2c` 这类别名按命令行出现顺序取最后一次有效值，避免用户在“额外参数”里显式关闭某项后桌面预检仍误判为开启。额外参数解析会保留 `""` / `''` 生成的空字符串值，避免 `--download-danmaku-formats ""` 这类显式空值被丢弃后把 URL 误吞为选项值；反斜杠只在转义空白、引号或反斜杠本身时消费，其它场景按字面保留，避免 Windows 路径被解析成 `C:ffmpegbin...`。

## 参数兼容路线

当前策略是“保留用户可见习惯，内部统一新语义”：

| 类型 | 当前实现 |
| --- | --- |
| 新旧命令 | 默认命令负责下载；`info` 是 Go 版额外便利命令；`--only-show-info` 保持原版式参数 |
| 短参数 | 保留 `-p`、`-F`、`-M`、`-q`、`-e`、`-c` 等常用短参数 |
| 布尔参数 | 命令行和配置文件均支持 `--flag false` / `--flag=false` 风格 |
| 帮助文本 | `-?`、`-h`、`--help` 统一输出中文参数说明，默认下载、`info` 和 `serve` 共用相同帮助注册策略 |
| 旧编码参数 | `--only-hevc`、`--only-avc`、`--only-av1` 转成 `--encoding-priority` |
| 旧命名参数 | `--add-dfn-subfix`、`--no-padding-page-num` 转成文件名模板 |
| 旧排序参数 | `--bandwith-ascending` 转成视频和音频升序 |
| 旧 aria2 代理参数 | `--aria2c-proxy` 转成 `--aria2c-args --all-proxy=...` |

2026-06-18 已对照源仓库 `BBDown/CommandLineInvoker.cs` 完成参数注册审计：原版暴露 78 个用户可见参数名，Go 版 `newDownloadFlagSet` 全量覆盖；新增 `TestDownloadFlagSetCoversOriginalBBDownFlags` 固化这份清单，防止后续整理 CLI 或配置合并逻辑时漏掉旧参数。Go 版额外参数边界保持清晰：下载命令额外提供 `?`、`h`、`help`、`version`、`--video-index` 和 `--audio-index`；其中 `--video-index` / `--audio-index` 是为脚本和桌面端补足非交互式选流能力的扩展参数，服务模式额外提供 `serve --listen/-l`。

2026-06-27 新增非交互式选流参数。`MyOption` 使用字符串保存 `VideoIndex` / `AudioIndex`，以区分“留空自动选择”和“显式选择 0 号流”；CLI 在打印和排序可用流后通过 `internal/bbdown.SelectedTrackIndexes` 解析显式序号，服务父进程预测 `SavePaths` 也复用同一 helper，越界或非数字直接失败，避免桌面任务静默下载错轨道或服务结果暴露不会生成的路径。配置文件、服务 JSON 子进程参数和桌面端表单共用同一字段，下载命令以外的 `info` / “仅查看”不追加这两个下载选择参数。`internal/bbdown.NormalizeTrackIndexOptions` 统一清理当前模式不会使用的序号：仅查看清掉视频和音频序号，仅音频清掉视频序号，仅视频清掉音频序号；桌面表单归一化和服务子进程参数生成都复用这段逻辑，避免被忽略的坏序号进入任务历史、复制命令或子进程日志。互斥 only 参数是例外：`AudioOnly && VideoOnly` 会先经 `normalizeOptionsForCompatibility` 退回普通完整下载，再校验和转发两个序号，避免在兼容归一化前把普通下载真正要使用的坏序号藏掉。

2026-06-18 已把 CLI 私有归一化下沉到 `internal/bbdown`，由命令行和服务模式共用。服务父进程使用静默归一化来预测路径和选轨，子进程继续保留用户可见的废弃参数提示；`--aria2c-proxy` 的转换保持幂等，避免服务父子进程各执行一次后重复追加同一个 `--all-proxy`。

后续补参数时优先按这个顺序做：

1. 对照原版 README 和 parser 找到用户可见行为。
2. 把参数落到 `MyOption`，补配置文件合并测试。
3. 在下载链路里只消费统一后的 `MyOption` 字段。
4. 更新 `docs/usage.md` 的参数索引和 `docs/replica-progress.md` 的对照表。

## 信息抓取路线

`ResolveAvid` 先把用户输入统一成内部 ID：

| 内部 ID | 含义 | Fetcher |
| --- | --- | --- |
| 数字 Aid | 普通视频 | `NormalInfoFetcher` |
| `ep:<id>` | 番剧/国际版番剧 | `BangumiInfoFetcher` / `IntlBangumiInfoFetcher` |
| `cheese:<id>` | 课程 | `CheeseInfoFetcher` |
| `mid:<id>` | 空间投稿 | `SpaceVideoFetcher` |
| `favId:<fid>:<mid>` | 收藏夹 | `FavListFetcher` |
| `listBizId:<sid>` | 合集 | `MediaListFetcher` |
| `seriesBizId:<sid>` | 系列 | `SeriesListFetcher` |

特殊处理：

| 场景 | 当前实现 |
| --- | --- |
| b23 短链 | HEAD 跟随跳转后继续解析 |
| `ss` / `md` | 先换取目标 `ep` |
| 版权视频 AV 跳 EP | 普通视频接口发现 bangumi redirect 时保留 `epid` |
| 互动视频 | `x/player.so` 无图版本时回退 `x/player/v2`，再请求 `x/stein/edgeinfo_v2` |
| 空间投稿 | WBI 接口失败或返回风控 HTML/`-352` 时尝试旧接口，旧接口 `-799` 短退避重试；`403748305` 及 16 个补充 mid 已验证 WEB 登录态可稳定生成非空完整 URL 文件 |

## 播放流解析路线

播放地址统一由 `ExtractTracks` 输出 `ParsedTracks`：

| 字段 | 含义 |
| --- | --- |
| `VideoTracks` | DASH 视频轨道，包含清晰度、编码、带宽、时长 |
| `AudioTracks` | 普通音频轨道 |
| `BackgroundAudios` | 背景音 |
| `RoleAudioLists` | 角色音频 |
| `ExtraPoints` | 普通章节或番剧 clip 章节点 |
| `Clips` | FLV 分段 URL |
| `Dfns` | FLV 可重解析清晰度 |
| `WebJSONString` | 调试用原始播放 JSON |

通道策略：

| 通道 | 实现要点 |
| --- | --- |
| WEB | 默认走 `x/player/wbi/playurl`；UGC 空播放体带 `v_voucher` 时回退旧 `x/player/playurl` |
| TV | 走 TV playurl，使用 TV appkey/sign，番剧和 UGC 路径分开 |
| APP | 先尝试 APP gRPC，失败时回退 APP REST |
| INTL | 支持国际版播放接口和字幕接口 |
| BiliPlus | 自定义 `--host` / `--ep-host` / `--area` / `--access-token`，按 BiliPlus appkey/sign 组装请求 |
| FLV | 解析 `durl`，交互模式可按用户选择 qn 重新请求 |

轨道排序由清晰度优先级、编码优先级、升序选项共同决定。下载前会根据 only 模式裁剪视频/音频/背景音/角色音。

## 下载与混流路线

下载路径：

| 分支 | 行为 |
| --- | --- |
| DASH | 分别下载 `.video.mp4`、`.audio.m4a`，再混流 |
| FLV | 下载多个 `.clip*.mp4`，先 `MergeFLV`，再按需混流 |
| `--skip-mux` | 保留临时音视频或 FLV merged 文件 |
| `--audio-only` | 输出 `.m4a` |
| `--video-only` | 不下载普通音频、背景音、角色音 |
| `--sub-only` / `--cover-only` / `--danmaku-only` | 直接下载对应资源后退出当前分 P |

资源下载：

| 能力 | 实现 |
| --- | --- |
| 多线程下载 | 音视频媒体 URL 用 Range 分片；弹幕、字幕、封面等普通资源走单线程流式读取，兼容无 `Content-Length` 响应 |
| aria2c | `--use-aria2c` 交给外部 `aria2c` |
| CLI 进度 | 普通 CLI 在 TTY 中按资源写入字节刷新单行进度、百分比和速度；服务子进程仍只发送隐藏增量事件 |
| PCDN / UPOS | 默认替换 PCDN/UPOS host，可用 `--allow-pcdn` 或 `--upos-host` 调整 |
| force-http | 默认把媒体 URL 的 `https` 替换为 `http`，CMCC 等例外保留 |
| 单页重试 | 每个分 P 下载失败最多重试当前页，避免重复已完成分 P |

混流：

| 能力 | 当前状态 |
| --- | --- |
| ffmpeg 混流 | 默认路径，支持 metadata、封面、字幕、章节、音频语言 |
| MP4Box 混流 | `--use-mp4box` 或杜比视界低版本 ffmpeg 自动切换 |
| HEVC 兼容 | macOS 场景保留 `hvc1` 兼容处理 |
| 混流封面 | CLI 调用层按源仓库 `File.Exists(coverPath)` 语义传递封面路径，muxer 层保留非空封面参数 |
| 背景音/角色音 | 下载后作为附加音频材料参与混流 |

桌面版工具路径策略：

- 设置抽屉暴露 FFmpeg、MP4Box、aria2c 路径，保存到用户配置目录的 `preferences.json`，创建任务时分别映射为 `--ffmpeg-path`、`--mp4box-path`、`--aria2c-path`。
- 路径参数只注入下载任务；`info` / “仅查看”子命令不支持这些下载依赖参数，因此不会追加，避免参数解析失败。
- 桌面端先生成表单参数，再追加“额外参数”；工具路径预检会从最终 `Args` 中读取同名值参数的最后一次出现值。高级用户在额外参数里覆盖 `--ffmpeg-path`、`--mp4box-path` 或 `--aria2c-path` 时，预检、任务字段和复制命令都会统一到 CLI 实际生效的路径。
- 同名文件策略由 `MyOption.FileExistsAction` 统一承载。`rename` 在页面保存路径格式化后、所有派生产物生成前解析同组流水号；服务父进程在同一任务内维护已预留输出组，多个分 P 使用相同自定义模板时仍能依次预测原名、` (1)`、` (2)`，与 CLI 子进程按页下载后的实际路径一致。`skip` 在主输出与资源下载入口跳过非空目标；`overwrite` 在下载入口清理目标、`.tmp`、`.aria2` 和目标专属多线程分片，并在混流前移除旧最终输出，避免旧文件让失败混流被误判为成功。多线程分片枚举、合并和清理只接受 `00000_<当前目标基名>.vclip/.aclip`，不会把同目录其他任务分片混入当前文件或误删。Wails 偏好、任务历史、重试、表单回填和服务 `SavePaths` 预测复用同一枚举与路径解析函数。
- 桌面 helper 默认复制到系统用户配置目录；测试和排障脚本可通过 `BBDOWN_GO_RUNTIME_DIR` 覆盖运行目录，隔离临时 helper、二维码和任务历史，避免自动化测试把假 helper 写入真实桌面目录后影响后续下载任务。
- `TaskManager` 创建时固定本实例的任务历史路径。下载 helper 的输出消费和进程收尾都在后台 goroutine 中执行，即使测试或嵌入环境随后切换 `BBDOWN_GO_RUNTIME_DIR`，旧任务也只会写回所属运行目录，不会污染另一个桌面实例的 `tasks.json`。
- macOS 从 Finder 启动 `.app` 时没有用户 zsh PATH，helper 子进程会主动扩展 `/opt/homebrew/bin`、`/usr/local/bin`、`/opt/local/bin`，覆盖 Homebrew / MacPorts 的常见安装位置；桌面壳在固定路径失败后还会通过 `/bin/zsh -lc 'command -v <tool>'` 读取用户登录环境，把终端可用但不在 GUI PATH 中的工具解析成绝对路径。桌面历史任务和偏好可能保留旧机器、旧 Homebrew 前缀或迁移后的失效绝对路径；开始任务前会在显式路径不存在时重新按工具名探测，命中后回写任务参数，避免旧记录阻断当前机器的可用 FFmpeg。桌面壳预检仍找不到默认 FFmpeg 时，会降级把 `--ffmpeg-path ffmpeg` 交给 helper 做最终解析，避免 GUI 进程 PATH 与 helper PATH 不一致时提前失败。桌面壳还会把当前目录、桌面可执行目录、`.app/Contents/Resources`、helper 源目录和用户配置运行目录写入 helper 的 `PATH` 与 `BBDOWN_GO_TOOL_DIRS`，覆盖 helper 被复制后 `AppDir` 变化导致看不到 app 资源目录或同目录工具的情况。
- CLI 层的二进制查找会在当前目录、应用目录、`BBDOWN_GO_TOOL_DIRS`、PATH 后继续扫描常见工具目录；macOS 下仍未命中时，也会通过 `/bin/zsh -lc 'command -v <tool>'` 做最后兜底。这样桌面 helper、命令行显式传入 `--ffmpeg-path ffmpeg`，以及其它 GUI 环境启动的 CLI 都使用同一套依赖解析边界。
- 混流工具预检只覆盖真正会进入媒体混流或 FLV 合并的任务；`OnlyShowInfo` 会跳过所有外部下载/混流工具检查，因为它只解析并打印信息，不下载资源。`CoverOnly`、`DanmakuOnly` 和 `SubOnly` 会跳过 FFmpeg / MP4Box 检查，避免只下载 sidecar 资源时被本机混流依赖挡住；`AudioOnly` 和 `VideoOnly` 只保留单轨输出，也不会要求 FFmpeg。上述任务如果显式开启 `UseAria2c`，仍检查 aria2c。`SkipMux` 启动阶段也先放行，以免 DASH 分轨保留任务被 FFmpeg 误挡；但 FLV 多分段任务在解析到 `Clips` 后、下载分段前会专门解析 FFmpeg，因为 `.flv-merged.mp4` 仍需要本地合并分段。单个 FLV clip 按原版直接移动为 merged 输出，不要求 FFmpeg。
- CLI 层缺失工具错误保留原版风格的 `找不到可执行的...文件` 前缀，同时追加对应显式路径参数、已检查位置和写错的显式路径；桌面端即使漏过预检，helper 透出的错误也能直接指导用户填 `--ffmpeg-path` / `--mp4box-path` / `--aria2c-path`。桌面启动前预检如果确认找不到 FFmpeg，会在任务日志和状态栏追加短提示：完整下载需要 FFmpeg 混流，可先点击“检测工具”，macOS 下可执行 `brew install ffmpeg`。如果预检放行后 helper 子进程才输出 `找不到可执行的ffmpeg文件`、`找不到可执行的mp4box文件` 或 `找不到可执行的aria2c文件` 并失败，任务收尾也会从日志中提取对应工具提示，避免界面只显示 `exit status 1`。
- CLI `doctor` 子命令复用同一套工具查找 API，先生成内部诊断结构，再渲染为中文文本或 `--json` 结构化输出；输出包含版本、构建时间、可执行文件路径、平台架构、Go 运行时、程序目录、当前目录、登录态文件存在性、外部工具路径和工具版本摘要，文件和工具命中路径会规范为绝对路径。它只做登录态文件存在性判断，不读取或打印 Cookie/token 内容；外部工具版本只取 `-version` / `--version` 输出的第一条非空行，失败时留空，不阻止诊断继续。未找到外部工具时，文本输出会追加可使用的路径参数和安装建议，JSON 工具项也会带 `flag` / `installHint` 字段，便于用户或脚本直接给出下一步；如果用户显式传入不可用的工具路径或命令名，文本会回显“当前指定值不可用”，JSON 工具项会带 `input` 字段，便于区分未安装和填错路径。已找到工具只保留 `flag`，不会再带安装建议造成误解。桌面端“检测工具”按钮复用桌面壳路径解析逻辑，把裸命令名修正为绝对路径，并把版本摘要写入日志区；未找到时只写短提示，包含输入框/路径参数、安装建议和失效显式路径，避免把完整搜索目录刷进任务日志。桌面工具路径统一通过 `desktopToolPathValue` 归一化，表单值为空或为“自动”时按未显式指定处理，不写入任务参数、偏好或 `doctor --json` 参数，避免把 UI 自动探测语义误判成显式坏路径。“复制诊断”按钮则复用内置 helper 执行 `doctor --json`，并带上当前表单里的有效工具路径，便于用户把 ffmpeg/MP4Box/aria2c 的机器可读探测结果直接反馈回来。复制成功后桌面日志只保留工具状态和版本摘要，缺失工具会追加 `input`、对应路径参数和安装建议，完整 JSON 只进入剪贴板，避免排障信息把任务日志刷屏。

桌面版选流策略：

- 表单暴露“视频流”和“音频流”，偏好保存到 `preferences.json`，下载任务映射为 `--video-index` / `--audio-index`。
- 没有流列表时控件允许手填纯序号；当前选中任务日志包含 `共计N条视频流/音频流` 时，前端改为原生下拉项展示 `0. ...` 选项；创建任务时只把前导序号传给 CLI。
- 序号输入只注入下载任务；“仅查看”用于查看流列表，不携带选流参数，避免用户还没看到列表时误以为会下载。
- 任务重试会复制原任务序号，保证失败后重新执行仍下载同一组轨道。

## 文件命名路线

`FormatSavePath` 负责模板变量替换、非法字符清洗和默认 `.mp4` 后缀。

默认行为：

| 场景 | 默认模板 |
| --- | --- |
| 单 P | `<videoTitle>` |
| 多 P / 未完结番剧 | `<videoTitle>/[P<pageNumberWithZero>]<pageTitle>` |

关键兼容点：

| 兼容点 | 实现 |
| --- | --- |
| `-F` / `--file-pattern` | 单 P 模板 |
| `-M` / `--multi-file-pattern` | 多 P 模板 |
| 旧 `--add-dfn-subfix` | 转换为含 `<dfn>` 的模板并提示废弃 |
| 旧 `--no-padding-page-num` | 转换为不补零的分 P 模板并提示废弃 |
| 日期格式 | 支持 `yyyy-MM-dd_HH-mm-ss`、`fff`、`zzz` 等 .NET 风格到 Go layout 的转换 |
| 清洗规则 | 控制字符和 Windows 非法字符统一替换为 `_`，末尾点和空格裁剪 |
| 下载上下文标题 | CLI 和服务预测在进入保存路径/混流 metadata 前调用 `DownloadTitle`，对齐原版 `DownloadPageAsync` 对首尾点号视频标题的 `_` / `_fix` 修正；`FormatSavePath` 直接调用仍保持函数级行为 |

## 服务模式技术路线

当前服务模式不把下载器重写成内存内任务执行器，而是复用 CLI 子进程：

| 项 | 当前取舍 |
| --- | --- |
| 下载行为一致性 | 子进程执行同一套默认下载链路，避免服务模式和 CLI 模式行为分叉 |
| 参数映射 | JSON 反序列化为 `ServeRequestOptions`，再转回 CLI 参数启动当前二进制 |
| 添加任务响应 | `/add-task` 保持原版 `Results.Ok()` 风格，HTTP 200 只表示已接收任务；坏 JSON、空 Url 或明显无效的 `VideoIndex` / `AudioIndex` 返回 HTTP 400 `输入有误`，任务详情通过 `/get-tasks/running` 和 `/get-tasks/finished` 查询 |
| 任务集合查询 | `/get-tasks/` 返回 running 和 finished 两个数组，`/get-tasks/running` 与 `/get-tasks/finished` 只返回各自队列 |
| 按 Aid 查询 | `/get-tasks/{aid}` 会按 Aid 查 running 和 finished 任务，命中返回 `DownloadTask` JSON，未命中返回 HTTP 404 |
| 已完成任务清理 | `/remove-finished` 和 `/remove-finished/` 清空全部 finished，`/remove-finished/failed` 清空失败任务，`/remove-finished/{aid}` 删除指定 Aid，语义对齐原版 `BBDownApiServer.cs` 与 `json-api-doc.md` |
| 路由回归 | `Run` 复用 `routes()` 构建实际 HTTP 路由，路由级单测通过 `ServeHTTP` 验证任务查询、路径变量和清理接口绑定 |
| CORS | 对齐原版 `AllowAnyOrigin` / `AllowAnyMethod` / `AllowAnyHeader`，普通响应写入跨域头，预检 `OPTIONS` 在进入业务路由前返回 204 |
| 去重 | 添加任务时先 `ResolveAvid`，同 Aid 正在运行时不重复启动 |
| 归档跳过 | `SaveArchivesToFile` 命中 `BBDown.archives` 的分 P 对齐原版下载前跳过语义；服务端预测保存路径时同步过滤已归档分 P，避免 finished 暴露不存在的产物 |
| 进度 | 父进程解析 `开始解析P...`、普通 DASH 视频/音频下载、背景/角色音频、FLV 片段下载、`下载P...完毕`、DASH/FLV 合并、`任务完成` 等日志；下载完成只推进到页内 75%，合并阶段推进到页内 90%，最终完成才置 100% |
| 传输统计 | 子进程在 HTTP 下载写入过程中发送隐藏增量事件，并在每个下载动作完成后发送完成事件；父进程优先按事件刷新速度和累计字节，文件轮询按预测路径关联临时下载文件相对任务启动基线的增长量兜底；封面/弹幕/字幕等资源 only 和 AudioOnly/VideoOnly 单轨任务成功结束时用最终输出文件的增长量校正短任务漏计 |
| 只展示信息 | `OnlyShowInfo` 对齐原版 `Program.DownloadPageAsync` 的早退语义：只解析并打印轨道，不预测下载产物路径，也不把不存在的媒体文件写入 `SavePaths` |
| 隐藏流列表 | `HideStreams` 对齐原版 `Program.DownloadPageAsync` 的打印控制：服务 JSON 传入后由子进程抑制可用视频/音频/字幕流列表日志，但仍完成解析并写入 finished 状态 |
| 展示全部分 P | `ShowAll` 只影响分 P 标题展示，真实选择仍由 `SelectPage` 决定；服务回归验证全部 716 个分 P 标题可展示，但只解析选中的 P1 |
| 自定义命名模板 | `FilePattern` 用于单 P 输出，`MultiFilePattern` 用于多 P / 未完结番剧输出；真实服务回归验证父进程 `SavePaths` 与子进程实际封面文件路径一致 |
| 旧清晰度后缀参数 | `AddDfnSubfix` 在父进程预测路径前静默归一化为 `<dfn>` 命名模板，子进程继续打印原版兼容废弃提示；真实服务回归验证最终文件名带 `[360P 流畅]` |
| 旧编码/带宽参数 | `OnlyHevc` 会归一化为 `EncodingPriority=hevc`，`BandwithAscending` 会归一化为视频和音频升序；真实服务回归验证 `OnlyShowInfo` 下子进程打印原版兼容提示并按 HEVC 优先、音频带宽升序展示 |
| 互斥 only 参数 | `AudioOnly` 与 `VideoOnly` 同时为 true 时按原版 `Program.Methods.cs` 清空两个开关，服务父进程和子进程都退回普通完整混流，真实回归验证输出主 `.mp4` 而不是单轨产物 |
| SkipMux + 单轨 only | DASH `SkipMux` 与 `AudioOnly` / `VideoOnly` 组合时不会进入 mux，服务端 `SavePaths` 指向 CLI 实际保留的 `.audio.m4a` 或 `.video.mp4`，真实回归验证另一轨不会下载 |
| 错误摘要 | 解析失败、信息抓取失败和子进程退出失败会写入 `ErrorStage` 与 `ErrorMessage`；`ErrorStage` 粗分 `resolve` / `info` / `download`，摘要会截断并脱敏 cookie/token |
| 回调 | 任务结束后把 `DownloadTask` JSON POST 到 `CallBackWebHook`；发送使用 10 秒超时，非法 URL 或发送失败不影响任务进入 finished；本地 callback 接收器已完成真实任务回归 |

截至 2026-06-19 已完成三十四类不依赖会员权限的真实服务任务回归：临时启动 `serve` 后，分别 POST 封面 only 任务、单 P FilePattern CoverOnly 任务、多 P MultiFilePattern CoverOnly 任务、弹幕 XML/ASS only 任务、字幕 only 任务、OnlyShowInfo 任务、OnlyShowInfo + HideStreams 任务、OnlyShowInfo + ShowAll 任务、OnlyShowInfo + 旧编码升序参数任务、归档命中跳过任务、AudioOnly 任务、VideoOnly 任务、AudioOnly + VideoOnly 互斥归一化任务、带 `CallBackWebHook` 的封面任务、FLV `SkipMux` 任务、DASH `SkipMux` 任务、DASH `SkipMux` + AudioOnly 任务、DASH `SkipMux` + VideoOnly 任务、DASH 完整混流任务、DASH 完整混流 + AddDfnSubfix 任务、DASH 完整混流 + DownloadDanmaku 任务、DASH 完整混流 + 字幕任务、DASH 完整混流 + SkipCover 任务、DASH 完整混流 + Language metadata 任务、DASH 完整混流 + SimplyMux 任务、多 P CoverOnly 任务、无效输入失败任务、信息不完整失败 callback 任务、子进程依赖失败 callback 任务、TV API CoverOnly 任务、APP API CoverOnly 任务、INTL API CoverOnly 任务、INTL API 完整混流任务、INTL API 完整混流 + SkipSubtitle 任务，并验证 `/add-task` 输入校验、`/get-tasks/` 集合查询、`/get-tasks/running`、`/get-tasks/finished`、`/get-tasks/{aid}` 按 Aid 查询、`/remove-finished/failed`、`/remove-finished/{aid}`、`/remove-finished/` 清理 finished 队列和 CORS 预检。封面任务最终 `/get-tasks/finished` 返回成功任务，`Progress=1`、`SavePaths` 含 jpg 输出、`ErrorMessage=null`、`TotalDownloadedBytes=107932`；单 P FilePattern CoverOnly 任务返回 `single/BV1qt4y1X7TW-626497566-P1.jpg`，多 P MultiFilePattern CoverOnly 任务返回 `multi/BV18dEr6DErw/P2-【日】《异环》卡厄斯角色PV丨未尽的抉择.jpg`，两个路径均与实际封面文件一致；弹幕任务返回 XML/ASS 两个保存路径，生成 125172 bytes XML 和 124347 bytes ASS，`TotalDownloadedBytes=249519`；字幕 only 任务返回实际 `.ai-zh.srt` 产物路径和 1194 bytes 统计；OnlyShowInfo 任务只展示轨道信息，不再预测不存在的 `.mp4`，finished 返回 `SavePaths=[]`、`TotalDownloadedBytes=0`；OnlyShowInfo + HideStreams 任务在同样早退语义下不会向 `server.log` 输出可用流列表；OnlyShowInfo + ShowAll 任务展示全部分 P 标题但只解析 `SelectPage` 选中的页；OnlyShowInfo + 旧编码升序参数任务验证旧 `OnlyHevc` 与 `BandwithAscending` 参数会传入子进程并按 HEVC 优先、音频带宽升序展示，同时保持 `SavePaths=[]`、`TotalDownloadedBytes=0`；归档命中跳过任务不再预测不存在的封面或媒体产物，finished 返回 `SavePaths=[]`、`TotalDownloadedBytes=0`；AudioOnly 任务返回最终 `.m4a` 路径和 9093762 bytes 统计；VideoOnly 任务返回最终 `.mp4` 路径和 10024687 bytes 统计；AudioOnly + VideoOnly 互斥归一化任务按原版语义退回普通完整混流，返回主 `.mp4` 路径并清理临时音视频；callback 任务收到的 webhook body 与 finished 关键字段一致，非法 callback URL 不会阻止解析失败任务写入 finished failed，callback 发送超时边界已有代码级回归；普通 DASH 视频/音频/混流、背景/角色音频和 FLV 合并的页内进度已有代码级回归，避免单 P 任务长期停在 0 后直接完成；FLV `SkipMux` 任务返回 `SavePaths=["喜洋洋蓝蓝路.flv-merged.mp4"]`，与 CLI 实际保留产物一致；DASH `SkipMux` 任务返回 `.video.mp4` 与 `.audio.m4a` 两个真实保留产物；DASH `SkipMux` + AudioOnly 任务只返回 `.audio.m4a`，不下载视频且不混流；DASH `SkipMux` + VideoOnly 任务只返回 `.video.mp4`，不下载音频且不混流；DASH 完整混流任务返回最终 `.mp4`，临时音视频文件已在混流后清理；DASH 完整混流 + AddDfnSubfix 任务返回带 `[360P 流畅]` 清晰度后缀的最终 `.mp4`，确认父进程预测路径和子进程旧参数归一化一致；DASH 完整混流 + DownloadDanmaku 任务保留主 `.mp4` 保存路径并额外生成 XML/ASS 弹幕；DASH 完整混流 + 字幕任务会下载 AI 字幕后参与混流，并在成功后清理临时字幕；DASH 完整混流 + SkipCover 任务返回最终 `.mp4`，临时目录没有 `.jpg` / `.png` / `.webp` 封面文件且日志不出现“下载封面”；DASH 完整混流 + Language metadata 任务用 `ffprobe` 确认首条音频流 `language=jpn`；DASH 完整混流 + SimplyMux 任务用 `ffprobe` 确认未写入 BBDown 容器级 `title/comment/artist/album/creation_time`，只保留上游媒体自带 `description`；多 P CoverOnly 任务验证 P1/P2/P4 三个输出路径和资源 only 字节校正；`/add-task` 输入校验验证坏 JSON 和空 Url 返回 400，合法请求外壳返回 200 后异步写入任务状态；无效输入任务验证解析失败会进入 finished failed 并携带 `ErrorStage="resolve"` 与 `ErrorMessage`；`/get-tasks/`、`/get-tasks/running` 和 `/get-tasks/finished` 验证 running/finished 队列不会串队列，路由级单测额外确认静态路径不会被 `{id}` 误捕获；`/get-tasks/{aid}` 验证可按 Aid 取回 finished failed，未知 Aid 返回 404，同 Aid 同时存在时 running 优先；`/remove-finished/failed` 验证失败任务可按原版语义从 finished 队列移除；`/remove-finished/{aid}` 验证只删除指定 Aid；`/remove-finished/` 验证可清空全部 finished；CORS 验证 `OPTIONS /get-tasks/` 返回 204，普通 GET 和预检响应都带 `Access-Control-Allow-Origin/Methods/Headers=*`；信息不完整失败任务验证合法 av 但 0 分 P 时会进入 finished failed，记录 `ErrorStage="info"`、`ErrorMessage="未获取到分P信息"`，并触发失败 callback；子进程依赖失败任务验证下载前置依赖不可用时会进入 finished failed，记录 `ErrorStage="download"`、`ErrorMessage="找不到可执行的aria2c文件: exit status 1"`，并触发失败 callback；TV API CoverOnly 任务验证服务 JSON 的 `UseTvApi` 会传入子进程并加载本地 TV token；APP API CoverOnly 任务验证服务 JSON 的 `UseAppApi` 会传入子进程并加载临时 `BBDownApp.data`；INTL API CoverOnly 任务验证国际版播放页 URL 可经 `UseIntlApi` 服务任务输出番剧封面路径；INTL API 完整混流任务验证国际版播放页 URL 可经 `UseIntlApi` 服务任务完成音视频下载、字幕下载和最终 `.mp4` 混流，并在成功后清理临时音视频与字幕；INTL API 完整混流 + SkipSubtitle 任务验证服务 JSON 的 `SkipSubtitle=true` 会传入子进程，跳过字幕下载与字幕混流，日志不出现 `.srt/.ass` 字幕产物。真实 callback 回归曾暴露资源 only 短任务最终字节漏计，DASH 回归曾暴露子进程 stdout/stderr 管道自然关闭被误判为任务失败，字幕 only 回归曾暴露 `SavePaths` 误报 `.mp4` 且字节为 0，OnlyShowInfo 回归曾暴露未下载任务误报预测 `.mp4`，归档跳过回归曾暴露未下载任务误报预测 jpg，AudioOnly 回归曾暴露短任务字节只统计到 467532 而最终 `.m4a` 为 9093762 bytes，现均已修复。服务模式后续继续补更多下载类型样本和失败场景真实回归。

2026-06-18 23:26 已补 TV API 完整混流服务回归：`UseTvApi=true` 的 `BV1qt4y1X7TW` 服务任务会把 TV 通道参数传给子进程并加载本地 TV token，完成真实音视频下载与混流；finished 返回最终 `.mp4`，临时音视频和封面清理完成，`server.log` 不泄漏 `access_token`、cookie 或内部传输事件。

2026-06-18 23:36 已补 APP API 完整混流服务回归：临时 `BBDownApp.data` 先通过 `--use-app-api --only-show-info` 预检，服务任务随后用 `UseAppApi=true` 完成真实音视频下载与混流；finished 返回最终 `.mp4`，临时音视频和封面清理完成，`server.log` 不泄漏 `access_token`、cookie 或内部传输事件。

2026-06-18 23:48 已补 INTL API 完整混流服务回归：`https://www.bilibili.tv/en/play/36571/30623439` 先通过 `--use-intl-api --only-show-info --select-page 177` 预检，服务任务随后用 `UseIntlApi=true` 完成真实音视频下载、字幕下载与混流；finished 返回 `凡人修仙传/[P177]177.mp4`，临时音视频和字幕清理完成，`server.log` 不泄漏 `access_token`、cookie 或内部传输事件。

2026-06-18 23:57 已补 INTL API 完整混流 + SkipSubtitle 服务回归：同一国际版样本在服务 JSON 中设置 `SkipSubtitle=true` 后仍成功输出 `凡人修仙传/[P177]177.mp4`；`server.log` 不再出现“下载字幕”或 `.srt/.ass`，混流阶段为“开始合并音视频”，确认服务模式能把跳过字幕参数传给子进程并保持输出路径、字节统计和日志脱敏正常。

2026-06-19 00:36 已补 DASH 完整混流 + SkipCover 服务回归：`BV1qt4y1X7TW` 在服务 JSON 中设置 `SkipCover=true` 后仍成功输出最终 `.mp4`；临时工作目录没有 `.jpg` / `.png` / `.webp` 封面文件，`server.log` 不出现“下载封面”，确认服务模式能把跳过封面参数传给子进程并保持输出路径、字节统计和日志脱敏正常。

2026-06-19 00:47 已补 DASH 完整混流 + Language metadata 服务回归：`BV1qt4y1X7TW` 在服务 JSON 中设置 `Language="jpn"` 后仍成功输出最终 `.mp4`；`ffprobe 8.1.1` 读取首条音频流 `stream_tags=language` 返回 `jpn`，确认服务模式能把音轨语言参数传给子进程并写入 mux metadata。

2026-06-19 00:56 已补 DASH 完整混流 + SimplyMux 服务回归：`BV1qt4y1X7TW` 在服务 JSON 中设置 `SimplyMux=true` 后仍成功输出最终 `.mp4`；`ffprobe 8.1.1` 读取容器级 `format_tags=title,comment,description,artist,album,creation_time` 时只返回上游媒体自带 `description`，未出现 BBDown normally 注入的 `title`、`comment`、`artist`、`album`、`creation_time`。

2026-06-19 01:05 已补 OnlyShowInfo + HideStreams 服务回归：`BV1qt4y1X7TW` 在服务 JSON 中同时设置 `OnlyShowInfo=true` 和 `HideStreams=true` 后 finished 仍保持 `SavePaths=[]`、`TotalDownloadedBytes=0`；`server.log` 不输出可用视频/音频/字幕流列表，确认服务模式能把隐藏流列表参数传给子进程并保持只解析不下载语义。

2026-06-19 01:12 已补 OnlyShowInfo + ShowAll 服务回归：`BV1Kb411W75N` 在服务 JSON 中同时设置 `OnlyShowInfo=true`、`ShowAll=true` 和 `SelectPage="1"` 后 finished 仍保持 `SavePaths=[]`、`TotalDownloadedBytes=0`；`server.log` 出现最后一页 `P716` 和 `共计 716 个分P, 已选择：1`，且 `开始解析P...` 仅 P1 一行，确认服务模式能把展示全部分 P 参数传给子进程且不改变实际选择范围。

2026-06-19 01:17 已补 DASH 完整混流 + AddDfnSubfix 服务回归：`BV1qt4y1X7TW` 在服务 JSON 中设置 `AddDfnSubfix=true` 后 finished 返回带 `[360P 流畅]` 后缀的最终 `.mp4`；实际文件存在且大小为 19134766 bytes，确认服务父进程预测 `SavePaths` 前的旧参数静默归一化与子进程实际输出路径一致。

2026-06-19 01:25 已补 OnlyShowInfo + 旧编码升序参数服务回归：`BV1qt4y1X7TW` 在服务 JSON 中同时设置 `OnlyShowInfo=true`、`OnlyHevc=true` 和 `BandwithAscending=true` 后，finished 仍保持 `SavePaths=[]`、`TotalDownloadedBytes=0`；`server.log` 出现 `--only-hevc/-hevc` 与 `--bandwith-ascending` 的原版兼容废弃提示，轨道列表按 HEVC 优先和音频带宽升序展示，确认旧参数归一化传入子进程且不破坏只展示信息的早退语义。

2026-06-19 01:31 已补 AudioOnly + VideoOnly 互斥归一化服务回归：`BV1qt4y1X7TW` 在服务 JSON 中同时设置 `AudioOnly=true` 和 `VideoOnly=true` 后，finished 返回普通完整混流的主 `.mp4`，日志出现视频下载、音频下载和合并音视频；实际最终 mp4 为 19134766 bytes，确认父进程预测路径、子进程参数和原版互斥 only 参数归一化保持一致。

2026-06-19 01:43 已补 DASH SkipMux + 单轨 only 服务回归：`BV1qt4y1X7TW` 分别使用 `AudioOnly=true, SkipMux=true` 和 `VideoOnly=true, SkipMux=true` 执行服务任务；前者 finished 只返回 `.audio.m4a` 且日志只有音频下载，后者只返回 `.video.mp4` 且日志只有视频下载，两个任务都没有“开始合并音视频”，确认父进程路径预测和子进程跳过混流后的真实保留产物一致。

2026-06-19 01:52 已补服务模式自定义命名模板回归：`BV1qt4y1X7TW` 单 P CoverOnly 使用 `FilePattern="single/<bvid>-<aid>-P<pageNumber>"` 后，finished 返回 `single/BV1qt4y1X7TW-626497566-P1.jpg`；`BV18dEr6DErw` 多 P CoverOnly 使用 `MultiFilePattern="multi/<bvid>/P<pageNumber>-<pageTitle>"` 和 `SelectPage=2` 后，finished 返回 `multi/BV18dEr6DErw/P2-【日】《异环》卡厄斯角色PV丨未尽的抉择.jpg`；两个实际文件均存在，确认服务 JSON 命名模板传递、父进程路径预测和子进程真实输出一致。

2026-06-19 02:14 已补服务模式 `/remove-finished` 无尾斜杠路由兼容：源仓库 `json-api-doc.md` 把清空 finished 队列的端点写为 `/remove-finished`，Go 服务端现在同时接受 `/remove-finished` 与 `/remove-finished/`；`TestApiServerRemoveFinishedRoutes` 覆盖无尾斜杠、带尾斜杠、`/remove-finished/failed` 和 `/remove-finished/{aid}`，防止文档端点和实际路由分叉。

2026-06-18 21:59 已复验结构化失败阶段字段的真实 HTTP 行为：同一 `serve --listen http://127.0.0.1:23507` 实例内，`not-a-bili-id` finished 返回 `ErrorStage="resolve"`，`av999999999999` finished 返回 `ErrorStage="info"`，缺失 `aria2c` 的 `BV1qt4y1X7TW` CoverOnly 任务 finished 返回 `ErrorStage="download"`；三者都保持 `IsSuccessful=false`、失败摘要脱敏和 `TotalDownloadedBytes=0`。

2026-06-18 22:10 已复验服务模式 `OnlyShowInfo` 真实 HTTP 行为：`serve --listen http://127.0.0.1:23509` 下 POST `BV1qt4y1X7TW`、`OnlyShowInfo=true`、`SelectPage=1`，finished 返回 `Aid=626497566`、`IsSuccessful=true`、`Progress=1`、`SavePaths=[]`、`TotalDownloadedBytes=0`、`ErrorStage=null`、`ErrorMessage=null`；临时工作目录没有媒体产物，符合原版只展示信息不下载的语义。

2026-06-18 22:25 已复验服务模式 `SaveArchivesToFile` 归档命中跳过真实 HTTP 行为：在临时二进制目录预置 `BBDown.archives` 内容 `626497566|` 后启动 `serve --listen http://127.0.0.1:23511`，POST `BV1qt4y1X7TW`、`SaveArchivesToFile=true`、`CoverOnly=true`、`SelectPage=1`，finished 返回 `Aid=626497566`、`IsSuccessful=true`、`Progress=1`、`SavePaths=[]`、`TotalDownloadedBytes=0`、`ErrorStage=null`、`ErrorMessage=null`；临时工作目录没有媒体产物，符合原版归档命中后下载前跳过的语义。

继续改进方向：

| 方向 | 约束 |
| --- | --- |
| 更实时的进度 | 优先增强 CLI 日志、下载增量事件和文件轮询，不急于把下载循环搬入服务进程 |
| 更准的字节统计 | 只统计网络下载相关临时文件增长量，继续避免 mux 输出重复计入 |
| 更完整的任务错误 | 已写入 `ErrorStage` 和 `ErrorMessage`；后续可继续细分下载阶段错误类型和补真实失败样本 |
| 更稳定的路径预测 | 跟随 `FormatSavePath` 和 only 模式变化同步补服务预测测试 |

## 配置与登录路线

配置文件：

```text
命令行 args
  -> 第一次解析拿到 --config-file
  -> MergeConfigArgs 读取 BBDown.config
  -> 配置文件参数追加在命令行前
  -> 命令行显式参数覆盖配置文件同名参数
  -> 第二次解析得到最终 MyOption
```

配置文件解析对齐源仓库 `BBDownConfigParser.cs` 的常见写法：每行可以写独立选项、`--flag value`、布尔字面值或 `--flag=value`，空行和 `#` 注释会跳过。`MergeConfigArgs` 会先建立命令行显式参数集合，再合并配置文件中未被覆盖的项；带等号的值参数作为单个 token 处理，避免把下一行配置误吞为参数值。`TestMergeConfigArgs` 覆盖短参数别名、显式布尔值、命令行覆盖配置值、以及 `--file-pattern=...` 后续配置项保留。

认证：

| 文件 | 用途 |
| --- | --- |
| `BBDown.data` | WEB Cookie |
| `BBDownTV.data` | TV access token |
| `BBDownApp.data` | APP access token |

`Config` 保存进程级 Cookie、Token、Host、EpHost、TvHost、Area 和 WBI salt。HTTP 请求层根据 URL 自动附加 UA、Cookie、Referer，并对 GET 与 POST 表单响应统一处理 gzip/deflate 解码；TV 二维码登录等表单接口因此与原版 `HttpClientHandler.AutomaticDecompression` 行为保持一致。WEB/TV 登录成功后只把凭据写入本地 `BBDown.data` / `BBDownTV.data`，标准输出只提示文件已保存，不打印 `SESSDATA`、`bili_jct` 或 `access_token` 明文。TV 登录轮询优先识别响应里的 `data.access_token`，再处理 `86038` 过期、`86039`/`86101` 等待扫码、`86090` 等待确认等状态；这样贴近原版“非等待状态读取 token”的行为，也避免接口成功码变化时漏判。`BBDownApp.data` 的读取路径已用临时文件验证，`--use-app-api` 会加载本地 token 并走 APP API；普通视频和多 P 播放流已有真实样本，会员/大会员能力仍需要有权限样本单独证明，当前账号非会员时不作为复刻进度阻塞。

## 测试与回归路线

测试分三层：

| 层级 | 覆盖内容 | 命令 |
| --- | --- | --- |
| 单元测试 | URL 解析、签名、配置合并、CLI 参数注册覆盖、路径格式化、轨道解析、字幕/弹幕、登录文件写入与日志脱敏、服务进度、传输统计、CORS 和任务查询/清理接口 | `go test ./...` |
| 本地打包验证 | release 跨平台构建、压缩包仅含应用程序、CGO 关闭后的单二进制构建，打包时注入 UTC 构建时间，并用 `verify-artifacts.sh` 拦截 README/docs 或多余文件 | `VERSION=vX.Y.Z ./scripts/build-release.sh` + `./scripts/verify-artifacts.sh cli dist/release/*` |
| 桌面打包验证 | 当前目标系统原生构建桌面应用和同版本 CLI helper；macOS 输出 `.app` zip 并校验 `minos 11.0`，Windows/Linux 输出桌面程序加同目录 helper，并用 `verify-artifacts.sh` 固化包内容规则 | `VERSION=vX.Y.Z ./scripts/build-desktop.sh` + `./scripts/verify-artifacts.sh desktop dist/desktop/*` |
| 线上样本回归 | B 站真实接口的普通视频、多 P、TV、APP、INTL、课程、FLV、空间投稿、服务模式、杜比视界、BiliPlus | 写入 `docs/regression-samples.md` |

回归策略：

| 类型 | 优先命令 |
| --- | --- |
| 普通解析 | `go run ./cmd/bbdown info <BV>` |
| 播放流低副作用验证 | `go run ./cmd/bbdown --only-show-info <BV>` |
| 文件输出验证 | `--work-dir "$(mktemp -d)"` 搭配 `--cover-only`、`--danmaku-only`、`--sub-only`、`--skip-mux` |
| 服务模式验证 | 用临时目录添加小任务，再检查 `/get-tasks/running` 和 `/get-tasks/finished` |
| 登录/会员/代理 | 只记录样本类型、执行日期和结论，不记录 secret |

线上接口相关结论必须按“已验证 / 候选来源 / 未验证推断”区分。公开接口返回 HTML、`-352`、`-799` 或空播放体时，应先标记样本状态，再判断是否需要代码回退。

## 发布路线

本地验证：

```sh
go test ./...
./scripts/build-release.sh
./scripts/verify-artifacts.sh cli dist/release/*
./scripts/build-desktop.sh
./scripts/verify-artifacts.sh desktop dist/desktop/*
```

GitHub Actions 额度不足时使用本地聚合打包：

```sh
./scripts/build-local-artifacts.sh
gh release upload vX.Y.Z dist/local/release/* dist/local/desktop/* --repo lonnnnnng/BB-DL --clobber
```

`build-local-artifacts.sh` 不清理已有 `dist` 目录，只把本轮 CLI 资产写入 `dist/local/release`，把当前平台桌面资产写入 `dist/local/desktop`，随后复用 `verify-artifacts.sh` 做包结构校验。桌面版仍需要在目标系统本机或对应 runner 上构建；macOS 本机只能稳定产出 macOS 桌面包，Windows/Linux 桌面包应在各自平台执行同一脚本或单独运行 `scripts/build-desktop.sh`。

tag 发布：

```sh
git tag -a vX.Y.Z -m '发布 vX.Y.Z'
git push origin vX.Y.Z
```

`.github/workflows/release.yml` 会在 `v*` tag 推送后执行；也支持手动触发并填写 `release_tag`，用于某个平台资产失败后补传同版本 Release 资产：

1. `go test ./...`
2. `./scripts/build-release.sh`
3. `./scripts/verify-artifacts.sh cli dist/release/*` 校验 CLI 包只含应用程序，并在解包前拒绝损坏/空归档、绝对路径、`..`、`.` 段、反斜杠路径和解包后的符号链接
4. 创建或覆盖上传 GitHub Release CLI 资产
5. 在 macOS、Windows、Linux runner 上安装 Node/Wails 和系统 WebView 依赖，执行 `go test ./...` 与 `scripts/build-desktop.sh`
6. `./scripts/verify-artifacts.sh desktop ...` 校验桌面包只含桌面程序和 helper，不含 README/docs，并使用同一套归档路径安全检查
7. 上传三平台 `BB-DL` 桌面版资产；macOS 继续额外校验 `BB-DL.app` 的 `LSMinimumSystemVersion=11.0` 与 Mach-O `minos 11.0`

工作流当前使用 `actions/checkout@v7` 和 `actions/setup-go@v6`，用于规避旧主版本在 GitHub Actions 上的 Node.js 20 deprecation 警告。

当前发布资产矩阵：

| 系统 | CLI 资产 |
| --- | --- |
| macOS | `BB-DL_vX.Y.Z_cli_darwin_amd64.tar.gz`、`BB-DL_vX.Y.Z_cli_darwin_arm64.tar.gz` |
| Linux | `BB-DL_vX.Y.Z_cli_linux_amd64.tar.gz`、`BB-DL_vX.Y.Z_cli_linux_arm64.tar.gz` |
| Windows | `BB-DL_vX.Y.Z_cli_windows_amd64.zip`、`BB-DL_vX.Y.Z_cli_windows_arm64.zip` |

桌面发布资产：

| 系统 | 资产 |
| --- | --- |
| macOS | `BB-DL_vX.Y.Z_desktop_darwin_<arch>.zip`，内含 `BB-DL.app` |
| Windows | `BB-DL_vX.Y.Z_desktop_windows_<arch>.zip`，内含 `BB-DL.exe` 和 `BB-DL-cli.exe` helper |
| Linux | `BB-DL_vX.Y.Z_desktop_linux_<arch>.tar.gz`，内含 `BB-DL` 和 `BB-DL-cli` helper |

桌面 release job 使用三平台矩阵：先安装 Node 22、前端依赖和 Wails 2.13.0；Linux runner 安装 GTK3、WebKit2GTK 4.1、OpenGL、X11 和 xkbcommon 开发库并使用 `webkit2_41` tag，Windows runner 通过 MSYS2/Mingw 提供 CGO 与归档工具，macOS runner 使用系统 WebKit 打包 `.app`。macOS 桌面包继续显式设置 `MACOSX_DEPLOYMENT_TARGET=11.0` 和 `-mmacosx-version-min=11.0`，helper 也强制 external linker 继承同一部署目标；workflow 会用 `plutil` 与 `otool` 校验 `LSMinimumSystemVersion=11.0`，以及主程序和 helper 的 Mach-O `minos 11.0`，避免 `macos-latest` 新 SDK 把产物标成只能在构建机系统版本上运行。构建脚本把 helper 直接注入 `build/bin/BB-DL.app` 后再整体签名和归档，使本地安装包与 Release 包共用同一份完整 `.app`。

当前发布版本为 `v1.0.13`。该版本更新 Bilibili 衍生应用图标，保证 macOS Wails 构建产物内置同版本 CLI helper，修复 URL 后置 `--file-exists-action` 的参数重排，并阻止旧任务通过“填入下载表单”覆盖当前同名文件默认策略。

## 后续技术路线

路线拆成短期、中期和长期三段，优先级仍以“补齐原版行为”和“可回归”为准。

短期 P1：

| 优先级 | 事项 | 判断标准 |
| --- | --- | --- |
| P1 | 补杜比视界真实样本 | 未登录或登录样本能返回实际 `[杜比视界]` 轨道并完成解析 |
| P1 | 补 BiliPlus 真实代理样本 | 真实 host/token/area 可解析 INTL 播放流和字幕，不记录密钥 |
| P1 | 补空间投稿稳定样本 | 已有 `403748305` 及 16 个补充 mid 的 WEB 登录态通过样本；后续继续补未登录/登录态差异 |
| P1 | 补登录扫码回归 | mock 已覆盖 WEB/TV 登录文件生成和日志脱敏；WEB 与 TV 真实扫码均已完成，APP token 文件读取和 APP API 多 P 播放流已验证 |

中期 P2：

| 优先级 | 事项 | 判断标准 |
| --- | --- | --- |
| P2 | 继续提升服务模式和桌面端实时进度 | 普通 DASH 视频/音频/混流、背景/角色音频和 FLV 合并阶段已可推进页内进度；HTTP 下载已通过子进程增量事件刷新速度和字节；服务端和桌面端都已兼容 CLI 时间戳日志并显示单调百分比；桌面端无事件场景已用保存目录文件增长轮询兜底；后续继续补服务模式 aria2c 等外部下载器场景的观测能力 |
| P2 | 扩展会员/大会员/区域内容回归 | 当前账号非会员，此项只在具备有权限账号、公开替代样本或外部可复验证据时推进 |
| P2 | 补服务模式错误可观测性 | 基础 `ErrorMessage` 和 `ErrorStage` 已实现，封面 only、单 P FilePattern CoverOnly、多 P MultiFilePattern CoverOnly、弹幕 XML/ASS only、字幕 only、AudioOnly、VideoOnly、callback webhook、FLV SkipMux、DASH SkipMux、DASH SkipMux + AudioOnly/VideoOnly、DASH 完整混流、DASH 完整混流 + AddDfnSubfix、DASH 完整混流 + DownloadDanmaku、DASH 完整混流 + 字幕、DASH 完整混流 + SkipCover、DASH 完整混流 + Language metadata、DASH 完整混流 + SimplyMux、OnlyShowInfo + HideStreams、OnlyShowInfo + ShowAll、OnlyShowInfo + 旧编码升序参数、AudioOnly + VideoOnly 互斥归一化、多 P CoverOnly、TV API / APP API / INTL API 完整混流、INTL API 完整混流 + SkipSubtitle、TV/APP/INTL API CoverOnly、无效输入失败、信息不完整失败 callback、子进程依赖失败 callback、callback 非法 URL 忽略、`/add-task` 输入校验、任务集合查询、`/get-tasks/{aid}` 查询与 `/remove-finished` 三类清理真实任务已通过或有代码级回归；后续补更细错误类型和更多下载类型回归 |
| P2 | 完善 APP/TV/INTL 波动接口回归 | TV API、APP API 和 INTL API 服务完整混流、INTL API SkipSubtitle、TV/APP/INTL 服务 CoverOnly 已通过；后续补关键接口失败时的样本、日志和候选回退路径 |

长期 P3：

| 优先级 | 事项 | 判断标准 |
| --- | --- | --- |
| P3 | 梳理原版剩余边角参数 | 对照源仓库 README 和命令行 parser，补齐行为差异表 |
| P3 | 收敛内部模块边界 | 在不改变用户行为的前提下减少 CLI 层和下载层的耦合 |
| P3 | 发布链路持续维护 | GitHub Actions 当前已升至 `checkout@v7` / `setup-go@v6`；后续继续保持 Go 版本和 release 资产矩阵可用 |

## 验证纪律

- 代码级行为用单测覆盖，线上接口行为写入 `docs/regression-samples.md`。
- 真实样本优先使用 `info`、`--only-show-info`、`--cover-only`、`--sub-only` 等低副作用命令。
- 下载样本必须使用临时 `--work-dir`。
- 登录、会员、BiliPlus、代理样本只记录来源类型和执行日期，不记录真实 secret；账号没有权限的会员/大会员能力必须标成未验证或外部条件阻塞。
- 接口风控或样本失效时，文档必须标注为候选或失败，不把历史通过当作当前通过。
