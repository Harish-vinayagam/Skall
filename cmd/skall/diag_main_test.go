package main

import (
	"context"
	"os"
	"testing"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/Harish-vinayagam/Skall/internal/chat"
	"github.com/Harish-vinayagam/Skall/internal/groups"
	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/network/p2p"
	"github.com/Harish-vinayagam/Skall/internal/storage"
)

func TestDirectPeerDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Generate two identities
	idA, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate idA: %v", err)
	}
	idB, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate idB: %v", err)
	}

	// Build two nodes (no mDNS — direct connect)
	nodeA, err := p2p.BuildNodeNoMDNS(ctx, idA, []string{"/ip4/127.0.0.1/tcp/0"})
	if err != nil {
		t.Fatalf("build A: %v", err)
	}
	nodeB, err := p2p.BuildNodeNoMDNS(ctx, idB, []string{"/ip4/127.0.0.1/tcp/0"})
	if err != nil {
		t.Fatalf("build B: %v", err)
	}
	defer nodeA.Close()
	defer nodeB.Close()

	// Connect A → B directly
	addrInfoB := libp2ppeer.AddrInfo{ID: nodeB.ID(), Addrs: nodeB.Addrs()}
	if err := nodeA.Connect(ctx, addrInfoB); err != nil {
		t.Fatalf("connect error: %v", err)
	}

	// Open stores
	dbPathA := t.TempDir() + "/diag-a.db"
	dbPathB := t.TempDir() + "/diag-b.db"
	dbA, err := storage.Open(dbPathA)
	if err != nil {
		t.Fatalf("open dbA: %v", err)
	}
	dbB, err := storage.Open(dbPathB)
	if err != nil {
		t.Fatalf("open dbB: %v", err)
	}
	defer dbA.Close()
	defer dbB.Close()

	// Start services
	svcA := chat.NewP2PService(ctx, idA, nodeA, dbA, groups.NewManager())
	svcB := chat.NewP2PService(ctx, idB, nodeB, dbB, groups.NewManager())
	defer svcA.Close()
	defer svcB.Close()

	// Wait for monitorPeers to tick (ticks every 2s)
	time.Sleep(3 * time.Second)

	peersA, err := svcA.ListPeers()
	if err != nil {
		t.Fatalf("ListPeers A err: %v", err)
	}
	if len(peersA) == 0 || !peersA[0].Connected {
		t.Fatalf("expected connected peer in A, got %+v", peersA)
	}

	peersB, err := svcB.ListPeers()
	if err != nil {
		t.Fatalf("ListPeers B err: %v", err)
	}
	if len(peersB) == 0 || !peersB[0].Connected {
		t.Fatalf("expected connected peer in B, got %+v", peersB)
	}
	_ = os.Remove(dbPathA)
	_ = os.Remove(dbPathB)
}

