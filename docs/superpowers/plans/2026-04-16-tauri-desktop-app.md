# Agent Usage Desktop — Tauri 桌面应用实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有 agent-usage Web 应用改造为 Tauri v2 桌面应用，Go 后端作为 sidecar 运行，React 前端重写，支持系统托盘/开机自启/通知。

**Architecture:** Tauri v2 shell 包装 Go sidecar 进程 + React WebView 前端。Rust 层负责进程管理、托盘、自启、通知。Go 后端仅新增 `--port` flag、`/api/health` 端点和 CORS 中间件。

**Tech Stack:** Tauri v2, Rust, React 18, TypeScript, Vite, shadcn/ui, Tailwind CSS, echarts-for-react, react-i18next, Go 1.25

**Spec:** `docs/superpowers/specs/2026-04-16-tauri-desktop-app-design.md`

---

## File Structure

### Go 后端改动（最小）

- Modify: `main.go` — 新增 `--port` flag
- Modify: `internal/server/server.go` — 新增 `/api/health` 端点 + CORS 中间件
- Create: `internal/server/server_test.go` — health 和 CORS 测试

### Tauri Rust 层

- Create: `src-tauri/Cargo.toml` — Rust 依赖
- Create: `src-tauri/tauri.conf.json` — Tauri 配置（窗口、sidecar、插件）
- Create: `src-tauri/build.rs` — Tauri 构建脚本
- Create: `src-tauri/icons/` — 应用图标（Tauri 生成）
- Create: `src-tauri/src/main.rs` — 入口，注册插件和命令
- Create: `src-tauri/src/sidecar.rs` — Go 进程生命周期管理
- Create: `src-tauri/src/tray.rs` — 系统托盘
- Create: `src-tauri/src/commands.rs` — Tauri commands（前端调用的 Rust 函数）

### React 前端

- Create: `package.json` — 项目依赖
- Create: `vite.config.ts` — Vite + Tauri 配置
- Create: `tsconfig.json` / `tsconfig.node.json`
- Create: `index.html` — Vite 入口 HTML
- Create: `src/main.tsx` — React 入口
- Create: `src/App.tsx` — 路由和布局
- Create: `src/lib/api.ts` — API client（封装 fetch 调用）
- Create: `src/lib/i18n.ts` — react-i18next 配置
- Create: `src/lib/locales/en.json` — 英文翻译
- Create: `src/lib/locales/zh.json` — 中文翻译
- Create: `src/lib/utils.ts` — 格式化工具函数（fmtCost, fmtTokens 等）
- Create: `src/components/Layout.tsx` — 顶部导航栏 + 页面容器
- Create: `src/components/StatCard.tsx` — 统计卡片组件
- Create: `src/components/TimeRangeSelector.tsx` — 时间范围选择器
- Create: `src/components/ChartCard.tsx` — 图表卡片容器
- Create: `src/pages/Dashboard.tsx` — Dashboard 页面
- Create: `src/pages/Sessions.tsx` — Sessions 页面
- Create: `src/pages/Settings.tsx` — 设置页面
- Create: `src/styles/globals.css` — Tailwind 全局样式 + 主题变量

### 构建与 CI

- Create: `.github/workflows/desktop.yml` — 桌面应用 CI/CD
- Modify: `.gitignore` — 添加 node_modules、dist、src-tauri/target 等

---

## Task 1: Go 后端改动 — `--port` flag + `/api/health` + CORS

**Files:**
- Modify: `main.go:29` — 新增 `--port` flag
- Modify: `internal/server/server.go:30-44` — 新增 health 端点 + CORS 中间件
- Create: `internal/server/server_test.go`

- [ ] **Step 1: 写 health 端点和 CORS 的测试**

```go
// internal/server/server_test.go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/briqt/agent-usage/internal/storage"
)

func tempDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestHealthEndpoint(t *testing.T) {
	db := tempDB(t)
	srv := New(db, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", resp["status"])
	}
}

func TestCORSHeaders(t *testing.T) {
	db := tempDB(t)
	srv := New(db, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	req.Header.Set("Origin", "tauri://localhost")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("missing CORS header, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/server/... -run "TestHealth|TestCORS" -v`
Expected: FAIL — `Handler` method not defined

- [ ] **Step 3: 实现 health 端点、CORS 中间件、Handler 方法**

修改 `internal/server/server.go`，将路由注册提取到 `Handler()` 方法，添加 health 端点和 CORS：

```go
// 在 Server struct 下方添加 Handler 方法，Start 调用 Handler

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/cost-by-model", s.handleCostByModel)
	mux.HandleFunc("/api/cost-over-time", s.handleCostOverTime)
	mux.HandleFunc("/api/tokens-over-time", s.handleTokensOverTime)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/session-detail", s.handleSessionDetail)

	return corsMiddleware(mux)
}

func (s *Server) Start() error {
	log.Printf("server: listening on %s", s.addr)
	return http.ListenAndServe(s.addr, s.Handler())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/server/... -run "TestHealth|TestCORS" -v`
Expected: PASS

- [ ] **Step 5: 添加 `--port` flag 到 main.go**

修改 `main.go:29-35`：

```go
configPath := flag.String("config", "", "path to config file")
portFlag := flag.Int("port", 0, "override server port")
flag.Parse()

cfg, err := config.Load(config.ResolveConfigPath(*configPath))
if err != nil {
	log.Fatalf("config: %v", err)
}

if *portFlag > 0 {
	cfg.Server.Port = *portFlag
}
```

- [ ] **Step 6: 运行全部测试确认无回归**

Run: `go test ./... -v`
Expected: ALL PASS

- [ ] **Step 7: 提交**

```bash
git add main.go internal/server/server.go internal/server/server_test.go
git commit -m "feat: add --port flag, /api/health endpoint, and CORS support

Prepares Go backend for Tauri sidecar integration."
```

---

## Task 2: 项目脚手架 — Tauri + React + Vite 初始化

**Files:**
- Create: `package.json`
- Create: `vite.config.ts`
- Create: `tsconfig.json`
- Create: `tsconfig.node.json`
- Create: `index.html`
- Create: `src/main.tsx`
- Create: `src/App.tsx`
- Create: `src/styles/globals.css`
- Create: `src-tauri/Cargo.toml`
- Create: `src-tauri/tauri.conf.json`
- Create: `src-tauri/build.rs`
- Create: `src-tauri/src/main.rs`
- Modify: `.gitignore`

- [ ] **Step 1: 初始化 npm 项目并安装依赖**

```bash
npm init -y
npm install react react-dom
npm install -D typescript @types/react @types/react-dom \
  vite @vitejs/plugin-react \
  tailwindcss @tailwindcss/vite \
  @tauri-apps/cli@latest @tauri-apps/api@latest
```

- [ ] **Step 2: 创建 vite.config.ts**

```typescript
// vite.config.ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const host = process.env.TAURI_DEV_HOST;

export default defineConfig(async () => ({
  plugins: [react(), tailwindcss()],
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
    host: host || false,
    hmr: host ? { protocol: "ws", host, port: 1421 } : undefined,
    watch: { ignored: ["**/src-tauri/**"] },
  },
}));
```

- [ ] **Step 3: 创建 tsconfig.json 和 tsconfig.node.json**

```json
// tsconfig.json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "baseUrl": ".",
    "paths": { "@/*": ["./src/*"] }
  },
  "include": ["src"]
}
```

```json
// tsconfig.node.json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2023"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "strict": true
  },
  "include": ["vite.config.ts"]
}
```

- [ ] **Step 4: 创建 index.html（Vite 入口）**

```html
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Agent Usage</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 5: 创建 src/styles/globals.css（Tailwind + 主题变量）**

```css
/* src/styles/globals.css */
@import "tailwindcss";

@theme {
  --color-card: #ffffff;
  --color-card-foreground: #0f172a;
  --color-border: #e2e8f0;
  --color-muted: #64748b;
  --color-accent: #2563eb;
  --color-accent-hover: #1d4ed8;
  --color-green: #16a34a;
}

