# Local Agent Usage And Session Explorer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把应用收敛为本机多 Agent 用量分析与会话回溯工具，移除配置管理，并完成 Claude Code、Codex 会话时间线、全文搜索、原始记录查看及本地观测 RPM/TPM。

**Architecture:** 继续以 Agent 原始 JSONL/SQLite 为事实来源，在现有采集流程中同步产出非重叠 token 记录和标准会话事件。SQLite 只保存会话元数据、标准化正文、原始字节定位和 FTS5 索引；原始记录按需从源文件读取。REST API 统一使用 `(source, session_id)` 标识会话，React 前端只依赖标准事件契约，不解析 Agent 私有格式。

**Tech Stack:** Go 1.23、modernc SQLite/FTS5、React 19、TypeScript 6、Vite 8、Vitest、Testing Library、ECharts 6、Tauri 2/Rust。

---

## Scope And Delivery Rules

- 首期深度回溯仅覆盖 `claude` 与 `codex`；`openclaw`、`opencode` 继续保留已有统计能力并显示 `stats_only`。
- 不删除现有配置管理数据库表，不运行 `VACUUM`，不删除用户的 Agent 原始文件。
- 不实现团队、云同步、Agent 交接、内置终端、Hermes、PNG/PDF 分享或最终视觉定稿。
- 每个任务完成后执行所列聚焦测试并提交；任务 13 再执行全量验证。
- 开始执行前使用 `superpowers:using-git-worktrees` 创建隔离 worktree；当前文档只定义实施步骤，不创建 worktree。

## File Map

### Remove

- `internal/configmanager/`：整套 Provider、MCP、Skills、同步和备份实现。
- `internal/server/config_handlers.go`：配置管理 REST API。
- `internal/storage/configstore.go`、`internal/storage/configstore_test.go`：配置管理持久化。
- `src/pages/Config.tsx`、`src/pages/config/`：配置管理页面。
- `src/components/ConfirmPanel.tsx`、`src/components/SyncStatus.tsx`、`src/components/ToolTargets.tsx`：仅配置管理使用的组件。

### Create

- `internal/collector/jsonl.go`、`internal/collector/jsonl_test.go`：不会丢失半行、能给出精确字节定位的 JSONL 读取器。
- `internal/collector/events.go`：标准事件类型、适配器接口和解析上下文。
- `internal/collector/claude_events.go`、`internal/collector/claude_events_test.go`：Claude Code 事件适配器。
- `internal/collector/codex_events.go`、`internal/collector/codex_events_test.go`：Codex 事件适配器。
- `internal/storage/session_events.go`、`internal/storage/session_events_test.go`：来源状态、事件、FTS、原始定位和索引清理。
- `internal/storage/throughput.go`、`internal/storage/throughput_test.go`：固定分钟与滚动 60 秒吞吐计算。
- `internal/storage/breakdown.go`、`internal/storage/breakdown_test.go`：Agent、模型、项目明细聚合。
- `internal/server/session_handlers.go`、`internal/server/session_handlers_test.go`：会话搜索、时间线、原始记录和索引重建 API。
- `internal/server/throughput_handlers.go`、`internal/server/throughput_handlers_test.go`：本地观测 RPM/TPM API。
- `src/test/setup.ts`：前端测试环境。
- `src/lib/types.ts`：REST DTO 的唯一前端定义。
- `src/lib/usageFilters.ts`、`src/lib/usageFilters.test.ts`：总览与会话页共享筛选语义。
- `src/components/sessions/SessionList.tsx`、`SessionTimeline.tsx`、`EventCard.tsx`、`EventInspector.tsx`：会话中心分区。
- `src/pages/Dashboard.test.tsx`、`src/pages/Sessions.test.tsx`、`src/pages/Settings.test.tsx`：页面行为测试。

### Modify

- `main.go`：删除配置管理启动与轮询，保留采集、价格和 HTTP 服务。
- `internal/storage/sqlite.go`、`queries.go`、`api.go`：复合会话身份、新迁移和查询契约。
- `internal/collector/claude.go`、`claude_process.go`、`claude_test.go`：字节精确增量扫描与事件落库。
- `internal/collector/codex.go`、`codex_test.go`：字节精确增量扫描与事件落库。
- `internal/server/server.go`、`server_test.go`：移除配置路由并注册会话与吞吐路由。
- `src/lib/api.ts`：支持任意查询参数、结构化错误和事件原始数据请求。
- `src/App.tsx`、`src/components/Layout.tsx`：主导航只保留总览、会话、设置。
- `src/components/TimeRangeSelector.tsx`：变成总览与会话共享的筛选控制区。
- `src/pages/Dashboard.tsx`、`Sessions.tsx`、`Settings.tsx`：新信息架构。
- `src/lib/locales/en.json`、`zh.json`：删除配置文案并加入回溯、覆盖状态和本地观测吞吐文案。
- `src-tauri/src/commands.rs`、`main.rs`：移除 Skills CLI 命令，暴露已有 sidecar 重启能力。
- `package.json`、`package-lock.json`、`vite.config.ts`：加入前端测试工具。
- `go.mod`、`go.sum`：移除配置管理独占依赖。

---

### Task 1: Add The Frontend Test Harness

**Files:**
- Modify: `package.json`
- Modify: `package-lock.json`
- Modify: `vite.config.ts`
- Create: `src/test/setup.ts`
- Create: `src/lib/utils.test.ts`

- [ ] **Step 1: Install the test dependencies and add scripts**

Run:

```bash
npm install --save-dev vitest jsdom @testing-library/react @testing-library/jest-dom @testing-library/user-event
```

Add these scripts to `package.json`:

```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "test": "vitest run",
    "test:watch": "vitest",
    "tauri": "tauri"
  }
}
```

- [ ] **Step 2: Write a failing DOM test**

