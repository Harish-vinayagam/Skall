// Package chat defines the application service interface between the Bubble Tea
// TUI and the lower-level networking / storage packages.
//
// The UI depends solely on ChatService. Concrete implementations (P2PService)
// are wired up in cmd/skall/main.go, keeping UI code free of network details.
package chat

import "github.com/Harish-vinayagam/Skall/internal/identity"

// ChatService is the single point of contact between the TUI and the application.
// All methods are safe to call concurrently.
type ChatService interface {
	// LocalIdentity returns the identity of the running node.
	LocalIdentity() identity.Identity

	// ConnectionStatus returns the current network status.
	ConnectionStatus() ConnectionStatus

	// ConnectedPeerCount returns the number of currently connected p2p peers.
	ConnectedPeerCount() int

	// ListConversations returns a summary of all known conversations (direct +
	// group), ordered newest-last-message first.
	ListConversations() ([]Conversation, error)

	// ListGroups returns all known groups.
	ListGroups() ([]Group, error)

	// ListPeers returns all known remote peers (connected or not).
	ListPeers() ([]Peer, error)

	// GetMessages fetches the last `limit` messages for a conversation.
	// conversationID is a PeerID (direct) or GroupID (group).
	GetMessages(conversationID string, limit int) ([]DisplayMessage, error)

	// SendDirect sends a direct message to the peer identified by peerID.
	SendDirect(peerID, body string) error

	// SendGroup sends a message to the group identified by groupID.
	SendGroup(groupID, body string) error

	// Subscribe returns a channel on which the caller receives real-time
	// Events. The channel is buffered. Each call returns a new channel;
	// the caller must read continuously to avoid blocking the service.
	Subscribe() <-chan Event

	// Close shuts down the service and all underlying resources.
	Close() error
}
