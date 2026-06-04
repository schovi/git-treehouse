package pathutil

import (
	"path/filepath"
	"strings"
	"unicode"
)

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
		template = "{repo_parent}/{branch}"
	}
	sanitizedBranch := SanitizeBranch(branch)
	values := map[string]string{
		"{repo}":        repoRoot,
		"{repo_parent}": filepath.Dir(repoRoot),
		"{branch}":      sanitizedBranch,
	}
	result := template
	for placeholder, value := range values {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	if !filepath.IsAbs(result) {
		result = filepath.Join(repoRoot, result)
	}
	return filepath.Clean(result)
}
