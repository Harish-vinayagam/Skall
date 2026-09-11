package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Harish-vinayagam/Skall/internal/chat"
)

// chatViewModel owns the right panel: scrollable message history + text input.
type chatViewModel struct {
	title          string // conversation name shown in header
	conversationID string

	messages []chat.DisplayMessage
	vp       viewport.Model
	input    textinput.Model

	focused bool
	width   int
	height  int

	// rebuilt lazily
	contentDirty bool
}

func newChatView() chatViewModel {
	ti := textinput.New()
	ti.Placeholder = "Type a message…"
	ti.CharLimit = 2000

	vp := viewport.New(0, 0)
	vp.SetContent("")

	return chatViewModel{
		input:        ti,
		vp:           vp,
		contentDirty: true,
	}
}

func (m chatViewModel) Init() tea.Cmd { return nil }

func (m chatViewModel) Update(msg tea.Msg) (chatViewModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case MsgNewMessage:
		if msg.ConversationID == m.conversationID {
			m.messages = append(m.messages, msg.Message)
			m.contentDirty = true
		}

	case tea.KeyMsg:
		if !m.focused {
			// Only allow viewport scrolling when not focused on input.
			var vpCmd tea.Cmd
			m.vp, vpCmd = m.vp.Update(msg)
			cmds = append(cmds, vpCmd)
			return m, tea.Batch(cmds...)
		}

		switch msg.String() {
		case "enter":
			if body := strings.TrimSpace(m.input.Value()); body != "" {
				m.input.Reset()
				return m, func() tea.Msg {
					return msgSendRequest{
						conversationID: m.conversationID,
						body:           body,
					}
				}
			}
		case "pgup", "ctrl+b":
			m.vp.HalfViewUp()
		case "pgdown", "ctrl+f":
			m.vp.HalfViewDown()
		default:
			var tiCmd tea.Cmd
			m.input, tiCmd = m.input.Update(msg)
			cmds = append(cmds, tiCmd)
		}

	default:
		// Let viewport handle mouse wheel etc.
		var vpCmd tea.Cmd
		m.vp, vpCmd = m.vp.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m chatViewModel) View() string {
	if m.conversationID == "" {
		placeholder := msgEmptyStyle.Render("Select a conversation from the sidebar\nor press ? for help")
		style := panelStyle
		if m.focused {
			style = panelFocusStyle
		}
		return style.Width(m.width - 2).Height(m.height - 2).Render(placeholder)
	}

	// Header
	header := chatHeaderStyle.Width(m.width - 4).Render("  " + m.title)

	// Viewport content
	if m.contentDirty {
		m.vp.SetContent(m.buildContent())
		m.vp.GotoBottom()
		m.contentDirty = false // Note: View is called on a copy — flag is informational
	}
	vpView := m.vp.View()

	// Input box
	inputStyle := inputBorder
	if m.focused {
		inputStyle = inputBorderFocus
		m.input.Focus()
	} else {
		m.input.Blur()
	}
	prompt := inputPrompt.Render("> ")
	inputBox := inputStyle.Width(m.width - 6).Render(prompt + m.input.View())

	// Separator
	sep := strings.Repeat("─", m.width-4)

	content := header + "\n" + sep + "\n" + vpView + "\n" + sep + "\n" + inputBox

	style := panelStyle
	if m.focused {
		style = panelFocusStyle
	}
	return style.Width(m.width - 2).Height(m.height - 2).Render(content)
}

func (m chatViewModel) buildContent() string {
	if len(m.messages) == 0 {
		return msgEmptyStyle.Render("No messages yet. Say hello!")
	}
	var sb strings.Builder
	for _, msg := range m.messages {
		ts := msgTimestamp.Render(msg.Timestamp.Format("15:04"))
		var sender string
		if msg.IsOutbound {
			sender = msgSenderSelf.Render("You")
		} else {
			sender = msgSenderOther.Render(msg.SenderName)
		}
		body := msgBodyStyle.Render(msg.Body)
		line := fmt.Sprintf("%s  %s: %s", ts, sender, body)
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// LoadConversation replaces the active conversation.
func (m *chatViewModel) LoadConversation(id, title string, messages []chat.DisplayMessage) {
	m.conversationID = id
	m.title = title
	m.messages = messages
	m.contentDirty = true
	m.input.Reset()
	m.rebuildViewport()
	m.vp.GotoBottom()
}

// AppendMessage adds a message and scrolls to bottom.
func (m *chatViewModel) AppendMessage(msg chat.DisplayMessage) {
	m.messages = append(m.messages, msg)
	m.contentDirty = true
	m.vp.SetContent(m.buildContent())
	m.vp.GotoBottom()
}

func (m *chatViewModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.rebuildViewport()
}

func (m *chatViewModel) SetFocused(f bool) {
	m.focused = f
	if f {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

func (m *chatViewModel) rebuildViewport() {
	// viewport height = panel height - borders - header - sep - input - sep
	vpH := m.height - 2 - 2 - 1 - 1 - inputHeight - 1
	if vpH < 1 {
		vpH = 1
	}
	vpW := m.width - 4
	if vpW < 1 {
		vpW = 1
	}
	m.vp.Width = vpW
	m.vp.Height = vpH
	if m.contentDirty {
		m.vp.SetContent(m.buildContent())
		m.vp.GotoBottom()
	}
}

// msgSendRequest is an internal msg used to bubble send requests up to AppModel.
type msgSendRequest struct {
	conversationID string
	body           string
}
