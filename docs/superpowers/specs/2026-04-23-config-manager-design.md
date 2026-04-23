# Config Manager Design Spec

Agent 配置统一管理功能，类似 cc-switch，集成到 agent-usage-desktop 项目中。

## 目标

在现有 token 用量追踪的基础上，新增对 4 个 AI agent 工具（Claude Code、Codex、OpenCode、OpenClaw）的配置统一管理能力，覆盖 Provider 配置切换、MCP Server 管理、Skills 管理三大功能。

## 核心需求

1. 快捷 Profile 切换 + 单项精细编辑（混合模式）
2. 每次修改展示影响了哪些文件
3. 每个配置文件有用途说明 + 官方文档链接
4. 修改前自动备份，每文件 5 个滚动备份
5. MCP 和 Skills 做统一管理，一份配置同步多个工具
6. 支持跳转到对应文件夹
7. 双向同步：UI 修改同步到原生文件，外部修改回读到系统

## 覆盖的工具与配置文件

### Claude Code

| 文件 | macOS / Linux 路径 | Windows 路径 | 用途 | 文档 |
|------|-------------------|-------------|------|------|
| settings.json | ~/.claude/settings.json | %USERPROFILE%\.claude\settings.json | Provider 配置、权限、hooks | https://docs.anthropic.com/en/docs/claude-code/settings |
| .claude.json | ~/.claude.json | %USERPROFILE%\.claude.json | MCP server 定义（用户级） | https://docs.anthropic.com/en/docs/claude-code/mcp |
| skills/ | ~/.claude/skills/ | %USERPROFILE%\.claude\skills\ | 用户级 Skills | https://docs.anthropic.com/en/docs/claude-code/skills |

### Codex CLI

| 文件 | macOS / Linux 路径 | Windows 路径 | 用途 | 文档 |
|------|-------------------|-------------|------|------|
| auth.json | ~/.codex/auth.json | %USERPROFILE%\.codex\auth.json | API key / OAuth 凭证 | https://github.com/openai/codex |
| config.toml | ~/.codex/config.toml | %USERPROFILE%\.codex\config.toml | Provider、MCP servers、模型配置 | https://github.com/openai/codex |
| skills/ | ~/.agents/skills/ | %USERPROFILE%\.agents\skills\ | 用户级 Skills（跨工具共享目录） | https://github.com/openai/codex |

### OpenCode

| 文件 | macOS / Linux 路径 | Windows 路径 | 用途 | 文档 |
|------|-------------------|-------------|------|------|
| opencode.json | ~/.config/opencode/opencode.json | %USERPROFILE%\.config\opencode\opencode.json | Provider、MCP servers、全局配置 | https://opencode.ai/docs/config |
| skills/ | ~/.config/opencode/skills/ | %USERPROFILE%\.config\opencode\skills\ | 用户级 Skills | https://opencode.ai/docs/config |

### OpenClaw

| 文件 | macOS / Linux 路径 | Windows 路径 | 用途 | 文档 |
|------|-------------------|-------------|------|------|
| openclaw.json | ~/.openclaw/openclaw.json | %USERPROFILE%\.openclaw\openclaw.json | Provider、MCP servers、agent 配置 | https://docs.openclaw.ai/gateway/configuration |
| skills/ | ~/.openclaw/skills/ | %USERPROFILE%\.openclaw\skills\ | 用户级 Skills | https://docs.openclaw.ai/gateway/configuration |

## 架构设计

### 与现有代码的关系

`internal/configmanager` 管理的是外部 AI 工具的配置文件，与 `internal/config`（管理本应用自身的 config.yaml）完全独立，互不影响。

### 现有基础设施变更

- CORS 中间件（`internal/server/server.go`）需从 `GET, OPTIONS` 扩展为 `GET, POST, PUT, DELETE, OPTIONS`
- HTTP 路由需升级为支持方法分发和路径参数的方案（Go 1.22+ 增强 ServeMux 支持 `GET /path/{id}` 模式，无需引入第三方依赖）
- 前端 `src/lib/api.ts` 需新增支持 POST/PUT/DELETE 的请求方法（当前只有 GET），添加 `mutateAPI(method, path, body)` 辅助函数

### 新增包：`internal/configmanager`

与 `internal/collector` 平级，核心组件：