Create `src/lib/utils.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { getTimeRange } from "./utils";

describe("getTimeRange", () => {
  it("uses the exact custom date boundaries", () => {
    expect(getTimeRange("custom", "2026-07-01", "2026-07-23")).toEqual({
      from: "2026-07-01",
      to: "2026-07-23",
    });
  });

  it("runs with the shared DOM matcher setup", () => {
    document.body.innerHTML = "<main>usage</main>";
    expect(document.querySelector("main")).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run the test and confirm the harness is not configured**

Run: `npm test -- src/lib/utils.test.ts`

Expected: FAIL because the default Node environment has no `document` and the shared `jest-dom` matcher is not registered.

- [ ] **Step 4: Configure Vitest**

Change `vite.config.ts` to import `defineConfig` from `vitest/config`, keep the existing Vite options, and add:

```ts
test: {
  environment: "jsdom",
  setupFiles: ["./src/test/setup.ts"],
  css: true,
},
```

Create `src/test/setup.ts`:

```ts
import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

afterEach(() => {
  cleanup();
  localStorage.clear();
});
```

- [ ] **Step 5: Verify and commit**

Run: `npm test -- src/lib/utils.test.ts && npm run build`

Expected: one passing test and a successful TypeScript/Vite build.

```bash
git add package.json package-lock.json vite.config.ts src/test/setup.ts src/lib/utils.test.ts
git commit -m "test: add frontend test harness"
```

---

### Task 2: Remove Backend Configuration Management

**Files:**
- Modify: `main.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`
- Delete: `internal/server/config_handlers.go`
- Delete: `internal/configmanager/`
- Delete: `internal/storage/configstore.go`
- Delete: `internal/storage/configstore_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Replace config-manager server tests with an absence test**

Keep health, CORS, date validation, and existing usage endpoint tests in `internal/server/server_test.go`. Delete config-specific fixtures and tests, then add:

```go
func TestConfigManagementRoutesAreNotRegistered(t *testing.T) {
	db := tempDB(t)
	handler := New(db, "127.0.0.1:0").Handler()

	for _, path := range []string{
		"/api/config/profiles",
		"/api/config/mcp",
		"/api/config/skills",
		"/api/config/sync/status",
		"/api/config/backups",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", path, w.Code)
		}
	}
}
```

- [ ] **Step 2: Run the focused test and verify it fails to compile**

Run: `go test ./internal/server -run TestConfigManagementRoutesAreNotRegistered -count=1`

Expected: FAIL because `server.New` still requires a `*configmanager.Manager` and config routes remain registered.

- [ ] **Step 3: Remove the backend product surface**

Change the server constructor and struct to:

```go
type Server struct {
	db   *storage.DB
	addr string
}

func New(db *storage.DB, addr string) *Server {
	return &Server{db: db, addr: addr}
}
```

Remove every `/api/config/*` registration from `Handler()`. Delete the files and package listed above. In `main.go`, remove `configmanager` imports, backup directory creation, manager bootstrap, the 30-second inbound-sync goroutine, and manager arguments; start the server with:

```go
srv := server.New(db, addr)
```

Do not alter migration `005_config_manager` or `006_skill_variants`; old tables must remain readable but idle.

- [ ] **Step 4: Remove unused modules and verify no runtime reference remains**

Run:

```bash
go mod tidy
rg -n "configmanager|/api/config/|BurntSushi|go-keyring" --glob '!docs/**' --glob '!go.sum' .
```

Expected: `rg` exits 1 with no matches; `github.com/BurntSushi/toml` and `github.com/zalando/go-keyring` are absent from `go.mod` and `go.sum` after tidy.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./...`

Expected: all Go tests pass while pre-existing config tables remain untouched.

```bash
git add -A main.go internal/configmanager internal/server internal/storage/configstore.go internal/storage/configstore_test.go go.mod go.sum
git commit -m "refactor: remove configuration management backend"
```

---

### Task 3: Remove The Configuration UI And Skills Installer

**Files:**
- Modify: `src/App.tsx`
- Modify: `src/components/Layout.tsx`
- Create: `src/components/Layout.test.tsx`
- Delete: `src/pages/Config.tsx`
- Delete: `src/pages/config/`
- Delete: `src/components/ConfirmPanel.tsx`
- Delete: `src/components/SyncStatus.tsx`
- Delete: `src/components/ToolTargets.tsx`
- Modify: `src/lib/locales/en.json`
- Modify: `src/lib/locales/zh.json`
- Modify: `src-tauri/src/commands.rs`
- Modify: `src-tauri/src/main.rs`

- [ ] **Step 1: Write a failing navigation test**

Create `src/components/Layout.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import Layout from "./Layout";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "zh", changeLanguage: vi.fn() },
  }),
}));