:root {
  font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

.dark {
  --color-card: #0e1223;
  --color-card-foreground: #f1f5f9;
  --color-border: #1e293b;
  --color-muted: #94a3b8;
  --color-accent: #3b82f6;
  --color-accent-hover: #2563eb;
  --color-green: #22c55e;
}
```

- [ ] **Step 6: 创建 src/main.tsx 和 src/App.tsx（最小骨架）**

```tsx
// src/main.tsx
import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./styles/globals.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
```

```tsx
// src/App.tsx
function App() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <h1 className="text-2xl font-bold p-8">Agent Usage Desktop</h1>
      <p className="px-8 text-muted">App shell is working.</p>
    </div>
  );
}

export default App;
```

- [ ] **Step 7: 初始化 Tauri**

```bash
npx tauri init
```

在交互提示中填入：
- App name: `Agent Usage`
- Window title: `Agent Usage`
- Web assets path: `../dist`
- Dev URL: `http://localhost:1420`
- Dev command: `npm run dev`
- Build command: `npm run build`

- [ ] **Step 8: 配置 Tauri sidecar**

修改 `src-tauri/tauri.conf.json`，在 `bundle` 下添加 sidecar 声明：

```json
{
  "bundle": {
    "externalBin": ["binaries/agent-usage"]
  }
}
```

修改 `src-tauri/src/main.rs` 为最小入口：

```rust
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    tauri::Builder::default()
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
```

- [ ] **Step 9: 更新 .gitignore**

追加以下内容到 `.gitignore`：

```
# Tauri & Frontend
node_modules/
dist/
src-tauri/target/
src-tauri/binaries/
*.tsbuildinfo
```

- [ ] **Step 10: 验证前端 dev server 能启动**

Run: `npm run dev`
Expected: Vite dev server 在 http://localhost:1420 启动，浏览器能看到 "Agent Usage Desktop"

注意：这一步只验证前端，Tauri 完整启动需要 sidecar 二进制，在后续 task 中处理。

- [ ] **Step 11: 提交**

```bash
git add package.json vite.config.ts tsconfig.json tsconfig.node.json \
  index.html src/main.tsx src/App.tsx src/styles/globals.css \
  src-tauri/ .gitignore
git commit -m "feat: initialize Tauri v2 + React + Vite project scaffold"
```

---

## Task 3: Rust Sidecar 管理 — Go 进程生命周期

**Files:**
- Create: `src-tauri/src/sidecar.rs`
- Create: `src-tauri/src/commands.rs`
- Modify: `src-tauri/src/main.rs`
- Modify: `src-tauri/Cargo.toml`

- [ ] **Step 1: 创建 sidecar.rs — Go 进程管理**

```rust
// src-tauri/src/sidecar.rs
use std::sync::atomic::{AtomicU16, Ordering};
use std::sync::Mutex;
use tauri::Manager;
use tauri_plugin_shell::ShellExt;
use tauri_plugin_shell::process::{CommandChild, CommandEvent};

pub struct SidecarState {
    pub port: AtomicU16,
    pub child: Mutex<Option<CommandChild>>,
}

/// Find an available TCP port
fn find_available_port() -> u16 {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    listener.local_addr().unwrap().port()
}

/// Wait for the Go sidecar to be ready by polling /api/health
async fn wait_for_health(port: u16) -> Result<(), String> {
    let url = format!("http://127.0.0.1:{}/api/health", port);
    let client = reqwest::Client::new();
    for _ in 0..50 {
        // 50 * 100ms = 5s timeout
        if let Ok(resp) = client.get(&url).send().await {
            if resp.status().is_success() {
                return Ok(());
            }
        }
        tokio::time::sleep(std::time::Duration::from_millis(100)).await;
    }
    Err("Sidecar health check timed out".into())
}

/// Start the Go sidecar process
pub async fn start_sidecar(app: &tauri::AppHandle) -> Result<u16, String> {
    let port = find_available_port();

    // Resolve config path: ~/.config/agent-usage/config.yaml
    let home = dirs::home_dir().ok_or("Cannot find home directory")?;
    let config_dir = home.join(".config").join("agent-usage");
    let config_path = config_dir.join("config.yaml");

    // Create default config if it doesn't exist
    if !config_path.exists() {
        std::fs::create_dir_all(&config_dir).map_err(|e| e.to_string())?;
        // Go binary handles defaults when config file is missing, so just touch it
    }

    let sidecar_command = app
        .shell()
        .sidecar("agent-usage")
        .map_err(|e| e.to_string())?
        .args(["--config", config_path.to_str().unwrap(), "--port", &port.to_string()]);

    let (mut rx, child) = sidecar_command.spawn().map_err(|e| e.to_string())?;

    // Log sidecar output in background
    let app_handle = app.clone();
    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(line) => {
                    println!("[sidecar] {}", String::from_utf8_lossy(&line));
                }
                CommandEvent::Stderr(line) => {
                    eprintln!("[sidecar] {}", String::from_utf8_lossy(&line));
                }
                CommandEvent::Terminated(payload) => {
                    eprintln!("[sidecar] terminated: {:?}", payload);
                    // Auto-restart on crash
                    let restart_handle = app_handle.clone();
                    tauri::async_runtime::spawn(async move {
                        eprintln!("[sidecar] attempting restart...");
                        match crate::sidecar::restart_sidecar(&restart_handle).await {
                            Ok(port) => println!("[sidecar] restarted on port {}", port),
                            Err(e) => {
                                eprintln!("[sidecar] restart failed: {}", e);
                                let _ = restart_handle.emit("sidecar-crashed", ());
                            }
                        }
                    });
                }
                _ => {}
            }
        }
    });

    // Wait for health check
    wait_for_health(port).await?;

    // Store child process handle and port
    let state = app.state::<SidecarState>();
    state.port.store(port, Ordering::Relaxed);
    *state.child.lock().unwrap() = Some(child);

    Ok(port)
}

/// Kill the sidecar process gracefully (SIGTERM then SIGKILL)
pub fn kill_sidecar(app: &tauri::AppHandle) {
    let state = app.state::<SidecarState>();
    if let Some(child) = state.child.lock().unwrap().take() {
        // child.kill() sends SIGKILL; Tauri's shell plugin doesn't expose SIGTERM directly,
        // so we kill immediately. For graceful shutdown, the Go process handles SIGTERM
        // via its own signal handler if needed.
        let _ = child.kill();
    }
}

/// Restart sidecar after crash
pub async fn restart_sidecar(app: &tauri::AppHandle) -> Result<u16, String> {
    kill_sidecar(app);
    tokio::time::sleep(std::time::Duration::from_millis(500)).await;
    start_sidecar(app).await
}
```

- [ ] **Step 2: 创建 commands.rs — Tauri commands**

```rust
// src-tauri/src/commands.rs
use crate::sidecar::SidecarState;
use std::sync::atomic::Ordering;
use tauri::{State, Manager};

#[tauri::command]
pub fn get_sidecar_port(state: State<SidecarState>) -> u16 {
    state.port.load(Ordering::Relaxed)
}

#[tauri::command]
pub fn get_cost_threshold(app: tauri::AppHandle) -> f64 {
    let path = app.path().app_data_dir().unwrap().join("settings.json");
    if let Ok(data) = std::fs::read_to_string(&path) {
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&data) {
            return v["cost_threshold"].as_f64().unwrap_or(10.0);
        }
    }
    10.0
}

#[tauri::command]
pub fn set_cost_threshold(app: tauri::AppHandle, threshold: f64) -> Result<(), String> {
    let dir = app.path().app_data_dir().unwrap();
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    let path = dir.join("settings.json");
    let settings = serde_json::json!({ "cost_threshold": threshold });
    std::fs::write(&path, serde_json::to_string_pretty(&settings).unwrap())
        .map_err(|e| e.to_string())
}
```

