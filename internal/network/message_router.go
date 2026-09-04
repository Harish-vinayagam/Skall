package network

import (
	"errors"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

var (
	ErrSenderNotIdentified = errors.New("sender is not identified")
	ErrSenderMismatch      = errors.New("sender identity mismatch")
	ErrMissingRecipient    = errors.New("missing recipient")
	ErrUnknownPeer         = errors.New("unknown peer")
	ErrPeerDisconnected    = errors.New("peer disconnected")
)

type MessageRouter struct {
	registry *PeerRegistry
}

func NewMessageRouter(registry *PeerRegistry) *MessageRouter {
	return &MessageRouter{registry: registry}
}

func (r *MessageRouter) RouteDirect(sender peerConnection, message protocol.Message) error {
	senderPeerID, ok := r.registry.PeerID(sender)
	if !ok {
		return ErrSenderNotIdentified
	}
	if message.SenderID != senderPeerID {
		return ErrSenderMismatch
	}
	if message.RecipientID == "" {
		return ErrMissingRecipient
	}

	recipient, ok := r.registry.Lookup(message.RecipientID)
	if !ok {
		return ErrUnknownPeer
	}
	if !recipient.enqueue(message) {
		r.registry.Unregister(recipient)
		return ErrPeerDisconnected
	}
	return nil
}
