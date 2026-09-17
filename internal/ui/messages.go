// Package ui contains the Bubble Tea TUI for SKALL.
//
// Custom tea.Msg types are defined here so that all UI message routing uses
// concrete types rather than bare strings. All messages carry only data visible
// in the chat layer — never raw network or database structures.
package ui

import "github.com/Harish-vinayagam/Skall/internal/chat"

// MsgNewMessage is dispatched when a new message arrives in any conversation.
type MsgNewMessage struct {
	ConversationID string
	Message        chat.DisplayMessage
}

// MsgPeersUpdated is dispatched when the peer list changes (connect/disconnect).
type MsgPeersUpdated struct{}

// MsgConversationsUpdated is dispatched when the conversation list should be refreshed.
type MsgConversationsUpdated struct{}

// MsgStatusChanged is dispatched when the overall connection status changes.
type MsgStatusChanged struct {
	Status    chat.ConnectionStatus
	PeerCount int
}

// MsgError is dispatched when a non-fatal background error occurs.
type MsgError struct {
	Err error
}

// MsgSendResult is dispatched after an attempt to send a message.
type MsgSendResult struct {
	Err error
}