- [ ] **Step 3: 更新 main.rs — 注册 sidecar 和 commands**

```rust
// src-tauri/src/main.rs
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod commands;
mod sidecar;

use sidecar::SidecarState;
use std::sync::Mutex;

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .manage(SidecarState {
            port: AtomicU16::new(0),
            child: Mutex::new(None),
        })
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            // Focus existing window when second instance is launched
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
                let _ = window.set_focus();
            }
        }))
        .setup(|app| {
            let handle = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                match sidecar::start_sidecar(&handle).await {
                    Ok(port) => {
                        println!("Sidecar started on port {}", port);
                    }
                    Err(e) => {
                        eprintln!("Failed to start sidecar: {}", e);
                    }
                }
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::get_sidecar_port,
            commands::get_cost_threshold,
            commands::set_cost_threshold,
        ])
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::Destroyed = event {
                sidecar::kill_sidecar(window.app_handle());
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
```

- [ ] **Step 4: 更新 Cargo.toml 添加依赖**

确保 `src-tauri/Cargo.toml` 包含：

```toml
[dependencies]
tauri = { version = "2", features = [] }
tauri-plugin-shell = "2"
tauri-plugin-single-instance = "2"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
reqwest = { version = "0.12", features = ["json"] }
tokio = { version = "1", features = ["time"] }
dirs = "6"
```

- [ ] **Step 5: 构建 Go sidecar 二进制用于本地测试**

```bash
go build -o src-tauri/binaries/agent-usage-$(rustc -vV | grep host | awk '{print $2}') .
```

- [ ] **Step 6: 验证 Tauri 能编译**

Run: `cd src-tauri && cargo check`
Expected: 编译通过，无错误

- [ ] **Step 7: 提交**

```bash
git add src-tauri/src/sidecar.rs src-tauri/src/commands.rs src-tauri/src/main.rs src-tauri/Cargo.toml
git commit -m "feat: implement Rust sidecar lifecycle management

Starts Go binary on available port, polls /api/health,
handles crash events and cleanup on exit."
```

---

## Task 4: 前端基础设施 — API Client + i18n + 工具函数

**Files:**
- Create: `src/lib/api.ts`
- Create: `src/lib/i18n.ts`
- Create: `src/lib/locales/en.json`
- Create: `src/lib/locales/zh.json`
- Create: `src/lib/utils.ts`

- [ ] **Step 1: 安装 i18n 依赖**

```bash
npm install react-i18next i18next i18next-browser-languagedetector
npm install echarts echarts-for-react react-router-dom
```

- [ ] **Step 2: 创建 API client**

```typescript
// src/lib/api.ts
import { invoke } from "@tauri-apps/api/core";

let sidecarPort: number | null = null;

async function getPort(): Promise<number> {
  if (sidecarPort) return sidecarPort;
  // In Tauri, get port from Rust backend
  try {
    sidecarPort = await invoke<number>("get_sidecar_port");
  } catch {
    // Fallback for dev mode without Tauri
    sidecarPort = 9800;
  }
  return sidecarPort;
}

function buildQuery(params: {
  from: string;
  to: string;
  granularity?: string;
  source?: string;
}): string {
  const q = new URLSearchParams();
  q.set("from", params.from);
  q.set("to", params.to);
  if (params.granularity) q.set("granularity", params.granularity);
  if (params.source) q.set("source", params.source);
  q.set("tz_offset", String(new Date().getTimezoneOffset()));
  return q.toString();
}

export async function fetchAPI<T>(path: string, params: {
  from: string;
  to: string;
  granularity?: string;
  source?: string;
}): Promise<T> {
  const port = await getPort();
  const url = `http://127.0.0.1:${port}/api/${path}?${buildQuery(params)}`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`API error: ${res.status}`);
  return res.json();
}

export async function getWebUIUrl(): Promise<string> {
  const port = await getPort();
  return `http://127.0.0.1:${port}`;
}
```

- [ ] **Step 3: 创建 i18n 配置和翻译文件**

从现有 `app.js` 的 I18N 对象提取翻译，创建 `src/lib/locales/en.json` 和 `src/lib/locales/zh.json`。

```typescript
// src/lib/i18n.ts
import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import en from "./locales/en.json";
import zh from "./locales/zh.json";

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: { en: { translation: en }, zh: { translation: zh } },
    fallbackLng: "en",
    interpolation: { escapeValue: false },
    detection: {
      order: ["localStorage", "navigator"],
      lookupLocalStorage: "au-lang",
    },
  });

export default i18n;
```

```json
// src/lib/locales/en.json
{
  "title": "Usage Analytics",
  "totalCost": "Total Cost",
  "totalTokens": "Total Tokens",
  "sessions": "Sessions",
  "prompts": "Prompts",
  "apiCalls": "API Calls",
  "cacheHitRate": "Cache Hit Rate",
  "costByModel": "Cost by Model",
  "costOverTime": "Cost Trend",
  "tokenUsage": "Token Usage",
  "sessionLog": "Session Log",
  "source": "Source",
  "project": "Project",
  "branch": "Branch",
  "time": "Time",
  "tokens": "Tokens",
  "cost": "Cost",
  "model": "Model",
  "calls": "Calls",
  "input": "Input",
  "output": "Output",
  "cacheRead": "Cache Read",
  "cacheCreate": "Cache Write",
  "refresh": "Refresh",
  "settings": "Settings",
  "openWebUI": "Open Web UI",
  "today": "Today",
  "thisWeek": "This Week",
  "thisMonth": "This Month",
  "thisYear": "This Year",
  "last3d": "Last 3 Days",
  "last7d": "Last 7 Days",
  "last30d": "Last 30 Days",
  "custom": "Custom",
  "allSources": "All Sources",
  "claudeCode": "Claude Code",
  "codex": "Codex",
  "openClaw": "OpenClaw",
  "openCode": "OpenCode",
  "noSessions": "No sessions found in this period.",
  "filterProject": "Filter by project...",
  "autostart": "Launch at Login",
  "theme": "Theme",
  "language": "Language",
  "light": "Light",
  "dark": "Dark",
  "system": "System",
  "notification": "Notifications",
  "dailyCostThreshold": "Daily Cost Alert Threshold",
  "enabled": "Enabled",
  "disabled": "Disabled",
  "justNow": "just now",
  "mAgo": "m ago",
  "hAgo": "h ago",
  "dAgo": "d ago",
  "unitMin": "min",
  "unitSec": "sec",
  "autoOn": "Auto",
  "autoOff": "Auto",
  "to": "to",
  "gran_1m": "1 min",
  "gran_30m": "30 min",
  "gran_1h": "1 hour",
  "gran_6h": "6 hours",
  "gran_12h": "12 hours",
  "gran_1d": "1 day",
  "gran_1w": "1 week",
  "gran_1M": "1 month"
}
```

```json
// src/lib/locales/zh.json
{
  "title": "使用分析",
  "totalCost": "总费用",
  "totalTokens": "总 Tokens",
  "sessions": "会话数",
  "prompts": "Prompt 数",
  "apiCalls": "API 调用数",
  "cacheHitRate": "缓存命中率",
  "costByModel": "模型费用占比",
  "costOverTime": "费用趋势",
  "tokenUsage": "Token 用量",
  "sessionLog": "会话记录",
  "source": "来源",
  "project": "项目",
  "branch": "分支",
  "time": "时间",
  "tokens": "Tokens",
  "cost": "费用",
  "model": "模型",
  "calls": "调用次数",
  "input": "输入",
  "output": "输出",
  "cacheRead": "缓存读取",
  "cacheCreate": "缓存写入",
  "refresh": "刷新",
  "settings": "设置",
  "openWebUI": "打开 Web UI",
  "today": "今天",
  "thisWeek": "本周",
  "thisMonth": "本月",
  "thisYear": "今年",
  "last3d": "近3天",
  "last7d": "近7天",
  "last30d": "近30天",
  "custom": "自定义",
  "allSources": "全部来源",
  "claudeCode": "Claude Code",
  "codex": "Codex",
  "openClaw": "OpenClaw",
  "openCode": "OpenCode",
  "noSessions": "当前时间段内暂无会话数据。",
  "filterProject": "按项目筛选...",
  "autostart": "开机自启",
  "theme": "主题",
  "language": "语言",
  "light": "浅色",
  "dark": "深色",
  "system": "跟随系统",
  "notification": "通知",
  "dailyCostThreshold": "日费用告警阈值",
  "enabled": "已启用",
  "disabled": "已禁用",
  "justNow": "刚刚",
  "mAgo": "分钟前",
  "hAgo": "小时前",
  "dAgo": "天前",
  "unitMin": "分钟",
  "unitSec": "秒",
  "autoOn": "自动",
  "autoOff": "自动",
  "to": "至",
  "gran_1m": "1 分钟",
  "gran_30m": "30 分钟",
  "gran_1h": "1 小时",
  "gran_6h": "6 小时",
  "gran_12h": "12 小时",
  "gran_1d": "1 天",
  "gran_1w": "1 周",
  "gran_1M": "1 个月"
}
```

（zh.json 内容从现有 app.js I18N.zh 提取，加上新增的 settings/openWebUI/autostart/notification 等 key）

- [ ] **Step 4: 创建工具函数**

```typescript
// src/lib/utils.ts
export function fmtTokens(n: number): string {
  if (n >= 1e6) return (n / 1e6).toFixed(1) + "M";
  if (n >= 1e3) return (n / 1e3).toFixed(1) + "K";
  return String(n);
}

