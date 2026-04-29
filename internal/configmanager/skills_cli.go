package configmanager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	agentUsageSkillRepo = "hongshuo-wang/agent-usage-desktop"
	agentUsageSkillName = "agent-usage-desktop"
)

var agentUsageSkillAgentOrder = []string{"claude", "codex", "opencode", "openclaw"}

var agentUsageSkillAgentNames = map[string]string{
	"claude":   "claude-code",
	"codex":    "codex",
	"opencode": "opencode",
	"openclaw": "openclaw",
}

type SkillsCLIActionResult struct {
	Command string `json:"command"`
	Output  string `json:"output"`
}

func detectSkillsCLI() SkillCLIStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := runSkillsCLI(ctx, "--help")
	status := SkillCLIStatus{Command: "npx --yes skills --help"}
	if ctx.Err() == context.DeadlineExceeded {
		status.Message = "skills detection timed out"
		return status
	}
	if err != nil {
		status.Message = firstNonEmptyLine(output)
		if status.Message == "" || status.Message == "npx skills available" {
			status.Message = "skills command unavailable"
		}
		return status
	}

	status.Available = true
	status.Message = firstNonEmptyLine(output)
	return status
}

func runSkillsCLI(ctx context.Context, args ...string) (string, error) {
	commandArgs := append([]string{"--yes", "skills"}, args...)
	cmd := exec.CommandContext(ctx, "npx", commandArgs...)

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		cmd.Dir = home
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func skillsCLIMessage(output string, fallback string) string {
	line := firstNonEmptyLine(output)
	if line == "" || line == "npx skills available" {
		return fallback
	}
	return line
}

func normalizeAgentUsageSkillAgents(agents []string, defaultAll bool) ([]string, error) {
	if agents == nil {
		if defaultAll {
			return append([]string(nil), agentUsageSkillAgentOrder...), nil
		}
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("agents are required")
	}

	requested := make(map[string]struct{}, len(agents))
	for _, agent := range agents {
		key := strings.ToLower(strings.TrimSpace(agent))
		if _, ok := agentUsageSkillAgentNames[key]; !ok {
			return nil, fmt.Errorf("invalid agent %q", agent)
		}
		requested[key] = struct{}{}
	}

	normalized := make([]string, 0, len(requested))
	for _, key := range agentUsageSkillAgentOrder {
		if _, ok := requested[key]; ok {
			normalized = append(normalized, key)
		}
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("agents are required")
	}
	return normalized, nil
}

func buildAgentUsageSkillCommandArgs(action string, agents []string) ([]string, error) {
	normalizedAgents, err := normalizeAgentUsageSkillAgents(agents, true)
	if err != nil {
		return nil, err
	}

	args := make([]string, 0, 8+len(normalizedAgents)*2)
	switch action {
	case "install":
		args = append(args, "add", agentUsageSkillRepo, "--global", "--skill", agentUsageSkillName)
	case "uninstall":
		args = append(args, "remove", agentUsageSkillName, "--global")
	default:
		return nil, fmt.Errorf("invalid action %q", action)
	}

	for _, agent := range normalizedAgents {
		args = append(args, "--agent", agentUsageSkillAgentNames[agent])
	}
	args = append(args, "--yes")
	return args, nil
}

func skillsCLICommand(args []string) string {
	return "npx --yes skills " + strings.Join(args, " ")
}

func (m *Manager) InstallAgentUsageSkill(agents []string) (*SkillsCLIActionResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args, err := buildAgentUsageSkillCommandArgs("install", agents)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	output, err := m.runSkillsCLIFn(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("install agent-usage skill: %s", skillsCLIMessage(output, err.Error()))
	}

	return &SkillsCLIActionResult{
		Command: skillsCLICommand(args),
		Output:  skillsCLIMessage(output, "agent-usage skill installed"),
	}, nil
}

func (m *Manager) UninstallAgentUsageSkill(agents []string) (*SkillsCLIActionResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args, err := buildAgentUsageSkillCommandArgs("uninstall", agents)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	output, err := m.runSkillsCLIFn(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("remove agent-usage skill: %s", skillsCLIMessage(output, err.Error()))
	}

	return &SkillsCLIActionResult{
		Command: skillsCLICommand(args),
		Output:  skillsCLIMessage(output, "agent-usage skill removed"),
	}, nil
}
