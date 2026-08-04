package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNOnBranchRowShowsEnterHint(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}
	model.filter = filterBranches

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if model.branchWorktreeDialog != nil {
		t.Fatal("n on branch should not open branch worktree dialog")
	}
	if model.flash != "press Enter to create a worktree for this branch" {
		t.Fatalf("flash = %q, want branch Enter hint", model.flash)
	}
	if cmd == nil {
		t.Fatal("n on branch should return flash timeout command")
	}
}

func TestBranchWorktreeDialogAddsExistingBranchWorktree(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|git worktree add /repo/.worktrees/main/feature-branch feature/branch": {},
	}}
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.runner = runner
	model.branchWorktreeDialog = &branchWorktreeDialog{
		branch: gitdata.Branch{Name: "feature/branch"},
		path:   "/repo/.worktrees/main/feature-branch",
	}

	model, cmd := model.updateBranchWorktree(tea.KeyMsg{Type: tea.KeyEnter})

	if model.loading != "creating…" {
		t.Fatalf("loading = %q, want creating…", model.loading)
	}
	if cmd == nil {
		t.Fatal("Enter checkout returned nil command")
	}
	message := cmd().(checkoutMsg)
	if message.err != nil {
		t.Fatalf("checkout command error = %v", message.err)
	}
	if message.path != "/repo/.worktrees/main/feature-branch" {
		t.Fatalf("checkout path = %q, want dialog path", message.path)
	}
	if len(runner.commands) != 1 || runner.commands[0] != "/repo/main|git worktree add /repo/.worktrees/main/feature-branch feature/branch" {
		t.Fatalf("commands = %v, want git worktree add existing branch", runner.commands)
	}
}

func TestCreateWorktreeCopiesFilesAndRunsApprovedPostCreate(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(repoRoot, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".env"), []byte("TOKEN=1\n"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runner := &recordingRunner{}
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = runner
	model.state.Repo.Root = repoRoot
	model.state.Repo.MainBranch = "main"
	model.repoConfig = appconfig.RepoConfig{
		CopyUntracked: []string{".env"},
		PostCreate:    "npm install",
	}
	model.hooksApproved = true
	model.createDialog.input.SetValue("feature/hook")
	path := model.createPathPreview()

	model, cmd := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("create returned nil command")
	}
	message := cmd().(createMsg)
	if message.err != nil {
		t.Fatalf("create command error = %v", message.err)
	}
	if !message.created {
		t.Fatal("create command should mark worktree created")
	}
	addCommand := repoRoot + "|git worktree add -b feature/hook " + path + " main"
	hookCommand := path + "|sh -c npm install"
	addIndex := slices.Index(runner.commands, addCommand)
	hookIndex := slices.Index(runner.commands, hookCommand)
	if addIndex == -1 || hookIndex == -1 || hookIndex <= addIndex {
		t.Fatalf("commands = %v, want hook after worktree add", runner.commands)
	}
	copied, err := os.ReadFile(filepath.Join(path, ".env"))
	if err != nil {
		t.Fatalf("ReadFile(copied .env) error = %v", err)
	}
	if string(copied) != "TOKEN=1\n" {
		t.Fatalf("copied .env = %q, want TOKEN", string(copied))
	}
	if len(runner.envCommands) != 1 || runner.envCommands[0].command != hookCommand {
		t.Fatalf("env commands = %+v, want post_create hook", runner.envCommands)
	}
	for _, wantEnv := range []string{
		"GTH_EVENT=post_create",
		"GTH_WORKTREE_PATH=" + path,
		"GTH_WORKTREE_BRANCH=feature/hook",
		"GTH_REPO_ROOT=" + repoRoot,
		"GTH_MAIN_BRANCH=main",
	} {
		if !slices.Contains(runner.envCommands[0].env, wantEnv) {
			t.Fatalf("hook env missing %q: %#v", wantEnv, runner.envCommands[0].env)
		}
	}
}

