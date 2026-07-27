# BB-DL 使用说明

更新时间：2026-07-27 10:09:10 北京时间

`BB-DL` 是 BBDown 的 Go 复刻版，命令行参数尽量贴近原版。当前默认命令用于下载，另有 `help`、`version`、`info`、`login`、`logintv`、`doctor`、`serve` 子命令。

## 快速开始

下载 release 包后，建议先确认版本、外部依赖和一个低副作用解析样本：

```sh
./BB-DL --version
./BB-DL version
ffmpeg -version
./BB-DL info BV1qt4y1X7TW
```

启动和 `--version` / `version` 只输出当前版本与构建时间；release 包会在打包时写入 UTC 构建时间，本地直接 `go build` 未注入时显示 `unknown`。

最小下载命令：

```sh
./BB-DL --work-dir ~/Downloads/bilibili BV1qt4y1X7TW
./BB-DL download --work-dir ~/Downloads/bilibili BV1qt4y1X7TW
```

只看可用流、不下载：

```sh
./BB-DL --only-show-info BV1qt4y1X7TW
```

如果要复用原版 BBDown 的登录文件，把 `BBDown.data`、`BBDownTV.data` 或 `BBDownApp.data` 放在程序所在目录或当前执行目录，再运行对应命令即可。

## 安装与依赖

本地构建：

```sh
go build -o BB-DL ./cmd/bbdown
```

常用外部依赖：

| 工具 | 什么时候需要 |
| --- | --- |
| `ffmpeg` | 默认混流、FLV 分段合并、封面/字幕/章节写入 |
| `MP4Box` | 使用 `--use-mp4box`，或低版本 `ffmpeg` 处理杜比视界时自动切换 |
| `aria2c` | 使用 `--use-aria2c` 时 |

`info` / `--only-show-info` 不下载任何资源，因此不需要 `ffmpeg`、`MP4Box` 或 `aria2c`。`--cover-only`、`--sub-only`、`--danmaku-only` 不需要混流工具；`--audio-only` 和 `--video-only` 只保留单条媒体轨，也不需要 `ffmpeg`。如果这些资源或单轨下载任务同时显式开启 `--use-aria2c`，仍需要能找到 `aria2c`。`--skip-mux` 对 DASH 音视频分轨不会混流，启动预检会先放行；如果解析到的是多个 FLV 分段，仍需要 `ffmpeg` 把分段合成 `.flv-merged.mp4`。

从 GitHub Release 获取预编译包：

```sh
gh release download v1.0.13 --repo lonnnnnng/BB-DL --pattern '*darwin_arm64*'
```

GitHub Actions 额度不足或需要本机临时打包时：

```sh
./scripts/build-local-artifacts.sh
gh release upload vX.Y.Z dist/local/release/* dist/local/desktop/* --repo lonnnnnng/BB-DL --clobber
```

`dist/local/release` 是 CLI 跨平台包，`dist/local/desktop` 是当前系统桌面包；脚本会自动调用 `verify-artifacts.sh`，确保 release 包仍只包含应用程序本体，不带 README 或 docs。

release 包只包含应用程序本体：

| 内容 | 说明 |
| --- | --- |
| `BB-DL` / `BB-DL.exe` | 可执行文件 |

桌面应用名为 `BB-DL`，release 额外提供 `BB-DL_vX.Y.Z_desktop_darwin_<arch>.zip`、`BB-DL_vX.Y.Z_desktop_windows_<arch>.zip` 和 `BB-DL_vX.Y.Z_desktop_linux_<arch>.tar.gz`。macOS 包顶层只包含 `BB-DL.app`；Windows/Linux 包只包含 `BB-DL` 桌面程序和同目录 `BB-DL-cli` helper，不打包 README 或 docs。独立 CLI 包名为 `BB-DL_vX.Y.Z_cli_<os>_<arch>`，包内只有 `BB-DL` / `BB-DL.exe`。程序会自动探测 `/opt/homebrew/bin/ffmpeg`、`/usr/local/bin/ffmpeg` 等常见路径，并会在固定路径失败后读取 zsh 登录环境中的 `command -v ffmpeg` 结果；桌面版还会把 `.app/Contents/Resources`、helper 源目录和运行目录写入子进程 `PATH` 与 `BBDOWN_GO_TOOL_DIRS`，避免内置 helper 被复制到用户配置目录后看不到随 app 放置的工具。如果默认完整下载仍提示“找不到可执行的ffmpeg文件”，说明当前机器没有可用混流工具，或桌面端无法访问它。在创建任务表单的 `FFmpeg` 输入框填写完整路径，或在额外参数中传 `--ffmpeg-path /path/to/ffmpeg`；只想先保留分轨时可改用“仅音频”“仅视频”或勾选“跳过混流”。当前开发版的 CLI/helper 缺失工具错误会同时提示对应参数名和已检查的位置，方便直接判断是未安装、PATH 不一致还是显式路径写错。

桌面版会在用户配置目录保存任务历史 `tasks.json`，包含任务参数、日志、完成文件、字节数、进度和时间戳。重新打开应用后会恢复任务列表；关键状态会立即保存，运行中的任务最多每 5 秒刷新一次历史。历史文件最多保留最近 200 个任务，每个任务日志最多保留最后 256 KiB。之前仍在运行的任务会显示为“已停止”，因为旧 helper 子进程已经无法被新窗口继续控制。桌面版启动任务前会把内置 CLI helper 复制到用户配置目录，当前开发版会按内容确认是否需要覆盖，避免升级后继续调用旧 helper 而出现已修复的 `ffmpeg` 查找问题。任务列表里的“重试失败/停止”会把失败和已停止任务按原参数重新排队，同名文件处理方式也会随任务历史、重试和“填入表单”保留；“清理已结束任务”会一次清理成功、失败和已停止任务，等待、运行和停止中任务会保留。

