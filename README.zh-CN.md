<p align="center">
  <img src="docs/assets/logo.svg" alt="Agent Usage Logo" width="128" height="128">
</p>

<h1 align="center">Agent Usage</h1>

<p align="center">
  在本地统一统计和分析 AI 编程 Agent 的用量、费用、吞吐和会话。
</p>

<p align="center">
  <a href="README.md">English</a>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-2563EB.svg" alt="Apache 2.0 许可证"></a>
  <a href="https://tauri.app"><img src="https://img.shields.io/badge/Tauri-2-24C8DB?logo=tauri&logoColor=white" alt="Tauri 2"></a>
  <a href="https://react.dev"><img src="https://img.shields.io/badge/React-19-149ECA?logo=react&logoColor=white" alt="React 19"></a>
  <a href="https://www.typescriptlang.org"><img src="https://img.shields.io/badge/TypeScript-6-3178C6?logo=typescript&logoColor=white" alt="TypeScript 6"></a>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white" alt="Go 1.25+"></a>
  <a href="https://www.rust-lang.org"><img src="https://img.shields.io/badge/Rust-stable-D65A31?logo=rust&logoColor=white" alt="Rust stable"></a>
  <a href="https://sqlite.org"><img src="https://img.shields.io/badge/SQLite-local-0F80CC?logo=sqlite&logoColor=white" alt="SQLite"></a>
</p>

Agent Usage 读取这台电脑上已有的会话和用量记录，将不同来源归一化到本地 SQLite 索引，并通过专注的桌面工作区呈现。它面向个人查看自己的编程 Agent 活动，不需要账号，没有云端数据库，也不会上传会话内容。

<p align="center">
  <img src="docs/assets/overview.png" alt="Agent Usage 总览页，展示 Token、费用、吞吐和来源分析" width="960">
</p>

## 主要能力

- 对比受支持 Agent 的 Token 用量、估算费用、Prompt、会话、API 调用、RPM 和 TPM。
- 按时间范围、来源、模型和项目筛选，并将同一查询条件带入会话搜索。
- 搜索并检查 Claude Code 与 Codex 的可读事件时间线，包括工具调用和错误。
- 增量扫描、自动去重，并在需要时从源文件重建本地索引。
- 使用不可变定价快照，支持内置离线目录、LiteLLM 在线更新和 JSON 导入。
- 支持系统托盘、开机自启、费用通知、中英文界面和深浅色主题。

<p align="center">
  <img src="docs/assets/sessions.png" alt="Agent Usage 会话回溯页，包含搜索、时间线和事件详情" width="960">
</p>

## 支持的数据来源

