# BB-DL 复刻进度

更新时间：2026-07-27 10:09:10 北京时间

本文只记录当前代码快照，不保留逐版本流水账。原始实现参考 [nilaoda/BBDown](https://github.com/nilaoda/BBDown)，Go 复刻仓库为 [lonnnnnng/BB-DL](https://github.com/lonnnnnng/BB-DL)。

## 当前基线

| 项目 | 当前状态 |
| --- | --- |
| 当前版本 | `v1.0.12` |
| CLI | Go 单二进制 `BB-DL` / `BB-DL.exe` |
| 桌面端 | Wails 2 + React 19 + TypeScript + Vite |
| 桌面 helper | `BB-DL-cli` / `BB-DL-cli.exe` |
| 支持平台 | macOS amd64/arm64、Linux amd64/arm64、Windows amd64/arm64 |
| 上游参考目录 | `/Users/long/Documents/CodexProjects/Bilibili/BBDown`，保持原远端和独立 Git 历史 |
| Go 项目目录 | `/Users/long/Documents/CodexProjects/Bilibili/BB-DL` |

状态含义：

- `已实现`：代码、单元测试或真实样本已覆盖主要路径。
- `基本实现`：主路径可用，但仍有小范围上游差异或缺少特殊账号样本。
- `待权限验证`：实现已存在，但当前没有会员账号、代理配置或对应内容权限，不能给出真实通过结论。
- `超出原版`：Go 版额外提供的便利能力。

## 原版功能对照

### 输入与信息抓取

| 原版能力 | BB-DL 状态 | 说明 |
| --- | --- | --- |
| AV / BV 输入 | 已实现 | 支持 ID 和完整 URL |
| EP / SS / MD 番剧入口 | 已实现 | 支持普通番剧与番剧跳转 |
| 课程 Cheese | 已实现 | 支持课程信息和分集 |
| b23.tv 短链 | 已实现 | 跟随跳转后统一解析 |
| 收藏夹 | 已实现 | 支持 `favId` 入口 |
| 合集 / 系列 | 已实现 | 支持 `listBizId` / `seriesBizId` |
| 空间投稿 | 基本实现 | 支持 `mid:` 与空间 URL；接口风控时有 WBI/旧接口回退，稳定性仍受账号与风控影响 |
| 多 P 视频 | 已实现 | 支持单页、列表、范围、`ALL`、`LAST` |
| 互动视频 | 已实现 | 支持 edge 列表解析 |
| 国际版入口 | 基本实现 | 支持 INTL API；地区限制内容依赖用户提供的访问条件 |

### 播放流与选轨

| 原版能力 | BB-DL 状态 | 说明 |
| --- | --- | --- |
| WEB API | 已实现 | 支持 Cookie 登录态与 WBI 请求 |
| TV API | 已实现 | 支持 TV 二维码登录和 token 复用 |
| APP API | 基本实现 | 支持 APP token、gRPC/REST 回退 |
| 国际版 API | 基本实现 | 支持 access token、area 和 host 参数 |
| BiliPlus 代理 | 待权限验证 | 参数和请求链路已实现，缺少真实代理配置 |
| DASH / FLV | 已实现 | 覆盖 DASH 分轨和 FLV 分段 |
| AVC / HEVC / AV1 | 已实现 | 支持编码优先级和升降序 |
| 清晰度优先级 | 已实现 | 支持 `--dfn-priority` |
| 交互式选流 | 已实现 | `--interactive` / `-ia` |
| 非交互视频流选择 | 超出原版 | `--video-index`，供脚本和桌面端使用 |
| 非交互音频流选择 | 超出原版 | `--audio-index`，供脚本和桌面端使用 |
| 杜比视界 / 杜比音频 | 待权限验证 | 解析和混流分支已实现，缺少可公开复验的权限样本 |
| FLAC | 基本实现 | 轨道解析和混流路径已实现 |
| 背景音 / 角色音 | 基本实现 | 支持额外音轨下载与混流 |

### 下载与产物

| 原版能力 | BB-DL 状态 | 说明 |
| --- | --- | --- |
| 内置多线程下载 | 已实现 | 支持分片、重试和传输事件 |
| aria2c 下载 | 已实现 | 支持路径与额外参数 |
| UPOS host / PCDN / HTTP | 已实现 | 支持 host 替换、PCDN 开关和强制 HTTP |
| FFmpeg 混流 | 已实现 | 默认完整下载路径 |
| MP4Box 混流 | 已实现 | 可显式启用，杜比视界场景可回退 |
| 仅视频 / 仅音频 | 已实现 | 不需要混流工具 |
| 仅封面 / 仅字幕 / 仅弹幕 | 已实现 | 支持低副作用资源下载 |
| 跳过混流 / 字幕 / 封面 | 已实现 | 与 CLI、配置、服务、桌面端一致 |
| 字幕与 AI 字幕过滤 | 已实现 | 保存 SRT，支持 `--skip-ai` |
| 弹幕 XML / ASS | 已实现 | ASS 转换遵循同名文件策略 |
| 章节与元数据 | 已实现 | 混流阶段写入可用元数据 |
| 文件名模板 | 已实现 | 单 P / 多 P 模板和日期格式化 |
| 同名文件策略 | 超出原版 | `rename` 追加流水号、`skip` 跳过、`overwrite` 覆盖 |
| 下载归档 | 已实现 | 支持 `BBDown.archives` |

### CLI、配置与登录

| 能力 | BB-DL 状态 | 说明 |
| --- | --- | --- |
| 原版参数覆盖 | 已实现 | 78 个原版用户可见参数均有注册测试；完整说明见 `docs/usage.md` |
| 中文帮助 | 已实现 | `-h`、`--help`、`-?` 和子命令 help 均有中文说明 |
| 显式下载命令 | 超出原版 | `download` / `down`，也可直接传 URL |
| 信息命令 | 超出原版 | `info` 只解析不下载 |
| 环境诊断 | 超出原版 | `doctor` / `doctor --json` |
| 版本命令 | 已实现 | 只输出版本和构建时间 |
| 配置文件 | 已实现 | 兼容 `BBDown.config`，显式 CLI 参数优先 |
| WEB 二维码登录 | 已实现 | 写入兼容文件 `BBDown.data` |
| TV 二维码登录 | 已实现 | 写入兼容文件 `BBDownTV.data` |
| APP token | 已实现 | 读取 `BBDownApp.data` 或显式参数 |
| 彩色与缩进日志 | 超出原版 | 按信息、成功、警告、错误和进度场景着色；非终端可禁用颜色 |

### 服务模式

| 能力 | BB-DL 状态 | 说明 |
| --- | --- | --- |
| `POST /add-task` | 已实现 | JSON 字段复用 `MyOption` |
| 运行中 / 已完成任务查询 | 已实现 | 支持单任务和分组查询 |
| 删除已完成任务 | 已实现 | 支持全部、失败和指定 aid |
| callback webhook | 已实现 | 完成后回调 |
| `SavePaths` 预测 | 已实现 | 覆盖多 P、only、SkipMux 和流水号命名 |
| 进度与失败阶段 | 已实现 | 解析、下载、混流和失败摘要均可查询 |
| 下载字节与速度 | 基本实现 | 子进程隐藏传输事件优先，文件轮询兜底；精度受外部下载器和轮询周期影响 |

### 桌面端

| 能力 | BB-DL 状态 | 说明 |
| --- | --- | --- |
| 创建和管理下载任务 | 已实现 | 支持等待、开始、停止、重试、删除和自动续队 |
| 任务监控 | 已实现 | 百分比、字节、当前/平均速度、耗时、实时日志 |
| 视频流 / 音频流选择 | 已实现 | 先通过仅查看任务取得流列表，再选择显式序号 |
| 完成文件管理 | 已实现 | 打开、定位、复制单个或全部路径 |
| 参数设置 | 已实现 | 常用下载参数、工具路径、同名文件策略、主题和额外参数 |
| WEB / TV 登录 | 已实现 | 独立登录会话，不创建下载任务、不写入任务历史 |
| 历史恢复 | 已实现 | 最多 200 个任务，日志按 256 KiB 裁剪 |
| 工具检测 | 已实现 | FFmpeg、MP4Box、aria2c 路径、版本和安装提示 |
| 跨平台 UI | 已实现 | macOS、Windows、Linux 共用 React 界面和 Go 后端 |

## 发布产物

CLI 资产：

```text
BB-DL_v1.0.12_cli_darwin_amd64.tar.gz
BB-DL_v1.0.12_cli_darwin_arm64.tar.gz
BB-DL_v1.0.12_cli_linux_amd64.tar.gz
BB-DL_v1.0.12_cli_linux_arm64.tar.gz
BB-DL_v1.0.12_cli_windows_amd64.zip
BB-DL_v1.0.12_cli_windows_arm64.zip
```

桌面资产：

```text
BB-DL_v1.0.12_desktop_darwin_arm64.zip
BB-DL_v1.0.12_desktop_linux_amd64.tar.gz
BB-DL_v1.0.12_desktop_windows_amd64.zip
```

每个独立 CLI 包只含 `BB-DL` / `BB-DL.exe`。macOS 桌面包只含 `BB-DL.app`；Windows/Linux 桌面包只含 GUI 与 `BB-DL-cli` helper，不打包 README、Markdown 或 `docs/`。

## 兼容边界

品牌、仓库、Go module、CLI、桌面程序、helper 和发布资产统一使用 `BB-DL`。以下名称继续保留，不能机械改名：

- `BBDown.config`、`BBDown.data`、`BBDownTV.data`、`BBDownApp.data`、`BBDown.archives`：与原版兼容的运行文件。
- `BBDOWN_GO_RUNTIME_DIR`、`BBDOWN_GO_HELPER`、`BBDOWN_GO_TOOL_DIRS`：已发布版本使用的环境变量兼容接口。
- `__BBDOWN_GO_SERVER_TRANSFER__`：服务与桌面端之间的隐藏传输事件协议。
- `BBDown Go` 用户数据目录：保留已有任务历史、设置和登录态。
- `com.lonnnnnng.bbdown-go.desktop`：保留操作系统应用身份和升级连续性。

## 当前验证

发布前必须完成：

```sh
go test -race ./...
go vet ./...
npm run lint --prefix frontend
npm run build --prefix frontend
bash -n scripts/*.sh
./scripts/build-release.sh
./scripts/verify-artifacts.sh cli dist/release/*
./scripts/build-desktop.sh
./scripts/verify-artifacts.sh desktop dist/desktop/*
```

真实公开视频回归使用 `https://www.bilibili.com/video/BV1J9EB6xEAB`。未登录状态已验证信息解析、流列表、封面、单轨下载、同名文件策略和 FFmpeg 混流主路径；会员、杜比视界和真实 BiliPlus 代理场景受账号或配置限制，不写成已通过。

## 后续工作

1. 在具备相应权限后补会员、杜比视界、杜比音频和付费课程真实样本。
2. 在具备可用代理配置后补 BiliPlus 真实回归。
3. 持续补空间投稿和 APP/TV/INTL 接口风控样本。
4. 继续提高外部下载器场景下服务与桌面速度统计精度。
5. 每次能力或发布流程变化时，同步更新本文件、`README.md`、`docs/usage.md` 和 `docs/technical-implementation.md`。