- **ConfigManager** — 总协调器，持有所有 adapter 和 sync engine
- **Adapter 层** — 每个工具一个 adapter（claude.go、codex.go、opencode.go、openclaw.go），实现统一接口，负责读写该工具的原生配置文件格式
- **SyncEngine** — 双向同步引擎
- **BackupManager** — 滚动备份管理

### Adapter 接口

```go
type Adapter interface {
    Tool() string
    IsInstalled() bool
    GetProviderConfig() (*ProviderConfig, error)
    SetProviderConfig(cfg *ProviderConfig) ([]AffectedFile, error)
    GetMCPServers() ([]MCPServer, error)
    SetMCPServers(servers []MCPServer) ([]AffectedFile, error)
    GetSkillPaths() []string
    ConfigFiles() []ConfigFileInfo  // 路径、用途、文档链接
}
```

### 核心数据结构

```go
type ProviderConfig struct {
    APIKey   string            `json:"api_key"`
    BaseURL  string            `json:"base_url"`
    Model    string            `json:"model"`
    ModelMap map[string]string `json:"model_map"` // 逻辑名 → 实际模型名
}

type MCPServer struct {
    ID      int64             `json:"id"`
    Name    string            `json:"name"`
    Command string            `json:"command"`
    Args    []string          `json:"args"`
    Env     map[string]string `json:"env"`
    Enabled bool              `json:"enabled"`
}

type AffectedFile struct {
    Path      string `json:"path"`
    Tool      string `json:"tool"`
    Operation string `json:"operation"` // created/modified/deleted
    Diff      string `json:"diff"`      // 变更摘要
}

type ConfigFileInfo struct {
    Path        string `json:"path"`
    Tool        string `json:"tool"`
    Description string `json:"description"`
    DocURL      string `json:"doc_url"`
    Exists      bool   `json:"exists"`
}
```

### 数据流

```
用户操作 (前端) → REST API → ConfigManager → SQLite (SSOT)
    → SyncEngine → BackupManager (备份原文件) → Adapter (写入原生文件)

反向：定时扫描/手动刷新 → Adapter (读取) → SyncEngine (diff) → SQLite 更新
```

## 双向同步设计

### Outbound Sync（UI → 原生文件）

用户在 UI 操作后立即触发。写入前先读取原生文件做 diff，如果文件已被外部修改（跟 SQLite 里存的 last_known_hash 不一致），进入冲突处理。

### Inbound Sync（原生文件 → UI）

- 定时扫描（默认 30s）
- 用户手动点"刷新"

### 冲突检测

每次成功写入后，存储文件的 SHA-256 hash 到 sync_state 表。下次读取时对比 hash，不一致说明有外部变更。

### 冲突处理策略

- Provider 配置：外部变更优先
- MCP/Skills（我们管理的）：提示用户选择（保留外部 / 用我们的覆盖）
- 前端展示冲突 diff

### Merge 粒度

按配置项级别合并，不做行级 merge。只管理属于我们范围的字段，其他字段原样保留。

### 文件写入安全

- 使用原子写入：先写临时文件，再 rename 覆盖目标文件
- Outbound sync 的 read-diff-write 周期内对目标文件加 advisory lock（Unix: flock, Windows: LockFileEx），防止与外部工具并发写入导致数据丢失
- 写入完成后立即重新计算 hash 并更新 sync_state，确保 inbound scan 不会误判

## 安全：API Key 存储

`provider_profiles.config` 中的 `api_key` 字段涉及敏感信息。存储策略：

- API key 在 SQLite 中加密存储，使用 AES-256-GCM，密钥通过 OS keychain 管理（macOS: Keychain, Windows: Credential Manager），Go 使用 `github.com/zalando/go-keyring`
- 加密后的密文使用 base64 编码存储在 JSON 的 `api_key` 字段中（格式：`"enc:base64(nonce+ciphertext)"`，前缀 `enc:` 区分明文和密文）
- 无 keychain 环境的降级策略（headless Linux、CI、Docker、SSH）：使用基于 machine-id 派生的密钥进行加密，启动时输出警告日志提示安全性降低
- REST API 的 GET 响应中 API key 做掩码处理（只显示前 4 位 + 后 4 位），完整 key 仅在写入原生文件时解密
- Codex 的 auth.json 包含 OAuth 凭证，同样加密存储

## 数据模型（SQLite 新增表）

