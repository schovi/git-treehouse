package tui

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"strings"
	"sync"
	"testing"
)

type testRunner struct{}

func (runner testRunner) Run(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
	return nil, errors.New("unexpected command")
}

func (runner testRunner) RunWithEnv(_ context.Context, _ string, _ []string, _ string, _ ...string) ([]byte, error) {
	return nil, errors.New("unexpected command")
}

type recordingRunner struct {
	mutex       sync.Mutex
	commands    []string
	envCommands []recordedEnvCommand
	results     map[string]recordingResult
}

type recordingResult struct {
	output string
	err    error
}

type recordedEnvCommand struct {
	command string
	env     []string
}

func (runner *recordingRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	return runner.run(dir, nil, name, args...)
}

func (runner *recordingRunner) RunWithEnv(_ context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	return runner.run(dir, env, name, args...)
}

type cancelledHookRunner struct {
	*recordingRunner
}

func (runner cancelledHookRunner) RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	if name == "sh" && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return runner.recordingRunner.RunWithEnv(ctx, dir, env, name, args...)
}

func (runner *recordingRunner) run(dir string, env []string, name string, args ...string) ([]byte, error) {
	key := dir + "|" + name + " " + strings.Join(args, " ")
	runner.mutex.Lock()
	runner.commands = append(runner.commands, key)
	if env != nil {
		runner.envCommands = append(runner.envCommands, recordedEnvCommand{command: key, env: append([]string(nil), env...)})
	}
	result, ok := runner.results[key]
	runner.mutex.Unlock()
	if ok {
		return []byte(result.output), result.err
	}
	return nil, nil
}

func updateModel(t *testing.T, model Model, message tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := model.Update(message)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", updated)
	}
	return next, cmd
}

func firstDeleteMessage(t *testing.T, cmd tea.Cmd) deleteMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("delete command is nil")
	}
	batchMessage := cmd()
	batch, ok := batchMessage.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("delete command returned %T, want BatchMsg with delete command", batchMessage)
	}
	firstMessage := batch[0]()
	message, ok := firstMessage.(deleteMsg)
	if !ok {
		t.Fatalf("first delete batch message = %T, want deleteMsg", firstMessage)
	}
	return message
}

func firstCleanupMergedMessage(t *testing.T, cmd tea.Cmd) cleanupMergedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("cleanup command is nil")
	}
	batchMessage := cmd()
	batch, ok := batchMessage.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("cleanup command returned %T, want BatchMsg with cleanup command", batchMessage)
	}
	firstMessage := batch[0]()
	message, ok := firstMessage.(cleanupMergedMsg)
	if !ok {
		t.Fatalf("first cleanup batch message = %T, want cleanupMergedMsg", firstMessage)
	}
	return message
}

func testModelWithRows(rows []gitdata.Worktree) Model {
	for index := range rows {
		rows[index].LocalMetadataLoaded = true
	}
	return New(gitdata.State{
		Repo: gitdata.Repository{
			Root:           "/repo/main",
			ActiveWorktree: "/repo/main",
		},
		Rows: rows,
	}, appconfig.Default(), nil, false, false)
}

func stableLoadResults(worktreeList string) map[string]recordingResult {
	return map[string]recordingResult{
		"/repo/main|git rev-parse --show-toplevel":                         {output: "/repo/main\n"},
		"/repo/main|git rev-parse --git-common-dir":                        {output: ".git\n"},
		"/repo/main|git rev-parse --path-format=absolute --git-common-dir": {output: "/repo/main/.git\n"},
		"/repo/main|git worktree list --porcelain":                         {output: worktreeList},
		"/repo/main|git symbolic-ref --short refs/remotes/origin/HEAD":     {err: errors.New("no origin")},
		"/repo/main|git show-ref --verify --quiet refs/heads/main":         {},
		"/repo/main|git remote":                                            {},
	}
}

func hasCommand(commands []string, want string) bool {
	for _, command := range commands {
		if command == want {
			return true
		}
	}
	return false
}

func hasCleanupSkip(skips []cleanupMergedSkip, want cleanupMergedSkip) bool {
	for _, skip := range skips {
		if skip.name == want.name && skip.reason == want.reason {
			return true
		}
	}
	return false
}

func visibleBranches(model Model) []string {
	indexes := model.visibleIndexes()
	rows := model.tableRows()
	branches := make([]string, 0, len(indexes))
	for _, index := range indexes {
		branches = append(branches, rows[index].DisplayBranch())
	}
	return branches
}

// The detail region below the list is taller for some rows (a dirty worktree adds a
// Changes frame) than others, so sizing the list against the selected row's own
// detail made rows appear and disappear while navigating. The list must instead be
// sized for the tallest row and stay fixed.
