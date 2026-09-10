package p2p

import (
	"context"
	"log"

	"github.com/libp2p/go-libp2p/core/peer"
)

// mdnsNotifee implements the libp2p mdns.Notifee interface.
// When mDNS finds a peer on the LAN it calls HandlePeerFound, which connects
// to the peer using the node's libp2p host.
type mdnsNotifee struct {
	node *Node
	ctx  context.Context
}

// HandlePeerFound is called by the libp2p mDNS service each time a peer is
// discovered on the local network.
//
// Rules:
//   - Self-discovery is silently skipped (libp2p's mdns service already
//     filters this, but we guard defensively).
//   - Connect is called in a goroutine so it does not block the mDNS resolver.
//   - Duplicate connections are a no-op because libp2p's host.Connect is
//     idempotent when a connection already exists.
func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == n.node.ID() {
		return
	}
	go func() {
		if err := n.node.Connect(n.ctx, pi); err != nil {
			log.Printf("p2p: mDNS connect to %s: %v", pi.ID, err)
		}
	}()
}