func TestCreateWorktreeSkipsUnapprovedPostCreateWithWarning(t *testing.T) {
	runner := &recordingRunner{}
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = runner
	model.repoConfig = appconfig.RepoConfig{PostCreate: "npm install"}
	model.hooksApproved = false
	model.createDialog.input.SetValue("feature/hook")
	path := model.createPathPreview()

	model, cmd := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})

	message := cmd().(createMsg)
	if message.err != nil {
		t.Fatalf("create command error = %v", message.err)
	}
	if !message.created {
		t.Fatal("create command should mark worktree created")
	}
	addCommand := "/repo/main|git worktree add -b feature/hook " + path + " main"
	if !slices.Contains(runner.commands, addCommand) {
		t.Fatalf("commands = %v, want git worktree add", runner.commands)
	}
	if len(runner.envCommands) != 0 {
		t.Fatalf("env commands = %+v, want hook skipped", runner.envCommands)
	}
	if len(message.warnings) != 1 || message.warnings[0] != "post_create hook not approved; run git-treehouse allow" {
		t.Fatalf("warnings = %#v, want unapproved hook warning", message.warnings)
	}
	updated, _ := updateModel(t, model, message)
	if updated.selectedPath != path {
		t.Fatalf("selectedPath = %q, want created path", updated.selectedPath)
	}
	if !strings.Contains(updated.flash, "post_create hook not approved") {
		t.Fatalf("flash = %q, want warning", updated.flash)
	}
}

func TestCreateWorktreeHookFailureDoesNotSelectCreatedPath(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/.worktrees/main/feature-hook|sh -c npm install": {err: errors.New("install failed")},
	}}
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = runner
	model.repoConfig = appconfig.RepoConfig{PostCreate: "npm install"}
	model.hooksApproved = true
	model.createDialog.input.SetValue("feature/hook")
	path := model.createPathPreview()

	model, cmd := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	message := cmd().(createMsg)

	if message.err == nil || !message.created {
		t.Fatalf("message = %+v, want created hook failure", message)
	}
	updated, _ := updateModel(t, model, message)
	if updated.selectedPath != "" {
		t.Fatalf("selectedPath = %q, want empty after hook failure", updated.selectedPath)
	}
	if updated.createDialog == nil || !strings.Contains(updated.createDialog.error, "worktree created at "+path+", but post_create failed") {
		t.Fatalf("create dialog error = %q, want created hook failure", updated.createDialog.error)
	}
}

func TestCreateFailureAfterDialogCloseShowsFlash(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.createDialog = nil
	model.createInFlight = true

	updated, cmd := updateModel(t, model, createMsg{err: errors.New("create failed")})

	if updated.flash != "create failed" {
		t.Fatalf("flash = %q, want create failure", updated.flash)
	}
	if updated.createInFlight {
		t.Fatal("createInFlight should clear after create result")
	}
	if cmd == nil {
		t.Fatal("create failure flash should schedule clearing")
	}
}

func TestCreateDialogIgnoresEnterWhileCreateInFlight(t *testing.T) {
	runner := &recordingRunner{}
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = runner
	model.createDialog.input.SetValue("feature/guard")
	path := model.createPathPreview()

	model, firstCreate := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	if firstCreate == nil {
		t.Fatal("first Enter should start create")
	}
	if !model.createInFlight {
		t.Fatal("createInFlight should be set while create runs")
	}
	_, secondCreate := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	if secondCreate != nil {
		t.Fatal("second Enter should not start another create")
	}
	if message := firstCreate().(createMsg); message.err != nil {
		t.Fatalf("first create command error = %v", message.err)
	}
	command := "/repo/main|git worktree add -b feature/guard " + path + " main"
	createCount := 0
	for _, recorded := range runner.commands {
		if recorded == command {
			createCount++
		}
	}
	if createCount != 1 {
		t.Fatalf("create command count = %d, want 1; commands = %v", createCount, runner.commands)
	}
}