选中桌面任务后，可以在日志详情区复制复现命令或复制当前任务完整日志，便于把失败任务拿到终端复跑或反馈问题。完成文件区支持复制单个文件路径，也可以把当前任务全部完成文件路径按行复制出来，适合多 P、字幕或弹幕任务后续整理。

依赖查找顺序：

1. 显式路径参数：`--ffmpeg-path`、`--mp4box-path`、`--aria2c-path`。
2. 当前工作目录。
3. 程序所在目录。
4. `BBDOWN_GO_TOOL_DIRS` 里的额外目录，多个目录按系统路径分隔符分隔。
5. `PATH`。
6. macOS / Linux / Windows 的常见工具目录，例如 macOS 的 `/opt/homebrew/bin`、`/usr/local/bin`、`/opt/local/bin`。
7. macOS 下额外读取 zsh 登录环境中的 `command -v` 结果，用来覆盖 Finder 启动 `.app` 或其它 GUI 环境 PATH 过短的问题。

## 基本命令

```sh
./BB-DL "BV1qt4y1X7TW"
./BB-DL "https://www.bilibili.com/video/BV1qt4y1X7TW"
./BB-DL download "BV1qt4y1X7TW"
./BB-DL down "BV1qt4y1X7TW"
./BB-DL help
./BB-DL help --help
./BB-DL help login
./BB-DL version
./BB-DL version --help
./BB-DL info "BV1qt4y1X7TW"
./BB-DL login
./BB-DL login --help
./BB-DL logintv
./BB-DL logintv --help
./BB-DL doctor
./BB-DL serve --listen http://0.0.0.0:23333
```

命令说明：

根命令名大小写不敏感，例如 `HELP`、`Version`、`Doctor` 会按 `help`、`version`、`doctor` 处理；参数名仍建议按帮助文本中的小写形式输入。

| 命令 | 作用 |
| --- | --- |
| 默认命令 | 解析、下载、混流并保存视频，例如 `BB-DL BV1...` |
| `download` / `down` | 显式下载子命令，等价于默认命令，适合脚本里把动作写清楚 |
| `help [命令]` | 显示总览或指定命令帮助，例如 `help login`、`help download`；`help --help` / `help help` 显示 help 命令本身用法 |
| `version` | 显示当前版本和构建时间；`version --help` 显示该命令用法 |
| `info` | 只展示视频信息和可选流，不下载 |
| `login` | WEB 二维码登录，保存 `BBDown.data` |
| `logintv` | TV 二维码登录，保存 `BBDownTV.data` |
| `doctor` | 诊断版本、运行目录、登录态文件和外部工具路径 |
| `serve` | 启动兼容 BBDown 风格的 HTTP 任务服务 |

### 环境诊断

```sh
./BB-DL doctor
./BB-DL doctor --json
./BB-DL doctor --ffmpeg-path ffmpeg
./BB-DL doctor --ffmpeg-path /opt/homebrew/bin/ffmpeg --aria2c-path aria2c
```

`doctor` 会输出当前版本、构建时间、可执行文件路径、平台架构、Go 运行时、程序目录、当前目录、`BBDown.data` / `BBDownTV.data` / `BBDownApp.data` 是否存在，以及 `ffmpeg`、`MP4Box`、`aria2c` 的探测结果。外部工具如果可运行，会额外显示版本输出的第一行，便于判断 `ffmpeg` 是否过旧；如果未找到，会提示可使用的路径参数和安装建议；如果你显式传了不可用的 `--ffmpeg-path` / `--mp4box-path` / `--aria2c-path`，文本输出会回显“当前指定值不可用”。它只检查登录态文件是否存在且非空，不读取或打印 Cookie/token 内容。加 `--json` 时输出结构化 JSON，路径字段使用绝对路径，工具项会包含 `flag`，缺失工具项还会包含 `installHint`；显式指定值不可用时会额外包含 `input`，方便脚本、CI 或反馈问题时直接附带诊断结果。

`doctor` 支持的参数：

| 参数 | 说明 |
| --- | --- |
| `--ffmpeg-path <value>` | 指定 `ffmpeg` 可执行文件路径或命令名，用于诊断默认混流依赖 |
| `--mp4box-path <value>` | 指定 `MP4Box` 可执行文件路径或命令名，用于诊断 MP4Box 混流依赖 |
| `--aria2c-path <value>` | 指定 `aria2c` 可执行文件路径或命令名，用于诊断外部下载器依赖 |
| `--json` | 以 JSON 格式输出诊断结果 |
| `-?` / `-h` / `--help` | 显示 `doctor` 帮助 |

`login` / `logintv` 支持 `-?`、`-h` 和 `--help` 查看中文帮助；除此之外不接受额外参数，避免看帮助或误传参数时直接进入扫码登录。

## 登录与凭据

很多高码率、空间投稿、番剧或风控敏感接口需要登录态。第一次使用建议先完成 WEB 登录，再用低副作用的 `--only-show-info` 命令验证凭据是否被加载。

### WEB 登录

```sh
./BB-DL login
./BB-DL login --help
```

命令会生成 `qrcode.png`，同时在终端打印二维码。用哔哩哔哩客户端扫码并在手机端确认后，会在程序运行文件目录保存 `BBDown.data`。登录成功日志只提示文件已保存，不会把 `SESSDATA`、`bili_jct` 等 Cookie 明文打印出来。

验证 WEB 登录态：

```sh
./BB-DL --only-show-info BV1qt4y1X7TW
```

