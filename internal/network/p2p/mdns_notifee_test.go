package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestMdnsNotifeeConnects(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	// Simulate what the mDNS service would call when it finds a peer.
	notifee := &mdnsNotifee{node: a, ctx: context.Background()}
	notifee.HandlePeerFound(addrInfo(b))

	waitCondition(t, 3*time.Second, "a connected to b via notifee",
		func() bool {
			for _, p := range a.ConnectedPeers() {
				if p == b.ID() {
					return true
				}
			}
			return false
		},
	)
}

func TestMdnsNotifeeSkipsSelf(t *testing.T) {
	n := newTestNode(t)

	// Simulate self-discovery. This must never try to connect.
	notifee := &mdnsNotifee{node: n, ctx: context.Background()}
	notifee.HandlePeerFound(addrInfo(n))

	// Give the goroutine time to run.
	time.Sleep(100 * time.Millisecond)

	for _, p := range n.ConnectedPeers() {
		if p == n.ID() {
			t.Fatal("node connected to itself via notifee")
		}
	}
}

func TestMdnsNotifeeIdempotent(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	notifee := &mdnsNotifee{node: a, ctx: context.Background()}

	// Call HandlePeerFound multiple times for the same peer.
	for i := 0; i < 5; i++ {
		notifee.HandlePeerFound(addrInfo(b))
	}

	waitCondition(t, 3*time.Second, "a connected to b",
		func() bool {
			for _, p := range a.ConnectedPeers() {
				if p == b.ID() {
					return true
				}
			}
			return false
		},
	)

	// There should be exactly one connection, not five.
	conns := a.host.Network().ConnsToPeer(b.ID())
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection to b, got %d", len(conns))
	}
}

func TestMdnsNotifeeUnknownPeer(t *testing.T) {
	n := newTestNode(t)
	notifee := &mdnsNotifee{node: n, ctx: context.Background()}

	// A peer with no addresses — connect will fail gracefully (no panic).
	ghost := peer.AddrInfo{ID: "12D3KooWFakePeerID00000000000000000000000000000000000000"}
	notifee.HandlePeerFound(ghost)

	// Give goroutine time to run and fail quietly.
	time.Sleep(200 * time.Millisecond)

	for _, p := range n.ConnectedPeers() {
		if p == ghost.ID {
			t.Fatal("connected to ghost peer with no addresses")
		}
	}
}
