// Package p2p implements the SKALL libp2p networking layer.
//
// Architecture:
//
//	Application
//	    ↓
//	Host interface  (this file)
//	    ↓
//	Node struct     (libp2p implementation)
package p2p

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"

	libhost "github.com/libp2p/go-libp2p/core/host"
	libprotocol "github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

// ProtocolID is the application protocol identifier used on libp2p streams.
const ProtocolID = libprotocol.ID("/skall/msg/1.0.0")

// MessageHandler is called when an inbound message arrives on any stream.
// It is invoked from a goroutine spawned per stream; implementations must be
// goroutine-safe.
type MessageHandler func(msg protocol.Message, from peer.ID)

// Host is the application-facing interface for the SKALL p2p network layer.
// All application code depends only on this interface; the libp2p engine is
// an implementation detail.
type Host interface {
	// ID returns the libp2p peer.ID of this node.
	ID() peer.ID

	// Addrs returns the multiaddresses this node is listening on.
	Addrs() []ma.Multiaddr

	// Connect establishes a connection to the peer described by pi.
	// It is a no-op when already connected.
	Connect(ctx context.Context, pi peer.AddrInfo) error

	// SendMessage opens a stream to peerID, writes msg over it, then closes
	// the write side. One stream is used per message.
	SendMessage(ctx context.Context, peerID peer.ID, msg protocol.Message) error

	// SetMessageHandler registers the callback invoked when an inbound message
	// arrives from any peer. Replaces any previously registered handler.
	SetMessageHandler(h MessageHandler)

	// ConnectedPeers returns the peer.IDs of all currently connected peers.
	ConnectedPeers() []peer.ID

	// Close shuts down the host, all connections, and any discovery services.
	Close() error
}

// Node is the libp2p-backed implementation of Host.
type Node struct {
	host    libhost.Host
	mdnsSvc mdns.Service

	mu      sync.RWMutex
	handler MessageHandler
}

// newNode wraps a libp2p host.Host into a Node and registers the stream handler.
// callers (builder.go) also attach the mDNS service after construction.
func newNode(h libhost.Host) *Node {
	n := &Node{host: h}
	h.SetStreamHandler(ProtocolID, n.handleStream)
	return n
}

// ID returns the libp2p peer.ID of this node.
func (n *Node) ID() peer.ID {
	return n.host.ID()
}

// Addrs returns the multiaddresses this node listens on.
func (n *Node) Addrs() []ma.Multiaddr {
	return n.host.Addrs()
}

// Connect establishes (or verifies) a connection to the peer described by pi.
func (n *Node) Connect(ctx context.Context, pi peer.AddrInfo) error {
	if pi.ID == n.host.ID() {
		return nil // self-connect is a no-op
	}
	return n.host.Connect(ctx, pi)
}

// SendMessage opens a short-lived stream to peerID, writes msg in the SKALL
// length-prefixed JSON framing, then closes the write side.
// One stream per message — keeps stream lifecycle simple in Phase 8.
func (n *Node) SendMessage(ctx context.Context, peerID peer.ID, msg protocol.Message) error {
	if peerID == n.host.ID() {
		return errors.New("cannot send message to self")
	}

	s, err := n.host.NewStream(ctx, peerID, ProtocolID)
	if err != nil {
		return fmt.Errorf("open stream to %s: %w", peerID, err)
	}
	defer func() {
		// Always reset on error; on success the read side stays open briefly
		// so the remote can read before we fully close.
		_ = s.CloseWrite()
	}()

	if err := s.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		_ = s.Reset()
		return fmt.Errorf("set stream deadline: %w", err)
	}

	w := protocol.NewFrameWriter(s)
	if err := w.WriteMessage(msg); err != nil {
		_ = s.Reset()
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

// SetMessageHandler registers the callback for inbound messages.
func (n *Node) SetMessageHandler(h MessageHandler) {
	n.mu.Lock()
	n.handler = h
	n.mu.Unlock()
}

// ConnectedPeers returns the peer.IDs of all currently open connections.
func (n *Node) ConnectedPeers() []peer.ID {
	return n.host.Network().Peers()
}

// Close shuts down the mDNS service (if running) and the libp2p host.
func (n *Node) Close() error {
	if n.mdnsSvc != nil {
		if err := n.mdnsSvc.Close(); err != nil {
			log.Printf("p2p: mDNS close: %v", err)
		}
	}
	return n.host.Close()
}

// handleStream is registered as the libp2p stream handler for ProtocolID.
// It reads exactly one SKALL message per stream.
func (n *Node) handleStream(s network.Stream) {
	defer func() { _ = s.Close() }()

	if err := s.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		log.Printf("p2p: set read deadline: %v", err)
		_ = s.Reset()
		return
	}

	r := protocol.NewFrameReader(s)
	msg, err := r.ReadMessage()
	if err != nil {
		log.Printf("p2p: read message from %s: %v", s.Conn().RemotePeer(), err)
		_ = s.Reset()
		return
	}

	n.mu.RLock()
	h := n.handler
	n.mu.RUnlock()

	if h != nil {
		h(msg, s.Conn().RemotePeer())
	}
}
