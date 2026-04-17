# agent-usage-desktop

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)]()

Lightweight, cross-platform desktop app for tracking AI coding agent usage & costs.

**[中文文档](README_CN.md)**

Collects local session data from Claude Code, Codex, OpenClaw, OpenCode and other AI coding agents, calculates costs automatically, and presents token usage, cost trends, and session details through a built-in dashboard.

![Dashboard](docs/dashboard.png)

## Features

- **Local file parsing** — reads Claude Code, Codex CLI, OpenClaw session files and OpenCode SQLite database directly
- **Automatic cost calculation** — fetches model pricing from [litellm](https://github.com/BerriAI/litellm), supports backfill when prices update
- **SQLite storage** — single file, zero ops, data is correctable
- **Dashboard** — dark/light themed UI with ECharts: cost breakdown, token trends, session list
- **Incremental scanning** — watches for new sessions, deduplicates automatically
- **Cross-platform** — macOS, Windows, Linux
- **Native desktop app** — Tauri v2 with system tray, autostart, cost alert notifications, dark/light theme, i18n (EN/ZH)

## Install

Download the latest installer for your platform from [GitHub Releases](https://github.com/hongshuo-wang/agent-usage-desktop/releases):

| Platform | File |
|----------|------|
| macOS (Apple Silicon) | `Agent Usage_x.x.x_aarch64.dmg` |
| macOS (Intel) | `Agent Usage_x.x.x_x64.dmg` |
| Windows | `Agent Usage_x.x.x_x64-setup.exe` |
| Linux | `Agent Usage_x.x.x_amd64.AppImage` or `.deb` |

Launch the app — it runs in the system tray and starts collecting data automatically.

## Query Usage from Agent Conversations

The skill works standalone — no need to install or run the agent-usage-desktop server. It parses local JSONL session files directly. If the agent-usage-desktop server is detected, it automatically switches to API queries for more accurate cost data.

```bash
# Installed via vercel-labs/skills, supports Claude Code, Cursor, Kiro, and 40+ agents
npx skills add hongshuo-wang/agent-usage-desktop -y
```

Once installed, try: `check agent usage` or `agent usage stats`. See [`skills/agent-usage-desktop/SKILL.md`](skills/agent-usage-desktop/SKILL.md) for details.

## Configuration

The desktop app stores its config at `~/.config/agent-usage/config.yaml` (created on first launch with sensible defaults). You can also edit it from the app's settings.

```yaml
server:
  port: 9800
  bind_address: "127.0.0.1"

collectors:
  claude:
    enabled: true
    paths:
      - "~/.claude/projects"
    scan_interval: 60s
  codex:
    enabled: true
    paths:
      - "~/.codex/sessions"
    scan_interval: 60s
  openclaw:
    enabled: true
    paths:
      - "~/.openclaw/agents"
    scan_interval: 60s
  opencode:
    enabled: true
    paths:
      - "~/.local/share/opencode/opencode.db"
    scan_interval: 60s

storage:
  path: "./agent-usage.db"

pricing:
  sync_interval: 1h  # fetched from GitHub; set HTTPS_PROXY env var if this fails
```

## Supported Data Sources

| Source | Session Location | Format |
|--------|-----------------|--------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `~/.claude/projects/<project>/<session>.jsonl` | JSONL |
| [Codex CLI](https://github.com/openai/codex) | `~/.codex/sessions/<year>/<month>/<day>/<session>.jsonl` | JSONL |
| [OpenClaw](https://github.com/openclaw/openclaw) | `~/.openclaw/agents/<agentId>/sessions/<sessionId>.jsonl` | JSONL |
| [OpenCode](https://github.com/anomalyco/opencode) | `~/.local/share/opencode/opencode.db` | SQLite |

## Build from Source

If you prefer to build the app yourself instead of using the pre-built installers:

### Prerequisites

- [Go](https://go.dev/) 1.25+
- [Node.js](https://nodejs.org/) 20+
- [Rust](https://rustup.rs/) (stable)
- Platform-specific dependencies:
  - **Linux**: `libwebkit2gtk-4.1-dev`, `libappindicator3-dev`

### Steps

```bash
git clone https://github.com/hongshuo-wang/agent-usage-desktop.git
cd agent-usage-desktop

# 1. Install frontend dependencies
npm install

# 2. Build the Go sidecar for your platform (pick ONE):

#    macOS Apple Silicon:
CGO_ENABLED=0 go build -o src-tauri/binaries/agent-usage-aarch64-apple-darwin .

#    macOS Intel:
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o src-tauri/binaries/agent-usage-x86_64-apple-darwin .

#    Linux x86_64:
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o src-tauri/binaries/agent-usage-x86_64-unknown-linux-gnu .

#    Windows x86_64:
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o src-tauri/binaries/agent-usage-x86_64-pc-windows-msvc.exe .

# 3. Build the desktop app
npx tauri build
```

The installer will be in:
- **macOS**: `src-tauri/target/release/bundle/dmg/`
- **Windows**: `src-tauri/target/release/bundle/nsis/`
- **Linux**: `src-tauri/target/release/bundle/appimage/` or `deb/`

### Development (hot-reload)

```bash
# Build sidecar first (step 2 above), then:
npx tauri dev
```

## Dashboard

The built-in dashboard provides:

- **Sticky top bar** — time presets, granularity, source filter (Claude/Codex/OpenClaw/OpenCode), auto-refresh
- **Summary cards** — total tokens, cost, sessions, prompts, API calls
- **Token usage** — stacked bar chart (input/output/cache read/cache write)
- **Cost trend** — stacked bar chart by model with consistent color mapping
- **Cost by model** — doughnut chart with percentage labels
- **Session list** — sortable, filterable table with expandable per-model detail
- **Dark/Light theme** — system-aware with manual toggle
- **i18n** — English and Chinese
- **Timezone handling** — timestamps stored in UTC, displayed in your local timezone

## Cost Calculation

Pricing is fetched from [litellm's model price database](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json) and stored locally.

```
cost = (input - cache_read - cache_creation) × input_price
     + cache_creation × cache_creation_price
     + cache_read × cache_read_price
     + output × output_price
```

When prices update, historical records are automatically backfilled.

## Tech Stack

- **Tauri v2** — desktop app framework (Rust core + system WebView)
- **React 18** + TypeScript + Vite — frontend
- **Tailwind CSS v4** — styling
- **Go** — backend (pure Go, no CGO required)
- **SQLite** via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) — pure Go SQLite driver
- **ECharts** — charting

## Roadmap

- [ ] More agent sources (Cursor, Copilot, etc.)
- [ ] Export to CSV/JSON
- [x] ~~Cost alert notifications~~ — implemented
- [ ] Multi-user support

## Community

Join the discussion at [Linux.do](https://linux.do/t/topic/1922004).

## License

[Apache 2.0](LICENSE)
