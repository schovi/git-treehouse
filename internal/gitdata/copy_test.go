package gitdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyWorktreeFilesCopiesExistingRelativeFiles(t *testing.T) {
	sourceRoot := t.TempDir()
	destRoot := t.TempDir()
	writeCopySourceFile(t, filepath.Join(sourceRoot, ".env"), "TOKEN=1\n", 0600)
	writeCopySourceFile(t, filepath.Join(sourceRoot, "nested", "tool.sh"), "#!/bin/sh\n", 0755)

	copyErrors := CopyWorktreeFiles(sourceRoot, destRoot, []string{".env", "nested/tool.sh", "missing.local"})
	if len(copyErrors) != 0 {
		t.Fatalf("CopyWorktreeFiles() errors = %v, want none", copyErrors)
	}
	assertCopiedFile(t, filepath.Join(destRoot, ".env"), "TOKEN=1\n", 0600)
	assertCopiedFile(t, filepath.Join(destRoot, "nested", "tool.sh"), "#!/bin/sh\n", 0755)
	if _, err := os.Stat(filepath.Join(destRoot, "missing.local")); !os.IsNotExist(err) {
		t.Fatalf("missing source destination stat error = %v, want not exist", err)
	}
}

func TestCopyWorktreeFilesRejectsEscapingPathsAndContinues(t *testing.T) {
	sourceRoot := t.TempDir()
	destRoot := t.TempDir()
	writeCopySourceFile(t, filepath.Join(sourceRoot, "safe.txt"), "safe\n", 0644)

	copyErrors := CopyWorktreeFiles(sourceRoot, destRoot, []string{
		"/absolute",
		"../outside",
		"nested/../../outside",
		"safe.txt",
	})
	if len(copyErrors) != 3 {
		t.Fatalf("CopyWorktreeFiles() error count = %d, want 3: %v", len(copyErrors), copyErrors)
	}
	assertCopiedFile(t, filepath.Join(destRoot, "safe.txt"), "safe\n", 0644)
	if _, err := os.Stat(filepath.Join(destRoot, "outside")); !os.IsNotExist(err) {
		t.Fatalf("escaping destination stat error = %v, want not exist", err)
	}
}

func TestCopyWorktreeFilesReturnsOneErrorPerFailedEntry(t *testing.T) {
	sourceRoot := t.TempDir()
	destRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(sourceRoot, "directory"), 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	copyErrors := CopyWorktreeFiles(sourceRoot, destRoot, []string{"", "directory"})
	if len(copyErrors) != 2 {
		t.Fatalf("CopyWorktreeFiles() error count = %d, want 2: %v", len(copyErrors), copyErrors)
	}
	if !strings.Contains(copyErrors[1].Error(), "source is a directory") {
		t.Fatalf("CopyWorktreeFiles() second error = %v, want directory error", copyErrors[1])
	}
}

func writeCopySourceFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
}

func assertCopiedFile(t *testing.T, path, wantContents string, wantMode os.FileMode) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(contents) != wantContents {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, string(contents), wantContents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if info.Mode().Perm() != wantMode {
		t.Fatalf("Stat(%q).Mode() = %v, want %v", path, info.Mode().Perm(), wantMode)
	}
}
