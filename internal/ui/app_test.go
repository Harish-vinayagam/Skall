package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Harish-vinayagam/Skall/internal/chat"
	"github.com/Harish-vinayagam/Skall/internal/identity"
)

// --- mock ChatService ---

type mockService struct {
	local         identity.Identity
	conversations []chat.Conversation
	groups        []chat.Group
	peers         []chat.Peer
	messages      map[string][]chat.DisplayMessage
	status        chat.ConnectionStatus
	sendDirectErr error
	sendGroupErr  error
	subCh         chan chat.Event
}

func newMockService() *mockService {
	id, _ := identity.Generate()
	return &mockService{
		local:    id,
		messages: make(map[string][]chat.DisplayMessage),
		status:   chat.StatusConnected,
		subCh:    make(chan chat.Event, 16),
	}
}

func (m *mockService) LocalIdentity() identity.Identity        { return m.local }
func (m *mockService) ConnectionStatus() chat.ConnectionStatus { return m.status }
func (m *mockService) ConnectedPeerCount() int                 { return len(m.peers) }
func (m *mockService) ListConversations() ([]chat.Conversation, error) {
	return m.conversations, nil
}
func (m *mockService) ListGroups() ([]chat.Group, error) { return m.groups, nil }
func (m *mockService) ListPeers() ([]chat.Peer, error)   { return m.peers, nil }
func (m *mockService) GetMessages(id string, _ int) ([]chat.DisplayMessage, error) {
	return m.messages[id], nil
}
func (m *mockService) SendDirect(_, _ string) error  { return m.sendDirectErr }
func (m *mockService) SendGroup(_, _ string) error   { return m.sendGroupErr }
func (m *mockService) CreateGroup(_, _ string) error { return nil }
func (m *mockService) JoinGroup(_, _ string) error   { return nil }
func (m *mockService) LeaveGroup(_, _ string) error  { return nil }
func (m *mockService) DeleteGroup(_ string) error    { return nil }
func (m *mockService) Subscribe() <-chan chat.Event  { return m.subCh }
func (m *mockService) Close() error                  { close(m.subCh); return nil }

// helper: build an AppModel and fire Init commands synchronously (no real I/O).
func buildApp(t *testing.T, svc chat.ChatService) AppModel {
	t.Helper()
	m := New(svc)
	// Provide a terminal size so layout() doesn't divide by zero
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(AppModel)
}

// sendKey simulates a key press.
func sendKey(m AppModel, key string) AppModel {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(AppModel)
}

func sendSpecialKey(m AppModel, t tea.KeyType) AppModel {
	updated, _ := m.Update(tea.KeyMsg{Type: t})
	return updated.(AppModel)
}

// --- Tests ---

func TestInitialFocusIsSidebar(t *testing.T) {
	m := buildApp(t, newMockService())
	if m.focus != focusSidebar {
		t.Fatalf("expected initial focus=sidebar, got %v", m.focus)
	}
}

func TestHelpToggleWithQuestionMark(t *testing.T) {
	m := buildApp(t, newMockService())
	if m.showHelp {
		t.Fatal("help should be hidden initially")
	}
	m = sendKey(m, "?")
	if !m.showHelp {
		t.Fatal("help should be visible after pressing ?")
	}
	m = sendKey(m, "?")
	if m.showHelp {
		t.Fatal("help should be hidden after pressing ? again")
	}
}

func TestEscClosesHelp(t *testing.T) {
	m := buildApp(t, newMockService())
	m = sendKey(m, "?")
	if !m.showHelp {
		t.Fatal("help should be visible")
	}
	m = sendSpecialKey(m, tea.KeyEsc)
	if m.showHelp {
		t.Fatal("Esc should close help")
	}
}

func TestFocusSwitchRightLeft(t *testing.T) {
	m := buildApp(t, newMockService())
	// Start at sidebar
	if m.focus != focusSidebar {
		t.Fatal("expected sidebar focus")
	}
	// → moves to chat
	m = sendKey(m, "l")
	if m.focus != focusChat {
		t.Fatalf("expected chat focus after l, got %v", m.focus)
	}
	// ← moves back to sidebar
	m = sendKey(m, "h")
	if m.focus != focusSidebar {
		t.Fatalf("expected sidebar focus after h, got %v", m.focus)
	}
}

func TestArrowRightSwitchesFocusToChat(t *testing.T) {
	m := buildApp(t, newMockService())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(AppModel)
	if m.focus != focusChat {
		t.Fatalf("expected chat focus after right arrow, got %v", m.focus)
	}
}

