package p2p

import (
	"context"
	"testing"
	"time"
)

func TestBootstrapManagerConfiguration(t *testing.T) {
	nodeA := newTestNode(t)
	nodeB := newTestNode(t)

	addrB := nodeB.Addrs()[0].String() + "/p2p/" + nodeB.ID().String()
	addrSelf := nodeA.Addrs()[0].String() + "/p2p/" + nodeA.ID().String()

	// 1. Valid bootstrap peer including self (self should be filtered)
	bm, err := NewBootstrapManager(nodeA, []string{addrB, addrSelf}, DefaultBackoffConfig())
	if err != nil {
		t.Fatalf("NewBootstrapManager failed: %v", err)
	}
	defer bm.Close()

	peers := bm.Peers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer after filtering self, got %d", len(peers))
	}
	if peers[0].ID != nodeB.ID() {
		t.Errorf("got peer ID %s, want %s", peers[0].ID, nodeB.ID())
	}

	// 2. Invalid bootstrap address should return error
	_, err = NewBootstrapManager(nodeA, []string{"invalid-multiaddr"}, DefaultBackoffConfig())
	if err == nil {
		t.Fatal("expected error for invalid bootstrap address, got nil")
	}
}

func TestBootstrapConnectAll(t *testing.T) {
	nodeA := newTestNode(t)
	nodeB := newTestNode(t) // Live peer
	// Synthetic unreachable peer
	unreachableAddr := "/ip4/127.0.0.1/tcp/1/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN"

	addrB := nodeB.Addrs()[0].String() + "/p2p/" + nodeB.ID().String()

	bm, err := NewBootstrapManager(nodeA, []string{addrB, unreachableAddr}, DefaultBackoffConfig())
	if err != nil {
		t.Fatalf("NewBootstrapManager failed: %v", err)
	}
	defer bm.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	results := bm.ConnectAll(ctx)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	var liveOk, deadFailed bool
	for _, res := range results {
		if res.AddrInfo.ID == nodeB.ID() {
			if res.Err == nil {
				liveOk = true
			} else {
				t.Errorf("expected success for live peer, got: %v", res.Err)
			}
		} else {
			if res.Err != nil {
				deadFailed = true
			} else {
				t.Error("expected failure for unreachable peer, got nil error")
			}
		}
	}

	if !liveOk {
		t.Error("live peer connection did not succeed")
	}
	if !deadFailed {
		t.Error("unreachable peer did not fail gracefully")
	}
}

func TestBootstrapManagerGracefulShutdown(t *testing.T) {
	nodeA := newTestNode(t)
	bm, err := NewBootstrapManager(nodeA, []string{}, DefaultBackoffConfig())
	if err != nil {
		t.Fatalf("NewBootstrapManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	bm.Start(ctx)
	cancel()

	done := make(chan struct{})
	go func() {
		_ = bm.Close()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("bootstrap manager Close() timed out")
	}
}
