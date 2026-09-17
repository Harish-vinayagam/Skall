package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/Harish-vinayagam/Skall/internal/groups"
	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/network/p2p"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
	"github.com/Harish-vinayagam/Skall/internal/storage"
)

// stubHost is a minimal p2p.Host for unit tests.
type stubHost struct {
	mu      sync.Mutex
	handler p2p.MessageHandler
	sent    []protocol.Message
}

func (h *stubHost) ID() libp2ppeer.ID                                      { return "stub" }
func (h *stubHost) Addrs() []ma.Multiaddr                                  { return nil }
func (h *stubHost) Connect(_ context.Context, _ libp2ppeer.AddrInfo) error { return nil }
func (h *stubHost) ConnectedPeers() []libp2ppeer.ID                        { return nil }
func (h *stubHost) SetMessageHandler(fn p2p.MessageHandler) {
	h.mu.Lock()
	h.handler = fn
	h.mu.Unlock()
}
func (h *stubHost) SendMessage(_ context.Context, _ libp2ppeer.ID, msg protocol.Message) error {
	h.mu.Lock()
	h.sent = append(h.sent, msg)
	h.mu.Unlock()
	return nil
}
func (h *stubHost) Close() error { return nil }

// deliver simulates receiving an inbound message.
func (h *stubHost) deliver(msg protocol.Message, from libp2ppeer.ID) {
	h.mu.Lock()
	fn := h.handler
	h.mu.Unlock()
	if fn != nil {
		fn(msg, from)
	}
}

func makeTestService(t *testing.T) (*P2PService, *stubHost, *storage.Store) {
	t.Helper()
	db, err := storage.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	local, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	_ = db.UpsertIdentityMetadata(local)

	host := &stubHost{}
	grpMgr := groups.NewManager()
	svc := NewP2PService(context.Background(), local, host, db, grpMgr)
	t.Cleanup(func() { _ = svc.Close() })
	return svc, host, db
}

// TestSubscribeReceivesInboundEvent verifies that inbound messages are
// published to all active subscribers.
func TestSubscribeReceivesInboundEvent(t *testing.T) {
	svc, host, _ := makeTestService(t)

	ch := svc.Subscribe()

	inbound := protocol.NewChatMessage("remote-peer-000", svc.local.PeerID, "", "hello")
	host.deliver(inbound, "libp2p-stub-id")

	select {
	case ev := <-ch:
		if ev.Kind != EventNewMessage {
			t.Fatalf("expected EventNewMessage, got %v", ev.Kind)
		}
		if ev.Message.Body != "hello" {
			t.Fatalf("expected body %q, got %q", "hello", ev.Message.Body)
		}
		if ev.Message.IsOutbound {
			t.Fatal("inbound message should not be marked outbound")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

// TestMultipleSubscribersReceiveEvent verifies fan-out to multiple subscribers.
func TestMultipleSubscribersReceiveEvent(t *testing.T) {
	svc, host, _ := makeTestService(t)

	ch1 := svc.Subscribe()
	ch2 := svc.Subscribe()

	msg := protocol.NewChatMessage("peer-abc", svc.local.PeerID, "", "broadcast")
	host.deliver(msg, "p")

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Kind != EventNewMessage {
				t.Errorf("sub%d: expected EventNewMessage", i+1)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("sub%d: timed out", i+1)
		}
	}
}

// TestGetMessagesReturnsEmpty verifies that an unknown conversation returns
// an empty slice without error.
func TestGetMessagesReturnsEmpty(t *testing.T) {
	svc, _, _ := makeTestService(t)
	msgs, err := svc.GetMessages("unknown-peer", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

// TestLocalIdentityReturned verifies the service exposes the correct identity.
func TestLocalIdentityReturned(t *testing.T) {
	svc, _, _ := makeTestService(t)
	local := svc.LocalIdentity()
	if local.PeerID == "" {
		t.Fatal("LocalIdentity returned empty PeerID")
	}
}

// TestListGroupsEmpty verifies that no groups are returned when none are created.
func TestListGroupsEmpty(t *testing.T) {
	svc, _, _ := makeTestService(t)
	gs, err := svc.ListGroups()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gs) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(gs))
	}
}
