package main

import (
	"os"
	"strings"
	"testing"
)

func TestCanonicalReadmesDocumentProductContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path         string
		required     []string
		roadmapTerms [][]string
	}{
		{
			path: "README.md",
			required: []string{
				"[中文文档](README.zh-CN.md)",
				"local, single-user application",
				"Claude Code and Codex support deep session retrospective",
				"OpenCode and OpenClaw currently provide statistics only",
				"non-cached input",
				"cache read input",
				"cache creation input",
				"output tokens",
				"Reasoning output is informational and is a subset of output",
				"locally observed throughput, not provider quota or rate-limit utilization",
				"Raw agent files are the source of truth",
				"SQLite index can be rebuilt",
				"Session content is never uploaded",
				"Pricing sync is the only routine network request",
				"Overview, Sessions, and Settings",
				"npm install",
				"npx tauri dev",
				"npx tauri build",
				"go test ./...",
				"npm test",
				"npm run build",
				"cargo test --manifest-path src-tauri/Cargo.toml",
				"go build -o agent-usage-desktop .",
				"./agent-usage-desktop --config path/to/config.yaml",
				"./agent-usage-desktop --port 9800",
				"./agent-usage-desktop version",
				"agent-usage-{rust-target-triple}[.exe]",
				"agent-usage-aarch64-apple-darwin",
				"agent-usage-x86_64-apple-darwin",
				"agent-usage-x86_64-unknown-linux-gnu",
				"agent-usage-x86_64-pc-windows-msvc.exe",
				"https://github.com/hongshuo-wang/agent-usage-desktop",
				"https://linux.do/t/topic/1922004",
			},
			roadmapTerms: [][]string{
				{"Hermes", "session retrospective"},
				{"CLAUDE.md", "AGENTS.md", "memory", "project"},
				{"PNG/PDF", "project name", "GitHub", "Linux.do"},
			},
		},
		{
			path: "README.zh-CN.md",
			required: []string{
				"[English](README.md)",
				"本地单用户应用",
				"Claude Code 和 Codex 支持深度会话回溯",
				"OpenCode 和 OpenClaw 当前仅提供统计",
				"非缓存输入",
				"缓存读取输入",
				"缓存创建输入",
				"输出 token",
				"推理输出仅供参考，是输出 token 的子集",
				"本机观测到的吞吐量，不代表供应商配额或限流利用率",
				"Agent 原始文件是事实来源",
				"SQLite 索引可以重建",
				"不会上传会话内容",
				"价格同步是唯一的常规网络请求",
				"总览、会话和设置",
				"npm install",
				"npx tauri dev",
				"npx tauri build",
				"go test ./...",
				"npm test",
				"npm run build",
				"cargo test --manifest-path src-tauri/Cargo.toml",
				"go build -o agent-usage-desktop .",
				"./agent-usage-desktop --config path/to/config.yaml",
				"./agent-usage-desktop --port 9800",
				"./agent-usage-desktop version",
				"agent-usage-{rust-target-triple}[.exe]",
				"agent-usage-aarch64-apple-darwin",
				"agent-usage-x86_64-apple-darwin",
				"agent-usage-x86_64-unknown-linux-gnu",
				"agent-usage-x86_64-pc-windows-msvc.exe",
				"https://github.com/hongshuo-wang/agent-usage-desktop",
				"https://linux.do/t/topic/1922004",
			},
			roadmapTerms: [][]string{
				{"Hermes", "会话回溯"},
				{"CLAUDE.md", "AGENTS.md", "memory", "项目"},
				{"PNG/PDF", "项目名称", "GitHub", "Linux.do"},
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

			roadmap := markdownSection(content, "Roadmap", "路线图")
			bullets := markdownBullets(roadmap)
			if len(bullets) != len(tt.roadmapTerms) {
				t.Errorf("%s roadmap has %d items, want exactly %d: %#v", tt.path, len(bullets), len(tt.roadmapTerms), bullets)
			} else {
				for i, terms := range tt.roadmapTerms {
					for _, term := range terms {
						if !strings.Contains(bullets[i], term) {
							t.Errorf("%s roadmap item %d missing scope term %q: %q", tt.path, i+1, term, bullets[i])
						}
					}
				}
			}
			for _, forbidden := range []string{
				"multi-user", "Multi-user", "teams", "cloud sync",
				"configuration switching", "Provider/MCP/Skills", "agent handoff",
				"多用户", "团队", "云同步", "配置切换", "Agent 交接",
			} {
				if strings.Contains(roadmap, forbidden) {
					t.Errorf("%s roadmap contains out-of-scope term %q", tt.path, forbidden)
				}
			}
		})
	}
}

func TestLegacyChineseReadmeIsShortRedirect(t *testing.T) {
	t.Parallel()

	content := readContractFile(t, "README_CN.md")
	if !strings.Contains(content, "README.zh-CN.md") {
		t.Fatal("README_CN.md must redirect to README.zh-CN.md")
	}
	if lines := len(strings.Split(strings.TrimSpace(content), "\n")); lines > 5 {
		t.Fatalf("README_CN.md has %d lines, want a short compatibility redirect", lines)
	}
}

func TestCanonicalReadmesUseDesktopDatabasePath(t *testing.T) {
	t.Parallel()

	const desktopDatabasePath = `path: "~/.config/agent-usage/agent-usage.db"`
	for _, path := range []string{"README.md", "README.zh-CN.md"} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			content := readContractFile(t, path)
			if !strings.Contains(content, desktopDatabasePath) {
				t.Errorf("%s desktop config must use %s", path, desktopDatabasePath)
			}
		})
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

func markdownSection(content string, headings ...string) string {
	for _, heading := range headings {
		marker := "## " + heading
		start := strings.Index(content, marker)
		if start < 0 {
			continue
		}
		section := content[start+len(marker):]
		if end := strings.Index(section, "\n## "); end >= 0 {
			section = section[:end]
		}
		return section
	}
	return ""
}

func markdownBullets(section string) []string {
	var bullets []string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- [ ] ") {
			bullets = append(bullets, strings.TrimPrefix(line, "- [ ] "))
		}
	}
	return bullets
}