| 来源 | 默认位置 | 用量分析 | 会话回溯 |
| --- | --- | :---: | :---: |
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `~/.claude/projects` | 支持 | 支持 |
| [Codex CLI](https://github.com/openai/codex) | `~/.codex/sessions` | 支持 | 支持 |
| [OpenCode](https://github.com/anomalyco/opencode) | `~/.local/share/opencode/opencode.db` | 支持 | 暂不支持 |
| [OpenClaw](https://github.com/openclaw/openclaw) | `~/.openclaw/agents` | 支持 | 暂不支持 |

采集器会从已保存的文件偏移继续读取，避免重复解析未变化的 JSONL 内容。OpenCode 直接读取本地 SQLite 数据库。所有来源共用一套统计模型；v2.0 的深度事件索引仅支持 Claude Code 和 Codex。

## 安装

从 [GitHub Releases](https://github.com/hongshuo-wang/agent-usage-desktop/releases) 下载安装包：

| 平台 | v2.0 官方产物 |
| --- | --- |
| macOS Apple Silicon | `Agent.Usage_2.0.0_aarch64.dmg` |
| Windows x64 | `Agent.Usage_2.0.0_x64-setup.exe` |

macOS Intel 和 Linux 用户可以从源码构建，v2.0 暂不提供这两个平台的官方安装包。

macOS 构建目前未签名。如果将应用移动到 Applications 后，系统提示应用“已损坏”，请运行一次：

```bash
xattr -cr "/Applications/Agent Usage.app"
```

启动 Agent Usage 后，sidecar 会在仅限 loopback 的动态端口运行，并在后台扫描已启用的本地来源。

## 隐私与网络访问

Agent 原始文件始终是事实来源。SQLite 索引和配置保留在这台电脑上，并可随时重建。应用不会上传会话内容，本地 API 只绑定到 loopback 地址。

唯一的常规网络请求是从 [LiteLLM 的 jsDelivr 镜像](https://cdn.jsdelivr.net/gh/BerriAI/litellm@main/model_prices_and_context_window.json)更新模型价格。内置定价目录让首次运行也能离线估算费用。在线更新和手动导入都不会改写已经由历史定价快照确定的费用。

## 指标语义

Token 总量由四个互不重叠的分项组成：

```text
总输入     = 非缓存输入 + 缓存读取输入 + 缓存创建输入
总输出     = 输出 token
Token 总量 = 总输入 + 总输出
```

推理输出仅供参考，已经是输出 token 的子集，因此不会再次相加。RPM 和 TPM 表示本机观测吞吐，不代表服务商配额、剩余容量或限流利用率。界面显示的费用是本地估算，不是服务商账单。

## 配置

桌面应用首次启动时会创建 `~/.config/agent-usage/config.yaml`。也可以在设置中管理数据来源和应用行为。

```yaml
server:
  port: 9800
  bind_address: "127.0.0.1"

collectors:
  claude:
    enabled: true
    paths: ["~/.claude/projects"]
    scan_interval: 60s
  codex:
    enabled: true
    paths: ["~/.codex/sessions"]
    scan_interval: 60s
  openclaw:
    enabled: true
    paths: ["~/.openclaw/agents"]
    scan_interval: 60s
  opencode:
    enabled: true
    paths: ["~/.local/share/opencode/opencode.db"]
    scan_interval: 60s

storage:
  path: "~/.config/agent-usage/agent-usage.db"

pricing:
  sync_interval: 1h
```

## 从源码构建

前置条件：[Go](https://go.dev/) 1.25+、[Node.js](https://nodejs.org/) 24+ 和 [Rust](https://rustup.rs/) stable。Linux 还需要 `libwebkit2gtk-4.1-dev` 和 `libappindicator3-dev`。

```bash
npm install

# macOS Apple Silicon sidecar
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go build -o src-tauri/binaries/agent-usage-aarch64-apple-darwin .

npx tauri dev
```

生产构建需要先准备与目标平台匹配的 sidecar，再运行 `npx tauri build`：

```bash
# macOS Intel
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
  go build -o src-tauri/binaries/agent-usage-x86_64-apple-darwin .

# Linux x64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -o src-tauri/binaries/agent-usage-x86_64-unknown-linux-gnu .

# Windows x64
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -o src-tauri/binaries/agent-usage-x86_64-pc-windows-msvc.exe .
```

Tauri 要求 sidecar 使用 `agent-usage-{rust-target-triple}[.exe]` 命名。

Go 后端也可以独立运行：

```bash
go build -o agent-usage-desktop .
./agent-usage-desktop
./agent-usage-desktop --config path/to/config.yaml
./agent-usage-desktop --port 9800
./agent-usage-desktop version
```

运行完整的本地检查：

```bash
go test ./...
go vet ./...
npm test
npm run build
cargo test --manifest-path src-tauri/Cargo.toml
```

## 架构

React 和 TypeScript 前端运行在 Tauri v2 中。Rust 负责桌面窗口、托盘、通知、开机自启和 Go sidecar 生命周期。纯 Go sidecar 采集本地记录，使用 `modernc.org/sqlite`（无需 CGO）保存归一化索引，按定价快照估算费用，并向界面提供仅限 loopback 的 REST API。

## 参与贡献

欢迎提交 Issue 和 Pull Request。开发流程请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)，安全问题请按照 [SECURITY.md](SECURITY.md) 私下报告。

## 社区

- [GitHub Issues](https://github.com/hongshuo-wang/agent-usage-desktop/issues)

## 许可证

Agent Usage 使用 [Apache License 2.0](LICENSE) 开源。
