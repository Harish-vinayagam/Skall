package p2p

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

// --- helpers ---

// newTestNode creates a libp2p Node wired to a fresh generated identity,
// listening on a random loopback port. mDNS is disabled so tests are isolated.
func newTestNode(t *testing.T) *Node {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("identity.Generate: %v", err)
	}
	n, err := BuildNodeNoMDNS(context.Background(), id, []string{"/ip4/127.0.0.1/tcp/0"})
	if err != nil {
		t.Fatalf("BuildNodeNoMDNS: %v", err)
	}
	t.Cleanup(func() { _ = n.Close() })
	return n
}

// addrInfo returns a peer.AddrInfo for n that another node can use to connect.
func addrInfo(n *Node) peer.AddrInfo {
	return peer.AddrInfo{ID: n.ID(), Addrs: n.Addrs()}
}

// waitCondition polls cond every 5 ms until it returns true or timeout expires.
func waitCondition(t *testing.T, timeout time.Duration, label string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition %q not met within %v", label, timeout)
}

// testMessage creates a minimal valid protocol.Message for testing.
func testMessage(senderID, body string) protocol.Message {
	return protocol.NewChatMessage(senderID, "", "", body)
}

// --- host creation tests ---

func TestBuildNodeFromIdentity(t *testing.T) {
	n := newTestNode(t)

	if n.ID() == "" {
		t.Fatal("node ID is empty")
	}
	if len(n.Addrs()) == 0 {
		t.Fatal("node has no listen addresses")
	}
}

func TestPeerIdentityConsistency(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Derive expected libp2p peer.ID from the identity.
	expectedPID, err := id.LibP2PPeerID()
	if err != nil {
		t.Fatalf("LibP2PPeerID: %v", err)
	}

	n, err := BuildNodeNoMDNS(context.Background(), id, []string{"/ip4/127.0.0.1/tcp/0"})
	if err != nil {
		t.Fatalf("BuildNodeNoMDNS: %v", err)
	}
	defer n.Close()

	if n.ID() != expectedPID {
		t.Fatalf("node peer.ID = %q, want %q", n.ID(), expectedPID)
	}
}

func TestBuildNodeDifferentListenAddr(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	n, err := BuildNodeNoMDNS(context.Background(), id, []string{"/ip4/127.0.0.1/tcp/0"})
	if err != nil {
		t.Fatalf("BuildNodeNoMDNS: %v", err)
	}
	defer n.Close()

	if len(n.Addrs()) == 0 {
		t.Fatal("expected at least one listen address")
	}
}

// --- connection tests ---

func TestPeerConnection(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	waitCondition(t, 2*time.Second, "b connected to a",
		func() bool {
			for _, p := range b.ConnectedPeers() {
				if p == a.ID() {
					return true
				}
			}
			return false
		},
	)
}

func TestConnectedPeers(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)
	c := newTestNode(t)

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("Connect b: %v", err)
	}
	if err := a.Connect(ctx, addrInfo(c)); err != nil {
		t.Fatalf("Connect c: %v", err)
	}

	waitCondition(t, 2*time.Second, "a connected to b and c",
		func() bool { return len(a.ConnectedPeers()) >= 2 },
	)

	peers := a.ConnectedPeers()
	peerSet := make(map[peer.ID]bool, len(peers))
	for _, p := range peers {
		peerSet[p] = true
	}
	if !peerSet[b.ID()] {
		t.Errorf("b not in a's connected peers")
	}
	if !peerSet[c.ID()] {
		t.Errorf("c not in a's connected peers")
	}
}

func TestDuplicateConnect(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	// Second connect to the same peer must not error.
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("duplicate Connect: %v", err)
	}
}

func TestSelfConnectNoop(t *testing.T) {
	n := newTestNode(t)
	// Connecting to self must be a no-op (no error, no crash).
	if err := n.Connect(context.Background(), addrInfo(n)); err != nil {
		t.Fatalf("self-connect returned error: %v", err)
	}
}

func TestDisconnectHandling(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	waitCondition(t, 2*time.Second, "connected",
		func() bool { return len(a.ConnectedPeers()) > 0 },
	)

	// Closing b should cause a to observe the disconnection.
	_ = b.Close()

	waitCondition(t, 3*time.Second, "a sees disconnect",
		func() bool {
			for _, p := range a.ConnectedPeers() {
				if p == b.ID() {
					return false
				}
			}
			return true
		},
	)
}

// --- stream and messaging tests ---

func TestStreamCreation(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	waitCondition(t, 2*time.Second, "connected",
		func() bool { return len(a.ConnectedPeers()) > 0 },
	)

	s, err := a.host.NewStream(ctx, b.ID(), ProtocolID)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	_ = s.Reset()
}

