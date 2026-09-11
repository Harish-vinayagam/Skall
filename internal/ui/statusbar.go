package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Harish-vinayagam/Skall/internal/chat"
)

// statusBarModel renders the top strip: app name + connection status + peer count.
type statusBarModel struct {
	status    chat.ConnectionStatus
	peerCount int
	width     int
	errMsg    string // transient error to display
}

func newStatusBar() statusBarModel {
	return statusBarModel{status: chat.StatusConnecting}
}

func (m statusBarModel) Init() tea.Cmd { return nil }

func (m statusBarModel) Update(msg tea.Msg) (statusBarModel, tea.Cmd) {
	switch msg := msg.(type) {
	case MsgStatusChanged:
		m.status = msg.Status
		m.peerCount = msg.PeerCount
	case MsgError:
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
		} else {
			m.errMsg = ""
		}
	}
	return m, nil
}

func (m statusBarModel) View() string {
	dot := statusDotPending.String()
	statusLabel := "Connecting"
	switch m.status {
	case chat.StatusConnected:
		dot = statusDotOnline.String()
		statusLabel = "Connected"
	case chat.StatusDisconnected:
		dot = statusDotOffline.String()
		statusLabel = "Disconnected"
	}

	left := statusAppName.String() + "  " + dot + "  " + statusText.Render(statusLabel)
	right := statusText.Render(fmt.Sprintf("%d peer(s)", m.peerCount))

	if m.errMsg != "" {
		right = errorBannerStyle.Render(" ⚠ " + m.errMsg)
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	bar := left + strings.Repeat(" ", gap) + right
	return statusBarStyle.Width(m.width).Render(bar)
}

func (m *statusBarModel) SetWidth(w int) { m.width = w }
