# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Build & Run

### Desktop app (Tauri)

```bash
npm install                               # install frontend deps
npx tauri dev                             # dev mode (hot-reload frontend + Rust)
npx tauri build                           # production build → .app/.dmg/.msi/.deb
```

Before `tauri build`, place the Go sidecar binary in `src-tauri/binaries/`:

```bash
# macOS arm64 example:
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go build -o src-tauri/binaries/agent-usage-aarch64-apple-darwin .

# macOS x86_64:
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
  go build -o src-tauri/binaries/agent-usage-x86_64-apple-darwin .

# Linux:
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -o src-tauri/binaries/agent-usage-x86_64-unknown-linux-gnu .

# Windows:
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -o src-tauri/binaries/agent-usage-x86_64-pc-windows-msvc.exe .
```

Binary naming must match Tauri's `externalBin` convention: `agent-usage-{rust-target-triple}[.exe]`.

### Go backend (standalone, for development)

```bash
go build -o agent-usage-desktop .                # build binary
./agent-usage-desktop                             # run (reads config.yaml by default)
./agent-usage-desktop --config path/to/config.yaml
./agent-usage-desktop --port 9800                 # override server port
./agent-usage-desktop version                     # print version info
```

## Testing

```bash
go test ./...                             # all tests
go test ./internal/collector/...          # single package
go test ./internal/storage/... -run TestDedup  # single test
```

No CGO required — the SQLite driver (`modernc.org/sqlite`) is pure Go.

## Architecture

Single-binary Go application that collects AI coding agent token usage from local JSONL session files, stores it in SQLite, and serves a REST API. Also ships as a Tauri v2 desktop app wrapping the same Go backend as a sidecar process.

**Data flow:** Collectors scan session dirs → parse JSONL → write to SQLite (with dedup) → pricing synced from litellm → costs calculated → served via REST API.

### Desktop app (Tauri)

The desktop app uses a layered architecture:

- **Go sidecar** — the same Go binary, bundled inside the Tauri app via `externalBin`. Tauri's Rust layer spawns it with `--port <dynamic>` and `--config ~/.config/agent-usage/config.yaml`. Port is discovered via `find_available_port()` (bind to `:0`), health-checked via `/api/health`.
- **Rust layer** (`src-tauri/src/`) — manages sidecar lifecycle (start, health check, crash recovery), system tray, autostart, OS notifications, and Tauri commands.
- **React frontend** (`src/`) — React 18 + TypeScript + Vite + Tailwind CSS v4. Communicates with Go backend via HTTP (port obtained through Tauri `invoke`). i18n (en/zh), dark/light/system theme.

Key Rust files:
- `src-tauri/src/main.rs` — app setup, plugin registration, sidecar spawn, notification loop
- `src-tauri/src/sidecar.rs` — `SidecarState` (AtomicU16 port + Mutex child), start/kill/restart
- `src-tauri/src/commands.rs` — Tauri commands: get_sidecar_port, get/set_cost_threshold, get/set_notifications_enabled
- `src-tauri/src/tray.rs` — system tray: Show Panel, Quit; left-click toggles window
- `src-tauri/capabilities/default.json` — Tauri v2 permissions (shell, autostart, notification)

### Key packages

- `internal/collector` — Source-specific JSONL parsers. Each collector implements `Scan()` which walks session directories and calls `processFile()` for incremental parsing. File offsets tracked in `file_state` table to avoid re-reading. `claude.go`/`claude_process.go` is the reference implementation for adding new sources. Collectors must normalize token fields to match the non-overlapping semantics defined below. Collectors also extract individual user prompt events (with timestamps) into the `prompt_events` table for time-accurate prompt counting.
- `internal/storage` — SQLite layer. `sqlite.go` has schema + versioned migrations (tracked via `meta` table with `migration_{id}` keys, each runs once), `queries.go` handles writes, `api.go` handles reads, `costs.go` does cost recalculation. All DB access serialized through a mutex (`DB.mu`). Key tables: `usage_records` (per-API-call token/cost data), `sessions` (session metadata), `prompt_events` (per-prompt timestamps for time-range queries), `pricing` (model prices), `file_state` (scan offsets and parser context for incremental scanning).
- `internal/pricing` — Fetches model prices from litellm's GitHub JSON. Cost formula: `input × input_price + cache_creation × cache_creation_price + cache_read × cache_read_price + output × output_price`.
- `internal/server` — HTTP server with REST API endpoints (`/api/stats`, `/api/cost-by-model`, etc.). `/api/stats` returns aggregate metrics including `cache_hit_rate` (ratio of cache read tokens to total input tokens). All endpoints accept `from`, `to`, `source` (optional: `claude`/`codex`/`openclaw`), and time-series endpoints accept `granularity`. Invalid dates or reversed ranges return `400` with a JSON error message.
- `internal/config` — YAML config loader. Search order: `--config` flag → `./config.yaml`. Supports `~` expansion in paths.

### Token semantics

All token fields are **non-overlapping components** that sum to the total:

```
input_tokens              — non-cached input (mutually exclusive with cache fields)
cache_read_input_tokens   — input tokens served from cache
cache_creation_input_tokens — input tokens written to cache
output_tokens             — total output tokens
reasoning_output_tokens   — reasoning tokens (subset of output, informational only)

total_input  = input_tokens + cache_read_input_tokens + cache_creation_input_tokens
total_output = output_tokens
total_tokens = total_input + total_output
```

If a data source reports `input_tokens` as the total (including cache), the collector must subtract cache tokens before storing. If a source reports non-cached input natively, store as-is.

### Deduplication

Usage records are deduped via a unique index on `(session_id, model, timestamp, input_tokens, output_tokens)`. Incremental file scanning uses stored offsets in `file_state` to resume from where it left off. For data sources where session metadata only appears at the top of the file (Codex, OpenClaw), `file_state.scan_context` stores parser state (sessionID, cwd, version, model) as JSON so incremental scans can restore context without re-reading the file from the beginning.

## Conventions

- Conventional Commits (`feat:`, `fix:`, `refactor:`, etc.).
- Subagents used for this project must not use any model lower than `gpt-5.4`.
- Version/commit/date injected via ldflags at build time.
- Desktop app frontend lives in `src/` (React + TypeScript).
- Tauri config is in `src-tauri/tauri.conf.json`. CSP restricts connect-src to `127.0.0.1` and `localhost`.
- CI/CD: `.github/workflows/desktop.yml` builds for macOS (arm64 + x86_64), Windows, Linux via `tauri-apps/tauri-action`.