export function fmtCost(n: number): string {
  if (n >= 1) return "$" + n.toFixed(2);
  return "$" + n.toFixed(4);
}

export function localDateStr(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export type TimePreset = "today" | "thisWeek" | "thisMonth" | "thisYear" | "last3d" | "last7d" | "last30d" | "custom";

export function getTimeRange(preset: TimePreset, customFrom?: string, customTo?: string): { from: string; to: string } {
  const now = new Date();
  const todayStr = localDateStr(now);
  switch (preset) {
    case "today": return { from: todayStr, to: todayStr };
    case "thisWeek": {
      const d = new Date(now);
      d.setDate(d.getDate() - ((d.getDay() + 6) % 7));
      return { from: localDateStr(d), to: todayStr };
    }
    case "thisMonth": return { from: todayStr.slice(0, 8) + "01", to: todayStr };
    case "thisYear": return { from: todayStr.slice(0, 5) + "01-01", to: todayStr };
    case "last3d": { const d = new Date(now); d.setDate(d.getDate() - 2); return { from: localDateStr(d), to: todayStr }; }
    case "last7d": { const d = new Date(now); d.setDate(d.getDate() - 6); return { from: localDateStr(d), to: todayStr }; }
    case "last30d": { const d = new Date(now); d.setDate(d.getDate() - 29); return { from: localDateStr(d), to: todayStr }; }
    case "custom": return { from: customFrom || todayStr, to: customTo || todayStr };
  }
}

export function relativeTime(ts: string, t: (key: string) => string): string {
  if (!ts) return "-";
  const d = new Date(ts.replace(" ", "T").replace(" +0000 UTC", "Z"));
  if (isNaN(d.getTime())) return ts.slice(0, 16);
  const diff = Math.floor((Date.now() - d.getTime()) / 1000);
  if (diff < 60) return t("justNow") || "just now";
  if (diff < 3600) return Math.floor(diff / 60) + (t("mAgo") || "m ago");
  if (diff < 86400) return Math.floor(diff / 3600) + (t("hAgo") || "h ago");
  if (diff < 604800) return Math.floor(diff / 86400) + (t("dAgo") || "d ago");
  return d.toLocaleDateString();
}

export const CHART_COLORS = ["#3b82f6", "#22c55e", "#f59e0b", "#ef4444", "#8b5cf6", "#06b6d4", "#f97316", "#ec4899"];
```

- [ ] **Step 5: 更新 src/main.tsx 引入 i18n**

```tsx
// src/main.tsx
import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./lib/i18n";
import "./styles/globals.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
```

- [ ] **Step 6: 验证编译通过**

Run: `npx tsc --noEmit`
Expected: 无类型错误

- [ ] **Step 7: 提交**

```bash
git add src/lib/ src/main.tsx package.json package-lock.json
git commit -m "feat: add API client, i18n, and utility functions"
```

---

## Task 5: 前端 Layout + 导航 + 主题切换

**Files:**
- Create: `src/components/Layout.tsx`
- Modify: `src/App.tsx` — 添加路由

- [ ] **Step 1: 安装 shadcn/ui 基础**

```bash
npx shadcn@latest init
```

选择：TypeScript, default style, CSS variables for colors, `src/components/ui` 作为组件路径。

然后安装需要的组件：

```bash
npx shadcn@latest add button select tabs
```

- [ ] **Step 2: 创建 Layout.tsx**

```tsx
// src/components/Layout.tsx
import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router-dom";
import { open } from "@tauri-apps/plugin-shell";
import { getWebUIUrl } from "../lib/api";

const navItems = [
  { path: "/", label: "title" },
  { path: "/sessions", label: "sessionLog" },
  { path: "/settings", label: "settings" },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const { t, i18n } = useTranslation();
  const location = useLocation();

  const handleOpenWebUI = async () => {
    const url = await getWebUIUrl();
    open(url);
  };

  const toggleTheme = () => {
    const current = localStorage.getItem("au-theme") || "system";
    const next = current === "light" ? "dark" : current === "dark" ? "system" : "light";
    localStorage.setItem("au-theme", next);
    applyTheme(next);
  };

  const toggleLang = () => {
    const next = i18n.language === "en" ? "zh" : "en";
    i18n.changeLanguage(next);
    localStorage.setItem("au-lang", next);
  };

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-50 border-b border-border bg-background/85 backdrop-blur-sm">
        <div className="mx-auto max-w-[1200px] px-6 py-3 flex items-center justify-between">
          <nav className="flex items-center gap-6">
            {navItems.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                className={`text-sm font-medium transition-colors ${
                  location.pathname === item.path
                    ? "text-foreground"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {t(item.label)}
              </Link>
            ))}
          </nav>
          <div className="flex items-center gap-3">
            <button onClick={handleOpenWebUI} className="text-sm text-muted-foreground hover:text-foreground">
              {t("openWebUI")}
            </button>
            <button onClick={toggleTheme} className="text-sm text-muted-foreground hover:text-foreground">
              {t("theme")}
            </button>
            <button onClick={toggleLang} className="text-sm text-muted-foreground hover:text-foreground">
              {i18n.language.toUpperCase()}
            </button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-[1200px] px-6 py-6">
        {children}
      </main>
    </div>
  );
}

function applyTheme(theme: string) {
  const resolved = theme === "system"
    ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
    : theme;
  document.documentElement.classList.toggle("dark", resolved === "dark");
}
```

- [ ] **Step 3: 更新 App.tsx 添加路由**

```tsx
// src/App.tsx
import { BrowserRouter, Routes, Route } from "react-router-dom";
import Layout from "./components/Layout";

function Dashboard() {
  return <div>Dashboard placeholder</div>;
}
function Sessions() {
  return <div>Sessions placeholder</div>;
}
function Settings() {
  return <div>Settings placeholder</div>;
}

function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/sessions" element={<Sessions />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
}

export default App;
```

- [ ] **Step 4: 安装 Tauri shell 插件**

```bash
npm install @tauri-apps/plugin-shell
```

在 `src-tauri/Cargo.toml` 添加：`tauri-plugin-shell = "2"`（如果 Task 3 未添加）

在 `src-tauri/tauri.conf.json` 的 `plugins` 中添加 shell 权限。

- [ ] **Step 5: 验证前端编译和导航工作**

Run: `npm run dev`
Expected: 浏览器中能看到导航栏，点击能切换页面

- [ ] **Step 6: 提交**

```bash
git add src/components/Layout.tsx src/App.tsx
git commit -m "feat: add layout with navigation, theme toggle, and routing"
```

---

## Task 6: Dashboard 页面 — 统计卡片 + 时间选择器

**Files:**
- Create: `src/components/StatCard.tsx`
- Create: `src/components/TimeRangeSelector.tsx`
- Create: `src/pages/Dashboard.tsx`
- Modify: `src/App.tsx` — 替换 placeholder

- [ ] **Step 1: 创建 StatCard 组件**

```tsx
// src/components/StatCard.tsx
interface StatCardProps {
  label: string;
  value: string;
  color: string;
}

export default function StatCard({ label, value, color }: StatCardProps) {
  return (
    <div className="relative bg-card border border-border rounded-xl p-4 shadow-sm hover:shadow-md transition-shadow overflow-hidden">
      <div className={`absolute left-0 top-0 w-1 h-full`} style={{ backgroundColor: color }} />
      <div className="text-xs text-muted-foreground font-semibold uppercase tracking-wide">{label}</div>
      <div className="text-2xl font-bold mt-1 font-mono">{value}</div>
    </div>
  );
}
```

- [ ] **Step 2: 创建 TimeRangeSelector 组件**

包含时间预设按钮、自定义日期范围、粒度选择、source 筛选。从现有 app.js 的控制栏逻辑移植。

```tsx
// src/components/TimeRangeSelector.tsx
import { useTranslation } from "react-i18next";
import { TimePreset } from "../lib/utils";

const PRESETS: TimePreset[] = ["today", "thisWeek", "thisMonth", "thisYear", "last3d", "last7d", "last30d", "custom"];
const GRANULARITIES = ["1m", "30m", "1h", "6h", "12h", "1d", "1w", "1M"];
const SOURCES = [
  { value: "", label: "allSources" },
  { value: "claude", label: "claudeCode" },
  { value: "codex", label: "codex" },
  { value: "openclaw", label: "openClaw" },
  { value: "opencode", label: "openCode" },
];

interface Props {
  preset: TimePreset;
  onPresetChange: (p: TimePreset) => void;
  granularity: string;
  onGranularityChange: (g: string) => void;
  source: string;
  onSourceChange: (s: string) => void;
  onRefresh: () => void;
  customFrom?: string;
  customTo?: string;
  onCustomFromChange?: (v: string) => void;
  onCustomToChange?: (v: string) => void;
}

export default function TimeRangeSelector({
  preset, onPresetChange, granularity, onGranularityChange,
  source, onSourceChange, onRefresh, customFrom, customTo,
  onCustomFromChange, onCustomToChange,
}: Props) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-wrap items-center gap-3">
      {/* Preset buttons */}
      <div className="flex bg-card border border-border rounded-lg p-0.5">
        {PRESETS.map((p) => (
          <button
            key={p}
            onClick={() => { onPresetChange(p); localStorage.setItem("au-preset", p); }}
            className={`px-3 py-1.5 rounded-md text-xs font-medium transition-colors ${
              preset === p ? "bg-accent text-white" : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {t(p)}
          </button>
        ))}
      </div>

      {/* Custom date range */}
      {preset === "custom" && (
        <div className="flex items-center gap-2">
          <input type="date" value={customFrom} onChange={(e) => onCustomFromChange?.(e.target.value)}
            className="bg-card border border-border rounded-lg px-3 py-1.5 text-sm" />
          <span className="text-muted-foreground text-sm">{t("to")}</span>
          <input type="date" value={customTo} onChange={(e) => onCustomToChange?.(e.target.value)}
            className="bg-card border border-border rounded-lg px-3 py-1.5 text-sm" />
        </div>
      )}

      {/* Granularity */}
      <select value={granularity}
        onChange={(e) => { onGranularityChange(e.target.value); localStorage.setItem("au-granularity", e.target.value); }}
        className="bg-card border border-border rounded-lg px-3 py-1.5 text-sm">
        {GRANULARITIES.map((g) => (
          <option key={g} value={g}>{t(`gran_${g}`)}</option>
        ))}
      </select>

      {/* Source filter */}
      <select value={source}
        onChange={(e) => { onSourceChange(e.target.value); localStorage.setItem("au-source", e.target.value); }}
        className="bg-card border border-border rounded-lg px-3 py-1.5 text-sm">
        {SOURCES.map((s) => (
          <option key={s.value} value={s.value}>{t(s.label)}</option>
        ))}
      </select>

      {/* Refresh */}
      <button onClick={onRefresh}
        className="ml-auto bg-accent text-white px-3 py-1.5 rounded-lg text-sm font-medium hover:bg-accent/90">
        {t("refresh")}
      </button>
    </div>
  );
}
```

（完整实现参考现有 `app.js` 的 `buildControls()` 和 preset/granularity/source 逻辑）

- [ ] **Step 3: 创建 Dashboard 页面（统计卡片部分）**

```tsx
// src/pages/Dashboard.tsx
import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { fetchAPI } from "../lib/api";
import { fmtCost, fmtTokens, getTimeRange, TimePreset } from "../lib/utils";
import StatCard from "../components/StatCard";
import TimeRangeSelector from "../components/TimeRangeSelector";

