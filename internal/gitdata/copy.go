package gitdata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CopyWorktreeFiles(sourceRoot, destRoot string, files []string) []error {
	var errs []error
	for _, file := range files {
		relativePath, err := cleanWorktreeFilePath(file)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		sourcePath := filepath.Join(sourceRoot, relativePath)
		info, err := os.Stat(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		if info.IsDir() {
			errs = append(errs, fmt.Errorf("%s: source is a directory", file))
			continue
		}

		contents, err := os.ReadFile(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}

		destPath := filepath.Join(destRoot, relativePath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		mode := info.Mode().Perm()
		if err := os.WriteFile(destPath, contents, mode); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		if err := os.Chmod(destPath, mode); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
		}
	}
	return errs
}

func cleanWorktreeFilePath(file string) (string, error) {
	if file == "" {
		return "", fmt.Errorf("%q is not a repo-relative file", file)
	}
	if filepath.IsAbs(file) {
		return "", fmt.Errorf("%q is absolute", file)
	}
	clean := filepath.Clean(file)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q escapes the repository", file)
	}
	return clean, nil
}