describe("Layout", () => {
  it("shows only overview, sessions, and settings navigation", () => {
    render(
      <MemoryRouter>
        <Layout><div>content</div></Layout>
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "title" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "sessionLog" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "settings" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "config" })).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test and verify the old navigation is exposed**

Run: `npm test -- src/components/Layout.test.tsx`

Expected: FAIL because the `config` link is still present.

- [ ] **Step 3: Delete the configuration routes, pages, components, and locale keys**

Make `navItems` exactly:

```ts
const navItems = [
  { path: "/", label: "title" },
  { path: "/sessions", label: "sessionLog" },
  { path: "/settings", label: "settings" },
];
```

Remove all `/config` lazy imports and nested routes from `src/App.tsx`. Delete locale entries used only by removed files; verify remaining locale key sets are identical with a JSON-aware test or script, not manual visual comparison.

- [ ] **Step 4: Remove Tauri Skills CLI execution**

In `src-tauri/src/commands.rs`, remove `SkillsCliActionResult`, the Agent mapping/constants, shell command builder/runner, install/uninstall commands, and their tests. Keep settings and sidecar-port commands. In `src-tauri/src/main.rs`, remove both Skills commands from `generate_handler!`.

Verify:

```bash
rg -n "install_agent_usage_skill|uninstall_agent_usage_skill|SkillsCliActionResult|npx --yes skills" src src-tauri/src
```

Expected: no matches.

- [ ] **Step 5: Verify and commit**

Run: `npm test && npm run build && cargo test --manifest-path src-tauri/Cargo.toml`

Expected: frontend tests/build and Rust tests pass.

```bash
git add -A src src-tauri/src
git commit -m "refactor: remove configuration management ui"
```

---

### Task 4: Add Composite Session Identity And Event Index Schema

**Files:**
- Modify: `internal/storage/sqlite.go`
- Modify: `internal/storage/queries.go`
- Modify: `internal/storage/api.go`
- Modify: `internal/storage/storage_test.go`
- Create: `internal/storage/session_events.go`
- Create: `internal/storage/session_events_test.go`

- [ ] **Step 1: Write migration and event-storage tests first**

Add tests covering all of these assertions:

```go
func TestSessionIdentityIncludesSource(t *testing.T) {
	db := tempDB(t)
	for _, source := range []string{"claude", "codex"} {
		err := db.UpsertSession(&SessionRecord{Source: source, SessionID: "same-id", Prompts: 1})
		if err != nil {
			t.Fatalf("UpsertSession(%s): %v", source, err)
		}
	}

	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE session_id='same-id'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("sessions = %d, want 2", count)
	}
}

func TestClearSessionContentPreservesUsage(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "s1", Model: "model", Timestamp: ts,
		InputTokens: 10, OutputTokens: 5,
	}); err != nil {
		t.Fatal(err)
	}
	sourceID, err := db.UpsertSessionSource(&SessionSource{
		Source: "claude", SessionID: "s1", Path: "/tmp/s1.jsonl",
		ParserVersion: "test-v1", CoverageStatus: "complete", SourceStatus: "available",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSessionEvents([]SessionEventRecord{{
		SessionSourceID: sourceID, Source: "claude", SessionID: "s1",
		EventType: "user_message", Timestamp: ts, Content: "hello", RawOffset: 0, RawLength: 10,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := db.ClearSessionContent("claude", "s1", "missing_source"); err != nil {
		t.Fatal(err)
	}

	var usageCount, eventCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM usage_records WHERE source=? AND session_id=?`, "claude", "s1").Scan(&usageCount); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM session_events WHERE source=? AND session_id=?`, "claude", "s1").Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if usageCount != 1 || eventCount != 0 {
		t.Fatalf("usage=%d events=%d, want usage=1 events=0", usageCount, eventCount)
	}
}
```

Also open a database with pre-migration `sessions`, `usage_records`, and `prompt_events`, run `Open` again, and assert every old total is unchanged.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/storage -run 'TestSessionIdentityIncludesSource|TestClearSessionContentPreservesUsage|TestSessionEvent' -count=1`

Expected: FAIL because `sessions.session_id` is globally unique and event-index methods do not exist.

- [ ] **Step 3: Add migration `007_session_event_index`**

The migration must perform these operations in one transaction:

```sql
CREATE TABLE sessions_v2 (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source TEXT NOT NULL,
  session_id TEXT NOT NULL,
  project TEXT DEFAULT '',
  cwd TEXT DEFAULT '',
  version TEXT DEFAULT '',
  git_branch TEXT DEFAULT '',
  start_time DATETIME,
  prompts INTEGER DEFAULT 0,
  UNIQUE(source, session_id)
);
INSERT INTO sessions_v2(source,session_id,project,cwd,version,git_branch,start_time,prompts)
SELECT source,session_id,project,cwd,version,git_branch,start_time,prompts FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_v2 RENAME TO sessions;

DROP INDEX IF EXISTS idx_usage_dedup;
CREATE UNIQUE INDEX idx_usage_dedup
ON usage_records(source,session_id,model,timestamp,input_tokens,output_tokens);
DROP INDEX IF EXISTS idx_prompt_dedup;
CREATE UNIQUE INDEX idx_prompt_dedup
ON prompt_events(source,session_id,timestamp);
```

Then create `session_sources`, `session_events`, `session_events_fts`, and synchronization triggers. Use these stable columns:

```sql
CREATE TABLE session_sources (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source TEXT NOT NULL,
  session_id TEXT NOT NULL,
  source_kind TEXT NOT NULL DEFAULT 'jsonl',
  path TEXT NOT NULL UNIQUE,
  parser_version TEXT NOT NULL,
  head_hash TEXT NOT NULL DEFAULT '',
  file_size INTEGER NOT NULL DEFAULT 0,
  indexed_offset INTEGER NOT NULL DEFAULT 0,
  coverage_status TEXT NOT NULL DEFAULT 'partial',
  source_status TEXT NOT NULL DEFAULT 'available',
  malformed_lines INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  last_indexed_at DATETIME,
  UNIQUE(source, session_id, path)
);

CREATE TABLE session_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_source_id INTEGER NOT NULL,
  source TEXT NOT NULL,
  session_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  source_event_type TEXT NOT NULL DEFAULT '',
  timestamp DATETIME,
  role TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  tool_name TEXT NOT NULL DEFAULT '',
  tool_call_id TEXT NOT NULL DEFAULT '',
  tool_input TEXT NOT NULL DEFAULT '',
  tool_output TEXT NOT NULL DEFAULT '',
  event_status TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER,
  raw_offset INTEGER NOT NULL,
  raw_length INTEGER NOT NULL,
  raw_index INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY(session_source_id) REFERENCES session_sources(id) ON DELETE CASCADE,
  UNIQUE(session_source_id, raw_offset, raw_index)
);

CREATE VIRTUAL TABLE session_events_fts USING fts5(
  content, tool_name, tool_input, tool_output,
  content='session_events', content_rowid='id', tokenize='unicode61'
);
```

Add insert/delete/update triggers so FTS never depends on callers remembering a second write.

- [ ] **Step 4: Implement focused storage methods**

Define these public contracts in `session_events.go`:

```go
type SessionSource struct {
	ID             int64
	Source         string
	SessionID      string
	Path           string
	ParserVersion  string
	HeadHash       string
	FileSize       int64
	IndexedOffset  int64
	CoverageStatus string
	SourceStatus   string
	MalformedLines int
	LastError      string
}

