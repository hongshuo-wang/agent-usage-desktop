package main

import (
	"os"
	"strings"
	"testing"
)

func TestCanonicalReadmesDocumentProductContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		required []string
	}{
		{
			path: "README.md",
			required: []string{
				`src="docs/assets/logo.svg"`,
				`href="README.zh-CN.md"`,
				"desktop app for local usage, cost, throughput, and session analytics",
				"designed for one person",
				`src="docs/assets/overview.png"`,
				`src="docs/assets/sessions.png"`,
				"Claude Code and Codex",
				"deep event indexing is intentionally limited to Claude Code and Codex",
				"macOS Apple Silicon",
				"Windows x64",
				"macOS Intel and Linux users can build from source",
				"Raw agent files remain the source of truth",
				"Session content is never uploaded",
				"only routine network request is model-pricing refresh",
				"non-cached input + cache read input + cache creation input",
				"locally observed throughput, not provider quota",
				`path: "~/.config/agent-usage/agent-usage.db"`,
				"npm install",
				"npx tauri dev",
				"npx tauri build",
				"go test ./...",
				"go vet ./...",
				"npm test",
				"npm run build",
				"cargo test --manifest-path src-tauri/Cargo.toml",
				"agent-usage-{rust-target-triple}[.exe]",
				"https://github.com/hongshuo-wang/agent-usage-desktop",
			},
		},
		{
			path: "README.zh-CN.md",
			required: []string{
				`src="docs/assets/logo.svg"`,
				`href="README.md"`,
				"在本地统一统计和分析 AI 编程 Agent",
				"面向个人查看自己的编程 Agent 活动",
				`src="docs/assets/overview.png"`,
				`src="docs/assets/sessions.png"`,
				"Claude Code 和 Codex",
				"深度事件索引仅支持 Claude Code 和 Codex",
				"macOS Apple Silicon",
				"Windows x64",
				"macOS Intel 和 Linux 用户可以从源码构建",
				"Agent 原始文件始终是事实来源",
				"不会上传会话内容",
				"唯一的常规网络请求",
				"非缓存输入 + 缓存读取输入 + 缓存创建输入",
				"本机观测吞吐，不代表服务商配额",
				`path: "~/.config/agent-usage/agent-usage.db"`,
				"npm install",
				"npx tauri dev",
				"npx tauri build",
				"go test ./...",
				"go vet ./...",
				"npm test",
				"npm run build",
				"cargo test --manifest-path src-tauri/Cargo.toml",
				"agent-usage-{rust-target-triple}[.exe]",
				"https://github.com/hongshuo-wang/agent-usage-desktop",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			content := readContractFile(t, tt.path)
			for _, required := range tt.required {
				if !strings.Contains(content, required) {
					t.Errorf("%s missing product contract text %q", tt.path, required)
				}
			}
			if strings.Contains(content, "## Roadmap") || strings.Contains(content, "## 路线图") {
				t.Errorf("%s must describe released behavior instead of an untracked roadmap", tt.path)
			}
		})
	}
}

func TestReadmeAssetsExist(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"docs/assets/logo.svg",
		"docs/assets/overview.png",
		"docs/assets/sessions.png",
	} {
		if info, err := os.Stat(path); err != nil {
			t.Errorf("README asset %s: %v", path, err)
		} else if info.Size() == 0 {
			t.Errorf("README asset %s is empty", path)
		}
	}
}

func TestLegacyChineseReadmeRemoved(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("README_CN.md"); !os.IsNotExist(err) {
		t.Fatalf("README_CN.md should be removed; got %v", err)
	}
}

func readContractFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