如果已读到本地 Cookie，日志会出现 `加载本地cookie...`，随后会执行账号登录检查。二维码显示过期时，重新运行 `./BB-DL login` 并扫描最新生成的二维码即可；远程桌面、聊天窗口或图片预览容易缓存旧图，排查时优先确认当前目录里的 `qrcode.png` 是最新文件。

### TV 登录

```sh
./BB-DL logintv
./BB-DL logintv --help
```

TV 登录同样使用客户端扫码，成功后保存 `BBDownTV.data`。这个 token 主要服务于 `--use-tv-api` / `--tv` 通道，适合需要 TV 播放接口的样本。

验证 TV token：

```sh
./BB-DL --use-tv-api --only-show-info BV1qt4y1X7TW
```

如果已读到本地 TV token，日志会出现 `加载本地token...`。TV 登录过程中可能看到 `等待扫码...`、`扫码成功, 请确认...` 或二维码过期提示；过期后重新执行 `logintv`，不要继续扫描旧二维码。

### APP token 与手动凭据

当前没有独立的 APP 扫码登录命令；需要 APP 通道时，可以把兼容的 token 写入 `BBDownApp.data`，或临时使用 `--access-token` / `--token`：

```sh
./BB-DL --use-app-api --only-show-info BV1...
./BB-DL --use-app-api --access-token <token> --only-show-info BV1...
```

WEB Cookie 也可以用 `--cookie` / `-c` 临时覆盖本地文件：

```sh
./BB-DL --cookie '<cookie>' --only-show-info BV1...
```

命令行传入的 Cookie 和 token 会留在 shell 历史、进程参数或录屏里；长期使用优先放在本地凭据文件，不要写入文档、提交记录和回归样本。

### 凭据文件位置与退出登录

程序会从两个位置读取运行文件：可执行文件所在目录、当前执行目录。常见文件如下：

| 文件 | 用途 |
| --- | --- |
| `BBDown.data` | WEB Cookie，由 `login` 生成 |
| `BBDownTV.data` | TV token，由 `logintv` 生成 |
| `BBDownApp.data` | APP token，需要手动准备或从兼容来源复制 |

没有单独的 `logout` 子命令；需要退出登录或切换账号时，删除对应凭据文件后重新扫码即可。删除前确认路径，避免误删其他目录里的同名测试文件。

## 支持的输入

当前支持以下主要输入形态：

| 类型 | 示例 |
| --- | --- |
| 普通视频 | `BV1...`、`av123`、`https://www.bilibili.com/video/BV...` |
| 番剧 | `ep123`、`ss123`、`md123`、番剧播放页 |
| 国际版番剧 | `https://www.bilibili.tv/en/play/.../...` |
| 课程 | `https://www.bilibili.com/cheese/play/ss...`、`cheese/ep...` |
| b23 短链 | `https://b23.tv/...` |
| 收藏夹 | `https://space.bilibili.com/<mid>/favlist` |
| 合集/系列 | `channel/collectiondetail?sid=...`、`channel/seriesdetail?sid=...` |
| 空间投稿 | `https://space.bilibili.com/<mid>` |

空间投稿公开接口容易触发风控，当前 Go 版已有 WBI 到旧接口回退，但真实回归仍建议降频、使用临时目录、必要时带登录态。

## 常用工作流

普通视频下载：

```sh
./BB-DL --work-dir ~/Downloads/bilibili BV1...
```

先看信息，再下载指定流：

```sh
./BB-DL --only-show-info BV1...
./BB-DL --dfn-priority "1080P 高清,720P 高清" --encoding-priority "hevc,avc" BV1...
```

按终端中显示的序号手动选择视频流和音频流：

```sh
./BB-DL --interactive BV1...
./BB-DL -ia BV1...
./BB-DL --video-index 3 --audio-index 1 BV1...
```

下载多 P 的部分页面：

```sh
./BB-DL --select-page 1,2,LAST BV18dEr6DErw
```

只保存音视频临时文件，方便检查混流前产物：

```sh
./BB-DL --skip-mux --work-dir "$(mktemp -d)" BV1...
```

需要登录态时：

```sh
./BB-DL login
./BB-DL logintv
./BB-DL --only-show-info --use-tv-api BV1...
```

扫码成功后会在本地生成 `BBDown.data` 或 `BBDownTV.data`。命令只提示凭据文件已保存，不会把 `SESSDATA`、`bili_jct` 或 `access_token` 明文打印到终端。TV 登录会输出等待扫码、等待确认等状态；如果短时间内多次重试，请确认扫描的是最新生成的 `qrcode.png`，远程界面或聊天界面展示图片时尤其要避免扫到旧图缓存。

日志在真实终端中会按类型着色：时间戳为深灰，标题和轨道为青色，警告为黄色，错误为红色，下载进度为绿色。轨道列表和已选择的流会自动缩进，便于和带时间戳的阶段日志区分。需要纯文本输出时设置 `NO_COLOR=1`；在重定向或非 TTY 场景仍想强制彩色输出时设置 `BBDOWN_FORCE_COLOR=1`。

下载番剧、课程或国际版内容时，优先先跑 `--only-show-info`，确认接口、分 P 和轨道可用后再正式下载。

## 常用下载场景

查看可用流：

```sh
./BB-DL info BV1qt4y1X7TW
./BB-DL --only-show-info BV1qt4y1X7TW
```

选择分 P：

```sh
./BB-DL --select-page 1 BV1...
./BB-DL --select-page 1,2,LAST BV1...
./BB-DL --select-page 3-8 BV1...
./BB-DL --select-page ALL BV1...
```

`LAST`、`LATEST`、`NEW` 会替换为最后一个分 P 序号。URL 本身带 `?p=2` 且没有显式 `--select-page` 时，会自动选择对应分 P。

选择清晰度和编码：

