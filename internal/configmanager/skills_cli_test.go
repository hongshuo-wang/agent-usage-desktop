package configmanager

import (
	"context"
	"strings"
	"testing"
)

func TestManagerInstallAgentUsageSkillBuildsExpectedCommandForSingleAgent(t *testing.T) {
	db := openManagerTestDB(t)
	mgr := NewManager(db, t.TempDir(), WithEncryptionKey(make([]byte, 32)))

	var called []string
	mgr.runSkillsCLIFn = func(_ context.Context, args ...string) (string, error) {
		called = append([]string(nil), args...)
		return "installed\n", nil
	}

	result, err := mgr.InstallAgentUsageSkill([]string{"codex"})
	if err != nil {
		t.Fatalf("InstallAgentUsageSkill() error = %v", err)
	}

	wantParts := []string{
		"add",
		agentUsageSkillRepo,
		"--global",
		"--skill", agentUsageSkillName,
		"--agent", "codex",
		"--yes",
	}
	if strings.Join(called, " ") != strings.Join(wantParts, " ") {
		t.Fatalf("InstallAgentUsageSkill() args = %v, want %v", called, wantParts)
	}
	if result.Command != skillsCLICommand(wantParts) {
		t.Fatalf("InstallAgentUsageSkill() command = %q, want %q", result.Command, skillsCLICommand(wantParts))
	}
	if result.Output != "installed" {
		t.Fatalf("InstallAgentUsageSkill() output = %q, want %q", result.Output, "installed")
	}
}

func TestManagerInstallAgentUsageSkillBuildsExpectedCommandForMultipleAgentsInStableOrder(t *testing.T) {
	db := openManagerTestDB(t)
	mgr := NewManager(db, t.TempDir(), WithEncryptionKey(make([]byte, 32)))

	var called []string
	mgr.runSkillsCLIFn = func(_ context.Context, args ...string) (string, error) {
		called = append([]string(nil), args...)
		return "installed\n", nil
	}

	result, err := mgr.InstallAgentUsageSkill([]string{"openclaw", "claude", "codex", "claude"})
	if err != nil {
		t.Fatalf("InstallAgentUsageSkill() error = %v", err)
	}

	wantParts := []string{
		"add",
		agentUsageSkillRepo,
		"--global",
		"--skill", agentUsageSkillName,
		"--agent", "claude-code",
		"--agent", "codex",
		"--agent", "openclaw",
		"--yes",
	}
	if strings.Join(called, " ") != strings.Join(wantParts, " ") {
		t.Fatalf("InstallAgentUsageSkill() args = %v, want %v", called, wantParts)
	}
	if result.Command != skillsCLICommand(wantParts) {
		t.Fatalf("InstallAgentUsageSkill() command = %q, want %q", result.Command, skillsCLICommand(wantParts))
	}
}

func TestManagerUninstallAgentUsageSkillBuildsExpectedCommandForSingleAgent(t *testing.T) {
	db := openManagerTestDB(t)
	mgr := NewManager(db, t.TempDir(), WithEncryptionKey(make([]byte, 32)))

	var called []string
	mgr.runSkillsCLIFn = func(_ context.Context, args ...string) (string, error) {
		called = append([]string(nil), args...)
		return "removed\n", nil
	}

	result, err := mgr.UninstallAgentUsageSkill([]string{"claude"})
	if err != nil {
		t.Fatalf("UninstallAgentUsageSkill() error = %v", err)
	}

	wantParts := []string{
		"remove",
		agentUsageSkillName,
		"--global",
		"--agent", "claude-code",
		"--yes",
	}
	if strings.Join(called, " ") != strings.Join(wantParts, " ") {
		t.Fatalf("UninstallAgentUsageSkill() args = %v, want %v", called, wantParts)
	}
	if result.Command != skillsCLICommand(wantParts) {
		t.Fatalf("UninstallAgentUsageSkill() command = %q, want %q", result.Command, skillsCLICommand(wantParts))
	}
	if result.Output != "removed" {
		t.Fatalf("UninstallAgentUsageSkill() output = %q, want %q", result.Output, "removed")
	}
}

func TestNormalizeAgentUsageSkillAgentsRejectsEmptyList(t *testing.T) {
	_, err := normalizeAgentUsageSkillAgents([]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "agents are required") {
		t.Fatalf("normalizeAgentUsageSkillAgents() error = %v, want agents are required", err)
	}
}

func TestNormalizeAgentUsageSkillAgentsRejectsInvalidAgent(t *testing.T) {
	_, err := normalizeAgentUsageSkillAgents([]string{"invalid-tool"}, false)
	if err == nil || !strings.Contains(err.Error(), "invalid agent") {
		t.Fatalf("normalizeAgentUsageSkillAgents() error = %v, want invalid agent", err)
	}
}
