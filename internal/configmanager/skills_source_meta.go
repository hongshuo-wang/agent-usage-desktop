package configmanager

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func inferSkillSourceMeta(sourcePath, tool string) storage.SkillSourceMeta {
	now := sql.NullTime{Time: time.Now(), Valid: true}
	meta := storage.SkillSourceMeta{
		SourceType:   skillSourceTypeManual,
		SourceLabel:  "Manual",
		LastSyncedAt: now,
	}

	libraryRoot, err := skillsLibraryPath()
	if err == nil && pathSameOrWithin(sourcePath, libraryRoot) {
		return meta
	}

	trimmedTool := strings.TrimSpace(tool)
	if trimmedTool != "" && trimmedTool != "global" {
		return storage.SkillSourceMeta{
			SourceType:   skillSourceTypeImportedTool,
			SourceLabel:  "Imported from " + toolLabel(trimmedTool),
			LastSyncedAt: now,
		}
	}

	return storage.SkillSourceMeta{
		SourceType:   skillSourceTypeImportedLocal,
		SourceLabel:  "Imported from local folder",
		LastSyncedAt: now,
	}
}

func repoSkillSourceMeta(source storage.SkillRepoSourceRecord, subpath, readmeURL string) storage.SkillSourceMeta {
	now := sql.NullTime{Time: time.Now(), Valid: true}
	label := fmt.Sprintf("%s/%s", source.Owner, source.Repo)
	if subpath != "" {
		label += ":" + strings.Trim(subpath, "/")
	}
	return storage.SkillSourceMeta{
		SourceType:    skillSourceTypeRepo,
		SourceLabel:   label,
		RepoOwner:     source.Owner,
		RepoName:      source.Repo,
		RepoBranch:    source.Branch,
		RepoSubpath:   strings.Trim(subpath, "/"),
		ReadmeURL:     readmeURL,
		Updatable:     true,
		LastCheckedAt: now,
		LastSyncedAt:  now,
	}
}