```sh
./BB-DL --dfn-priority "1080P 高清,720P 高清" BV1...
./BB-DL --encoding-priority "hevc,avc,av1" BV1...
./BB-DL --video-ascending --audio-ascending BV1...
./BB-DL --video-index 3 --audio-index 1 BV1...
```

`--video-index` 和 `--audio-index` 使用可用流列表中打印的 0 起始序号。留空时仍按清晰度、编码和排序参数自动选择；非数字或负数序号会在下载启动前直接报错，显式序号越界会在拿到流列表后报错，避免下载到非预期轨道。只下载视频时会忽略 `--audio-index`，只下载音频时会忽略 `--video-index`。如果同时启用 `--audio-only` 和 `--video-only`，会按原版语义退回普通完整下载，此时两个序号都会重新作为有效选择参与校验。

API 通道：

```sh
./BB-DL --use-tv-api BV1...
./BB-DL --use-app-api BV1...
./BB-DL --use-intl-api "https://www.bilibili.tv/en/play/36571/30623439"
```

BiliPlus / 区域代理参数：

```sh
./BB-DL --use-intl-api \
  --host <play-api-host> \
  --ep-host <season-api-host> \
  --area th \
  --access-token <token> \
  --only-show-info <ep-or-url>
```

不要把真实 cookie、token 或代理密钥写进文档、提交记录和回归样本。

## only 与 skip 模式

只下载某类资源：

```sh
./BB-DL --video-only BV1...
./BB-DL --audio-only BV1...
./BB-DL --cover-only BV1...
./BB-DL --sub-only BV1...
./BB-DL --danmaku-only BV1...
```

跳过某些步骤：

```sh
./BB-DL --skip-mux BV1...
./BB-DL --skip-subtitle BV1...
./BB-DL --skip-cover BV1...
```

常见组合：

| 需求 | 命令 |
| --- | --- |
| 只保存音视频临时文件，不混流 | `--skip-mux` |
| 只保存字幕 | `--sub-only` |
| 只保存封面 | `--cover-only` |
| 下载弹幕 XML/ASS | `--download-danmaku --download-danmaku-formats "xml,ass"` |
| 不过滤 AI 字幕 | `--skip-ai=false` |

## 弹幕、字幕、封面

下载弹幕：

```sh
./BB-DL --download-danmaku --download-danmaku-formats "xml,ass" BV1...
./BB-DL --danmaku-only --download-danmaku-formats "xml,ass" BV1...
```

字幕默认会随下载流程尝试保存并混流；如果只想保存字幕：

```sh
./BB-DL --sub-only BV1...
./BB-DL --sub-only --skip-ai=false BV1...
```

封面默认用于混流封面写入；如果只想保存封面：

```sh
./BB-DL --cover-only BV1...
```

弹幕、字幕、封面等普通资源会按原版行为使用单线程流式下载；音视频媒体流才会按 `--multi-thread` 走分片下载。

## 命名模板

单 P 命名：

```sh
./BB-DL --file-pattern "<videoTitle>[<dfn>]" BV1...
```

多 P 命名：

```sh
./BB-DL --multi-file-pattern "<videoTitle>/[P<pageNumberWithZero>]<pageTitle>" BV1...
```

常用变量：

| 变量 | 含义 |
| --- | --- |
| `<videoTitle>` | 视频标题 |
| `<pageTitle>` | 分 P 标题 |
| `<pageNumber>` | 分 P 序号 |
| `<pageNumberWithZero>` | 补零分 P 序号 |
| `<bvid>` | BV 号 |
| `<aid>` | AV 号 |
| `<cid>` | CID |
| `<ownerName>` | UP 主名称 |
| `<ownerMid>` | UP 主 MID |
| `<dfn>` | 清晰度名称 |
| `<res>` | 分辨率 |
| `<fps>` | 帧率 |
| `<videoCodecs>` | 视频编码 |
| `<videoBandwidth>` | 视频带宽 |
| `<audioCodecs>` | 音频编码 |
| `<audioBandwidth>` | 音频带宽 |
| `<publishDate>` | 视频发布时间 |
| `<videoDate>` | 分 P 发布时间 |
| `<apiType>` | 播放接口类型 |

日期变量支持自定义格式：

```sh
./BB-DL --file-pattern "<publishDate:yyyy-MM-dd_HH-mm-ss>/<videoTitle>" BV1...
```

非法文件名字符会按原版行为替换为 `_`，路径分隔符统一按 `/` 处理。

同名文件处理：

```sh
# 在扩展名前追加流水号，例如 视频 (1).mp4、视频 (2).mp4
./BB-DL --file-exists-action rename BV1...

# 默认值；已有非空目标文件时跳过该文件
./BB-DL --file-exists-action skip BV1...

# 删除旧目标和断点续传状态后重新下载
./BB-DL --file-exists-action overwrite BV1...
```

`rename` 会先按页面输出基名检查主媒体、字幕、封面、弹幕和音视频临时分轨；任一同组文件存在时，整组统一改用首个可用的 ` (N)` 流水号。服务任务一次处理多个分 P 时也会在预测阶段预留本任务已经分配的基名，避免自定义模板相同时把多个页面都误报为同一路径。`skip` 保持默认兼容行为：非空目标文件跳过，缺失或零字节文件继续下载。`overwrite` 会清理目标文件、单线程 `.tmp`、aria2 `.aria2` 和当前目标对应的 `.vclip/.aclip` 分片，再重新获取资源；字幕与混流输出也会覆盖旧文件。多线程合并和清理只匹配当前目标基名，不会处理同目录其他任务的分片。

## 配置文件与认证文件

默认读取程序所在目录下的 `BBDown.config`，也可以通过 `--config-file` 指定：

```sh
./BB-DL --config-file ./BBDown.config BV1...
```

示例：

