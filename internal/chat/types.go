// Package chat provides the application service layer that sits between the
// Bubble Tea TUI and the lower-level networking / storage packages.
//
// The types defined here are pure data structures with no dependency on
// libp2p, SQLite, or any UI framework. They are the lingua franca shared
// between ChatService and the UI models.
package chat

import "time"

// ConversationType distinguishes between a one-on-one conversation and a group chat.
type ConversationType int

const (
	ConversationDirect ConversationType = iota + 1
	ConversationGroup
)

// Conversation is a summary entry shown in the sidebar list.
type Conversation struct {
	ID          string           // PeerID for direct, GroupID for group
	Name        string           // display name or group name
	Type        ConversationType // direct or group
	LastMessage string           // body of most recent message, for preview
	LastAt      time.Time        // timestamp of most recent message
	Unread      int              // unread message count (best-effort)
}

// DisplayMessage is a single chat message formatted for the TUI.
type DisplayMessage struct {
	ID         string
	SenderID   string
	SenderName string // resolved display name
	Body       string
	Timestamp  time.Time
	IsOutbound bool // true when sent by the local identity
}

// Peer represents a known remote peer.
type Peer struct {
	PeerID      string
	DisplayName string
	Connected   bool // true when currently connected via p2p
	LastSeen    time.Time
}

// Group is a chat group summary for the sidebar.
type Group struct {
	GroupID     string
	Name        string
	MemberCount int
	CreatedAt   time.Time
}

// ConnectionStatus describes the overall network state.
type ConnectionStatus int

const (
	StatusDisconnected ConnectionStatus = iota
	StatusConnecting
	StatusConnected
)

// String returns a short human-readable label.
func (s ConnectionStatus) String() string {
	switch s {
	case StatusConnecting:
		return "Connecting"
	case StatusConnected:
		return "Connected"
	default:
		return "Disconnected"
	}
}