interface DashboardStats {
  total_tokens: number;
  total_cost: number;
  total_sessions: number;
  total_prompts: number;
  total_calls: number;
  cache_hit_rate: number;
}

export default function Dashboard() {
  const { t } = useTranslation();
  const [preset, setPreset] = useState<TimePreset>(
    (localStorage.getItem("au-preset") as TimePreset) || "today"
  );
  const [granularity, setGranularity] = useState(localStorage.getItem("au-granularity") || "1h");
  const [source, setSource] = useState(localStorage.getItem("au-source") || "");
  const [stats, setStats] = useState<DashboardStats | null>(null);

  const fetchData = useCallback(async () => {
    const range = getTimeRange(preset);
    const params = { ...range, granularity, source: source || undefined };
    const s = await fetchAPI<DashboardStats>("stats", params);
    setStats(s);
  }, [preset, granularity, source]);

  useEffect(() => { fetchData(); }, [fetchData]);

  return (
    <div className="space-y-4">
      <TimeRangeSelector
        preset={preset} onPresetChange={setPreset}
        granularity={granularity} onGranularityChange={setGranularity}
        source={source} onSourceChange={setSource}
        onRefresh={fetchData}
      />
      <div className="grid grid-cols-6 gap-4">
        <StatCard label={t("totalTokens")} value={fmtTokens(stats?.total_tokens || 0)} color="#3b82f6" />
        <StatCard label={t("totalCost")} value={fmtCost(stats?.total_cost || 0)} color="#22c55e" />
        <StatCard label={t("sessions")} value={String(stats?.total_sessions || 0)} color="#f59e0b" />
        <StatCard label={t("prompts")} value={String(stats?.total_prompts || 0)} color="#f472b6" />
        <StatCard label={t("apiCalls")} value={fmtTokens(stats?.total_calls || 0)} color="#2563eb" />
        <StatCard label={t("cacheHitRate")} value={((stats?.cache_hit_rate || 0) * 100).toFixed(1) + "%"} color="#8b5cf6" />
      </div>
      {/* Charts will be added in Task 7 */}
    </div>
  );
}
```

- [ ] **Step 4: 更新 App.tsx 使用真实 Dashboard**

替换 Dashboard placeholder import 为 `src/pages/Dashboard.tsx`。

- [ ] **Step 5: 验证 Dashboard 页面渲染**

Run: `npm run dev`
Expected: 能看到 6 个统计卡片和时间选择器（数据需要 Go 后端运行才有值）

- [ ] **Step 6: 提交**

```bash
git add src/components/StatCard.tsx src/components/TimeRangeSelector.tsx src/pages/Dashboard.tsx src/App.tsx
git commit -m "feat: implement Dashboard page with stat cards and time range selector"
```

---

## Task 7: Dashboard 页面 — ECharts 图表

**Files:**
- Create: `src/components/ChartCard.tsx`
- Modify: `src/pages/Dashboard.tsx` — 添加三个图表

- [ ] **Step 1: 创建 ChartCard 容器组件**

```tsx
// src/components/ChartCard.tsx
import ReactECharts from "echarts-for-react";

