package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct{}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	hintStyle  = lipgloss.NewStyle().Faint(true)
)

func (m model) View() string {
	return titleStyle.Render("AppClone") + "\n\n" +
		"macOS app 克隆工具 — 骨架就绪\n\n" +
		hintStyle.Render("按 q 退出")
}

func Run() error {
	_, err := tea.NewProgram(model{}).Run()
	return err
}