func TestStreamCommunication(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	received := make(chan protocol.Message, 1)
	b.SetMessageHandler(func(msg protocol.Message, from peer.ID) {
		received <- msg
	})

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	waitCondition(t, 2*time.Second, "connected",
		func() bool { return len(a.ConnectedPeers()) > 0 },
	)

	want := testMessage(a.ID().String(), "stream-hello")
	if err := a.SendMessage(ctx, b.ID(), want); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	select {
	case got := <-received:
		if got.Body != want.Body {
			t.Fatalf("body = %q, want %q", got.Body, want.Body)
		}
		if got.SenderID != want.SenderID {
			t.Fatalf("senderID = %q, want %q", got.SenderID, want.SenderID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestSendMessageRoundTrip(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	var gotA, gotB protocol.Message
	var wg sync.WaitGroup
	wg.Add(2)

	a.SetMessageHandler(func(msg protocol.Message, _ peer.ID) {
		gotA = msg
		wg.Done()
	})
	b.SetMessageHandler(func(msg protocol.Message, _ peer.ID) {
		gotB = msg
		wg.Done()
	})

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	waitCondition(t, 2*time.Second, "connected",
		func() bool { return len(a.ConnectedPeers()) > 0 },
	)

	msgAtoB := testMessage(a.ID().String(), "hello from a")
	msgBtoA := testMessage(b.ID().String(), "hello from b")

	if err := a.SendMessage(ctx, b.ID(), msgAtoB); err != nil {
		t.Fatalf("a→b SendMessage: %v", err)
	}
	if err := b.SendMessage(ctx, a.ID(), msgBtoA); err != nil {
		t.Fatalf("b→a SendMessage: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for round-trip messages")
	}

	if gotB.Body != msgAtoB.Body {
		t.Errorf("b got %q, want %q", gotB.Body, msgAtoB.Body)
	}
	if gotA.Body != msgBtoA.Body {
		t.Errorf("a got %q, want %q", gotA.Body, msgBtoA.Body)
	}
}

func TestMessageRouting(t *testing.T) {
	// Three nodes: a sends to b, c should NOT receive anything.
	a := newTestNode(t)
	b := newTestNode(t)
	c := newTestNode(t)

	var cReceived int32
	c.SetMessageHandler(func(_ protocol.Message, _ peer.ID) {
		atomic.AddInt32(&cReceived, 1)
	})
	received := make(chan protocol.Message, 1)
	b.SetMessageHandler(func(msg protocol.Message, _ peer.ID) {
		received <- msg
	})

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("Connect a→b: %v", err)
	}
	if err := a.Connect(ctx, addrInfo(c)); err != nil {
		t.Fatalf("Connect a→c: %v", err)
	}
	waitCondition(t, 2*time.Second, "connected",
		func() bool { return len(a.ConnectedPeers()) >= 2 },
	)

	// Only send to b.
	msg := testMessage(a.ID().String(), "only for b")
	if err := a.SendMessage(ctx, b.ID(), msg); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	select {
	case got := <-received:
		if got.Body != msg.Body {
			t.Fatalf("b got %q, want %q", got.Body, msg.Body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for message on b")
	}

	time.Sleep(100 * time.Millisecond)
	if n := atomic.LoadInt32(&cReceived); n != 0 {
		t.Fatalf("c received %d messages, want 0", n)
	}
}

func TestSendMessageToSelfError(t *testing.T) {
	n := newTestNode(t)
	msg := testMessage(n.ID().String(), "self-send")
	if err := n.SendMessage(context.Background(), n.ID(), msg); err == nil {
		t.Fatal("expected error when sending message to self")
	}
}

func TestConcurrentSend(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	const n = 20
	var received int32
	b.SetMessageHandler(func(_ protocol.Message, _ peer.ID) {
		atomic.AddInt32(&received, 1)
	})

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	waitCondition(t, 2*time.Second, "connected",
		func() bool { return len(a.ConnectedPeers()) > 0 },
	)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg := testMessage(a.ID().String(), "concurrent")
			if err := a.SendMessage(ctx, b.ID(), msg); err != nil {
				// Log but don't fail — some concurrent streams may race on
				// stream limits; the important check is no data race.
				t.Logf("SendMessage %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	// Allow some delivery time.
	waitCondition(t, 3*time.Second, "messages delivered",
		func() bool { return atomic.LoadInt32(&received) > 0 },
	)
}

func TestSetMessageHandlerReplaces(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	var first, second int32
	b.SetMessageHandler(func(_ protocol.Message, _ peer.ID) { atomic.AddInt32(&first, 1) })
	b.SetMessageHandler(func(_ protocol.Message, _ peer.ID) { atomic.AddInt32(&second, 1) })

	ctx := context.Background()
	if err := a.Connect(ctx, addrInfo(b)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	waitCondition(t, 2*time.Second, "connected",
		func() bool { return len(a.ConnectedPeers()) > 0 },
	)

	if err := a.SendMessage(ctx, b.ID(), testMessage(a.ID().String(), "x")); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	waitCondition(t, 2*time.Second, "second handler called",
		func() bool { return atomic.LoadInt32(&second) > 0 },
	)
	if atomic.LoadInt32(&first) != 0 {
		t.Fatal("first (replaced) handler was called")
	}
}