```txt
--work-dir ~/Downloads/bilibili
--dfn-priority "1080P 高清,720P 高清"
--encoding-priority "hevc,avc"
--video-index 3
--audio-index 1
--download-danmaku
--download-danmaku-formats "xml,ass"
```

配置合并规则：

| 规则 | 行为 |
| --- | --- |
| 命令行显式参数优先 | 同名参数出现在命令行时，配置文件值不会覆盖它 |
| 支持原版常用短参数 | 如 `-p`、`-F`、`-M`、`-q`、`-e` |
| 支持布尔字面值 | 配置文件可写 `--skip-ai false` |
| 支持等号写法 | 配置文件可写 `--file-pattern=<videoTitle>`，不会影响下一行配置 |

运行文件：

| 文件 | 用途 |
| --- | --- |
| `BBDown.config` | 默认参数 |
| `BBDown.data` | WEB cookie |
| `BBDownTV.data` | TV token |
| `BBDownApp.data` | APP token |
| `BBDown.archives` | 归档跳过记录 |

认证和敏感信息注意事项：

| 场景 | 建议 |
| --- | --- |
| WEB 登录 | 使用 `login` 扫码，生成 `BBDown.data`；适合普通 Cookie 场景 |
| TV 登录 | 使用 `logintv` 扫码，生成 `BBDownTV.data`；适合 `--use-tv-api`，确认后可用 `--use-tv-api --only-show-info <BV>` 验证 token 是否被加载 |
| APP token | 使用 `BBDownApp.data` 或 `--access-token`；真实 APP token 不要写入仓库，当前已验证文件读取链路和普通 APP API 解析，不代表会员权限 |
| 临时 token | 可以用 `--access-token` / `--token` 传入，但不要保存到文档、提交记录或回归样本 |
| Cookie | 可以用 `--cookie` / `-c` 覆盖文件 Cookie；命令历史里也会留下明文，使用后注意清理 |
| 终端输出 | 登录成功日志已脱敏；如果开启 shell 录屏或复制命令输出，仍要确认没有手动粘贴真实凭据 |

## 下载控制

指定工作目录：

```sh
./BB-DL --work-dir ~/Downloads/bilibili BV1...
```

使用 aria2c：

```sh
./BB-DL --use-aria2c --aria2c-args "--all-proxy=http://127.0.0.1:7890" BV1...
```

UPOS / PCDN：

```sh
./BB-DL --upos-host upos-sz-mirrorcoso1.bilivideo.com BV1...
./BB-DL --allow-pcdn BV1...
./BB-DL --force-http=false BV1...
```

归档跳过：

```sh
./BB-DL --save-archives-to-file BV1...
```

多 P 延迟：

```sh
./BB-DL --delay-per-page 5 --select-page ALL BV1...
```

## 参数索引

本索引按当前 CLI 实际注册项整理，对齐 `BB-DL --help`、`BB-DL help <命令>`、`BB-DL info --help` 和 `BB-DL serve --help`。默认下载命令与 `info` 共用通道、信息、选择、路径和网络类参数；下载命令额外支持混流、only / skip、弹幕、aria2c 和旧参数兼容；`help` / `version` 是顶层元命令，不会把 `help` 或 `version` 当成视频地址解析；`serve` 只支持服务监听参数。`login` 与 `logintv` 只支持帮助参数，不接受会改变登录行为的额外参数。

### 通道、信息与元参数

| 参数 | 别名 | 适用命令 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `--use-tv-api` | `--tv` | 下载、`info` | `false` | 使用 TV 播放接口解析 |
| `--use-app-api` | `--app` | 下载、`info` | `false` | 使用 APP 接口解析，优先 gRPC，失败后回退 REST |
| `--use-intl-api` | `--intl` | 下载、`info` | `false` | 使用国际版接口解析 |
| `--only-show-info` | `--info` | 下载、`info` | `false` | 只展示视频信息和可用流，不下载；`info` 子命令本身就是这个用途 |
| `--show-all` |  | 下载、`info` | `false` | 展示全部分 P 信息 |
| `--hide-streams` | `--hs` | 下载、`info` | `false` | 隐藏可用视频、音频和字幕流列表 |
| `--debug` |  | 下载、`info` | `false` | 输出调试日志并保存接口 JSON |
| `--version` |  | 下载、`info` | `false` | 显示当前版本号 |
| `--help` | `-h`、`-?` | 下载、`info`、`login`、`logintv`、`serve` | `false` | 显示当前命令帮助，帮助文本内置中文参数说明；`login` / `logintv` 使用该参数时不会启动扫码 |

### 选择、清晰度与排序参数

| 参数 | 别名 | 适用命令 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `--select-page <value>` | `-p` | 下载、`info` | 空 | 选择分 P，支持 `ALL`、`1,2`、`1-3`、`LAST` / `LATEST` / `NEW` |
| `--dfn-priority <value>` | `-q` | 下载、`info` | 空 | 清晰度优先级，逗号分隔，例如 `1080P 高清,720P 高清` |
| `--encoding-priority <value>` | `-e` | 下载、`info` | 空 | 编码优先级，逗号分隔；支持 `hevc`、`avc`、`av1` |
| `--video-ascending` |  | 下载 | `false` | 视频流按低优先级到高优先级排序 |
| `--audio-ascending` |  | 下载 | `false` | 音频流按低码率到高码率排序 |
| `--interactive` | `--ia` | 下载 | `false` | 交互式按序号选择视频流和音频流 |
| `--video-index <value>` |  | 下载 | 空 | 非交互式按可用流列表序号选择视频流，序号从 `0` 开始；留空自动选择；非数字或负数会提前报错 |
| `--audio-index <value>` |  | 下载 | 空 | 非交互式按可用流列表序号选择音频流，序号从 `0` 开始；留空自动选择；非数字或负数会提前报错 |

