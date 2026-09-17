package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Harish-vinayagam/Skall/internal/chat"
)

// sidebarTab identifies which list is displayed in the sidebar.
type sidebarTab int

const (
	tabConversations sidebarTab = iota
	tabGroups
	tabPeers
	tabCount // sentinel
)

func (t sidebarTab) String() string {
	switch t {
	case tabConversations:
		return "Chats"
	case tabGroups:
		return "Groups"
	case tabPeers:
		return "Peers"
	}
	return ""
}

// sidebarModel owns the left panel: tab bar + item list.
type sidebarModel struct {
	// data
	conversations []chat.Conversation
	groups        []chat.Group
	peers         []chat.Peer

	// state
	activeTab sidebarTab
	cursor    int  // selected row within the active tab
	focused   bool // true when keyboard focus is on this panel

	width  int
	height int
}

func newSidebar() sidebarModel {
	return sidebarModel{
		activeTab: tabConversations,
		focused:   true,
	}
}

func (m sidebarModel) Init() tea.Cmd { return nil }

func (m sidebarModel) Update(msg tea.Msg) (sidebarModel, tea.Cmd) {
	switch msg := msg.(type) {
	case MsgConversationsUpdated:
		// data is refreshed by the parent AppModel; cursor clamped below
		m.clampCursor()

	case MsgPeersUpdated:
		m.clampCursor()

	case tea.KeyMsg:
		if !m.focused {
			return m, nil
		}
		switch msg.String() {
		case "tab":
			m.activeTab = (m.activeTab + 1) % tabCount
			m.cursor = 0
		case "shift+tab":
			m.activeTab = (m.activeTab + tabCount - 1) % tabCount
			m.cursor = 0
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < m.listLen()-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

// SelectedConversation returns the conversation or nil when a different tab is active.
func (m sidebarModel) SelectedConversation() *chat.Conversation {
	if m.activeTab != tabConversations || len(m.conversations) == 0 {
		return nil
	}
	idx := m.cursor
	if idx >= len(m.conversations) {
		return nil
	}
	c := m.conversations[idx]
	return &c
}

// SelectedGroupID returns the group ID when the groups tab is active.
func (m sidebarModel) SelectedGroupID() string {
	if m.activeTab != tabGroups || len(m.groups) == 0 {
		return ""
	}
	if m.cursor >= len(m.groups) {
		return ""
	}
	return m.groups[m.cursor].GroupID
}

// SelectedPeer returns the peer or nil when a different tab is active.
func (m sidebarModel) SelectedPeer() *chat.Peer {
	if m.activeTab != tabPeers || len(m.peers) == 0 {
		return nil
	}
	if m.cursor >= len(m.peers) {
		return nil
	}
	p := m.peers[m.cursor]
	return &p
}

func (m sidebarModel) View() string {
	// Tab row
	tabs := make([]string, tabCount)
	for i := sidebarTab(0); i < tabCount; i++ {
		label := i.String()
		if i == m.activeTab {
			tabs[i] = sidebarTabActive.Render(label)
		} else {
			tabs[i] = sidebarTabInactive.Render(label)
		}
	}
	tabRow := " " + strings.Join(tabs, sidebarTabInactive.Render(" │ "))

	// Item list
	inner := m.renderList()

	// Available height after tab row + borders
	listHeight := m.height - 2 - 2 // borders top/bottom + tab row
	if listHeight < 0 {
		listHeight = 0
	}

	// Trim to height
	lines := strings.Split(inner, "\n")
	if len(lines) > listHeight {
		lines = lines[:listHeight]
	}
	for len(lines) < listHeight {
		lines = append(lines, "")
	}
	listContent := tabRow + "\n" + strings.Join(lines, "\n")

	style := panelStyle
	if m.focused {
		style = panelFocusStyle
	}
	return style.
		Width(m.width - 2).
		Height(m.height - 2).
		Render(listContent)
}

func (m sidebarModel) renderList() string {
	var sb strings.Builder
	switch m.activeTab {
	case tabConversations:
		if len(m.conversations) == 0 {
			sb.WriteString(sidebarItemMuted.Render("(no conversations)"))
		}
		for i, c := range m.conversations {
			name := truncate(c.Name, m.width-5)
			var line string
			if c.Type == chat.ConversationGroup {
				name = "# " + name
			}
			if i == m.cursor {
				line = sidebarItemSelected.Render("> " + name)
			} else {
				line = sidebarItemStyle.Render("  " + name)
			}
			sb.WriteString(line + "\n")
		}
	case tabGroups:
		if len(m.groups) == 0 {
			sb.WriteString(sidebarItemMuted.Render("(no groups)"))
		}
		for i, g := range m.groups {
			name := truncate("# "+g.Name, m.width-5)
			members := fmt.Sprintf(" [%d]", g.MemberCount)
			var line string
			if i == m.cursor {
				line = sidebarItemSelected.Render("> " + name + members)
			} else {
				line = sidebarItemStyle.Render("  " + name + members)
			}
			sb.WriteString(line + "\n")
		}
	case tabPeers:
		if len(m.peers) == 0 {
			sb.WriteString(sidebarItemMuted.Render("(no peers)"))
		}
		for i, p := range m.peers {
			name := truncate(p.DisplayName, m.width-6)
			dot := " "
			if p.Connected {
				dot = sidebarDot.Render("●")
			}
			var line string
			if i == m.cursor {
				line = sidebarItemSelected.Render(dot + " " + name)
			} else {
				line = sidebarItemStyle.Render(dot + " " + name)
			}
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

func (m *sidebarModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *sidebarModel) SetFocused(f bool) { m.focused = f }

func (m sidebarModel) listLen() int {
	switch m.activeTab {
	case tabConversations:
		return len(m.conversations)
	case tabGroups:
		return len(m.groups)
	case tabPeers:
		return len(m.peers)
	}
	return 0
}

func (m *sidebarModel) clampCursor() {
	if l := m.listLen(); m.cursor >= l && l > 0 {
		m.cursor = l - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// truncate shortens s to at most maxLen runes, appending "…" if needed.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

// renderSidebarWidth is the rendered width of the sidebar (including borders).
func renderSidebarWidth() int {
	return lipgloss.Width(panelStyle.Width(sidebarWidth - 2).Render(""))
}
