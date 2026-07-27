# BB-DL Windows 验证说明

更新时间：2026-07-27 10:09:10 北京时间

旧报告记录的是 Fyne 桌面端和 `v1.0.8` 开发阶段，已经不代表当前实现。当前 Windows 桌面端使用 Wails 2 + React 19 + TypeScript，正式产物由 GitHub Actions 的 `windows-latest` runner 原生构建。

## 当前结论

| 项目 | 状态 |
| --- | --- |
| Windows CLI | 支持 amd64 / arm64，单文件 `BB-DL.exe` |
| Windows 桌面端 | 支持 amd64，Wails 原生构建 |
| 桌面 UI | 与 macOS、Linux 共用 React 前端和 Go 任务后端 |
| 下载能力 | 桌面端调用同版本 `BB-DL-cli.exe`，与 CLI 行为一致 |
| 中文帮助 | `BB-DL.exe --help` 和各子命令 help 均已覆盖 |
| 外部工具 | 支持自动查找和显式设置 FFmpeg、MP4Box、aria2c |
| 发布包结构 | 只含 `BB-DL.exe` 与 `BB-DL-cli.exe`，不含 README 或 docs |

## 发布资产

```text
BB-DL_v1.0.12_cli_windows_amd64.zip
BB-DL_v1.0.12_cli_windows_arm64.zip
BB-DL_v1.0.12_desktop_windows_amd64.zip
```

CLI 包只有一个 `BB-DL.exe`。桌面包顶层目录中只有：

```text
BB-DL.exe
BB-DL-cli.exe
```

## CI 构建路线

Windows release job 执行以下步骤：

1. 安装 Go、Node.js 22、前端依赖和 Wails CLI 2.13.0。
2. 使用 MSYS2 MINGW64 安装 GCC、zip 和 unzip。
3. 运行 Wails 根包测试。
4. 原生构建 `BB-DL.exe` 和 `BB-DL-cli.exe`。
5. 通过 `scripts/verify-artifacts.sh desktop` 检查归档路径安全、文件数量和发布包内容。
6. 上传同一 tag 的 Windows 桌面资产。

独立 CLI 使用 `CGO_ENABLED=0 GOOS=windows` 交叉编译，因此无需 Wails、WebView2 开发环境或 GCC。桌面端依赖 Windows WebView2，必须在 Windows runner 或 Windows 本机原生构建。

## 用户运行要求

- Windows 10/11。
- 桌面端需要系统 WebView2 Runtime；现代 Windows 10/11 通常已预装。
- 默认完整下载需要 FFmpeg。可把 `ffmpeg.exe` 加入 `PATH`，也可在设置中选择完整路径。
- 使用 `--use-mp4box` 时需要 MP4Box；使用 `--use-aria2c` 时需要 aria2c。
- CLI 使用 `--work-dir` 时，外部工具建议填写绝对路径，避免工作目录切换影响相对路径解析。

## 本地验证命令

```sh
go test ./cmd/bbdown ./internal/bbdown
npm ci --prefix frontend
npm run lint --prefix frontend
npm run build --prefix frontend
VERSION=v1.0.12 OUT_DIR="$PWD/dist/desktop" bash scripts/build-desktop.sh
bash scripts/verify-artifacts.sh desktop dist/desktop/BB-DL_*_desktop_windows_amd64.zip
```

线上低副作用回归样本：

```sh
BB-DL.exe info BV1J9EB6xEAB
BB-DL.exe --only-show-info BV1J9EB6xEAB
BB-DL.exe --work-dir <临时目录> --cover-only BV1J9EB6xEAB
```

会员、杜比视界、付费内容和真实代理场景受账号或配置限制，未验证时不应标记为通过。
