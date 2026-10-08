package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestDHTNamespaceConfiguration(t *testing.T) {
	node := newTestNode(t)
	ctx := context.Background()

	// 1. Default config check
	cfg := DefaultDHTConfig()
	if cfg.ProtocolPrefix != "/skall/kad/1.0.0" {
		t.Errorf("got ProtocolPrefix %s, want /skall/kad/1.0.0", cfg.ProtocolPrefix)
	}
	if cfg.Namespace != "skall.rendezvous.v2" {
		t.Errorf("got Namespace %s, want skall.rendezvous.v2", cfg.Namespace)
	}

	// 2. Client mode
	cfg.Mode = DHTModeClient
	dm, err := NewDHTManager(ctx, node.host, cfg)
	if err != nil {
		t.Fatalf("NewDHTManager client mode failed: %v", err)
	}
	defer dm.Close()

	if dm.Config().Mode != DHTModeClient {
		t.Errorf("got Mode %s, want %s", dm.Config().Mode, DHTModeClient)
	}

	// 3. Server mode
	cfgServer := DefaultDHTConfig()
	cfgServer.Mode = DHTModeServer
	dmServer, err := NewDHTManager(ctx, node.host, cfgServer)
	if err != nil {
		t.Fatalf("NewDHTManager server mode failed: %v", err)
	}
	_ = dmServer.Close()

	// 4. Disabled mode
	cfgDisabled := DefaultDHTConfig()
	cfgDisabled.Mode = DHTModeDisabled
	dmDisabled, err := NewDHTManager(ctx, node.host, cfgDisabled)
	if err != nil {
		t.Fatalf("NewDHTManager disabled mode failed: %v", err)
	}
	if dmDisabled != nil {
		t.Errorf("expected nil DHTManager for disabled mode, got %+v", dmDisabled)
	}

	// 5. Invalid mode
	cfgInvalid := DefaultDHTConfig()
	cfgInvalid.Mode = "invalid_mode"
	_, err = NewDHTManager(ctx, node.host, cfgInvalid)
	if err == nil {
		t.Fatal("expected error for invalid DHT mode, got nil")
	}
}

func TestDHTXORMetricAndRoutingTable(t *testing.T) {
	node := newTestNode(t)
	cfg := DefaultDHTConfig()
	dm, err := NewDHTManager(context.Background(), node.host, cfg)
	if err != nil {
		t.Fatalf("NewDHTManager: %v", err)
	}
	defer dm.Close()

	// Test distance metric identity: dist(a, a) == 0
	distSame := XORKeyDistance("peerA", "peerA")
	if distSame.Sign() != 0 {
		t.Errorf("expected dist(a, a) == 0, got %v", distSame)
	}

	// Symmetry: dist(a, b) == dist(b, a)
	dAB := XORKeyDistance("peerA", "peerB")
	dBA := XORKeyDistance("peerB", "peerA")
	if dAB.Cmp(dBA) != 0 {
		t.Errorf("expected dist(a, b) == dist(b, a), got %v vs %v", dAB, dBA)
	}

	// Routing table operations
	node2 := newTestNode(t)
	node3 := newTestNode(t)

	dm.AddPeer(addrInfo(node2))
	dm.AddPeer(addrInfo(node3))

	if dm.RoutingTableSize() != 2 {
		t.Errorf("RoutingTableSize = %d, want 2", dm.RoutingTableSize())
	}

	closest := dm.ClosestPeers(node.ID(), 1)
	if len(closest) != 1 {
		t.Errorf("ClosestPeers count = %d, want 1", len(closest))
	}

	dm.RemovePeer(node2.ID())
	if dm.RoutingTableSize() != 1 {
		t.Errorf("RoutingTableSize after remove = %d, want 1", dm.RoutingTableSize())
	}
}

func TestDHTDiscoveryBetweenPeers(t *testing.T) {
	nodeA := newTestNode(t)
	nodeB := newTestNode(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect nodes directly in libp2p
	if err := nodeA.Connect(ctx, addrInfo(nodeB)); err != nil {
		t.Fatalf("nodeA.Connect(nodeB): %v", err)
	}

	cfgA := DefaultDHTConfig()
	dmA, err := NewDHTManager(ctx, nodeA.host, cfgA)
	if err != nil {
		t.Fatalf("dmA: %v", err)
	}
	defer dmA.Close()

	cfgB := DefaultDHTConfig()
	dmB, err := NewDHTManager(ctx, nodeB.host, cfgB)
	if err != nil {
		t.Fatalf("dmB: %v", err)
	}
	defer dmB.Close()

	// B advertises in rendezvous namespace
	if err := dmB.Advertise(ctx, DefaultDHTNamespace, 5*time.Minute); err != nil {
		t.Fatalf("dmB.Advertise: %v", err)
	}

	// Give advertisement a moment to settle
	time.Sleep(100 * time.Millisecond)

	// A finds peers in rendezvous namespace
	foundPeers, err := dmA.FindPeers(ctx, DefaultDHTNamespace, 10)
	if err != nil {
		t.Fatalf("dmA.FindPeers: %v", err)
	}

	var foundB bool
	for pi := range foundPeers {
		if pi.ID == nodeB.ID() {
			foundB = true
			break
		}
	}

	if !foundB {
		t.Errorf("dmA did not discover peer B via DHT rendezvous namespace %s", DefaultDHTNamespace)
	}
}

func TestDHTGracefulShutdown(t *testing.T) {
	node := newTestNode(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := DefaultDHTConfig()
	cfg.PollInterval = 50 * time.Millisecond

	dm, err := NewDHTManager(ctx, node.host, cfg)
	if err != nil {
		t.Fatalf("NewDHTManager: %v", err)
	}

	dm.StartDiscovery(ctx, func(pi peer.AddrInfo) {})

	done := make(chan struct{})
	go func() {
		_ = dm.Close()
		close(done)
	}()

	select {
	case <-done:
		// Clean shutdown verified
	case <-time.After(2 * time.Second):
		t.Fatal("DHT Close() timed out")
	}
}