func TestOpenCreateStartsWithEmptyBranchName(t *testing.T) {
	model := Model{
		runner: testRunner{},
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/main"},
			Rows: []gitdata.Worktree{{
				Path:   "/repo/main",
				Branch: "feature/source",
			}},
		},
	}

	model, cmd := model.openCreate()

	if model.createDialog == nil {
		t.Fatal("openCreate() did not open create dialog")
	}
	if cmd == nil {
		t.Fatal("openCreate() should return input focus command")
	}
	if got := model.createDialog.input.Value(); got != "" {
		t.Fatalf("create branch input = %q, want empty", got)
	}
	if model.createDialog.error != "" {
		t.Fatalf("create dialog error = %q, want empty initial error", model.createDialog.error)
	}
	if !model.createDialog.input.Focused() {
		t.Fatal("create branch input should be focused")
	}
}

func TestCreateDialogTypingUpdatesBranchInputWithoutValidation(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	if got := model.createDialog.input.Value(); got != "fe" {
		t.Fatalf("create branch input = %q, want fe", got)
	}
	if model.createDialog.error != "" {
		t.Fatalf("typing should not validate immediately, got error %q", model.createDialog.error)
	}
}

func TestCreateDialogTextNavigationDoesNotChangeBase(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{
		{Label: "main (local)", Rev: "main"},
		{Label: "origin/main", Rev: "origin/main"},
	})
	model.createDialog.input.SetValue("ab")

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyLeft})
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRight})

	if got := model.createDialog.input.Value(); got != "axb" {
		t.Fatalf("left arrow should move text cursor, input = %q, want axb", got)
	}
	if got := model.createDialog.baseIndex; got != 0 {
		t.Fatalf("left/right should not change base index, got %d", got)
	}
}

func TestCreateDialogRenderShowsTypedBranchName(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.createDialog.input.SetValue("feature/login")

	output := model.renderCreateAtWidth(72)

	if !strings.Contains(output, "feature/login") {
		t.Fatalf("renderCreateAtWidth() should show typed branch name:\n%s", output)
	}
}

func TestCreateDialogRendersDetectedMultiplexerDestination(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.createDialog.destination = worktreeDestinationTmux

	output := ansi.Strip(model.renderCreateAtWidth(120))

	if !strings.Contains(output, "Enter create + open tmux window") {
		t.Fatalf("create dialog should name tmux destination:\n%s", output)
	}
}

func TestAllCreationRoutesReloadAndSelectMultiplexerWorktree(t *testing.T) {
	const path = "/repo/.worktrees/main/feature-new"
	const branch = "feature/new"
	tests := []struct {
		name   string
		model  func() Model
		result tea.Msg
	}{
		{
			name: "new branch",
			model: func() Model {
				model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
				model.createInFlight = true
				return model
			},
			result: createMsg{path: path, branch: branch, destination: worktreeDestinationTmux, created: true},
		},
		{
			name: "existing local branch",
			model: func() Model {
				model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
				model.branchWorktreeDialog = &branchWorktreeDialog{branch: gitdata.Branch{Name: branch}, path: path}
				return model
			},
			result: checkoutMsg{path: path, branch: branch, destination: worktreeDestinationTmux, createsWorktree: true, created: true},
		},
		{
			name: "pull request",
			model: func() Model {
				model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
				model.pullRequestDialog = &pullRequestCheckoutDialog{}
				return model
			},
			result: checkoutMsg{path: path, branch: branch, destination: worktreeDestinationZellij, createsWorktree: true, created: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			started, startCmd := updateModel(t, test.model(), test.result)
			if startCmd == nil {
				t.Fatal("successful creation should start the multiplexer action")
			}
			if started.selectedPath != "" {
				t.Fatalf("selectedPath = %q, want empty until reload", started.selectedPath)
			}

			reloading, reloadCmd := updateModel(t, started, worktreeDestinationOpenedMsg{path: path})
			if reloadCmd == nil || !reloading.refreshInFlight || reloading.refreshAnchor.path != path || reloading.createInFlight {
				t.Fatalf("multiplexer action should reload with the created path selected: %+v", reloading)
			}
			if reloading.createDialog != nil || reloading.branchWorktreeDialog != nil || reloading.pullRequestDialog != nil {
				t.Fatal("multiplexer action should return to the table before reloading")
			}

			state := reloading.state
			state.Rows = []gitdata.Worktree{
				{Path: "/repo/main", Branch: "main", IsMain: true, LocalMetadataLoaded: true},
				{Path: path, Branch: branch, LocalMetadataLoaded: true},
			}
			updated, _ := updateModel(t, reloading, reloadMsg{id: reloading.refreshID, state: state, completedAt: time.Now()})
			row, ok := updated.selectedTableRow()
			if !ok || !row.IsWorktree() || row.Worktree.Path != path {
				t.Fatalf("selected row = %+v, want created worktree %q", row, path)
			}
		})
	}
}

