# Agent Usage Desktop — Tauri 桌面应用设计文档

## 概述

将现有的 agent-usage Web 应用改造为 Tauri 桌面应用。核心 Go 后端作为 sidecar 运行，前端用 React + TypeScript 重写，Tauri Rust 层负责系统集成。现有 Web UI 完整保留，桌面端提供"打开 Web UI"按钮。

## 决策记录

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 桌面框架 | Tauri v2 | 用户指定 |
| 前端框架 | React + TypeScript | 生态最大，shadcn/ui 组件库可用，ECharts React 封装成熟 |
| Go 集成方式 | Sidecar 模式 | Go 代码几乎不改，风险最低 |
| UI 风格 | 浅色清爽为主 | 类 Notion/Stripe，支持深色切换 |
| 功能范围 | 现有功能 + 桌面集成 | 托盘、自启、后台、通知 |
| 目标平台 | macOS + Windows + Linux | 全平台 |
| 仓库策略 | 单仓库改造 | 项目规模不大，分仓库复杂度不值得 |
| Web UI | 完整保留 | 桌面端加按钮可打开浏览器访问 |

## 架构

```
┌─────────────────────────────────────────────┐
│              Tauri App Shell                 │
│  ┌────────────┐  ┌────────────────────────┐ │
│  │ Rust Core  │  │   React WebView        │ │
│  │            │  │                        │ │
│  │ • 托盘管理  │  │  • Dashboard 页面      │ │
│  │ • 自启动    │  │  • Sessions 页面       │ │
│  │ • 通知推送  │  │  • 设置页面            │ │
│  │ • Sidecar  │  │  • "打开 Web UI" 按钮  │ │
│  │   生命周期  │  │                        │ │
│  └──────┬─────┘  └───────────┬────────────┘ │
│         │                    │               │
│         │    管理进程         │  HTTP API     │
│         ▼                    ▼               │
│  ┌─────────────────────────────────────────┐ │
│  │         Go Sidecar (agent-usage)        │ │
│  │                                         │ │
│  │  • Collectors (Claude/Codex/OpenClaw/   │ │
│  │    OpenCode)                            │ │
│  │  • SQLite Storage                       │ │
│  │  • REST API (/api/*)                    │ │
│  │  • 原有 Web UI (go:embed static/)      │ │
│  └─────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘
```

### 数据流

1. Tauri 启动 → Rust 层找可用端口 → 拉起 Go sidecar (`--port {port}`)
2. Rust 轮询 `GET /api/health` 等待 Go 就绪
3. Go 就绪后 → Rust 通知 React 前端 → 前端加载并通过 `http://localhost:{port}/api/*` 获取数据
4. Go sidecar 内部：collectors 定时扫描 → 写入 SQLite → pricing 同步 → 成本计算
5. "打开 Web UI" 按钮 → 调用 `shell.open("http://localhost:{port}")` → 浏览器打开原有 Web UI

## Go 后端改动

改动很小，三处：

### 1. 命令行参数 `--port`

新增 `--port` flag，覆盖 config.yaml 中的 `server.port`。现有的 `--config` flag 和 `ResolveConfigPath` 逻辑完全保留。Tauri 启动 sidecar 时同时传入 `--config` 和 `--port`：

```
agent-usage --config ~/.config/agent-usage/config.yaml --port {port}
```

### 2. 健康检查端点

新增 `GET /api/health`，返回 `{"status":"ok"}`。Tauri 用此端点确认 Go 服务就绪。

### 3. CORS 支持

Go HTTP server 添加 CORS 中间件，允许来自 Tauri WebView 自定义协议（`tauri://localhost`）的跨域请求。仅在响应头中添加 `Access-Control-Allow-Origin`、`Access-Control-Allow-Methods`、`Access-Control-Allow-Headers`。

### 配置与数据文件路径策略

桌面应用场景下，Go sidecar 的 CWD 不可预测（可能在 app bundle 内部）。解决方案：

