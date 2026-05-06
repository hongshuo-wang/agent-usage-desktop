package configmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

type githubContentItem struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
	HTMLURL     string `json:"html_url"`
	URL         string `json:"url"`
}

func fetchGitHubRepoSkills(ctx context.Context, source storage.SkillRepoSourceRecord) ([]RepoDiscoveredSkill, error) {
	branch := strings.TrimSpace(source.Branch)
	if branch == "" {
		branch = "main"
	}
	root := strings.Trim(strings.TrimSpace(source.Subpath), "/")

	var discovered []RepoDiscoveredSkill
	seen := map[string]struct{}{}
	if err := walkGitHubContents(ctx, source.Owner, source.Repo, branch, root, func(item githubContentItem) error {
		if item.Type != "file" || item.Name != "SKILL.md" {
			return nil
		}
		dirPath := path.Dir(item.Path)
		if _, ok := seen[dirPath]; ok {
			return nil
		}
		seen[dirPath] = struct{}{}

		content, err := fetchHTTPText(ctx, item.DownloadURL)
		if err != nil {
			return err
		}
		metadata := parseSkillMetadataContent(content)
		name := strings.TrimSpace(metadata.Name)
		if name == "" {
			name = path.Base(dirPath)
		}
		discovered = append(discovered, RepoDiscoveredSkill{
			Name:        name,
			Description: strings.TrimSpace(metadata.Description),
			Path:        dirPath,
			ReadmeURL:   item.HTMLURL,
			Hash:        sha256Hex(content),
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return discovered, nil
}

func walkGitHubContents(ctx context.Context, owner, repo, branch, subpath string, visit func(githubContentItem) error) error {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path.Clean("/"+subpath), branch)
	apiURL = strings.Replace(apiURL, "/contents//", "/contents/", 1)
	body, err := fetchHTTPText(ctx, apiURL)
	if err != nil {
		return err
	}

	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "[") {
		var items []githubContentItem
		if err := json.Unmarshal([]byte(body), &items); err != nil {
			return err
		}
		for _, item := range items {
			if item.Type == "dir" {
				if err := walkGitHubContents(ctx, owner, repo, branch, item.Path, visit); err != nil {
					return err
				}
				continue
			}
			if err := visit(item); err != nil {
				return err
			}
		}
		return nil
	}

	var item githubContentItem
	if err := json.Unmarshal([]byte(body), &item); err != nil {
		return err
	}
	if item.Type == "dir" {
		return walkGitHubContents(ctx, owner, repo, branch, item.Path, visit)
	}
	return visit(item)
}

func fetchHTTPText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "agent-usage-desktop")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("github request failed: %s", strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func parseSkillMetadataContent(content string) skillMetadata {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return skillMetadata{}
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return skillMetadata{}
	}
	metadata := skillMetadata{}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			metadata.Name = value
		case "description":
			metadata.Description = value
		}
	}
	return metadata
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
