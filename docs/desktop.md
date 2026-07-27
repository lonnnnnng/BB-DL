# BB-DL 跨平台桌面版

更新时间：2026-07-27 10:09:10 北京时间

桌面应用名为 `BB-DL`，当前使用 Wails 2 + React 19 + TypeScript + Vite。macOS、Windows、Linux 共用同一套 React 界面和 Go 任务后端，并使用系统 WebView 渲染。桌面层只负责任务编排、参数设置、登录二维码、日志、进度和文件管理；解析、登录、下载和混流继续交给同版本 `BB-DL-cli` helper，避免桌面版与命令行行为分叉。

旧 Fyne 入口 `cmd/bbdown-desktop` 暂时保留作行为参考和兼容回归，不再用于当前桌面 Release 构建。

## 当前功能

| 功能 | 状态 |
| --- | --- |
| URL / BV / av / ep 输入、保存目录选择 | 已支持 |
| 下载、仅查看、仅视频、仅音频、仅封面、仅字幕、仅弹幕 | 已支持 |
| WEB、TV、APP、国际版通道 | 已支持 |
| 分 P、清晰度、编码、视频流、音频流选择 | 已支持；流列表来自当前任务日志，留空为自动选择 |
| 弹幕、跳过字幕、跳过封面、跳过混流、aria2c | 已支持 |
| 同名文件追加流水号、跳过、覆盖 | 已支持；默认跳过，可在新建任务或设置抽屉修改 |
| 完整 CLI 额外参数 | 已支持 |
| WEB / TV 二维码登录 | 已支持；二维码直接以 data URL 发送给前端，不暴露运行目录路径 |
| 登录会话隔离 | 已支持；登录使用专用对话框和临时进程，不进入下载任务、任务汇总或 `tasks.json` |
| 任务队列、开始、停止、重试、批量重试、自动续队 | 已支持；批量重试保留原有等待任务顺序 |
| 实时日志、日志搜索、跟随末尾、分类着色 | 已支持；前端最多渲染最近 4000 行，完整日志仍由后端保存 |
| 百分比、下载字节、当前/平均速度、耗时 | 已支持 |
| 任务历史恢复 | 已支持；最多 200 个任务，每个任务历史日志最多 256 KiB |
| 完成文件打开、定位、复制单个/全部路径 | 已支持 |
| FFmpeg、MP4Box、aria2c 路径与工具检测 | 已支持 |
| 复制任务命令、日志、`doctor --json` | 已支持 |
| 浅色、深色、跟随系统主题 | 已支持 |
| 退出运行中任务确认 | 已支持 |

## 界面结构

- 顶栏：版本、队列汇总、WEB/TV 登录、打开目录、主题和设置入口。
- 左栏：新建任务、下载模式、接口、流选择和额外参数。
- 中栏：任务筛选、搜索、队列主操作和任务进度。
- 右栏：选中任务指标、运行日志、登录二维码和完成文件。
- 底栏：固定显示全局操作结果与构建时间，不与任务状态混用。

窗口默认 `1440x900`，最小 `1024x680`。窄窗口通过 CSS 响应式规则调整布局；图标按钮使用 Lucide 图标并提供 tooltip/可访问名称。

## 开发环境

基础依赖：

- Go 1.26 或以上。
- Node.js 22 或以上。
- Wails CLI 2.13.0。
- 目标系统可用的 WebView 开发环境。

安装前端依赖和 Wails CLI：