- Rust 层启动 sidecar 时，始终显式传入 `--config` 参数
- 默认配置文件路径：`~/.config/agent-usage/config.yaml`（首次启动时 Rust 层自动生成默认配置）
- SQLite 数据库路径在配置中使用绝对路径：`~/.local/share/agent-usage/agent-usage.db`
- macOS/Linux 遵循 XDG 规范，Windows 使用 `%APPDATA%\agent-usage\`

其他所有代码（collectors、storage、pricing、现有 API、Web UI embed）不变。

## Rust 层

保持精简，约 400-600 行代码，职责：

### 1. Sidecar 生命周期管理

- 启动：找可用端口 → 生成默认配置（如不存在）→ 拉起 Go 二进制（`--config {path} --port {port}`）→ 轮询 health → 通知前端
- 退出：SIGTERM → 等待优雅退出 → 超时 SIGKILL
- 崩溃恢复：监控进程状态，异常退出时自动重启并通知前端
- 单实例保护：检测是否已有实例运行，如有则聚焦已有窗口而非启动第二个 sidecar

### 2. 系统托盘

- 托盘图标常驻
- 左键：显示/隐藏窗口
- 右键菜单：显示面板 / 打开 Web UI / 开机自启（勾选） / 退出
- 关闭窗口 → 隐藏到托盘（非退出）

### 3. 开机自启

使用 Tauri `autostart` 插件，统一 API：
- macOS: Login Items
- Windows: 注册表
- Linux: `.desktop` 文件

### 4. 通知推送

使用 Tauri `notification` 插件：
- 定时从 `/api/stats` 拉数据
- 日用量超过用户设定阈值时触发系统通知
- 阈值在设置页面配置，持久化到 Tauri 的 app data 目录（JSON 文件），与 Go 的 SQLite 数据库分离

## React 前端

### 技术栈

- React 18 + TypeScript
- Vite 构建
- shadcn/ui（Radix + Tailwind CSS）
- echarts-for-react
- @tauri-apps/api
- react-i18next：中英双语

### 页面结构

**Dashboard（首页）** — 1:1 还原现有 Web UI：
- 顶部统计卡片（Total Tokens、Total Cost、Sessions、Prompts、API Calls、Cache Hit Rate）
- 费用趋势图（按模型分色）
- Token 用量图（input/output/cache 分层）
- 费用按模型分布（饼图）
- 时间范围选择器（today/7d/30d/year/自定义）
- 粒度选择器、source 筛选（Claude/Codex/OpenClaw/OpenCode 四个源）、自动刷新

**Sessions** — session 列表：
- 排序、筛选
- 展开查看 per-model 详情

**Settings** — 设置页面：
- 通知阈值配置
- 开机自启开关
- 语言切换（中/英）
- 主题切换（浅色/深色/跟随系统）

### 布局

- 顶部导航栏：页面切换 + 右侧"打开 Web UI"按钮、主题、语言
- 无侧边栏，内容居中，最大宽度 ~1200px
- 不需要移动端适配

### UI 风格

- 浅色清爽为默认基调（白底 + 微灰卡片）
- 支持深色模式切换 + 跟随系统
- 设计语言参考 Notion / Stripe Dashboard

## 目录结构

```
agent-usage-desktop/
├── main.go                          # Go 后端入口（不变）
├── internal/                        # Go 后端逻辑（不变）
├── src-tauri/                       # Tauri Rust 层
│   ├── src/
│   │   ├── main.rs                  # 入口
│   │   ├── sidecar.rs               # Go 进程管理
│   │   ├── tray.rs                  # 系统托盘
│   │   └── notification.rs          # 通知逻辑
│   ├── binaries/                    # Go sidecar 二进制（构建时生成）
│   ├── icons/                       # 应用图标
│   ├── tauri.conf.json              # Tauri 配置
│   └── Cargo.toml
├── src/                             # React 前端
│   ├── components/                  # shadcn/ui 组件
│   ├── pages/
│   │   ├── Dashboard.tsx
│   │   ├── Sessions.tsx
│   │   └── Settings.tsx
│   ├── lib/                         # API client、i18n、utils
│   ├── App.tsx
│   └── main.tsx
├── package.json
├── vite.config.ts
├── tailwind.config.ts
├── tsconfig.json
├── go.mod / go.sum                  # Go 依赖（不变）
├── config.yaml                      # Go 配置（不变）
├── Dockerfile                       # Docker 构建（不变）
├── docker-compose.yml               # Docker 部署（不变）
└── .goreleaser.yaml                 # Go 构建（保留）
```

## 打包与分发

### 构建产物

| 平台 | 格式 |
|------|------|
| macOS | `.dmg`（Universal Binary: Intel + Apple Silicon） |
| Windows | `.msi` 安装包 + `.exe` 便携版 |
| Linux | `.deb` + `.AppImage` |

### CI/CD（GitHub Actions）

触发条件：`v*` tag push

构建流程（矩阵）：
1. 安装 Go → GoReleaser 编译目标平台的 Go sidecar 二进制
2. 将二进制放入 `src-tauri/binaries/`，按 Tauri 命名规范命名（如 `agent-usage-x86_64-apple-darwin`）
3. 安装 Node → `npm install` → Tauri build
4. 上传 release artifact

现有 Docker 构建流程不变，Docker 版继续使用原有 Web UI。

代码签名暂不实现，后续需要时再加。

## 不在范围内

- 数据导出（CSV/PDF）— 后续迭代
- 代码签名 — 后续需要时加
- 自动更新 — 后续迭代（计划使用 Tauri 内置 updater 插件）
- 移动端适配 — 桌面应用不需要
