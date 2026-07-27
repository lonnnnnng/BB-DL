# BB-DL

`BB-DL` 是对 [nilaoda/BBDown](https://github.com/nilaoda/BBDown) 的 Go 复刻，提供跨平台命令行工具和 Wails 桌面下载器。项目尽量保持 BBDown 的参数与下载行为，同时用 Go 单二进制降低运行时依赖。

文档入口：

- 使用说明和完整参数清单：[docs/usage.md](docs/usage.md)
- 跨平台桌面版：[docs/desktop.md](docs/desktop.md)
- 技术实现路线：[docs/technical-implementation.md](docs/technical-implementation.md)
- 复刻进度：[docs/replica-progress.md](docs/replica-progress.md)
- 线上样本回归：[docs/regression-samples.md](docs/regression-samples.md)

## 当前状态

- 已覆盖默认下载、`info`、`login`、`logintv`、`doctor`、`serve`。
- 已覆盖普通视频、多 P、互动视频、番剧、课程、国际版、收藏夹、合集、系列、空间投稿等入口。
- 已覆盖 DASH/FLV、AVC/HEVC/AV1、普通音频、杜比/E-AC-3、FLAC、背景音、角色音、字幕、AI 字幕过滤、弹幕 XML/ASS、封面、章节、ffmpeg/MP4Box 混流。
- 服务模式已支持任务 API、callback webhook、输出路径预测和按页/FLV 片段推进进度；封面、弹幕 only、字幕 only、AudioOnly/VideoOnly、callback、FLV SkipMux、DASH SkipMux、DASH 完整混流、多 P CoverOnly、TV/APP/INTL API CoverOnly 和无效输入失败路径已有真实任务回归，下载速度和已下载字节优先由子进程下载增量事件更新，文件轮询仅作兜底。
- 跨平台桌面版已迁移到 Wails 2 + React 19 + TypeScript，macOS/Windows/Linux 共用同一套前端与 Go 任务后端，支持任务创建、监控、重试、完成文件管理、任务历史恢复、复制命令/日志/诊断、工具检测、主题切换和常用参数设置，下载能力复用内置 CLI helper。

## 依赖

- Go 1.26 或以上。
- `ffmpeg`：默认混流依赖。
- `MP4Box`：使用 `--use-mp4box` 或遇到低版本 ffmpeg 处理杜比视界时需要。
- `aria2c`：仅在使用 `--use-aria2c` 时需要。

依赖查找顺序与原版接近：显式参数、当前工作目录、程序所在目录、`BBDOWN_GO_TOOL_DIRS`、`PATH`、系统常见工具目录；macOS 下还会读取 zsh 登录环境中的 `command -v` 结果，尽量覆盖 Finder / GUI 启动时 PATH 过短的问题。桌面版会把 `.app/Contents/Resources`、helper 源目录和运行目录传给内置 CLI helper，避免 helper 被复制到用户配置目录后看不到随 app 放置的工具。

## 构建

```sh
go build -o BB-DL ./cmd/bbdown
```

验证：

```sh
go test ./...
```

构建当前平台桌面版：

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
npm ci --prefix frontend
./scripts/build-desktop.sh
```

桌面端开发调试：

```sh
wails dev
```

推送 `v*` tag 时，Release 会生成 `BB-DL_vX.Y.Z_cli_<os>_<arch>` CLI 包和 `BB-DL_vX.Y.Z_desktop_<os>_<arch>` 桌面包。macOS 桌面包内是 `BB-DL.app`，Windows/Linux 桌面包内是 `BB-DL` 和同版本 `BB-DL-cli` helper；所有发布包均不包含 README 或 docs。

## 基本用法

```sh
./BB-DL "https://www.bilibili.com/video/BV..."
./BB-DL download "https://www.bilibili.com/video/BV..."
./BB-DL help
./BB-DL help download
./BB-DL version
./BB-DL info "BV..."
./BB-DL login
./BB-DL logintv
./BB-DL doctor
```

常用参数：

```sh
./BB-DL --select-page 1,3,LAST "BV..."
./BB-DL --dfn-priority "1080P 高清,720P 高清" --encoding-priority "hevc,avc" "BV..."
./BB-DL --interactive "BV..."
./BB-DL --video-index 3 --audio-index 1 "BV..."
./BB-DL --file-exists-action rename "BV..."
./BB-DL --download-danmaku --download-danmaku-formats "xml,ass" "BV..."
./BB-DL --sub-only "BV..."
./BB-DL --use-aria2c --aria2c-args "--all-proxy=http://127.0.0.1:7890" "BV..."
```

常用 only / skip 模式：

- `--video-only`
- `--audio-only`
- `--danmaku-only`
- `--cover-only`
- `--sub-only`
- `--skip-mux`
- `--skip-subtitle`
- `--skip-cover`

## 配置和登录文件

程序会在可执行文件所在目录和当前目录读取 BBDown 兼容的运行文件：

- `BBDown.config`：默认参数配置，命令行显式参数优先。
- `BBDown.data`：WEB 登录 Cookie。
- `BBDownTV.data`：TV 登录 token。
- `BBDownApp.data`：APP token。
- `BBDown.archives`：`--save-archives-to-file` 使用的下载归档。

示例 `BBDown.config`：

```txt
--work-dir ~/Downloads/bilibili
--dfn-priority "1080P 高清,720P 高清"
--encoding-priority "hevc,avc"
--file-exists-action rename
--download-danmaku
--download-danmaku-formats "xml,ass"
```

## 服务模式

启动：

```sh
./BB-DL serve --listen http://0.0.0.0:23333
```

接口：

- `POST /add-task`
- `GET /get-tasks/`
- `GET /get-tasks/running`
- `GET /get-tasks/finished`
- `GET /get-tasks/{aid}`
- `GET /remove-finished` 或 `GET /remove-finished/`
- `GET /remove-finished/failed`
- `GET /remove-finished/{aid}`

添加任务：

```sh
curl -X POST http://127.0.0.1:23333/add-task \
  -H 'Content-Type: application/json' \
  -d '{"Url":"BV1xx","CallBackWebHook":"http://127.0.0.1:8080/callback"}'
```

JSON 字段名沿用 `MyOption`，例如 `UseTvApi`、`UseAppApi`、`UseIntlApi`、`DfnPriority`、`EncodingPriority`、`SelectPage`、`WorkDir`、`UseAria2c`、`FileExistsAction`。

## 发布构建

本地跨平台打包：

```sh
./scripts/build-release.sh
./scripts/build-desktop.sh
./scripts/build-local-artifacts.sh
```

默认生成：

- macOS: `darwin/amd64`, `darwin/arm64`
- Linux: `linux/amd64`, `linux/arm64`
- Windows: `windows/amd64`, `windows/arm64`

`build-release.sh` 输出 CLI 跨平台包到 `dist/release`，每个压缩包只包含对应平台的 CLI 应用程序；`build-desktop.sh` 输出当前系统桌面包到 `dist/desktop`。如果 GitHub Actions 额度不足，直接运行 `./scripts/build-local-artifacts.sh`，它会把 CLI 包和当前平台桌面包写到 `dist/local` 并执行 `scripts/verify-artifacts.sh`，确认包内没有 README/docs 或多余文件。手动上传可使用：

```sh
gh release upload vX.Y.Z dist/local/release/* dist/local/desktop/* --repo lonnnnnng/BB-DL --clobber
```

推送 `v*` tag 时，`.github/workflows/release.yml` 会运行测试、打包、执行 `scripts/verify-artifacts.sh` 校验包内没有 README/docs 或多余文件，然后发布 GitHub Release；桌面版会在 macOS、Windows、Linux runner 上原生构建并上传独立桌面包，包内只包含桌面程序及运行所需 helper。若某个平台资产临时失败，可手动触发该 workflow 并填写 `release_tag` 补传同版本资产。

## 复刻进度维护

每次补齐 BBDown 原版能力后，同步更新 [docs/replica-progress.md](docs/replica-progress.md)，并把真实样本结果记入 [docs/regression-samples.md](docs/regression-samples.md)。