type SessionEventRecord struct {
	ID              int64
	SessionSourceID int64
	Source           string
	SessionID        string
	EventType        string
	SourceEventType  string
	Timestamp        time.Time
	Role             string
	Content          string
	ToolName         string
	ToolCallID       string
	ToolInput        string
	ToolOutput       string
	EventStatus      string
	DurationMS       *int64
	RawOffset        int64
	RawLength        int64
	RawIndex         int
}
```

Implement `UpsertSessionSource`, `InsertSessionEvents`, `DeleteSourceIndex`, `ClearSessionContent`, `MarkMissingSessionSources`, and event/list locator reads. Every multi-table mutation must hold `DB.mu` and use one SQL transaction.

Update `UpsertSession` to use `ON CONFLICT(source,session_id)`. Update all session joins to use both columns.

- [ ] **Step 5: Verify migration safety and commit**

Run: `go test ./internal/storage/... -count=1`

Expected: composite identity, FTS, dedup, content clearing, and legacy-upgrade tests pass.

```bash
git add internal/storage
git commit -m "feat: add session event index schema"
```

---

### Task 5: Add Byte-Accurate JSONL Indexing Infrastructure

**Files:**
- Create: `internal/collector/jsonl.go`
- Create: `internal/collector/jsonl_test.go`
- Create: `internal/collector/events.go`

- [ ] **Step 1: Write exact-offset and incomplete-line tests**

```go
func TestJSONLReaderUsesByteOffsetsAndKeepsPartialTail(t *testing.T) {
	input := []byte("{\"text\":\"中文\"}\r\n{\"id\":2}\n{\"partial\":")
	var got []JSONLRecord
	next, err := ReadJSONL(bytes.NewReader(input), 0, func(record JSONLRecord) error {
		got = append(got, record)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("records = %d, want 2", len(got))
	}
	if got[1].Offset != int64(len([]byte("{\"text\":\"中文\"}\r\n"))) {
		t.Fatalf("second offset = %d", got[1].Offset)
	}
	if next != int64(len([]byte("{\"text\":\"中文\"}\r\n{\"id\":2}\n"))) {
		t.Fatalf("next = %d", next)
	}
}
```

Add a second test with an 11 MiB JSON string to prove the old `bufio.Scanner` 10 MiB ceiling is gone.

- [ ] **Step 2: Run and verify failure**

Run: `go test ./internal/collector -run 'TestJSONLReader' -count=1`

Expected: FAIL because `JSONLRecord` and `ReadJSONL` do not exist.

- [ ] **Step 3: Implement the reader**

Use `bufio.Reader.ReadBytes('\n')`, not `Scanner`. The contract is:

```go
type JSONLRecord struct {
	Data      []byte
	Offset    int64
	RawLength int64
}

func ReadJSONL(r io.Reader, startOffset int64, visit func(JSONLRecord) error) (int64, error)
```

Only call `visit` for newline-terminated records. Strip `\n` and optional `\r` from `Data` and `RawLength`; advance the returned offset by the full physical record length. On EOF with a non-terminated tail, return the last complete offset so the next scan rereads that tail.

- [ ] **Step 4: Define the normalized adapter boundary**

Create `events.go` with exact event kinds and parser contracts:

```go
type EventKind string

const (
	EventUserMessage      EventKind = "user_message"
	EventAssistantMessage EventKind = "assistant_message"
	EventReasoning        EventKind = "reasoning"
	EventToolCall         EventKind = "tool_call"
	EventToolResult       EventKind = "tool_result"
	EventError            EventKind = "error"
	EventMetadata         EventKind = "metadata"
	EventUnknown          EventKind = "unknown"
)

type EventContext struct {
	Source    string
	SessionID string
	CWD       string
	Version   string
	Model     string
}

type NormalizedEvent struct {
	Kind            EventKind
	SourceEventType string
	Timestamp       time.Time
	Role            string
	Content         string
	ToolName        string
	ToolCallID      string
	ToolInput       string
	ToolOutput      string
	Status          string
	DurationMS      *int64
}

type SessionEventAdapter interface {
	ParserVersion() string
	Parse(raw []byte, context *EventContext) ([]NormalizedEvent, error)
}
```

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/collector -run 'TestJSONLReader' -count=1`

Expected: offset, CRLF, UTF-8, oversized line, and partial-tail tests pass.

```bash
git add internal/collector/jsonl.go internal/collector/jsonl_test.go internal/collector/events.go
git commit -m "feat: add byte accurate jsonl reader"
```

---

### Task 6: Index Claude Code Session Events

**Files:**
- Create: `internal/collector/claude_events.go`
- Create: `internal/collector/claude_events_test.go`
- Modify: `internal/collector/claude_process.go`
- Modify: `internal/collector/claude.go`
- Modify: `internal/collector/claude_test.go`

- [ ] **Step 1: Write adapter fixtures for every supported event**

Use JSONL fixture lines that contain a user text block, assistant text, thinking block, `tool_use`, `tool_result`, explicit error, and an unrecognized valid record. Assert this exact kind sequence:

```go
want := []EventKind{
	EventUserMessage,
	EventAssistantMessage,
	EventReasoning,
	EventToolCall,
	EventToolResult,
	EventError,
	EventUnknown,
}
```

Also assert that `tool_use.input` and `tool_result.content` are serialized as stable JSON/text without inventing system prompts or hidden tool schemas.

- [ ] **Step 2: Write collector lifecycle tests**

Extend `claude_test.go` to prove:

- an appended complete line adds exactly one event;
- an appended partial line adds nothing until its newline arrives;
- truncation or changed head hash rebuilds that source index without duplicating usage;
- parser version mismatch rebuilds normalized events;
- a malformed line increments `malformed_lines` but later valid lines still index;
- a removed source clears events/FTS but leaves `usage_records` and `sessions` metadata.

Run: `go test ./internal/collector -run 'TestClaude.*Event|TestClaude.*Source' -count=1`

Expected: FAIL because Claude event parsing and source reconciliation are absent.

- [ ] **Step 3: Implement `claudeEventAdapter`**

Use parser version `claude-events-v1`. Mapping rules are fixed:

| Claude record/block | Standard event |
| --- | --- |
| `type=user`, string or `text` content | `user_message` |
| `type=user`, `tool_result` block | `tool_result` |
| `type=assistant`, `text` block | `assistant_message` |
| `type=assistant`, `thinking` block | `reasoning` |
| `type=assistant`, `tool_use` block | `tool_call` |
| source error record or error block | `error` |
| known metadata such as compact/model change | `metadata` |
| other valid JSON record | `unknown` |

One source line may emit multiple events; assign `raw_index` by slice order.

- [ ] **Step 4: Integrate indexing into the existing scan**

Replace `bufio.Scanner` in `processFile` with `ReadJSONL`. Keep current token semantics. For each complete record:

1. update Claude session context;
2. extract usage and prompt events using the existing rules;
3. call the event adapter;
4. insert normalized events with that record's `Offset`, `RawLength`, and event index;
5. persist `file_state.last_offset` and `session_sources.indexed_offset` as the last complete offset, not `os.FileInfo.Size()`.

Calculate a SHA-256 hash of the first 4096 bytes. Rebuild only this source when size shrinks, head hash changes, or parser version changes. After each top-level Claude scan, call `MarkMissingSessionSources("claude", seenPaths)`.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/collector ./internal/storage -run 'Claude|SessionEvent|SessionSource' -count=1`

Expected: all Claude parsing, incremental, rebuild, malformed-line, and missing-source tests pass.

```bash
git add internal/collector/claude* internal/storage
git commit -m "feat: index claude code session events"
```

---

### Task 7: Index Codex Session Events

**Files:**
- Create: `internal/collector/codex_events.go`
- Create: `internal/collector/codex_events_test.go`
- Modify: `internal/collector/codex.go`
- Modify: `internal/collector/codex_test.go`

- [ ] **Step 1: Write Codex adapter and incremental-context tests**

Use real-shape `session_meta`, `turn_context`, `response_item`, and `event_msg` records. Cover user/assistant messages, reasoning, function/custom tool calls, function/custom tool output, errors, metadata, and unknown records. Assert that a second scan containing only appended response items restores `session_id`, `cwd`, `version`, and `model` from `file_state.scan_context`.

Run: `go test ./internal/collector -run 'TestCodex.*Event|TestCodex.*Context' -count=1`

Expected: FAIL because the Codex event adapter does not exist.

- [ ] **Step 2: Implement `codexEventAdapter`**

Use parser version `codex-events-v1` and these mappings:

| Codex payload | Standard event |
| --- | --- |
| `response_item.message`, role `user` | `user_message` |
| `response_item.message`, role `assistant` | `assistant_message` |
| `response_item.reasoning` | `reasoning` |
| `response_item.function_call` or `custom_tool_call` | `tool_call` |
| `response_item.function_call_output` or `custom_tool_call_output` | `tool_result` |
| error-bearing `event_msg` | `error` |
| model change in `turn_context` | `metadata` |
| other valid, visible record | `unknown` |

Keep `event_msg.token_count` as usage only; do not render token bookkeeping as conversation content.

- [ ] **Step 3: Replace Codex Scanner and persist raw locators**

Apply the same complete-line, hash, parser-version, truncation, malformed-line, and missing-source behavior as Task 6. Preserve the existing non-overlapping rule:

```go
record.InputTokens = usage.InputTokens - usage.CachedInputTokens
record.CacheReadInputTokens = usage.CachedInputTokens
record.OutputTokens = usage.OutputTokens
record.ReasoningOutputTokens = usage.ReasoningOutputTokens
```

Reject negative non-cached input as malformed usage instead of storing a negative token count.

- [ ] **Step 4: Verify cross-source identity and commit**

Run: `go test ./internal/collector ./internal/storage -run 'Codex|SessionIdentity|SessionEvent' -count=1`

Expected: all Codex event, usage, incremental, and source lifecycle tests pass, including a Claude and Codex session sharing the same `session_id`.

```bash
git add internal/collector/codex* internal/storage
git commit -m "feat: index codex session events"
```

---

### Task 8: Add Session Search, Timeline, Raw Record, And Index APIs

**Files:**
- Modify: `internal/storage/api.go`
- Modify: `internal/storage/session_events.go`
- Modify: `internal/server/server.go`
- Create: `internal/server/session_handlers.go`
- Create: `internal/server/session_handlers_test.go`

- [ ] **Step 1: Write API contract tests**

Seed two sources, models, projects, event types, and raw files. Test:

```text
GET /api/sessions?from=2026-07-01&to=2026-07-31&source=claude&model=claude-sonnet-4&project=fixture&q=fix&limit=20
GET /api/sessions/claude/s1/events?limit=100
GET /api/sessions/claude/s1/events/42/raw
POST /api/session-index/rebuild
```

Assertions must include source/model/project/date intersection, FTS matching across content/tool name/input/output, stable newest-first pagination, chronological events, source-bound event IDs, JSON/text raw response, 404 for unknown event, 410 for missing source, 400 for invalid dates/limits, and index rebuild preserving usage totals.

Run: `go test ./internal/server -run 'TestSession(Search|Events|Raw|Index)' -count=1`

Expected: FAIL because the routes and query objects do not exist.

- [ ] **Step 2: Define response and query contracts**

```go
type SessionQuery struct {
	From    time.Time
	To      time.Time
	Source  string
	Model   string
	Project string
	Search  string
	Limit   int
	Offset  int
}

type SessionSummary struct {
	Source          string   `json:"source"`
	SessionID       string   `json:"session_id"`
	Title           string   `json:"title"`
	Project         string   `json:"project"`
	CWD             string   `json:"cwd"`
	GitBranch       string   `json:"git_branch"`
	StartTime       string   `json:"start_time"`
	LastActivity    string   `json:"last_activity"`
	Models          []string `json:"models"`
	Prompts         int      `json:"prompts"`
	ToolCalls       int      `json:"tool_calls"`
	Errors          int      `json:"errors"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	CacheRead       int64    `json:"cache_read"`
	CacheCreate     int64    `json:"cache_create"`
	TotalTokens     int64    `json:"total_tokens"`
	TotalCost       float64  `json:"total_cost"`
	CoverageStatus  string   `json:"coverage_status"`
	SourceStatus    string   `json:"source_status"`
	MalformedLines  int      `json:"malformed_lines"`
	UnknownPrice    bool     `json:"unknown_price"`
}

type RawEventResponse struct {
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	Path        string `json:"path"`
	Offset      int64  `json:"offset"`
	Length      int64  `json:"length"`
}

type SessionEventResponse struct {
	ID              int64  `json:"id"`
	EventType       string `json:"event_type"`
	SourceEventType string `json:"source_event_type"`
	Timestamp       string `json:"timestamp"`
	Role            string `json:"role"`
	Content         string `json:"content"`
	ToolName        string `json:"tool_name"`
	ToolCallID      string `json:"tool_call_id"`
	ToolInput       string `json:"tool_input"`
	ToolOutput      string `json:"tool_output"`
	EventStatus     string `json:"event_status"`
	DurationMS      *int64 `json:"duration_ms"`
	HasRaw          bool   `json:"has_raw"`
}
```

Use the first non-empty user message as `title`, falling back to project/CWD and then session ID. FTS user input must be quoted/escaped as a literal phrase so punctuation cannot become an FTS operator.

- [ ] **Step 3: Implement safe raw reads and routes**

Register exact method routes:

```go
mux.HandleFunc("GET /api/sessions", s.handleSessions)
mux.HandleFunc("GET /api/sessions/{source}/{session_id}/events", s.handleSessionEvents)
mux.HandleFunc("GET /api/sessions/{source}/{session_id}/events/{event_id}/raw", s.handleRawSessionEvent)
mux.HandleFunc("POST /api/session-index/rebuild", s.handleRebuildSessionIndex)
```

Resolve the file path only from the stored event locator. Never accept a raw path from the request. Limit raw reads to 16 MiB, use `ReadAt`, and verify the event belongs to the route's source/session before opening the file. Return `content_type=json` only when `json.Valid(data)` is true; otherwise return `text`.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/storage ./internal/server -run 'Session|Raw|Index' -count=1`

Expected: all storage and handler contract tests pass.

```bash
git add internal/storage internal/server
git commit -m "feat: add session explorer api"
```

---

### Task 9: Add Local Observed RPM And TPM

**Files:**
- Create: `internal/storage/throughput.go`
- Create: `internal/storage/throughput_test.go`
- Create: `internal/server/throughput_handlers.go`
- Create: `internal/server/throughput_handlers_test.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Write calculation tests with a minute-boundary fixture**

Insert calls at `10:00:59`, `10:01:01`, and `10:02:30`. Assert that the first two calls produce a rolling peak RPM of 2 even though fixed-minute series contains separate buckets. Include non-cached input, cache read, cache creation, output, source, and model filters.

Use nearest-rank P95:

```go
func nearestRankP95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)
	index := int(math.Ceil(0.95*float64(len(values)))) - 1
	return values[index]
}
```

Run: `go test ./internal/storage -run TestThroughput -count=1`

Expected: FAIL because throughput calculation is absent.

- [ ] **Step 2: Implement a single ordered scan and rolling window**

Define the API model:

```go
type ThroughputValues struct {
	RPM         float64 `json:"rpm"`
	InputTPM    float64 `json:"input_tpm"`
	CacheRead   float64 `json:"cache_read_tpm"`
	CacheCreate float64 `json:"cache_create_tpm"`
	OutputTPM   float64 `json:"output_tpm"`
	TotalTPM    float64 `json:"total_tpm"`
}