interface ChartCardProps {
  title: string;
  option: object;
  className?: string;
}

export default function ChartCard({ title, option, className }: ChartCardProps) {
  return (
    <div className={`bg-card border border-border rounded-xl p-4 shadow-sm ${className || ""}`}>
      <h3 className="text-sm font-semibold mb-3">{title}</h3>
      <ReactECharts option={option} style={{ height: 260 }} notMerge={true} />
    </div>
  );
}
```

- [ ] **Step 2: 在 Dashboard 中添加 Token Usage 堆叠柱状图**

从现有 `app.js` 的 tokens chart 逻辑移植。调用 `/api/tokens-over-time`，渲染 input/output/cacheRead/cacheCreate 四层堆叠柱状图。

- [ ] **Step 3: 添加 Cost Trend 堆叠柱状图**

从现有 `app.js` 的 cost chart 逻辑移植。调用 `/api/cost-over-time`，按模型分色堆叠。

- [ ] **Step 4: 添加 Cost by Model 环形图**

从现有 `app.js` 的 pie chart 逻辑移植。调用 `/api/cost-by-model`。

- [ ] **Step 5: 布局排列**

```tsx
<div className="grid grid-cols-5 gap-4">
  <div className="col-span-5">
    <ChartCard title={t("tokenUsage")} option={tokensOption} />
  </div>
  <div className="col-span-3">
    <ChartCard title={t("costOverTime")} option={costOption} />
  </div>
  <div className="col-span-2">
    <ChartCard title={t("costByModel")} option={pieOption} />
  </div>
</div>
```

- [ ] **Step 6: 添加自动刷新逻辑**

移植现有 `app.js` 的 auto-refresh 逻辑（`setInterval` + 可配置间隔）。

- [ ] **Step 7: 验证图表渲染**

Run: `npm run dev`（同时运行 `go run . --port 9800`）
Expected: 三个图表正确渲染数据，自动刷新工作

- [ ] **Step 8: 提交**

```bash
git add src/components/ChartCard.tsx src/pages/Dashboard.tsx
git commit -m "feat: add ECharts visualizations to Dashboard

Token usage, cost trend, and cost by model charts."
```

---

## Task 8: Sessions 页面

**Files:**
- Create: `src/pages/Sessions.tsx`

- [ ] **Step 1: 创建 Sessions 页面**

从现有 `app.js` 的 session table 逻辑移植。包含：
- 可排序表头（source, project, branch, time, prompts, tokens, cost）
- 项目筛选输入框
- 分页（每页 20 条）
- 展开行查看 per-model 详情（调用 `/api/session-detail`）
- Source badge 样式（claude=蓝, codex=绿, openclaw=橙, opencode=紫）

```tsx
// src/pages/Sessions.tsx
import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { fetchAPI } from "../lib/api";
import { fmtCost, fmtTokens, getTimeRange, relativeTime, TimePreset } from "../lib/utils";
import TimeRangeSelector from "../components/TimeRangeSelector";

interface Session {
  session_id: string;
  source: string;
  project: string;
  cwd: string;
  git_branch: string;
  start_time: string;
  prompts: number;
  tokens: number;
  total_cost: number;
}

interface SessionDetail {
  model: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_create: number;
  cost_usd: number;
}

const PAGE_SIZE = 20;
const BADGE_COLORS: Record<string, string> = {
  claude: "bg-blue-500/10 text-blue-500 border-blue-500/20",
  codex: "bg-green-500/10 text-green-500 border-green-500/20",
  openclaw: "bg-orange-500/10 text-orange-500 border-orange-500/20",
  opencode: "bg-purple-500/10 text-purple-500 border-purple-500/20",
};