### 下载、混流与资源参数

| 参数 | 别名 | 适用命令 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `--multi-thread` | `--mt` | 下载、`info` | `true` | 启用 Range 多线程下载；`info` 接受该参数但不会实际下载 |
| `--use-aria2c` | `--aria2` | 下载 | `false` | 使用外部 `aria2c` 下载资源 |
| `--aria2c-args <value>` |  | 下载 | 空 | 传给 `aria2c` 的额外参数，例如 `"-x 16 -s 16"` |
| `--use-mp4box` |  | 下载 | `false` | 使用 `MP4Box` 混流；默认使用 `ffmpeg` |
| `--simply-mux` |  | 下载 | `false` | 简化混流，不写入 BBDown 额外 metadata |
| `--video-only` |  | 下载 | `false` | 只下载视频轨道，不下载音频 |
| `--audio-only` |  | 下载 | `false` | 只下载音频轨道，不下载视频 |
| `--danmaku-only` |  | 下载 | `false` | 只下载弹幕，不下载媒体 |
| `--cover-only` |  | 下载 | `false` | 只下载封面，不下载媒体 |
| `--sub-only` |  | 下载 | `false` | 只下载字幕，不下载媒体 |
| `--download-danmaku` | `--dd` | 下载 | `false` | 随媒体一起下载弹幕 |
| `--download-danmaku-formats <value>` | `--ddf` | 下载 | 空 | 弹幕保存格式，支持 `xml`、`ass`，可用逗号分隔 |
| `--skip-mux` |  | 下载 | `false` | 跳过混流，保留下载后的音视频分轨文件 |
| `--skip-subtitle` |  | 下载 | `false` | 跳过字幕下载和字幕混流 |
| `--skip-cover` |  | 下载 | `false` | 跳过封面下载 |
| `--skip-ai` |  | 下载、`info` | `true` | 跳过 AI 字幕 |
| `--save-archives-to-file` |  | 下载 | `false` | 把已下载 Aid 记录到 `BBDown.archives`，后续跳过已记录内容 |
| `--delay-per-page <value>` |  | 下载 | `0` | 多 P 下载时每页之间的等待秒数 |

### 路径、命名与外部程序参数

| 参数 | 别名 | 适用命令 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `--work-dir <value>` |  | 下载、`info` | 当前目录 | 下载工作目录 |
| `--file-pattern <value>` | `-F` | 下载、`info` | 空 | 单 P 输出文件名模板 |
| `--multi-file-pattern <value>` | `-M` | 下载、`info` | 空 | 多 P 输出文件名模板 |
| `--file-exists-action <value>` |  | 下载 | `skip` | 同名文件处理方式：`rename` 追加流水号、`skip` 跳过非空目标、`overwrite` 覆盖并重新下载 |
| `--ffmpeg-path <value>` |  | 下载 | 空 | 指定 `ffmpeg` 可执行文件路径 |
| `--mp4box-path <value>` |  | 下载 | 空 | 指定 `MP4Box` 可执行文件路径 |
| `--aria2c-path <value>` |  | 下载 | 空 | 指定 `aria2c` 可执行文件路径 |
| `--language <value>` |  | 下载、`info` | 空 | 写入混流音轨语言 metadata，例如 `jpn`、`eng`、`chi`；`info` 接受该参数但不会混流 |

### 网络、认证与接口参数

| 参数 | 别名 | 适用命令 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `--force-http` |  | 下载、`info` | `true` | 媒体下载地址尽量改用 `http` |
| `--force-replace-host` |  | 下载 | `true` | 替换可控 UPOS/PCDN host；设为 `false` 可保留原始 host |
| `--allow-pcdn` |  | 下载、`info` | `false` | 允许使用 PCDN 下载地址 |
| `--upos-host <value>` |  | 下载 | 空 | 指定替换用的 UPOS host |
| `--user-agent <value>` | `--ua` | 下载、`info` | 空 | 自定义请求 User-Agent |
| `--cookie <value>` | `-c` | 下载、`info` | 空 | 自定义 B 站 Cookie；建议优先使用 `login` 保存登录态 |
| `--access-token <value>` | `--token` | 下载、`info` | 空 | 自定义 access token，用于 TV / APP / 国际版等接口 |
| `--host <value>` |  | 下载、`info` | `api.bilibili.com` | 自定义 BiliPlus / 播放接口 host |
| `--ep-host <value>` |  | 下载、`info` | `api.bilibili.com` | 自定义番剧接口 host |
| `--tv-host <value>` |  | 下载、`info` | `api.snm0516.aisee.tv` | 自定义 TV 接口 host |
| `--area <value>` |  | 下载、`info` | 空 | 区域参数，用于 BiliPlus / 代理解析 |
| `--config-file <value>` |  | 下载、`info` | 空 | 指定配置文件，命令行显式参数会覆盖配置 |

### 旧参数兼容

| 参数 | 别名 | 适用命令 | 默认值 | 当前行为 |
| --- | --- | --- | --- | --- |
| `--only-hevc` | `--hevc` | 下载 | `false` | 转换为 `--encoding-priority hevc` 并提示 |
| `--only-avc` | `--avc` | 下载 | `false` | 转换为 `--encoding-priority avc` 并提示 |
| `--only-av1` | `--av1` | 下载 | `false` | 转换为 `--encoding-priority av1` 并提示 |
| `--add-dfn-subfix` |  | 下载 | `false` | 转换为包含 `<dfn>` 的文件名模板并提示 |
| `--no-padding-page-num` |  | 下载 | `false` | 转换为 `<pageNumber>` 模板并提示 |
| `--bandwith-ascending` |  | 下载 | `false` | 转换为视频和音频升序并提示 |
| `--aria2c-proxy <value>` |  | 下载 | 空 | 转换为 `aria2c` 的 `--all-proxy` 参数并提示 |

