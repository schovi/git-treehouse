package pathutil

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const DefaultTemplate = "{repo_parent}/.worktrees/{repo_name}/{branch}"

func SanitizeBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	branch = strings.TrimPrefix(branch, "refs/heads/")
	var builder strings.Builder
	lastDash := false
	for _, character := range branch {
		writeDash := character == '/' || character == '\\' || unicode.IsSpace(character)
		if writeDash {
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
			continue
		}
		builder.WriteRune(character)
		lastDash = false
	}
	return strings.Trim(builder.String(), "-")
}

func ApplyTemplate(template, repoRoot, branch string) string {
	if template == "" {
		template = DefaultTemplate
	}
	sanitizedBranch := SanitizeBranch(branch)
	values := map[string]string{
		"{repo}":        repoRoot,
		"{repo_name}":   filepath.Base(repoRoot),
		"{repo_parent}": filepath.Dir(repoRoot),
		"{branch}":      sanitizedBranch,
	}
	result := template
	for placeholder, value := range values {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	result = ExpandHome(result)
	if !filepath.IsAbs(result) {
		result = filepath.Join(repoRoot, result)
	}
	return filepath.Clean(result)
}

func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}