export default function Sessions() {
  const { t } = useTranslation();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [preset, setPreset] = useState<TimePreset>((localStorage.getItem("au-preset") as TimePreset) || "today");
  const [granularity, setGranularity] = useState(localStorage.getItem("au-granularity") || "1h");
  const [source, setSource] = useState(localStorage.getItem("au-source") || "");
  const [sortKey, setSortKey] = useState("start_time");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState("");
  const [expanded, setExpanded] = useState<Record<string, SessionDetail[] | null>>({});

  const fetchData = useCallback(async () => {
    const range = getTimeRange(preset);
    const data = await fetchAPI<Session[]>("sessions", { ...range, granularity, source: source || undefined });
    setSessions(data || []);
  }, [preset, granularity, source]);

  useEffect(() => { fetchData(); }, [fetchData]);

  // Filter, sort, paginate
  const filtered = sessions.filter((s) =>
    !filter || (s.project || s.cwd || "").toLowerCase().includes(filter.toLowerCase())
  );
  const sorted = [...filtered].sort((a, b) => {
    const va = (a as any)[sortKey] ?? "";
    const vb = (b as any)[sortKey] ?? "";
    const cmp = typeof va === "number" ? va - vb : String(va).localeCompare(String(vb));
    return sortDir === "asc" ? cmp : -cmp;
  });
  const totalPages = Math.max(1, Math.ceil(sorted.length / PAGE_SIZE));
  const paged = sorted.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  const toggleSort = (key: string) => {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else { setSortKey(key); setSortDir("desc"); }
  };

  const toggleExpand = async (sid: string) => {
    if (expanded[sid] !== undefined) {
      setExpanded((prev) => { const next = { ...prev }; delete next[sid]; return next; });
    } else {
      setExpanded((prev) => ({ ...prev, [sid]: null })); // loading
      const url = await getWebUIUrl();
      const res = await fetch(`${url}/api/session-detail?session_id=${encodeURIComponent(sid)}`);
      const data = await res.json();
      setExpanded((prev) => ({ ...prev, [sid]: data }));
    }
  };

  return (
    <div className="space-y-4">
      <TimeRangeSelector preset={preset} onPresetChange={setPreset}
        granularity={granularity} onGranularityChange={setGranularity}
        source={source} onSourceChange={setSource} onRefresh={fetchData} />

      <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden">
        <div className="p-5 border-b border-border flex items-center justify-between">
          <h3 className="text-base font-semibold">{t("sessionLog")}</h3>
          <input type="text" value={filter} onChange={(e) => { setFilter(e.target.value); setPage(1); }}
            placeholder={t("filterProject")}
            className="bg-background border border-border rounded-lg px-3 py-2 text-sm w-56" />
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr>
                {[
                  { key: "source", label: "source" },
                  { key: "project", label: "project" },
                  { key: "git_branch", label: "branch" },
                  { key: "start_time", label: "time" },
                  { key: "prompts", label: "prompts" },
                  { key: "tokens", label: "tokens" },
                  { key: "total_cost", label: "cost" },
                ].map((col) => (
                  <th key={col.key} onClick={() => toggleSort(col.key)}
                    className="text-left px-6 py-3 text-muted-foreground font-medium cursor-pointer hover:text-foreground">
                    {t(col.label)} {sortKey === col.key ? (sortDir === "asc" ? "▲" : "▼") : ""}
                  </th>
                ))}
                <th className="w-10" />
              </tr>
            </thead>
            <tbody>
              {paged.map((s) => (
                <>
                  <tr key={s.session_id} className="hover:bg-muted/30 transition-colors">
                    <td className="px-6 py-3">
                      <span className={`inline-flex px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase border ${BADGE_COLORS[s.source] || ""}`}>
                        {s.source}
                      </span>
                    </td>
                    <td className="px-6 py-3 max-w-[280px] truncate">{s.project || s.cwd || "-"}</td>
                    <td className="px-6 py-3">{s.git_branch || "-"}</td>
                    <td className="px-6 py-3">{relativeTime(s.start_time, t)}</td>
                    <td className="px-6 py-3">{s.prompts}</td>
                    <td className="px-6 py-3">{fmtTokens(s.tokens || 0)}</td>
                    <td className="px-6 py-3 font-medium text-green-500">{fmtCost(s.total_cost || 0)}</td>
                    <td className="px-6 py-3">
                      <button onClick={() => toggleExpand(s.session_id)}
                        className="w-7 h-7 rounded-md border border-border flex items-center justify-center hover:border-accent">
                        <span className={`transition-transform ${expanded[s.session_id] !== undefined ? "rotate-90" : ""}`}>▶</span>
                      </button>
                    </td>
                  </tr>
                  {expanded[s.session_id] !== undefined && (
                    <tr key={`${s.session_id}-detail`}>
                      <td colSpan={8} className="px-6 py-3 bg-muted/20">
                        {expanded[s.session_id] === null ? (
                          <span className="text-muted-foreground text-xs">Loading...</span>
                        ) : (
                          <table className="w-full text-xs">
                            <thead>
                              <tr>
                                <th className="text-left py-2 text-muted-foreground">{t("model")}</th>
                                <th className="text-left py-2 text-muted-foreground">{t("calls")}</th>
                                <th className="text-left py-2 text-muted-foreground">{t("input")}</th>
                                <th className="text-left py-2 text-muted-foreground">{t("output")}</th>
                                <th className="text-left py-2 text-muted-foreground">{t("cacheRead")}</th>
                                <th className="text-left py-2 text-muted-foreground">{t("cacheCreate")}</th>
                                <th className="text-left py-2 text-muted-foreground">{t("cost")}</th>
                              </tr>
                            </thead>
                            <tbody>
                              {(expanded[s.session_id] || []).map((d, i) => (
                                <tr key={i}>
                                  <td className="py-1.5">{d.model}</td>
                                  <td className="py-1.5">{d.calls}</td>
                                  <td className="py-1.5 font-mono">{fmtTokens(d.input_tokens)}</td>
                                  <td className="py-1.5 font-mono">{fmtTokens(d.output_tokens)}</td>
                                  <td className="py-1.5 font-mono">{fmtTokens(d.cache_read)}</td>
                                  <td className="py-1.5 font-mono">{fmtTokens(d.cache_create)}</td>
                                  <td className="py-1.5 font-mono text-green-500">{fmtCost(d.cost_usd)}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        )}
                      </td>
                    </tr>
                  )}
                </>
              ))}
            </tbody>
          </table>
        </div>
        {/* Pagination */}
        <div className="px-6 py-4 flex items-center justify-end gap-2">
          <span className="text-muted-foreground text-sm mr-auto">
            {Math.min((page - 1) * PAGE_SIZE + 1, sorted.length)}-{Math.min(page * PAGE_SIZE, sorted.length)} of {sorted.length}
          </span>
          <button disabled={page <= 1} onClick={() => setPage(page - 1)}
            className="px-3 py-1 border border-border rounded-lg text-sm disabled:opacity-50">←</button>
          <button disabled={page >= totalPages} onClick={() => setPage(page + 1)}
            className="px-3 py-1 border border-border rounded-lg text-sm disabled:opacity-50">→</button>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: 更新 App.tsx 使用真实 Sessions 页面**

- [ ] **Step 3: 验证 Sessions 页面**

Run: `npm run dev`
Expected: session 列表正确渲染，排序/筛选/分页/展开详情都工作

- [ ] **Step 4: 提交**

```bash
git add src/pages/Sessions.tsx src/App.tsx
git commit -m "feat: implement Sessions page with sortable table and detail expansion"
```

---

## Task 9: Settings 页面

**Files:**
- Create: `src/pages/Settings.tsx`

- [ ] **Step 1: 创建 Settings 页面**

```tsx
// src/pages/Settings.tsx
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { invoke } from "@tauri-apps/api/core";

export default function Settings() {
  const { t, i18n } = useTranslation();
  const [autostart, setAutostart] = useState(false);
  const [theme, setTheme] = useState(localStorage.getItem("au-theme") || "system");
  const [costThreshold, setCostThreshold] = useState(
    Number(localStorage.getItem("au-cost-threshold")) || 10
  );
  const [notificationsEnabled, setNotificationsEnabled] = useState(
    localStorage.getItem("au-notifications") !== "false"
  );

  // 主题切换
  const handleThemeChange = (value: string) => {
    setTheme(value);
    localStorage.setItem("au-theme", value);
    // apply theme to DOM
  };

  // 语言切换
  const handleLangChange = (value: string) => {
    i18n.changeLanguage(value);
    localStorage.setItem("au-lang", value);
  };

  // 开机自启（调用 Tauri autostart 插件）
  const handleAutostartToggle = async () => {
    try {
      if (autostart) {
        await invoke("plugin:autostart|disable");
      } else {
        await invoke("plugin:autostart|enable");
      }
      setAutostart(!autostart);
    } catch (e) {
      console.error("Autostart toggle failed:", e);
    }
  };

  // 通知阈值
  const handleThresholdChange = (value: number) => {
    setCostThreshold(value);
    localStorage.setItem("au-cost-threshold", String(value));
  };

  return (
    <div className="max-w-lg space-y-8">
      {/* Theme section */}
      {/* Language section */}
      {/* Autostart toggle */}
      {/* Notification threshold */}
    </div>
  );
}
```

- [ ] **Step 2: 更新 App.tsx 使用真实 Settings 页面**

- [ ] **Step 3: 验证 Settings 页面**

Run: `npm run dev`
Expected: 设置项正确渲染，主题/语言切换即时生效

- [ ] **Step 4: 提交**

```bash
git add src/pages/Settings.tsx src/App.tsx
git commit -m "feat: implement Settings page with theme, language, autostart, and notifications"
```

---

## Task 10: 系统托盘

**Files:**
- Create: `src-tauri/src/tray.rs`
- Modify: `src-tauri/src/main.rs`

- [ ] **Step 1: 创建 tray.rs**

```rust
// src-tauri/src/tray.rs
use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Manager,
};

pub fn create_tray(app: &tauri::AppHandle) -> tauri::Result<()> {
    let show = MenuItem::with_id(app, "show", "Show Panel", true, None::<&str>)?;
    let web_ui = MenuItem::with_id(app, "web_ui", "Open Web UI", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "Quit", true, None::<&str>)?;

    let menu = Menu::with_items(app, &[&show, &web_ui, &quit])?;

    TrayIconBuilder::new()
        .icon(app.default_window_icon().unwrap().clone())
        .menu(&menu)
        .on_menu_event(|app, event| match event.id.as_ref() {
            "show" => {
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.show();
                    let _ = window.set_focus();
                }
            }
            "web_ui" => {
                let handle = app.clone();
                tauri::async_runtime::spawn(async move {
                    let state = handle.state::<crate::sidecar::SidecarState>();
                    let port = state.port.load(std::sync::atomic::Ordering::Relaxed);
                    let url = format!("http://127.0.0.1:{}", port);
                    let _ = handle.shell().open(&url, None::<&str>);
                });
            }
            "quit" => {
                crate::sidecar::kill_sidecar(app);
                app.exit(0);
            }
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                let app = tray.app_handle();
                if let Some(window) = app.get_webview_window("main") {
                    if window.is_visible().unwrap_or(false) {
                        let _ = window.hide();
                    } else {
                        let _ = window.show();
                        let _ = window.set_focus();
                    }
                }
            }
        })
        .build(app)?;

    Ok(())
}
```

- [ ] **Step 2: 更新 main.rs 注册托盘 + 关闭时隐藏到托盘**

在 `setup` 中调用 `tray::create_tray(app.handle())?;`

修改窗口关闭事件：关闭窗口时隐藏而非退出。

```rust
.on_window_event(|window, event| {
    if let tauri::WindowEvent::CloseRequested { api, .. } = event {
        // Hide to tray instead of closing
        let _ = window.hide();
        api.prevent_close();
    }
})
```

- [ ] **Step 3: 验证托盘功能**

Run: `npm run tauri dev`
Expected: 系统托盘图标出现，右键菜单工作，关闭窗口隐藏到托盘

- [ ] **Step 4: 提交**

```bash
git add src-tauri/src/tray.rs src-tauri/src/main.rs
git commit -m "feat: add system tray with show/hide, open web UI, and quit"
```

---

## Task 11: 开机自启 + 通知

**Files:**
- Modify: `src-tauri/src/main.rs` — 注册 autostart 和 notification 插件
- Modify: `src-tauri/Cargo.toml` — 添加插件依赖

- [ ] **Step 1: 添加 Tauri 插件依赖**

```bash
cd src-tauri
cargo add tauri-plugin-autostart tauri-plugin-notification
```

在 `src-tauri/tauri.conf.json` 的 `plugins` 中添加权限。

- [ ] **Step 2: 在 main.rs 注册插件**

```rust
use tauri_plugin_autostart::MacosLauncher;

tauri::Builder::default()
    .plugin(tauri_plugin_shell::init())
    .plugin(tauri_plugin_autostart::init(MacosLauncher::LaunchAgent, None))
    .plugin(tauri_plugin_notification::init())
    // ...
```

- [ ] **Step 3: 在 Rust 层添加通知检查逻辑**

在 `setup` 中启动一个后台任务，每 5 分钟从 `/api/stats` 拉取数据，检查日用量是否超过阈值。

```rust
// 在 main.rs 的 setup 闭包中，sidecar 启动成功后添加：
let notify_handle = handle.clone();
tauri::async_runtime::spawn(async move {
    loop {
        tokio::time::sleep(std::time::Duration::from_secs(300)).await;
        let state = notify_handle.state::<SidecarState>();
        let port = state.port.load(std::sync::atomic::Ordering::Relaxed);
        if port == 0 { continue; }

        // Read threshold from app data settings.json
        let threshold = {
            let path = notify_handle.path().app_data_dir().unwrap().join("settings.json");
            std::fs::read_to_string(&path).ok()
                .and_then(|data| serde_json::from_str::<serde_json::Value>(&data).ok())
                .and_then(|v| v["cost_threshold"].as_f64())
                .unwrap_or(10.0)
        };

        let url = format!("http://127.0.0.1:{}/api/stats?from={}&to={}", port,
            chrono::Local::now().format("%Y-%m-%d"),
            chrono::Local::now().format("%Y-%m-%d"));

        if let Ok(resp) = reqwest::get(&url).await {
            if let Ok(stats) = resp.json::<serde_json::Value>().await {
                if let Some(cost) = stats["total_cost"].as_f64() {
                    if cost > threshold {
                        let _ = notify_handle.notification()
                            .builder()
                            .title("Agent Usage Alert")
                            .body(format!("Daily cost ${:.2} exceeds threshold ${:.2}", cost, threshold))
                            .show();
                    }
                }
            }
        }
    }
});
```

注意：需要在 `Cargo.toml` 添加 `chrono = "0.4"` 依赖。

- [ ] **Step 4: 验证自启和通知**

Run: `npm run tauri dev`
Expected: Settings 页面的自启开关能工作，通知在超阈值时触发

- [ ] **Step 5: 提交**

```bash
git add src-tauri/
git commit -m "feat: add autostart and notification support"
```

---

## Task 12: 集成测试 + 端到端验证

**Files:**
- 无新文件，验证所有功能

- [ ] **Step 1: 运行 Go 测试**

Run: `go test ./... -v`
Expected: ALL PASS

- [ ] **Step 2: 运行 TypeScript 类型检查**

Run: `npx tsc --noEmit`
Expected: 无错误

- [ ] **Step 3: 构建 Go sidecar 二进制**

```bash
go build -o src-tauri/binaries/agent-usage-$(rustc -vV | grep host | awk '{print $2}') .
```

- [ ] **Step 4: 构建 Tauri 应用**

Run: `npm run tauri build`
Expected: 在 `src-tauri/target/release/bundle/` 下生成平台安装包

- [ ] **Step 5: 端到端验证清单**

手动验证以下功能：
- [ ] 应用启动后 Go sidecar 自动运行
- [ ] Dashboard 6 个统计卡片显示数据
- [ ] 三个图表正确渲染
- [ ] 时间范围切换、粒度切换、source 筛选工作
- [ ] Sessions 页面排序、筛选、分页、展开详情工作
- [ ] "打开 Web UI" 按钮在浏览器中打开原有 Web UI
- [ ] 系统托盘：左键显示/隐藏，右键菜单工作
- [ ] 关闭窗口隐藏到托盘
- [ ] 主题切换（浅色/深色/跟随系统）
- [ ] 语言切换（中/英）
- [ ] Settings 页面各项设置生效

- [ ] **Step 6: 提交**

```bash
git commit -m "chore: verify end-to-end integration"
```

---

## Task 13: CI/CD — GitHub Actions 桌面构建

**Files:**
- Create: `.github/workflows/desktop.yml`

- [ ] **Step 1: 创建 desktop.yml**

```yaml
# .github/workflows/desktop.yml
name: Desktop Build

on:
  push:
    tags: ["v*"]
  workflow_dispatch:

jobs:
  build:
    strategy:
      fail-fast: false
      matrix:
        include:
          - platform: macos-latest
            target: aarch64-apple-darwin
            go_os: darwin
            go_arch: arm64
          - platform: macos-latest
            target: x86_64-apple-darwin
            go_os: darwin
            go_arch: amd64
          - platform: ubuntu-22.04
            target: x86_64-unknown-linux-gnu
            go_os: linux
            go_arch: amd64
          - platform: windows-latest
            target: x86_64-pc-windows-msvc
            go_os: windows
            go_arch: amd64

    runs-on: ${{ matrix.platform }}
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.25"

      - name: Build Go sidecar
        env:
          GOOS: ${{ matrix.go_os }}
          GOARCH: ${{ matrix.go_arch }}
          CGO_ENABLED: "0"
        run: |
          EXT=""
          if [ "${{ matrix.go_os }}" = "windows" ]; then EXT=".exe"; fi
          go build -ldflags "-s -w -X main.version=${{ github.ref_name }}" \
            -o src-tauri/binaries/agent-usage-${{ matrix.target }}${EXT} .

      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version: "20"

      - name: Install frontend dependencies
        run: npm ci

      - name: Install Linux dependencies
        if: matrix.platform == 'ubuntu-22.04'
        run: |
          sudo apt-get update
          sudo apt-get install -y libwebkit2gtk-4.1-dev libappindicator3-dev

      - name: Setup Rust
        uses: dtolnay/rust-toolchain@stable
        with:
          targets: ${{ matrix.target }}

      - name: Build Tauri app
        uses: tauri-apps/tauri-action@v0
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        with:
          tagName: ${{ github.ref_name }}
          releaseName: "Agent Usage ${{ github.ref_name }}"
          releaseBody: "Desktop application release"
          releaseDraft: true
```

- [ ] **Step 2: 验证 workflow 语法**

Run: `gh workflow view desktop.yml` 或检查 YAML 语法

- [ ] **Step 3: 提交**

```bash
git add .github/workflows/desktop.yml
git commit -m "ci: add GitHub Actions workflow for desktop builds

Builds Tauri app for macOS (arm64+x86_64), Windows, and Linux."
```

---

## 完成

所有 13 个 task 完成后，项目将具备：
- Go 后端支持 `--port` flag、`/api/health`、CORS
- Tauri v2 桌面应用，React 前端 1:1 还原现有 Web UI
- 系统托盘、开机自启、通知推送
- "打开 Web UI" 按钮保留原有 Web 访问方式
- 三平台 CI/CD 自动构建