### `serve` 子命令参数

| 参数 | 别名 | 适用命令 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `--listen <value>` | `-l` | `serve` | `http://0.0.0.0:23333` | 服务监听地址，支持 `http://host:port` 或 `host:port` |
| `--help` | `-h`、`-?` | `serve` | `false` | 显示 `serve` 参数帮助，帮助文本内置中文参数说明 |

## 服务模式

启动：

```sh
./BB-DL serve --listen http://0.0.0.0:23333
```

接口：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/add-task` | 添加下载任务 |
| `GET` | `/get-tasks/` | 获取运行中和已完成任务 |
| `GET` | `/get-tasks/running` | 获取运行中任务 |
| `GET` | `/get-tasks/finished` | 获取已完成任务 |
| `GET` | `/get-tasks/{aid}` | 按 Aid 获取任务 |
| `GET` | `/remove-finished` 或 `/remove-finished/` | 清空已完成任务 |
| `GET` | `/remove-finished/failed` | 清空失败任务 |
| `GET` | `/remove-finished/{aid}` | 删除指定已完成任务 |

服务接口默认允许任意来源、方法和请求头的 CORS 访问，浏览器预检 `OPTIONS` 会返回 204。

添加任务：

```sh
curl -X POST http://127.0.0.1:23333/add-task \
  -H 'Content-Type: application/json' \
  -d '{"Url":"BV1qt4y1X7TW","SelectPage":"1","WorkDir":"/tmp/BB-DL","CallBackWebHook":"http://127.0.0.1:8080/callback"}'
```

`/add-task` 与原版保持一致：HTTP 200 表示任务已接收，响应体不返回任务 JSON。任务状态请继续查询 `/get-tasks/running`、`/get-tasks/finished` 或 `/get-tasks/{aid}`。

JSON 字段名沿用 `MyOption`，例如 `UseTvApi`、`UseAppApi`、`UseIntlApi`、`DfnPriority`、`EncodingPriority`、`SelectPage`、`WorkDir`、`UseAria2c`、`VideoIndex`、`AudioIndex`、`FileExistsAction`。`FileExistsAction` 支持 `rename`、`skip`、`overwrite`，默认 `skip`；非法值会在 `/add-task` 阶段返回 HTTP 400。`VideoIndex` / `AudioIndex` 与 CLI 一样使用 0 起始序号；非数字或负数会在 `/add-task` 阶段直接返回 HTTP 400，越界会在任务拿到真实流列表后作为失败任务进入 finished。只下载音频时会忽略并清理 `VideoIndex`，只下载视频时会忽略并清理 `AudioIndex`，`OnlyShowInfo` 会清理两个序号，避免未使用字段进入子进程命令；如果 `AudioOnly` 和 `VideoOnly` 同时为 `true`，服务端会先按兼容逻辑退回普通下载，再校验和转发两个序号。

服务模式已用真实任务覆盖 `UseTvApi`、`UseAppApi` 和 `UseIntlApi` 的 CoverOnly 分支：TV API 样本会加载本地 TV token，APP API 样本使用临时 `BBDownApp.data`，INTL 样本可直接提交国际版播放页 URL。记录回归时只写凭据来源类型和执行日期，不写 `BBDownApp.data` 或 token 内容。

only 模式也可以通过服务提交，例如只下载 XML/ASS 弹幕：

```sh
curl -X POST http://127.0.0.1:23333/add-task \
  -H 'Content-Type: application/json' \
  -d '{"Url":"BV1qt4y1X7TW","WorkDir":"/tmp/BB-DL","DanmakuOnly":true,"DownloadDanmakuFormats":"xml,ass","SelectPage":"1"}'
