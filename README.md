# Agent Usage

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)]()

Agent Usage is a local, single-user application for understanding AI coding-agent usage, cost, throughput, and session history.

**[中文文档](README.zh-CN.md)**

It reads source data already stored by agents on this computer, builds a local SQLite index, and presents the result in a Tauri desktop app. Claude Code and Codex support deep session retrospective. OpenCode and OpenClaw currently provide statistics only.

<p align="center">
  <img src="docs/desktop.png" alt="Agent Usage desktop application" width="700">
</p>

## What It Provides

- Usage and cost trends across supported local agents
- Project, model, and source breakdowns
- Locally observed RPM/TPM throughput analysis
- Searchable Claude Code and Codex sessions with readable event timelines
- Automatic incremental scanning and deduplication
- Cost alerts, system tray operation, autostart, light/dark themes, and English/Chinese UI

The main navigation is Overview, Sessions, and Settings. Overview summarizes usage and cost; Sessions supports filtering, full-text search, and retrospective timelines; Settings controls the local collectors and application behavior.

## Data Sources

| Source | Default location | Input format | Current depth |
| --- | --- | --- | --- |
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `~/.claude/projects/<project>/<session>.jsonl` | JSONL | Statistics and deep session retrospective |
| [Codex CLI](https://github.com/openai/codex) | `~/.codex/sessions/<year>/<month>/<day>/<session>.jsonl` | JSONL | Statistics and deep session retrospective |
| [OpenClaw](https://github.com/openclaw/openclaw) | `~/.openclaw/agents/<agentId>/sessions/<sessionId>.jsonl` | JSONL | Statistics only |
| [OpenCode](https://github.com/anomalyco/opencode) | `~/.local/share/opencode/opencode.db` | SQLite | Statistics only |

Collectors read incrementally and record file offsets so unchanged content is not parsed again. Supported sources are normalized into the same statistical model; only Claude Code and Codex currently populate the retrospective event index.

## Metric Semantics

Token totals use four non-overlapping components:

- **non-cached input**: input that was not served from or written to cache
- **cache read input**: input served from cache
- **cache creation input**: input written to cache
- **output tokens**: all generated output

The totals are calculated as:

```text
total input  = non-cached input + cache read input + cache creation input
total output = output tokens
total tokens = total input + total output
```

Reasoning output is informational and is a subset of output, so it is never added to `total output` again.

RPM and TPM are locally observed throughput, not provider quota or rate-limit utilization. They describe API calls and tokens visible in local source records. They do not reveal an account limit, remaining capacity, throttling state, or traffic that is absent from those files.

## Local Data and Network Access

Raw agent files are the source of truth. The local SQLite index can be rebuilt when source files or parser versions change. The default database and configuration stay on this computer.

Session content is never uploaded. The application binds its API to loopback by default. Pricing sync is the only routine network request: it reads model pricing from [litellm's GitHub data](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json) and stores an immutable local snapshot. Syncing does not rewrite costs that were already assigned to usage records.

## Install

Download a supported installer from [GitHub Releases](https://github.com/hongshuo-wang/agent-usage-desktop/releases):

| Platform | File |
| --- | --- |
| macOS (Apple Silicon) | `Agent Usage_x.x.x_aarch64.dmg` |
| Windows | `Agent Usage_x.x.x_x64-setup.exe` |

For macOS Intel and Linux, build from source.

Unsigned macOS builds may be reported as damaged. After moving the app into Applications, remove the quarantine attribute once:

```bash
xattr -cr "/Applications/Agent Usage.app"
```

Launch Agent Usage. It runs in the system tray and begins scanning enabled local sources.

## Configuration

The desktop app creates `~/.config/agent-usage/config.yaml` on first launch. Settings can also be changed in the app.

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

## Build From Source

Prerequisites:

- [Go](https://go.dev/) 1.25+
- [Node.js](https://nodejs.org/) 20+
- [Rust](https://rustup.rs/) stable
- Linux only: `libwebkit2gtk-4.1-dev` and `libappindicator3-dev`

Install dependencies, prepare the sidecar, and run the desktop app:

```bash
npm install

# macOS Apple Silicon
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go build -o src-tauri/binaries/agent-usage-aarch64-apple-darwin .

npx tauri dev
```

Production builds use the same sidecar step followed by:

```bash
npx tauri build
```

Tauri requires the sidecar name `agent-usage-{rust-target-triple}[.exe]`. Build the binary that matches the desktop target:

```bash
# macOS Apple Silicon
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go build -o src-tauri/binaries/agent-usage-aarch64-apple-darwin .

# macOS Intel
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
  go build -o src-tauri/binaries/agent-usage-x86_64-apple-darwin .

# Linux x86_64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -o src-tauri/binaries/agent-usage-x86_64-unknown-linux-gnu .

# Windows x86_64
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -o src-tauri/binaries/agent-usage-x86_64-pc-windows-msvc.exe .
```

The standalone Go backend is also useful during development:

```bash
go build -o agent-usage-desktop .
./agent-usage-desktop
./agent-usage-desktop --config path/to/config.yaml
./agent-usage-desktop --port 9800
./agent-usage-desktop version
```

Run all project checks with:

```bash
go test ./...
npm test
npm run build
cargo test --manifest-path src-tauri/Cargo.toml
```

## Cost Calculation

Prices are expressed per token and stored locally. Because the four token components are non-overlapping, cost is calculated without subtracting cache tokens from input again:

```text
cost = non-cached input × input price
     + cache creation input × cache creation price
     + cache read input × cache read price
     + output tokens × output price
```

Displayed cost is a local estimate, not a provider invoice. Each usage record is priced from the latest locally stored pricing snapshot available at the event time, so later pricing syncs do not recalculate its historical cost. Usage that could not be priced is reported as unpriced and its cost is excluded from the estimate.

Costs preserved from databases created before pricing snapshots are labeled legacy. They remain part of the local estimate, but their original pricing source cannot be traced.

## Technology

- Tauri v2 with a Rust process layer and system WebView
- React, TypeScript, Vite, Tailwind CSS, and ECharts
- Pure-Go backend and `modernc.org/sqlite`, with no CGO requirement

## Roadmap

- [ ] Hermes session retrospective support
- [ ] Read-only discovery of global `CLAUDE.md`, `AGENTS.md`, and memory files, with explicit project-level import
- [ ] Branded PNG/PDF BI sharing containing the project name, GitHub link, and Linux.do link

## Community

Source code and issues: [GitHub](https://github.com/hongshuo-wang/agent-usage-desktop)

Discussion: [Linux.do](https://linux.do/t/topic/1922004)

## License

[Apache 2.0](LICENSE)