所有新表通过一个新 migration（`migration_config_manager`）创建，遵循现有 `internal/storage/sqlite.go` 的 migration 机制（`meta` 表记录 migration key，每个 migration 只执行一次）。Migration 中需执行 `PRAGMA foreign_keys = ON` 以启用外键约束。

```sql
CREATE TABLE provider_profiles (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    is_active   BOOLEAN DEFAULT 0,
    config      TEXT NOT NULL,          -- JSON: {api_key, base_url, model_map}
    created_at  TIMESTAMP,
    updated_at  TIMESTAMP
);

CREATE TABLE profile_tool_targets (
    profile_id  INTEGER NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    tool        TEXT NOT NULL,
    enabled     BOOLEAN DEFAULT 1,
    tool_config TEXT,                   -- 工具特有的覆盖配置 JSON
    PRIMARY KEY (profile_id, tool)
);

CREATE TABLE mcp_servers (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    command     TEXT NOT NULL,
    args        TEXT,                   -- JSON array
    env         TEXT,                   -- JSON object
    enabled     BOOLEAN DEFAULT 1,
    created_at  TIMESTAMP
);

CREATE TABLE mcp_server_targets (
    server_id   INTEGER NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
    tool        TEXT NOT NULL,
    enabled     BOOLEAN DEFAULT 1,
    PRIMARY KEY (server_id, tool)
);

CREATE TABLE skills (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    source_path TEXT NOT NULL,
    description TEXT,
    enabled     BOOLEAN DEFAULT 1,
    created_at  TIMESTAMP
);

CREATE TABLE skill_targets (
    skill_id    INTEGER NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    tool        TEXT NOT NULL,
    method      TEXT DEFAULT 'symlink', -- symlink / copy
    enabled     BOOLEAN DEFAULT 1,
    PRIMARY KEY (skill_id, tool)
);

CREATE TABLE config_backups (
    id          INTEGER PRIMARY KEY,
    tool        TEXT NOT NULL,
    file_path   TEXT NOT NULL,
    backup_path TEXT NOT NULL,
    slot        INTEGER NOT NULL,       -- 1-5 滚动槽位
    created_at  TIMESTAMP,
    trigger     TEXT                    -- auto/manual/profile_switch
);

CREATE TABLE sync_state (
    tool        TEXT NOT NULL,
    file_path   TEXT NOT NULL,
    last_hash   TEXT NOT NULL,          -- SHA-256
    last_sync   TIMESTAMP,
    last_sync_dir TEXT,                 -- inbound/outbound（记录最近一次同步方向，用于审计）
    PRIMARY KEY (tool, file_path)
);
```

## 备份机制

### 存储路径（跨平台）

- macOS: `~/Library/Application Support/agent-usage/backups/`
- Linux: `~/.config/agent-usage/backups/`（遵循 XDG）
- Windows: `%APPDATA%\agent-usage\backups\`

Go 使用 `os.UserConfigDir()` 获取平台基础路径。

### 目录结构

备份文件直接以 `文件名.槽位.bak` 命名，不用文件名做目录名（避免 `.json` 扩展名目录的混淆）。元信息存在同名 `.meta` 文件中，保证 `.bak` 文件是原文件的完整副本，可直接手动恢复。

```
backups/
├── claude/
│   ├── settings.json.1.bak
│   ├── settings.json.1.meta
│   ├── settings.json.2.bak
│   ├── settings.json.2.meta
│   ├── .claude.json.1.bak
│   └── .claude.json.1.meta
├── codex/
│   ├── auth.json.1.bak
│   └── config.toml.1.bak
├── opencode/
│   └── opencode.json.1.bak
└── openclaw/
    └── openclaw.json.1.bak
```

### 滚动策略

- 每文件 5 个槽位，先顺序填充 1→2→3→4→5，填满后从 1 开始循环覆盖
- `.meta` 文件记录备份时间、触发原因、原文件 hash

### 触发时机

- Profile 切换前（profile_switch）
- 单项配置修改前（auto）
- 用户手动备份（manual）

### 回滚

先备份当前文件（消耗一个槽位），再用选中的备份覆盖原文件，触发 inbound sync 更新 SQLite。

## REST API

```
# Provider Profiles
GET    /api/config/profiles
POST   /api/config/profiles
PUT    /api/config/profiles/{id}
DELETE /api/config/profiles/{id}
POST   /api/config/profiles/{id}/activate

