package discovery

import (
	"context"
	"log"
)

// ConnectionManager is the narrow interface the Connector uses to manage peer
// connections. It is satisfied by *connection.Manager and can be faked in tests.
type ConnectionManager interface {
	// Connect initiates a connection to the peer at endpoint. It is a no-op if
	// the peer is already connecting or connected.
	Connect(ctx context.Context, peerID, endpoint string) error
	// Disconnect tears down the connection to peerID and removes it from the
	// manager's tracking state.
	Disconnect(peerID string) error
}

// Connector bridges a Discovery event stream to a ConnectionManager.
//
//	Discovery (events) ──► Connector ──► ConnectionManager
//
// On EventPeerAdded  → ConnectionManager.Connect is called.
// On EventPeerRemoved → ConnectionManager.Disconnect is called.
//
// Invalid PeerInfo is logged and silently skipped. The ConnectionManager is
// responsible for de-duplicating in-flight or established connections.
type Connector struct {
	mgr ConnectionManager
}

// NewConnector creates a Connector that drives mgr in response to discovery
// events. mgr must not be nil.
func NewConnector(mgr ConnectionManager) *Connector {
	if mgr == nil {
		panic("discovery.NewConnector: mgr must not be nil")
	}
	return &Connector{mgr: mgr}
}

// Run reads events from the supplied channel and drives mgr accordingly.
// It blocks until ctx is cancelled or events is closed, then returns ctx.Err()
// (or nil if events was closed).
//
// Run is safe to call from a single goroutine. Spawn a goroutine if you need
// it to run concurrently:
//
//	go connector.Run(ctx, disc.Events())
func (c *Connector) Run(ctx context.Context, events <-chan Event) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				// Discovery channel closed — nothing more to process.
				return nil
			}
			c.handle(ctx, ev)
		}
	}
}

func (c *Connector) handle(ctx context.Context, ev Event) {
	if !ev.Peer.IsValid() {
		log.Printf("discovery: skipping invalid peer info in event %v: %+v", ev.Type, ev.Peer)
		return
	}

	switch ev.Type {
	case EventPeerAdded:
		if err := c.mgr.Connect(ctx, ev.Peer.PeerID, ev.Peer.Endpoint()); err != nil {
			log.Printf("discovery: connect to peer %s (%s): %v", ev.Peer.PeerID, ev.Peer.Endpoint(), err)
		}
	case EventPeerRemoved:
		if err := c.mgr.Disconnect(ev.Peer.PeerID); err != nil {
			log.Printf("discovery: disconnect peer %s: %v", ev.Peer.PeerID, err)
		}
	default:
		log.Printf("discovery: unknown event type %v", ev.Type)
	}
}
