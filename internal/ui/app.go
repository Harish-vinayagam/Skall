// Package ui implements the Bubble Tea terminal interface for SKALL.
//
// AppModel is the root model. It:
//   - owns all sub-models (statusBar, sidebar, chatView, helpView)
//   - delegates keyboard events to the focused sub-model
//   - listens to the ChatService event channel and converts events to tea.Msg
//   - calls ChatService for data refresh and message sending
//
// The UI never touches TCP, libp2p, or SQLite directly.
package ui

import (
	"context"
	"fmt"
	"log"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Harish-vinayagam/Skall/internal/chat"
)

// focusRegion tracks which panel has keyboard focus.
type focusRegion int

const (
	focusSidebar focusRegion = iota
	focusChat
)

// AppModel is the root Bubble Tea model.
type AppModel struct {
	svc    chat.ChatService
	ctx    context.Context
	cancel context.CancelFunc

	// sub-models
	statusBar statusBarModel
	sidebar   sidebarModel
	chatView  chatViewModel
	helpView  helpViewModel

	focus    focusRegion
	showHelp bool
	quitting bool

	// terminal dimensions
	width  int
	height int

	// last error (shown in status bar transiently)
	lastErr error
}

// New creates an AppModel. The caller must call Close when done.
func New(svc chat.ChatService) AppModel {
	ctx, cancel := context.WithCancel(context.Background())
	m := AppModel{
		svc:       svc,
		ctx:       ctx,
		cancel:    cancel,
		statusBar: newStatusBar(),
		sidebar:   newSidebar(),
		chatView:  newChatView(),
		helpView:  newHelpView(),
		focus:     focusSidebar,
	}
	m.sidebar.SetFocused(true)
	m.chatView.SetFocused(false)
	return m
}

// Init starts background listeners.
func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.listenEvents(),
		m.refreshAll(),
		viewport.Sync(m.chatView.vp),
	)
}

