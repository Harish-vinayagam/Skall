package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// helpEntry describes one keyboard shortcut.
type helpEntry struct {
	key  string
	desc string
}

var helpEntries = []helpEntry{
	{key: "Tab / Shift+Tab", desc: "Cycle sidebar tabs (Chats / Groups / Peers)"},
	{key: "↑ / k", desc: "Move selection up"},
	{key: "↓ / j", desc: "Move selection down"},
	{key: "Enter (sidebar)", desc: "Open selected conversation"},
	{key: "→ / l", desc: "Focus chat input"},
	{key: "← / h", desc: "Focus sidebar"},
	{key: "Enter (input)", desc: "Send message"},
	{key: "PgUp / Ctrl+B", desc: "Scroll messages up"},
	{key: "PgDn / Ctrl+F", desc: "Scroll messages down"},
	{key: "?", desc: "Toggle this help screen"},
	{key: "Ctrl+C / q", desc: "Quit SKALL"},
}

// helpViewModel is the full-screen ? overlay.
type helpViewModel struct {
	width  int
	height int
}

func newHelpView() helpViewModel { return helpViewModel{} }

func (m helpViewModel) Init() tea.Cmd { return nil }

func (m helpViewModel) Update(msg tea.Msg) (helpViewModel, tea.Cmd) {
	return m, nil
}

func (m helpViewModel) View() string {
	title := helpTitleStyle.Render("SKALL Keyboard Shortcuts")

	var rows []string
	for _, e := range helpEntries {
		key := helpKeyStyle.Render(e.key)
		desc := helpDescStyle.Render(e.desc)
		rows = append(rows, key+" "+desc)
	}

	footer := "\n" + helpDescStyle.Render("Press ? or Esc to close")
	body := title + "\n" + strings.Join(rows, "\n") + footer

	overlay := helpOverlayStyle.Render(body)

	// Centre in terminal
	ow := lipgloss.Width(overlay)
	oh := lipgloss.Height(overlay)
	padLeft := (m.width - ow) / 2
	padTop := (m.height - oh) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	if padTop < 0 {
		padTop = 0
	}

	var lines []string
	for i := 0; i < padTop; i++ {
		lines = append(lines, "")
	}
	for _, line := range strings.Split(overlay, "\n") {
		lines = append(lines, strings.Repeat(" ", padLeft)+line)
	}
	return strings.Join(lines, "\n")
}

func (m *helpViewModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}
