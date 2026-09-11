package chat

// EventKind identifies the category of a service event.
type EventKind int

const (
	// EventNewMessage fires when a new inbound or outbound message is stored.
	EventNewMessage EventKind = iota + 1
	// EventPeerConnected fires when a remote peer connects.
	EventPeerConnected
	// EventPeerDisconnected fires when a remote peer disconnects.
	EventPeerDisconnected
	// EventStatusChanged fires when the overall ConnectionStatus changes.
	EventStatusChanged
	// EventError fires when a non-fatal background error occurs.
	EventError
)

// Event is a value pushed by the ChatService to its subscribers.
// Subscribers receive events via the channel returned by Subscribe.
type Event struct {
	Kind EventKind

	// ConversationID is set for EventNewMessage — it is the PeerID (direct)
	// or GroupID (group) that identifies which conversation was updated.
	ConversationID string

	// Message is set for EventNewMessage.
	Message DisplayMessage

	// Peer is set for EventPeerConnected / EventPeerDisconnected.
	Peer Peer

	// Status is set for EventStatusChanged.
	Status ConnectionStatus

	// Err is set for EventError.
	Err error
}