func TestSidebarTabCycleWithTab(t *testing.T) {
	m := buildApp(t, newMockService())
	// Tab cycles through sidebar tabs
	initial := m.sidebar.activeTab
	m = sendSpecialKey(m, tea.KeyTab)
	next := m.sidebar.activeTab
	if next == initial {
		t.Fatal("Tab should change sidebar tab")
	}
	// Cycle back
	m = sendSpecialKey(m, tea.KeyTab)
	m = sendSpecialKey(m, tea.KeyTab)
	if m.sidebar.activeTab != initial {
		t.Fatal("Three Tabs should return to initial tab")
	}
}

func TestMsgStatusChangedUpdatesStatusBar(t *testing.T) {
	m := buildApp(t, newMockService())
	updated, _ := m.Update(MsgStatusChanged{Status: chat.StatusDisconnected, PeerCount: 0})
	m = updated.(AppModel)
	if m.statusBar.status != chat.StatusDisconnected {
		t.Fatalf("status bar should reflect disconnected status")
	}
}

func TestMsgErrorDisplayedInStatusBar(t *testing.T) {
	m := buildApp(t, newMockService())
	updated, _ := m.Update(MsgError{Err: errors.New("test error")})
	m = updated.(AppModel)
	if m.statusBar.errMsg == "" {
		t.Fatal("status bar should show error message")
	}
}

func TestConversationsLoadedOnUpdate(t *testing.T) {
	svc := newMockService()
	svc.conversations = []chat.Conversation{
		{ID: "peer-1", Name: "Alice", Type: chat.ConversationDirect},
		{ID: "peer-2", Name: "Bob", Type: chat.ConversationDirect},
	}
	m := buildApp(t, svc)
	// Trigger conversation refresh
	updated, _ := m.Update(MsgConversationsUpdated{})
	m = updated.(AppModel)
	if len(m.sidebar.conversations) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(m.sidebar.conversations))
	}
}

func TestWindowResizeUpdatesLayout(t *testing.T) {
	m := buildApp(t, newMockService())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = updated.(AppModel)
	if m.width != 200 || m.height != 50 {
		t.Fatalf("expected 200×50, got %d×%d", m.width, m.height)
	}
}

func TestViewDoesNotPanic(t *testing.T) {
	m := buildApp(t, newMockService())
	// Should not panic even with no conversations or messages
	_ = m.View()
	// With help open
	m = sendKey(m, "?")
	_ = m.View()
}

func TestSidebarNavigationJK(t *testing.T) {
	svc := newMockService()
	svc.conversations = []chat.Conversation{
		{ID: "a", Name: "Alice", Type: chat.ConversationDirect},
		{ID: "b", Name: "Bob", Type: chat.ConversationDirect},
	}
	m := buildApp(t, svc)
	updated, _ := m.Update(MsgConversationsUpdated{})
	m = updated.(AppModel)

	if m.sidebar.cursor != 0 {
		t.Fatalf("initial cursor should be 0, got %d", m.sidebar.cursor)
	}
	m = sendKey(m, "j")
	if m.sidebar.cursor != 1 {
		t.Fatalf("j should move cursor to 1, got %d", m.sidebar.cursor)
	}
	m = sendKey(m, "j") // should not exceed list
	if m.sidebar.cursor != 1 {
		t.Fatalf("j at end should not exceed 1, got %d", m.sidebar.cursor)
	}
	m = sendKey(m, "k")
	if m.sidebar.cursor != 0 {
		t.Fatalf("k should move cursor to 0, got %d", m.sidebar.cursor)
	}
}

func TestSelectPeerFromPeersTab(t *testing.T) {
	svc := newMockService()
	svc.peers = []chat.Peer{
		{PeerID: "peer-1", DisplayName: "Alice", Connected: true},
	}
	m := buildApp(t, svc)
	updated, _ := m.Update(MsgPeersUpdated{})
	m = updated.(AppModel)

	// Switch to Peers tab (tab from Chats -> Groups -> Peers)
	m = sendSpecialKey(m, tea.KeyTab)
	m = sendSpecialKey(m, tea.KeyTab)
	if m.sidebar.activeTab != tabPeers {
		t.Fatalf("expected activeTab=tabPeers, got %v", m.sidebar.activeTab)
	}

	// Press Enter to open chat with peer
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(AppModel)

	if m.focus != focusChat {
		t.Fatalf("expected focus=focusChat after pressing Enter on peer, got %v", m.focus)
	}
	if m.chatView.conversationID != "peer-1" {
		t.Fatalf("expected chatView.conversationID='peer-1', got %q", m.chatView.conversationID)
	}
}