```

服务端会把任务转成当前二进制的子进程下载命令，因此同一个 JSON 字段和 CLI 参数基本是一一对应关系。布尔字段使用 JSON 布尔值，字符串字段使用字符串，例如：

```json
{
  "Url": "BV1qt4y1X7TW",
  "SelectPage": "1,2,LAST",
  "WorkDir": "/tmp/BB-DL",
  "UseTvApi": true,
  "DfnPriority": "1080P 高清,720P 高清",
  "EncodingPriority": "hevc,avc",
  "DownloadDanmaku": true,
  "DownloadDanmakuFormats": "xml,ass",
  "CallBackWebHook": "http://127.0.0.1:8080/callback"
}
```

任务字段：

| 字段 | 说明 |
| --- | --- |
| `Aid` | 任务主 Aid |
| `Progress` | 解析、普通 DASH 下载、FLV 分段、混流/合并等日志阶段估算 |
| `DownloadSpeed` | server 子进程下载增量事件优先统计；文件轮询仅在事件缺失或外部下载器场景下兜底估算 |
| `TotalDownloadedBytes` | server 子进程下载增量/完成事件优先统计，文件轮询按任务启动基线后的增长量兜底；封面/弹幕/字幕 only 和单轨音/视频任务成功结束时会按最终输出文件增长量校正 |
| `SavePaths` | 预测或最终输出路径；`rename` 会返回解析流水号后的实际路径；`OnlyShowInfo` 或 `SaveArchivesToFile` 归档命中跳过任务不下载文件，因此成功时也返回 `[]` |
| `IsSuccessful` | 子进程退出状态 |
| `ErrorStage` | 失败阶段；`resolve` 表示输入/Aid 解析失败，`info` 表示视频信息或分 P 信息失败，`download` 表示子进程下载/混流阶段失败；成功或暂无错误时为 `null` |
| `ErrorMessage` | 失败任务的短错误摘要；成功或暂无错误时为 `null` |

回调 webhook 收到的 body 与任务字段一致。回调只作为任务结束通知，非法 URL、发送失败或 10 秒内没有响应都不会影响任务进入 finished。`Progress`、`DownloadSpeed` 和 `TotalDownloadedBytes` 是父进程通过子进程日志、隐藏下载增量/完成事件和下载文件增长量估算的结果，适合做任务看板，不建议当作精确计费或审计数据；隐藏事件只在服务父子进程之间传递，不会出现在 `server.log`。普通 DASH 任务会按视频下载、音频下载、下载完成和混流阶段推进页内进度，FLV 任务会按分段下载和合并阶段推进页内进度；最终仍以 `任务完成` 日志置为 100%。`OnlyShowInfo` 任务只解析并展示流信息，成功 finished 时 `SavePaths=[]`、`TotalDownloadedBytes=0`；开启 `SaveArchivesToFile` 且命中 `BBDown.archives` 的任务会在下载前跳过，对应分 P 不会出现在 `SavePaths`。如果最终输出文件在任务启动前已经存在且非空，CLI 会跳过下载，服务 finished 会保留对应 `SavePaths`，但 `TotalDownloadedBytes=0`。封面、弹幕、字幕这类资源 only 任务，以及 `AudioOnly` / `VideoOnly` 这类单轨任务，会在成功结束时用最终输出文件相对任务启动基线的增长量校正 `TotalDownloadedBytes`，避免短任务最后一次文件增长被轮询间隔漏掉；字幕 only 任务的 `SavePaths` 会指向实际 `.srt` / `.ass` 产物，AudioOnly 任务的 `SavePaths` 会指向最终 `.m4a` 产物，VideoOnly 任务的 `SavePaths` 会指向最终 `.mp4` 产物。普通 DASH 混流任务的 `SavePaths` 指向最终 `.mp4`；普通下载附带 `DownloadDanmaku` 时，`SavePaths` 仍只指向主媒体文件，弹幕 XML/ASS 作为伴生产物保留在工作目录，`TotalDownloadedBytes` 会计入弹幕 XML 下载字节。DASH `--skip-mux` 任务的 `SavePaths` 会指向保留的 `.video.mp4` / `.audio.m4a` 文件，FLV `--skip-mux` 任务会指向 `.flv-merged.mp4` 文件；`TotalDownloadedBytes` 仍按下载增长量估算，不保证等于本地混流或合并后的文件体积。无效输入、信息抓取失败或子进程失败会进入 finished failed；`ErrorStage` 可用于任务看板粗分失败阶段，`ErrorMessage` 会截断并脱敏 cookie/token 等认证信息，只用于排障提示。

## 调试与回归

打开调试：

```sh
./BB-DL --debug --only-show-info BV1...
```

`--debug` 会输出运行参数，并把播放接口原始 JSON 写入 `debug_*.json`。这些文件可能包含接口返回细节，提交前需要确认没有敏感信息。

常用低副作用回归：

```sh
go test ./...
go run ./cmd/bbdown info BV1qt4y1X7TW
go run ./cmd/bbdown --only-show-info --select-page 1,2,LAST BV18dEr6DErw
work_dir="$(mktemp -d)"
go run ./cmd/bbdown --work-dir "$work_dir" --cover-only BV1qt4y1X7TW
```

更多样本见 `docs/regression-samples.md`。

## 常见故障

| 现象 | 排查方向 |
| --- | --- |
| `ffmpeg` 找不到 | 先用 `ffmpeg -version` 确认可执行；CLI 可用 `--ffmpeg-path ffmpeg` 或 `--ffmpeg-path /opt/homebrew/bin/ffmpeg`，报错会列出对应参数和已检查位置；桌面版会探测常见目录并读取 zsh 登录环境，仍失败时在 FFmpeg 输入框填写完整路径；旧偏好中的裸命令名会自动解析为绝对路径 |
| 杜比视界混流失败 | 升级 `ffmpeg` 到 5.0 以上，或安装 `MP4Box` 并加 `--use-mp4box` |
| 弹幕 XML 下载报 `missing content length` | 当前版本的普通资源已改为单线程流式下载；若仍遇到此错误，先确认运行的是最新构建 |
| 空间投稿返回 HTML / `-352` / `-799` | B 站公开接口风控，降低频率、带登录态或换样本；不要把失败当作复刻逻辑一定坏了 |
| TV 登录扫码后长时间没反应 | 确认手机端已经点确认，并确认扫的是最新二维码；命令会周期性输出 `86039`/`86090` 等状态，成功后只提示 `BBDownTV.data` 已保存，不打印 token |
| 只有 `accept_quality` 里出现 126 但流列表没有杜比视界 | 这只是候选清晰度，必须实际 `dash.video.id=126` 才算拿到杜比视界轨道 |
| 国际版或区域内容无流 | 先尝试 `--use-intl-api --only-show-info`，BiliPlus 场景需检查 `--host`、`--ep-host`、`--area`、`--access-token` |
| 配置文件没有生效 | 确认 `--config-file` 指向正确文件；命令行显式参数会覆盖配置文件同名参数 |
| 输出路径和预期不同 | 用 `--debug` 查看 `Format Before` / `Format After`，检查单 P/多 P 模板和 URL `?p=` 选择 |
| 服务模式速度不稳定 | HTTP 下载已通过子进程增量事件实时更新；aria2c、极短任务和本地混流阶段仍可能依赖完成事件或文件轮询，因此看板速度可能有抖动 |
| 默认下载没有选择流提示 | 默认按排序后的第一条视频/音频流自动下载；要按序号选择，终端里用 `--interactive` / `-ia`，脚本或桌面端用 `--video-index` / `--audio-index` |
