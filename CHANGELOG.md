# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [2.0.0] - 2026-07-29

### English

#### Highlights

- Rebuilt Agent Usage around a focused, token-first desktop workspace with shared time, source, model, and project filters.
- Added searchable Claude Code and Codex session retrospectives with event timelines, tool details, errors, and full-text search.
- Added locally observed RPM/TPM analytics, usage breakdowns, high-usage markers, and query-aware navigation between Overview and Sessions.
- Added immutable model-pricing snapshots, pricing provenance and coverage, manual catalog import, online refresh, and a bundled offline fallback.
- Added local settings for collectors, pricing, index diagnostics, notifications, appearance, language, and autostart.
- Introduced a new light Agent Usage identity and synchronized English/Chinese open-source documentation.

#### Changed

- Removed the general-purpose provider, MCP, skills, backup, and configuration-management center introduced in v1.1.0; v2.0 is deliberately scoped to local usage and session analytics.
- Reworked JSONL readers and session indexing for byte-accurate incremental scans, atomic updates, source identity checks, and bounded recovery from malformed or partial files.
- Historical costs are now tied to event-time pricing snapshots and are not silently recalculated by later catalog refreshes.
- The application database migrates automatically and rebuilds derived scan state when required; raw agent source files are never modified.

#### Distribution

- Official installers are provided for macOS Apple Silicon and Windows x64.
- macOS Intel and Linux remain supported through source builds for v2.0.

### 中文

#### 主要更新

- 将 Agent Usage 重构为以 Token 为核心的桌面工作区，并统一时间、来源、模型和项目筛选条件。
- 新增 Claude Code 与 Codex 会话回溯中心，支持事件时间线、工具详情、错误信息和全文搜索。
- 新增本机观测 RPM/TPM、用量拆分、高用量时段标记，以及总览与会话之间的查询联动。
- 新增不可变模型定价快照、定价来源与覆盖率、手动目录导入、在线更新和内置离线回退。
- 新增本地数据来源、定价、索引诊断、通知、外观、语言和开机自启设置。
- 启用全新的浅色品牌标识，并同步重写中英文开源文档。

#### 重要变化

- 移除 v1.1.0 引入的通用 Provider、MCP、Skills、备份和配置管理中心；v2.0 明确聚焦本地用量与会话分析。
- 重构 JSONL 读取与会话索引，支持字节级增量扫描、原子更新、源身份校验和对损坏或不完整文件的有限恢复。
- 历史费用现在绑定事件发生时的定价快照，后续目录更新不会静默重算已有费用。
- 应用数据库会自动迁移，并在需要时重建派生扫描状态；Agent 原始文件不会被修改。

#### 发布范围

- 官方安装包支持 macOS Apple Silicon 和 Windows x64。
- v2.0 的 macOS Intel 与 Linux 用户需要从源码构建。

## [1.1.0] - 2026-05-06

### Added
- Desktop configuration management for providers, MCP servers, skills, backups, and synchronization.
- Skills inventory and CLI support views.

### Fixed
- Stabilized backup layout and split large frontend chunks.
- Corrected skills CLI button and inventory states.

## [1.0.0] - 2026-04-23

### Added
- Global source filter (Claude/Codex/OpenClaw) applied to all API endpoints and charts
- API Calls stat card with backend COUNT(*) query
- Sticky top bar merging header and controls into one component
- Empty state graphics for charts when no data
- IBM Plex Mono / Fira Code for stat card numbers
- Project column text truncation with ellipsis
- Responsive breakpoints: 4-col → 2-col → 1-col stats grid
- Inter font loaded from Google Fonts
- Stat card hover lift animation
- Refresh button continuous spin animation
- OpenClaw badge styling

### Changed
- Panel order: Tokens → Cost → Sessions → Prompts (stat cards), Token Usage → Cost Trend → Cost by Model (charts)
- Charts layout: Token Usage full-width, Cost Trend 3/5, Cost by Model 2/5
- Cost trend chart: stacked bar by model (was line chart)
- Pie chart legend: top horizontal with scroll (was right vertical)
- Model color consistency: same model gets same color across pie and bar charts
- Header backdrop-filter fixed with proper RGB CSS variables

### Fixed
- Filter `<synthetic>` model records from Claude Code collector
- Filter `delivery-mirror` internal records from OpenClaw collector
- Clean up synthetic/delivery-mirror records from database on startup
- GetSessions double source filter bug (source param appended twice)
- API date validation: returns 400 JSON error for invalid dates or reversed ranges

## [0.1.0] - 2026-04-03

### Added
- Claude Code session JSONL parser
- Codex CLI session JSONL parser
- SQLite storage with automatic schema migration
- litellm pricing sync with cost backfill
- Web dashboard with ECharts (dark theme)
  - Summary cards: total cost, tokens, sessions, prompts
  - Cost by model (pie chart)
  - Cost over time (line chart)
  - Token usage over time (line chart)
  - Daily sessions (bar chart)
  - Session list table
  - Date range filter
- REST API endpoints for all dashboard data
- Incremental file scanning with deduplication
- GoReleaser CI/CD for cross-platform releases
- Bilingual documentation (English + Chinese)
- Unit tests for collectors, pricing calculation, and storage layer
- Godoc comments on all exported types and functions
- GitHub issue templates (bug report, feature request) and PR template
- Unique index on usage_records for crash-recovery deduplication
- Docker support: multi-stage Dockerfile with distroless runtime
- Docker Compose for one-command deployment
- Docker CI/CD workflow for multi-arch images (amd64 + arm64) on ghcr.io
- `--config` CLI flag with search order: flag > `/etc/agent-usage/config.yaml` > `./config.yaml`
- Docker-specific config (`config.docker.yaml`) with 0.0.0.0 bind and container paths

### Changed
- Server binds to `127.0.0.1` by default instead of `0.0.0.0`
- Added `bind_address` config option for server
- Default database filename changed from `devobs.db` to `agent-usage.db`
- INSERT statements use `INSERT OR IGNORE` for idempotent crash recovery