```sh
npm ci --prefix frontend
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

启动 Wails 开发模式：

```sh
wails dev
```

只调试 React 界面时可运行：

```sh
npm run dev --prefix frontend
```

此时前端使用 `frontend/src/mock.ts` 的演示任务，不会调用 Go 后端或真实下载。

## 构建当前平台

```sh
./scripts/build-desktop.sh
```

脚本会先由 Wails 构建桌面程序，再构建同版本 CLI helper，输出到 `dist/desktop`：

| 平台 | 输出 |
| --- | --- |
| macOS | `BB-DL_vX.Y.Z_desktop_darwin_<arch>.zip`，顶层只有 `BB-DL.app` |
| Windows | `BB-DL_vX.Y.Z_desktop_windows_<arch>.zip`，目录内只有 `BB-DL.exe` 和 `BB-DL-cli.exe` |
| Linux | `BB-DL_vX.Y.Z_desktop_linux_<arch>.tar.gz`，目录内只有 `BB-DL` 和 `BB-DL-cli` |

Release 包不会包含 README 或 `docs/`。macOS helper 位于 `BB-DL.app/Contents/Resources/BB-DL-cli`，应用会对 helper 和整个 `.app` 做 ad-hoc 签名。

Wails 桌面应用应在目标系统原生构建：

- macOS 使用系统 WebKit；构建脚本固定 `MACOSX_DEPLOYMENT_TARGET=11.0`，主程序和 helper 都校验为 `minos 11.0`。
- Windows 使用系统 WebView2；CI 通过 MSYS2/Mingw 提供 Go CGO 编译和归档工具。
- Ubuntu 24.04 安装 `libgtk-3-dev`、`libwebkit2gtk-4.1-dev`、`libgl1-mesa-dev`、`xorg-dev`、`libxkbcommon-dev`，构建和测试使用 `webkit2_41` tag。

推送 `v*` tag 或手动触发 `release.yml` 后，三套目标系统 runner 会分别执行测试、Wails 构建和 `scripts/verify-artifacts.sh` 包结构校验。`v1.0.12` 已由 macOS、Windows、Linux runner 全部构建并上传，三个桌面包下载复验通过。

## 后端与事件

Wails 绑定入口是 `app.go`，任务生命周期由 `task_manager.go` 管理，参数/持久化/进度协议位于 `desktop_core.go`，工具和系统操作位于 `desktop_tools.go`。

前端初始化调用 `GetBootstrap()`，之后主要通过事件增量更新：

| 事件 | 用途 |
| --- | --- |
| `task:added` / `task:updated` / `task:finished` | 单任务创建、运行态和终态更新 |
| `tasks:reset` | 运行状态切换后刷新全队列操作权限 |
| `task:log-reset` / `task:log` | 开始执行时重置日志，运行中追加新行 |
| `login:updated` / `login:qrcode` / `login:finished` | 独立更新登录状态、二维码和终态，不进入下载任务列表 |
| `queue:summary` | 刷新等待、运行、完成、失败和停止数量 |
| `app:status` | 刷新底部全局状态 |

后端同一时间只运行一个 helper。`RetryFailedTasks` 只把失败/停止任务的副本追加到队尾，自动队列始终通过 `StartNextPending` 启动最早等待任务。启动前工具预检、目录创建或 helper 准备失败也会写入失败日志，并在开启自动队列时继续下一条任务。

同名文件策略保存在 `preferences.json` 和任务历史中，并映射为 `--file-exists-action`。重试、历史恢复和“填入下载表单”都会保留任务原值；旧偏好或旧任务缺少该字段时自动回退为 `skip`。高级用户仍可在额外参数中追加同名 CLI 参数，额外参数位于表单参数之后，因此最后一个值生效。

## 进度与日志协议

- helper stdout/stderr 普通行进入完整任务日志，并通过 `task:log` 实时发送给前端。
- `__BBDOWN_GO_SERVER_TRANSFER__` 隐藏事件不会显示在日志中；后端按资源路径和字节增量计算已下载字节与速度。
- 未收到隐藏事件时，每秒扫描保存目录相对任务启动基线的下载文件增长量作为 aria2c 等外部下载器兜底。
- 百分比从中文 CLI 日志识别解析、视频、音频、附加音轨、分 P 完成、混流和最终完成阶段，且只单调前进。
- 任务结束时终态日志先实时发送，再广播任务终态，避免界面漏掉最后一行。

## 流选择

使用“仅查看”任务解析目标视频，日志会列出视频流和音频流。选中该任务后，左侧“视频流”“音频流”会显示可选项；选中的前导序号分别映射为 `--video-index` 和 `--audio-index`。选择“自动”表示不传显式序号。

`仅音频`会清理视频序号，`仅视频`会清理音频序号。非法或负数序号会在创建任务时失败，越界序号会在 helper 取得真实流列表后失败。

## 外部工具

默认完整下载需要 FFmpeg；MP4Box 和 aria2c 按任务参数启用。桌面端在创建和启动任务时按以下范围查找：

1. 表单或额外 CLI 参数中的显式路径。
2. 当前目录、桌面程序目录和 `.app/Contents/Resources`。
3. `BBDOWN_GO_TOOL_DIRS` 和 `PATH`。
4. Homebrew、MacPorts 等系统常见目录。
5. macOS zsh 登录环境中的 `command -v` 结果。

工具路径为空或填写“自动”时不写入 helper 参数。完整下载找不到 FFmpeg 时，界面会保留失败日志和可执行的安装提示；macOS 可使用 `brew install ffmpeg`。

## 运行文件位置

| 平台 | 目录 |
| --- | --- |
| macOS | `~/Library/Application Support/BBDown Go` |
| Windows | `%AppData%\BBDown Go` |
| Linux | `~/.config/BBDown Go` |

`BB-DL` 继续使用原 `BBDown Go` 数据目录，保证从旧版升级后任务历史、偏好和登录态可以直接沿用。该兼容目录保存复制后的 `BB-DL-cli` helper、`preferences.json`、`tasks.json`、`BBDown.data`、`BBDownTV.data`、`BBDownApp.data`、`BBDown.config`、`BBDown.archives` 和临时 `qrcode.png`。这些文件可能包含登录凭据，不应提交到 Git。

测试或排障可设置 `BBDOWN_GO_RUNTIME_DIR` 指向临时目录。helper 查找优先级为 `BBDOWN_GO_HELPER`、桌面程序同目录、macOS Resources、当前目录；复制到运行目录时会比较文件内容，避免升级后继续使用旧 helper。

## 当前验证

2026-07-26 已完成：

- `go test ./...`。
- `npm run lint --prefix frontend` 和 `npm run build --prefix frontend`。
- macOS arm64 Wails `.app` 构建、ad-hoc 签名、包结构和 `minos 11.0` 校验。
- `https://www.bilibili.com/video/BV1J9EB6xEAB` 未登录真实解析，识别 6 条视频流和 3 条音频流。
- 1440x900、1040x720 的浅色/深色界面，以及设置抽屉和控件文字溢出检查。

`v1.0.12` 的 Windows/Linux Wails 桌面包已在真实 GitHub runner 完成测试、构建、包结构校验和上传；发布后下载复验确认包内分别只有 `BB-DL.exe` / `BB-DL` 与同版本 CLI helper。
