package onboarding

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Action int

const (
	ActionContinue Action = iota
	ActionInstall
	ActionSkip
)

type Info struct {
	Shell             string
	ActivationCommand string
	InstallPath       string
	ReloadCommand     string
}

type Result struct {
	Action Action
}

type model struct {
	info     Info
	selected int
	result   Result
	width    int
	height   int
}

var (
	borderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("65"))
	titleStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	bodyStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	mutedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	commandStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	buttonStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	buttonFocusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("62")).
				Bold(true)
)

func Run(output io.Writer, info Info) (Result, error) {
	program := tea.NewProgram(newModel(info), tea.WithOutput(output), tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return Result{}, err
	}
	model, ok := finalModel.(model)
	if !ok {
		return Result{Action: ActionContinue}, nil
	}
	return model.result, nil
}

func newModel(info Info) model {
	return model{
		info:   info,
		result: Result{Action: ActionContinue},
	}
}

func (model model) Init() tea.Cmd {
	return nil
}

func (model model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
	case tea.KeyMsg:
		switch message.String() {
		case "tab", "right", "down":
			model.selected = (model.selected + 1) % len(model.actions())
		case "shift+tab", "left", "up":
			model.selected = (model.selected + len(model.actions()) - 1) % len(model.actions())
		case "i":
			model.result = Result{Action: ActionInstall}
			return model, tea.Quit
		case "s":
			model.result = Result{Action: ActionSkip}
			return model, tea.Quit
		case "c", "esc", "q":
			model.result = Result{Action: ActionContinue}
			return model, tea.Quit
		case "enter":
			model.result = Result{Action: model.actions()[model.selected]}
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model model) View() string {
	lines := []string{
		bodyStyle.Render("Git Treehouse can open the selected worktree, but a standalone child process cannot change your current shell directory."),
		bodyStyle.Render("The gth shell wrapper lets Git Treehouse write the selected path to a temporary file, then your shell cd's there after the TUI exits."),
		"",
		mutedStyle.Render("Detected shell: ") + bodyStyle.Render(model.info.Shell),
		"",
		mutedStyle.Render("For this shell session:"),
		commandStyle.Render("  " + model.info.ActivationCommand),
		"",
		mutedStyle.Render("Install target:"),
		bodyStyle.Render("  " + model.info.InstallPath),
		mutedStyle.Render("After installing, reload with:"),
		commandStyle.Render("  " + model.info.ReloadCommand),
		"",
		model.renderButtons(),
		"",
		mutedStyle.Render("Tab/arrow keys move · Enter selects · q continues to app"),
	}
	return model.renderFrame(strings.Join(lines, "\n"))
}

func (model model) renderButtons() string {
	labels := []string{"Install for me", "Don't show again", "Continue to app"}
	parts := make([]string, 0, len(labels))
	for index, label := range labels {
		text := fmt.Sprintf(" %s ", label)
		if index == model.selected {
			parts = append(parts, buttonFocusStyle.Render(text))
		} else {
			parts = append(parts, buttonStyle.Render("["+text+"]"))
		}
	}
	return strings.Join(parts, "  ")
}

func (model model) actions() []Action {
	return []Action{ActionInstall, ActionSkip, ActionContinue}
}

func (model model) renderFrame(content string) string {
	width := model.width
	if width <= 0 {
		width = 100
	}
	width = min(width, 120)
	width = max(width, 40)
	innerWidth := width - 4
	title := " " + titleStyle.Render("Set up gth shell integration") + " "
	titleWidth := lipgloss.Width(title)
	ruleWidth := max(0, width-titleWidth-3)

	lines := []string{
		borderStyle.Render("╭─") + title + borderStyle.Render(strings.Repeat("─", ruleWidth)+"╮"),
	}
	contentLines := make([]string, 0)
	wrapStyle := lipgloss.NewStyle().Width(innerWidth)
	for _, line := range strings.Split(content, "\n") {
		contentLines = append(contentLines, strings.Split(wrapStyle.Render(line), "\n")...)
	}
	if model.height > 2 {
		contentHeight := model.height - 2
		if len(contentLines) > contentHeight {
			contentLines = contentLines[:contentHeight]
		}
		for len(contentLines) < contentHeight {
			contentLines = append(contentLines, "")
		}
	}
	for _, line := range contentLines {
		lines = append(lines, borderStyle.Render("│ ")+padStyled(line, innerWidth)+borderStyle.Render(" │"))
	}
	lines = append(lines, borderStyle.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	if model.height > 0 && len(lines) > model.height {
		lines = lines[:model.height]
	}
	return strings.Join(lines, "\n")
}

func padStyled(value string, width int) string {
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}