func TestMultiplexerStartFailureReloadsCreatedWorktreeAndShowsError(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.createInFlight = true
	path := "/repo/.worktrees/main/feature-new"

	updated, reloadCmd := updateModel(t, model, worktreeDestinationOpenedMsg{path: path, err: errors.New("open tmux window: executable not found")})

	if reloadCmd == nil || !updated.refreshInFlight || updated.refreshAnchor.path != path {
		t.Fatalf("failed multiplexer start should still reload created worktree: %+v", updated)
	}
	if updated.createDialog != nil || !strings.Contains(updated.flash, "open tmux window") {
		t.Fatalf("failure should close the dialog and report the action error: %+v", updated)
	}
}

func TestCreateDialogRenderShowsLivePathPreview(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.state.Repo.Root = "/repo/git-treehouse"
	model.createDialog.input.SetValue("feature/login")

	output := model.renderCreateAtWidth(100)

	want := filepath.Join("/repo", ".worktrees", "git-treehouse", "feature-login")
	if !strings.Contains(output, want) {
		t.Fatalf("renderCreateAtWidth() should show path %q:\n%s", want, output)
	}
}

func TestCreateDialogRendersLivePathCollision(t *testing.T) {
	forceANSIProfile(t)
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = &recordingRunner{}
	repoRoot := filepath.Join(t.TempDir(), "git-treehouse")
	model.state.Repo.Root = repoRoot
	targetPath := filepath.Join(filepath.Dir(repoRoot), ".worktrees", "git-treehouse", "feature-login")
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", targetPath, err)
	}

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature/login")})
	output := model.renderCreateAtWidth(200)
	want := "target path already exists: " + targetPath

	if !strings.Contains(output, lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(want)) {
		t.Fatalf("renderCreateAtWidth() should show styled live collision error %q:\n%s", want, output)
	}

	model, command := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("Enter should stay blocked on a path collision")
	}
	if got := model.createDialog.error; got != want {
		t.Fatalf("Enter collision error = %q, want %q", got, want)
	}
}

func TestCreateDialogDoesNotRenderCollisionForAvailablePath(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.state.Repo.Root = filepath.Join(t.TempDir(), "git-treehouse")

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature/login")})
	output := model.renderCreateAtWidth(200)

	if strings.Contains(output, "target path already exists:") {
		t.Fatalf("renderCreateAtWidth() should not show collision for available path:\n%s", output)
	}
}

func TestCreateDialogConfigShortcutCreatesAndOpensConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.config = appconfig.Config{
		Editor:       "true",
		PathTemplate: "{repo_parent}/custom/{branch}",
	}

	_, cmd := model.updateCreate(tea.KeyMsg{Type: tea.KeyCtrlO})
	if cmd == nil {
		t.Fatal("ctrl+o should return config editor command")
	}
	message := cmd()
	opened, ok := message.(configOpenedMsg)
	if !ok {
		t.Fatalf("config command message = %T, want configOpenedMsg", message)
	}
	if opened.err != nil {
		t.Fatalf("config command error = %v", opened.err)
	}

	path, err := appconfig.Path()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(content), `path_template = "{repo_parent}/custom/{branch}"`) {
		t.Fatalf("config should contain current path template:\n%s", content)
	}
}