type ThroughputPoint struct {
	Minute string `json:"minute"`
	ThroughputValues
}

type ThroughputResult struct {
	AverageActiveMinute ThroughputValues  `json:"average_active_minute"`
	PeakRolling60s      ThroughputValues  `json:"peak_rolling_60s"`
	P95Rolling60s       ThroughputValues  `json:"p95_rolling_60s"`
	Series              []ThroughputPoint `json:"series"`
}
```

Query matching `usage_records` once in timestamp order. Build fixed local-minute buckets for average/series and use a two-pointer window with the exact predicate `timestamp > t-60s && timestamp <= t` at every record timestamp. Calculate each peak and P95 dimension independently. `InputTPM` is non-cached input; `TotalTPM` sums input, cache read, cache creation, and output.

- [ ] **Step 3: Add and test the endpoint**

Register:

```go
mux.HandleFunc("GET /api/throughput", s.handleThroughput)
```

Accept `from`, `to`, `source`, `model`, and `tz_offset`. Reject reversed dates and invalid timezone offsets with 400. The response and locale copy must call the data “local observed” and must not include quota, limit, remaining, or utilization fields.

Run: `go test ./internal/storage ./internal/server -run Throughput -count=1`

Expected: storage and API tests pass, including source/model filtering and the cross-minute rolling peak.

- [ ] **Step 4: Commit**

```bash
git add internal/storage/throughput* internal/server/throughput* internal/server/server.go
git commit -m "feat: add observed rpm and tpm analytics"
```

---

### Task 10: Add Overview Breakdown Data And Rebuild The Information Hierarchy

**Files:**
- Modify: `internal/storage/api.go`
- Modify: `internal/storage/storage_test.go`
- Create: `internal/storage/breakdown.go`
- Create: `internal/storage/breakdown_test.go`
- Modify: `internal/server/server.go`
- Modify: `src/lib/api.ts`
- Create: `src/lib/types.ts`
- Create: `src/lib/usageFilters.ts`
- Create: `src/lib/usageFilters.test.ts`
- Modify: `src/components/TimeRangeSelector.tsx`
- Modify: `src/pages/Dashboard.tsx`
- Create: `src/pages/Dashboard.test.tsx`

- [ ] **Step 1: Write backend breakdown tests**

Add a `GET /api/usage-breakdown?dimension=source|model|project` contract with:

```go
type UsageBreakdown struct {
	Key          string  `json:"key"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
	Sessions     int     `json:"sessions"`
	Calls        int     `json:"calls"`
	CacheHitRate float64 `json:"cache_hit_rate"`
	UnknownPrice bool    `json:"unknown_price"`
}
```

Assert non-overlapping token sums, session counts by `(source,session_id)`, cache rate denominator including all input components, and unknown price via `LEFT JOIN pricing`, not `cost_usd=0`.

- [ ] **Step 2: Run backend tests and implement the aggregation**

Run: `go test ./internal/storage ./internal/server -run 'Breakdown|DashboardStats' -count=1`

Expected before implementation: FAIL because the endpoint and fields do not exist.

Extend `DashboardStats` with exact input, output, cache-read, and cache-create totals. Implement only the three allowed dimensions using a switch-selected SQL expression; never concatenate the user-provided `dimension` into SQL.

- [ ] **Step 3: Write shared-filter and Dashboard tests**

Test that default range is `last7d`, local preferences persist, URL parameters override local preferences on entry from Dashboard, and clicking an Agent/model/project row navigates to `/sessions` with matching query parameters.

```ts
export type UsageFilters = {
  preset: TimePreset;
  from: string;
  to: string;
  source: string;
  model: string;
  project: string;
};
```

Run: `npm test -- src/lib/usageFilters.test.ts src/pages/Dashboard.test.tsx`

Expected: FAIL because shared filters and the new overview do not exist.

- [ ] **Step 4: Implement the three-level overview**

Build these un-nested page bands in order:

1. core metrics: total token, estimated cost, sessions, user messages, cache hit rate;
2. token trend and Agent composition;
3. model table, project ranking, and local-observed throughput summary/trend.

Remove the current approximations that multiply total token by `0.25` or `0.75`; every displayed input/output value must come from the API. Keep the current visual tokens only as temporary implementation styling. Add click-through navigation for Agent, model, project, and trend date.

Generalize `fetchAPI` query construction to accept `Record<string, string | number | undefined>` so model/project/search/pagination filters do not require string-built URLs.

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/storage ./internal/server -run 'Breakdown|DashboardStats' -count=1 && npm test -- src/lib/usageFilters.test.ts src/pages/Dashboard.test.tsx && npm run build`

Expected: all focused backend/frontend tests and build pass.

```bash
git add internal/storage internal/server src
git commit -m "feat: rebuild usage overview hierarchy"
```

---

### Task 11: Build The Session Center And Event Inspector

**Files:**
- Modify: `src/lib/types.ts`
- Modify: `src/lib/api.ts`
- Modify: `src/pages/Sessions.tsx`
- Create: `src/pages/Sessions.test.tsx`
- Create: `src/components/sessions/SessionList.tsx`
- Create: `src/components/sessions/SessionTimeline.tsx`
- Create: `src/components/sessions/EventCard.tsx`
- Create: `src/components/sessions/EventInspector.tsx`
- Modify: `src/styles/globals.css`
- Modify: `internal/server/server.go`
- Modify: `internal/storage/api.go`

- [ ] **Step 1: Define frontend DTOs and write interaction tests**

Mirror backend JSON names in `types.ts`; do not redefine session/event interfaces inside page files. Test all of these behaviors with mocked HTTP responses:

- searchable/filterable list selects the newest session;
- inherited Dashboard filters are visible and can be cleared;
- tool calls and long tool results start collapsed;
- error events start expanded;
- clicking an event opens the inspector;
- raw data is not requested until “raw record” is activated;
- closing the inspector restores the center reading width;
- under 900px, selecting a session replaces the list and Back returns to it;
- `stats_only`, `partial`, `missing_source`, malformed, and unknown-price states show distinct text.

Run: `npm test -- src/pages/Sessions.test.tsx`

Expected: FAIL against the current expandable aggregate table.

- [ ] **Step 2: Implement the master-detail layout**

Use stable boundaries:

```tsx
type SessionEvent = {
  id: number;
  event_type: "user_message" | "assistant_message" | "reasoning" | "tool_call" | "tool_result" | "error" | "metadata" | "unknown";
  source_event_type: string;
  timestamp: string;
  role: string;
  content: string;
  tool_name: string;
  tool_call_id: string;
  tool_input: string;
  tool_output: string;
  event_status: string;
  duration_ms: number | null;
  has_raw: boolean;
};

type SessionListProps = {
  sessions: SessionSummary[];
  selectedKey: string | null;
  loading: boolean;
  onSelect: (session: SessionSummary) => void;
};

type SessionTimelineProps = {
  session: SessionSummary;
  events: SessionEvent[];
  onInspect: (event: SessionEvent) => void;
};

type EventInspectorProps = {
  session: SessionSummary;
  event: SessionEvent;
  onClose: () => void;
};
```

Desktop tracks are `minmax(300px,340px) minmax(420px,1fr)` plus an inspector of `minmax(320px,400px)` only while open. Do not place cards inside cards. Use a list/detail mode below 900px rather than compressing all three columns.

- [ ] **Step 3: Implement search, event paging, and on-demand raw data**

Debounce text search by 250 ms and send it to the backend `q` parameter; do not filter only the currently loaded page. Abort obsolete list/event requests with `AbortController`. Fetch 100 chronological events at a time. Raw content is requested only from the inspector and cached by event ID for the life of the selected session.

Render source-provided content only. For absent fields show the localized “source did not provide this data” state; do not synthesize a system prompt, request body, or hidden context.

- [ ] **Step 4: Remove the old aggregate detail endpoint**

After `Sessions.tsx` no longer calls it, delete `/api/session-detail`, `handleSessionDetail`, `GetSessionDetail`, and obsolete frontend interfaces. Keep model aggregation through the new session summary/breakdown contracts.

Run:

```bash
rg -n "session-detail|GetSessionDetail|DetailTable" --glob '!docs/**' .
```

Expected: no matches.

- [ ] **Step 5: Verify and commit**

Run: `npm test -- src/pages/Sessions.test.tsx && npm run build && go test ./internal/server ./internal/storage`

Expected: session interactions, build, and backend tests pass.

```bash
git add src internal/server internal/storage
git commit -m "feat: add session retrospective center"
```

---

### Task 12: Expand App-Only Settings And Clean Localization

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`
- Modify: `main.go`
- Modify: `src/pages/Settings.tsx`
- Create: `src/pages/Settings.test.tsx`
- Modify: `src/lib/locales/en.json`
- Modify: `src/lib/locales/zh.json`
- Modify: `src-tauri/src/commands.rs`
- Modify: `src-tauri/src/main.rs`

- [ ] **Step 1: Write config persistence and settings API tests**

Test atomic YAML save/load for enabled flags, paths, scan intervals, and pricing interval. Test `GET /api/settings/collectors`, `PUT /api/settings/collectors`, and `POST /api/session-index/rebuild`. Reject empty paths, intervals below 10 seconds, unknown collector names, and malformed durations. Assert settings updates do not expose storage paths, API keys, Provider settings, MCP, Skills, or backups.

Run: `go test ./internal/config ./internal/server -run 'Settings|SaveConfig' -count=1`

Expected: FAIL because persistence and settings routes do not exist.

- [ ] **Step 2: Add atomic config persistence and optional server config path**

Implement:

```go
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".agent-usage-config-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
```

Pass the resolved config path to `server.New` through a typed option so ordinary tests can still construct a server without settings mutation. PUT writes the file and returns `{ "restart_required": true }`.

- [ ] **Step 3: Expose sidecar restart and write frontend tests**

Add a Tauri command that delegates to existing lifecycle code:

```rust
#[tauri::command]
pub async fn restart_sidecar(app: tauri::AppHandle) -> Result<u16, String> {
    crate::sidecar::restart_sidecar(&app).await
}
```

Register it in `generate_handler!`. Test Settings rendering and mutation for language, theme, collector toggles/paths/intervals, pricing sync interval, notifications, cost threshold, and rebuild confirmation. Mock `invoke("restart_sidecar")` and assert it occurs only after a successful config update or index rebuild.

- [ ] **Step 4: Implement the Settings page and locale cleanup**

Use segmented controls for language/theme, switches for binary settings, and duration/path inputs for numeric/text settings. Keep OpenCode/OpenClaw collectors visible as statistics-only. Do not add Hermes until its collector exists.

Remove all orphaned config-management locale keys. Add a test that parses both JSON files and compares sorted key sets:

```ts
expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort());
```

- [ ] **Step 5: Verify and commit**

Run: `go test ./internal/config ./internal/server && npm test -- src/pages/Settings.test.tsx && npm run build && cargo test --manifest-path src-tauri/Cargo.toml`

Expected: Go, frontend, build, and Rust checks pass.

```bash
git add internal/config internal/server main.go src src-tauri/src
git commit -m "feat: add local application settings"
```

---

### Task 13: End-To-End Verification And Product Documentation

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/superpowers/specs/2026-07-23-local-agent-usage-session-explorer-design.md` only if implementation exposes a verified contract difference
- Create: `internal/collector/testdata/claude-session.jsonl`
- Create: `internal/collector/testdata/codex-session.jsonl`

- [ ] **Step 1: Add sanitized end-to-end fixtures**

Fixtures must contain no real user paths, API keys, repository secrets, or conversation data. Each fixture must cover a user message, assistant message, tool call, tool result, error, usage record, and an unknown event. Include a pair of usage records at `10:00:59` and `10:01:01` for rolling-window verification.

- [ ] **Step 2: Run the full automated suite from a clean dependency state**

Run:

```bash
go test ./... -count=1
npm test
npm run build
cargo test --manifest-path src-tauri/Cargo.toml
```

Expected: every command exits 0. Record exact failures before changing code; do not weaken assertions to make the suite green.

- [ ] **Step 3: Build and run the real sidecar against temporary data**

Create a temporary config that points Claude and Codex collectors to copied fixtures and storage to a temporary SQLite file. Run:

```bash
go build -o /tmp/agent-usage-desktop-e2e .
/tmp/agent-usage-desktop-e2e --config /tmp/agent-usage-desktop-e2e.yaml --port 19800
```

Verify with GET requests:

```bash
curl -fsS 'http://127.0.0.1:19800/api/health'
curl -fsS 'http://127.0.0.1:19800/api/stats?from=2026-07-01&to=2026-07-31'
curl -fsS 'http://127.0.0.1:19800/api/sessions?from=2026-07-01&to=2026-07-31&q=fixture'
curl -fsS 'http://127.0.0.1:19800/api/throughput?from=2026-07-01&to=2026-07-31'
```

Expected: health is `ok`, both sources appear, FTS finds the seeded message, and rolling peak RPM is 2. Stop the process before continuing.

- [ ] **Step 4: Perform the desktop workflow check**

Run: `npx tauri dev`

Verify in both a desktop-sized window and a width below 900px:

1. overview loads exact token components and local-observed throughput;
2. clicking Agent/model/project/date opens matching session filters;
3. search finds message and tool content;
4. tool calls are collapsed and errors expanded;
5. raw JSON is loaded only after explicit action;
6. removing a copied source preserves statistics and changes coverage state;
7. rebuild clears/recreates only the local index;
8. navigation contains only overview, sessions, and settings;
9. no content overlaps and every label fits its control.

- [ ] **Step 5: Update public documentation**

Document the local-only product positioning, supported depth (`Claude Code`, `Codex`), statistics-only sources (`OpenCode`, `OpenClaw`), token semantics, observed RPM/TPM limitations, source-of-truth behavior, privacy boundary, and exact build/test commands. Mention Hermes and branded PNG/PDF reports only in a roadmap section.

- [ ] **Step 6: Scan scope and commit**

Run:

```bash
rg -n "Provider|MCP|Skills manager|config switch|team|handoff" README.md README.zh-CN.md src internal/server main.go
rg -n "quota|remaining limit|rate limit utilization" src internal
git status --short
```

Expected: only deliberate roadmap/non-goal wording remains; no removed runtime surface or misleading quota claim remains; status contains only this release's files.

```bash
git add README.md README.zh-CN.md internal/collector/testdata
git add -f docs/superpowers/specs/2026-07-23-local-agent-usage-session-explorer-design.md
git commit -m "docs: document local usage and session explorer"
```

---

## Final Acceptance Checklist

- [ ] 主导航只有总览、会话、设置，配置管理 API 和运行时写入全部消失。
- [ ] 旧数据库升级后 token、费用、会话和 prompt 历史总数不变。
- [ ] Claude Code 与 Codex 的事件、工具、错误、原始定位和增量扫描测试完整通过。
- [ ] 原始来源消失只清理正文/FTS，保留统计和会话元数据。
- [ ] 搜索覆盖消息、工具名、参数和结果，所有会话身份均使用 `(source, session_id)`。
- [ ] 总览不再使用估算比例拆分 token；所有组件来自后端非重叠字段。
- [ ] 本地观测 RPM/TPM 同时提供活跃分钟平均、滚动峰值、滚动 P95、分钟趋势和 Agent/模型筛选。
- [ ] 页面明确说明本地观测值不是供应商额度或限流使用率。
- [ ] 会话中心符合 300–340px 列表、宽时间线、按需检查器和小屏两级页面布局。
- [ ] `go test ./...`、`npm test`、`npm run build`、`cargo test` 全部通过。