# MCP Servers
GET    /api/config/mcp
POST   /api/config/mcp
PUT    /api/config/mcp/{id}
DELETE /api/config/mcp/{id}
PUT    /api/config/mcp/{id}/targets

# Skills
GET    /api/config/skills
POST   /api/config/skills
PUT    /api/config/skills/{id}
DELETE /api/config/skills/{id}
PUT    /api/config/skills/{id}/targets

# Sync
POST   /api/config/sync
GET    /api/config/sync/status
POST   /api/config/sync/resolve

# Backups
GET    /api/config/backups
POST   /api/config/backups
POST   /api/config/backups/{id}/restore

# Files
GET    /api/config/files
```

打开文件目录功能由前端通过 Tauri `shell.open()` 实现，不经过 Go 后端。

所有写操作响应带 `affected_files` 字段。`/api/config/files` 返回每个文件的 description 和 doc_url。

### Profile 激活流程

1. 备份所有目标工具的配置文件
2. 将当前激活 profile 的 `is_active` 设为 0
3. 按顺序写入每个 enabled 的 tool target
4. 任一工具写入失败时：从备份恢复已写入的文件，重新激活原 profile，返回错误详情
5. 全部成功后将新 profile 的 `is_active` 设为 1，返回 affected_files

系统始终有且仅有一个激活的 profile（首次使用时从各工具现有配置自动创建 "Default" profile）。

### 错误处理

标准错误响应格式：

```json
{
    "error": "CONFLICT",
    "message": "File modified externally",
    "details": { "tool": "claude", "file": "settings.json" }
}
```

HTTP 状态码映射：
- 400 — 参数校验失败（缺少必填字段、格式错误）
- 404 — Profile/MCP/Skill 不存在
- 409 — 同步冲突（文件被外部修改）
- 500 — 内部错误（文件 I/O 失败等）

### 数据校验规则

- Profile: `name` 必填且唯一，`config.api_key` 必填
- MCP Server: `name` 必填且唯一，`command` 必填（可以是裸命令名或绝对路径）
- Skill: `name` 必填且唯一，`source_path` 必填且目录必须存在

### 打开文件目录

不走 Go 后端，前端直接调用 Tauri 的 `shell.open()` API（已有 `opener:default` 权限），避免 GET 请求产生副作用。

## Skills 跨平台同步

- macOS / Linux: 默认使用 symlink
- Windows: 默认使用 copy（创建 symlink 需要管理员权限或 Developer Mode）
- Adapter 在运行时检测平台，symlink 失败时自动降级为 copy
- `skill_targets.method` 记录实际使用的方式

## 前端设计

新增 `/config` 页面，导航栏加入 "Config" 入口，内部 4 个 Tab。Tab 使用 URL 子路由（`/config/providers`、`/config/mcp`、`/config/skills`、`/config/files`），支持深链接和浏览器后退。默认路由 `/config` 重定向到 `/config/providers`。

注意：Tauri webview 中 `BrowserRouter` 在硬刷新子路由时可能 404，需确保 Vite 构建配置或 Tauri webview 有 SPA fallback（现有路由已正常工作说明 fallback 已配置，新增子路由无需额外处理）。

### Tab 1: Providers

- 左侧 Profile 列表（激活项高亮），右侧详情编辑
- 下方工具勾选 + 同步状态图标
- 激活前预览即将修改的文件列表

### Tab 2: MCP Servers

- 统一 MCP server 列表，每行带工具勾选控制同步目标
- 新增/编辑弹窗：name、command、args、env
- 保存时展示"将修改以下文件"确认面板

### Tab 3: Skills

- 统一 skills 列表，每行带工具勾选 + 同步方式（symlink/copy）
- 支持从本地目录添加或 GitHub URL 拉取
- 每行有"打开目录"按钮

### Tab 4: Files & Backups

- 上半部分：所有配置文件列表（路径、工具、用途说明、文档链接、同步状态、打开目录按钮）
- 下半部分：备份历史（时间、触发原因、影响文件），支持一键回滚
- 冲突时文件行变警告色，展开 diff 视图让用户选择

### 通用交互

- 写操作前弹确认面板，列出即将修改的文件和变更摘要
- 页面顶部同步状态指示器（绿色正常 / 橙色冲突）
- 复用现有 i18n 体系（en/zh）