// Update is the Bubble Tea update function.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// ---- Terminal resize ----
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	// ---- Quit ----
	case tea.QuitMsg:
		m.quitting = true
		m.cancel()
		return m, tea.Quit

	// ---- Service events → tea.Msg ----
	case MsgStatusChanged:
		var sbCmd tea.Cmd
		m.statusBar, sbCmd = m.statusBar.Update(msg)
		cmds = append(cmds, sbCmd)
		return m, tea.Batch(cmds...)

	case MsgError:
		var sbCmd tea.Cmd
		m.statusBar, sbCmd = m.statusBar.Update(msg)
		cmds = append(cmds, sbCmd)
		return m, tea.Batch(cmds...)

	case MsgPeersUpdated:
		peers, err := m.svc.ListPeers()
		if err != nil {
			log.Printf("ui: list peers: %v", err)
		} else {
			m.sidebar.peers = peers
		}
		return m, nil

	case MsgConversationsUpdated:
		convs, err := m.svc.ListConversations()
		if err != nil {
			log.Printf("ui: list conversations: %v", err)
		} else {
			m.sidebar.conversations = convs
		}
		grps, err := m.svc.ListGroups()
		if err != nil {
			log.Printf("ui: list groups: %v", err)
		} else {
			m.sidebar.groups = grps
		}
		return m, nil

	case MsgNewMessage:
		// Refresh conversation list (last-message preview update)
		cmds = append(cmds, func() tea.Msg { return MsgConversationsUpdated{} })
		// If this message belongs to the active conversation, append it
		if msg.ConversationID == m.chatView.conversationID {
			m.chatView.AppendMessage(msg.Message)
		}
		return m, tea.Batch(cmds...)

	// ---- Internal send request ----
	case msgSendRequest:
		cmds = append(cmds, m.doSend(msg.conversationID, msg.body))
		return m, tea.Batch(cmds...)

	case MsgSendResult:
		if msg.Err != nil {
			var sbCmd tea.Cmd
			m.statusBar, sbCmd = m.statusBar.Update(MsgError{Err: msg.Err})
			cmds = append(cmds, sbCmd)
		}
		return m, tea.Batch(cmds...)

	// ---- Event channel ping ----
	case eventChannelMsg:
		cmds = append(cmds, m.handleServiceEvent(msg.ev))
		cmds = append(cmds, m.listenEvents()) // keep listening
		return m, tea.Batch(cmds...)

	// ---- Keyboard ----
	case tea.KeyMsg:
		// Global shortcuts first
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "q":
			if m.focus != focusChat || !m.chatView.input.Focused() {
				m.cancel()
				return m, tea.Quit
			}
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
		}

		if m.showHelp {
			return m, nil
		}

		// Focus switching
		switch msg.String() {
		case "right", "l":
			m.setFocus(focusChat)
			return m, nil
		case "left", "h":
			m.setFocus(focusSidebar)
			return m, nil
		case "enter":
			if m.focus == focusSidebar {
				cmds = append(cmds, m.openSelected())
				return m, tea.Batch(cmds...)
			}
		}

		// Delegate to focused sub-model
		switch m.focus {
		case focusSidebar:
			var sbCmd tea.Cmd
			m.sidebar, sbCmd = m.sidebar.Update(msg)
			cmds = append(cmds, sbCmd)
		case focusChat:
			var cvCmd tea.Cmd
			m.chatView, cvCmd = m.chatView.Update(msg)
			cmds = append(cmds, cvCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the full TUI.
func (m AppModel) View() string {
	if m.quitting {
		return ""
	}

	if m.showHelp {
		return m.helpView.View()
	}

	statusBar := m.statusBar.View()

	// Side-by-side layout
	sidebarView := m.sidebar.View()
	chatView := m.chatView.View()

	columns := lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, chatView)
	return lipgloss.JoinVertical(lipgloss.Left, statusBar, columns)
}

// --- layout ---

func (m *AppModel) layout() {
	sw := sidebarWidth
	cw := m.width - sw
	if cw < minChatWidth {
		cw = minChatWidth
		sw = m.width - cw
	}
	panelH := m.height - statusHeight

	m.statusBar.SetWidth(m.width)
	m.sidebar.SetSize(sw, panelH)
	m.chatView.SetSize(cw, panelH)
	m.helpView.SetSize(m.width, m.height)
}

func (m *AppModel) setFocus(r focusRegion) {
	m.focus = r
	m.sidebar.SetFocused(r == focusSidebar)
	m.chatView.SetFocused(r == focusChat)
}

// openSelected loads the currently selected sidebar item into the chat view.
func (m *AppModel) openSelected() tea.Cmd {
	if conv := m.sidebar.SelectedConversation(); conv != nil {
		msgs, err := m.svc.GetMessages(conv.ID, 100)
		if err != nil {
			log.Printf("ui: load messages: %v", err)
		}
		m.chatView.LoadConversation(conv.ID, conv.Name, msgs)
		m.setFocus(focusChat)
		return nil
	}
	if gID := m.sidebar.SelectedGroupID(); gID != "" {
		msgs, err := m.svc.GetMessages(gID, 100)
		if err != nil {
			log.Printf("ui: load group messages: %v", err)
		}
		m.chatView.LoadConversation(gID, gID, msgs)
		m.setFocus(focusChat)
		return nil
	}
	if peer := m.sidebar.SelectedPeer(); peer != nil {
		msgs, err := m.svc.GetMessages(peer.PeerID, 100)
		if err != nil {
			log.Printf("ui: load peer messages: %v", err)
		}
		m.chatView.LoadConversation(peer.PeerID, peer.DisplayName, msgs)
		m.setFocus(focusChat)
		return nil
	}
	return nil
}

// --- background listener ---

// eventChannelMsg wraps a service event for delivery to the Update loop.
type eventChannelMsg struct{ ev chat.Event }

// listenEvents returns a Cmd that blocks until one event arrives on the
// subscription channel, then returns it as an eventChannelMsg.
func (m AppModel) listenEvents() tea.Cmd {
	ch := m.svc.Subscribe()
	return func() tea.Msg {
		select {
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			return eventChannelMsg{ev}
		case <-m.ctx.Done():
			return nil
		}
	}
}

// handleServiceEvent converts a chat.Event into zero or one tea.Cmd.
func (m *AppModel) handleServiceEvent(ev chat.Event) tea.Cmd {
	switch ev.Kind {
	case chat.EventNewMessage:
		return func() tea.Msg {
			return MsgNewMessage{
				ConversationID: ev.ConversationID,
				Message:        ev.Message,
			}
		}
	case chat.EventPeerConnected, chat.EventPeerDisconnected:
		return func() tea.Msg { return MsgPeersUpdated{} }
	case chat.EventStatusChanged:
		status := ev.Status
		count := m.svc.ConnectedPeerCount()
		return func() tea.Msg {
			return MsgStatusChanged{Status: status, PeerCount: count}
		}
	case chat.EventError:
		err := ev.Err
		return func() tea.Msg { return MsgError{Err: err} }
	}
	return nil
}

// refreshAll performs an initial data load.
func (m AppModel) refreshAll() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return MsgConversationsUpdated{} },
		func() tea.Msg { return MsgPeersUpdated{} },
	)
}

// doSend calls the ChatService to send a message.
func (m AppModel) doSend(conversationID, body string) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		// Determine if it's a direct or group send
		grps, _ := svc.ListGroups()
		for _, g := range grps {
			if g.GroupID == conversationID {
				err := svc.SendGroup(conversationID, body)
				return MsgSendResult{Err: err}
			}
		}
		err := svc.SendDirect(conversationID, body)
		if err != nil {
			return MsgSendResult{Err: fmt.Errorf("send: %w", err)}
		}
		return MsgSendResult{}
	}
}
