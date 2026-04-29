package configmanager

import "os/exec"

func toolCommandName(tool string) (string, bool) {
	switch tool {
	case "claude":
		return "claude", true
	case "codex":
		return "codex", true
	case "opencode":
		return "opencode", true
	case "openclaw":
		return "openclaw", true
	default:
		return "", false
	}
}

func toolCLIAvailable(tool string) bool {
	command, ok := toolCommandName(tool)
	if !ok {
		return false
	}
	_, err := exec.LookPath(command)
	return err == nil
}
