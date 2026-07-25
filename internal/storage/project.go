package storage

import (
	"path"
	"strings"
)

// effectiveProject returns the stable project key used by both breakdowns and
// session summaries. Per-record project metadata wins over session metadata;
// cwd and session ID provide deterministic fallbacks for sources that omit it.
func effectiveProject(project, sessionProject, cwd, sessionID string) string {
	for _, value := range []string{project, sessionProject} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	if base := cwdBase(cwd); base != "" {
		return base
	}
	return strings.TrimSpace(sessionID)
}

func cwdBase(cwd string) string {
	cwd = strings.TrimSpace(strings.ReplaceAll(cwd, "\\", "/"))
	cwd = strings.TrimRight(cwd, "/")
	if cwd == "" {
		return ""
	}
	base := path.Base(cwd)
	if base == "." || base == "/" || base == ".." {
		return ""
	}
	return base
}
