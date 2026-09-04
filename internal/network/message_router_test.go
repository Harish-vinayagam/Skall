package network

import (
	"errors"
	"sync"
	"testing"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

type fakePeerConn struct {
	mu        sync.Mutex
	connected bool
	messages  []protocol.Message
}

func newFakePeerConn() *fakePeerConn {
	return &fakePeerConn{connected: true}
}

func (f *fakePeerConn) enqueue(message protocol.Message) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.connected {
		return false
	}
	f.messages = append(f.messages, message)
	return true
}

func (f *fakePeerConn) disconnect() {
	f.mu.Lock()
	f.connected = false
	f.mu.Unlock()
}

func (f *fakePeerConn) delivered() []protocol.Message {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]protocol.Message, len(f.messages))
	copy(out, f.messages)
	return out
}

func TestPeerLookup(t *testing.T) {
	registry := NewPeerRegistry()
	alice := newFakePeerConn()
	bob := newFakePeerConn()

	if err := registry.Register("alice", alice); err != nil {
		t.Fatalf("Register(alice) error = %v", err)
	}
	if err := registry.Register("bob", bob); err != nil {
		t.Fatalf("Register(bob) error = %v", err)
	}

	if _, ok := registry.Lookup("alice"); !ok {
		t.Fatal("expected to find alice")
	}
	peers := registry.List("alice")
	if len(peers) != 1 || peers[0] != "bob" {
		t.Fatalf("unexpected peer list: %#v", peers)
	}
}

func TestDirectRouting(t *testing.T) {
	registry := NewPeerRegistry()
	router := NewMessageRouter(registry)
	alice := newFakePeerConn()
	bob := newFakePeerConn()

	if err := registry.Register("alice", alice); err != nil {
		t.Fatalf("Register(alice) error = %v", err)
	}
	if err := registry.Register("bob", bob); err != nil {
		t.Fatalf("Register(bob) error = %v", err)
	}

	msg := protocol.NewChatMessage("alice", "bob", "", "hello bob")
	if err := router.RouteDirect(alice, msg); err != nil {
		t.Fatalf("RouteDirect() error = %v", err)
	}

	delivered := bob.delivered()
	if len(delivered) != 1 {
		t.Fatalf("expected 1 delivered message, got %d", len(delivered))
	}
	if delivered[0].SenderID != "alice" || delivered[0].RecipientID != "bob" || delivered[0].Body != "hello bob" {
		t.Fatalf("unexpected delivered message: %+v", delivered[0])
	}
}

func TestUnknownPeerRouting(t *testing.T) {
	registry := NewPeerRegistry()
	router := NewMessageRouter(registry)
	alice := newFakePeerConn()

	if err := registry.Register("alice", alice); err != nil {
		t.Fatalf("Register(alice) error = %v", err)
	}

	msg := protocol.NewChatMessage("alice", "unknown", "", "hello")
	err := router.RouteDirect(alice, msg)
	if !errors.Is(err, ErrUnknownPeer) {
		t.Fatalf("expected ErrUnknownPeer, got %v", err)
	}
}

func TestDisconnectedPeerRouting(t *testing.T) {
	registry := NewPeerRegistry()
	router := NewMessageRouter(registry)
	alice := newFakePeerConn()
	bob := newFakePeerConn()

	if err := registry.Register("alice", alice); err != nil {
		t.Fatalf("Register(alice) error = %v", err)
	}
	if err := registry.Register("bob", bob); err != nil {
		t.Fatalf("Register(bob) error = %v", err)
	}
	bob.disconnect()

	msg := protocol.NewChatMessage("alice", "bob", "", "hello")
	err := router.RouteDirect(alice, msg)
	if !errors.Is(err, ErrPeerDisconnected) {
		t.Fatalf("expected ErrPeerDisconnected, got %v", err)
	}
	if _, ok := registry.Lookup("bob"); ok {
		t.Fatal("expected disconnected peer to be removed from registry")
	}
}

func TestMessageDelivery(t *testing.T) {
	registry := NewPeerRegistry()
	router := NewMessageRouter(registry)
	alice := newFakePeerConn()
	bob := newFakePeerConn()

	if err := registry.Register("alice", alice); err != nil {
		t.Fatalf("Register(alice) error = %v", err)
	}
	if err := registry.Register("bob", bob); err != nil {
		t.Fatalf("Register(bob) error = %v", err)
	}

	msg := protocol.NewChatMessage("alice", "bob", "", "delivery test")
	if err := router.RouteDirect(alice, msg); err != nil {
		t.Fatalf("RouteDirect() error = %v", err)
	}

	delivered := bob.delivered()
	if len(delivered) != 1 || delivered[0].Body != "delivery test" {
		t.Fatalf("unexpected delivery: %#v", delivered)
	}
}

func TestConcurrentMessages(t *testing.T) {
	registry := NewPeerRegistry()
	router := NewMessageRouter(registry)
	alice := newFakePeerConn()
	bob := newFakePeerConn()

	if err := registry.Register("alice", alice); err != nil {
		t.Fatalf("Register(alice) error = %v", err)
	}
	if err := registry.Register("bob", bob); err != nil {
		t.Fatalf("Register(bob) error = %v", err)
	}

	const total = 100
	var wg sync.WaitGroup
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func() {
			defer wg.Done()
			msg := protocol.NewChatMessage("alice", "bob", "", "ping")
			if err := router.RouteDirect(alice, msg); err != nil {
				t.Errorf("RouteDirect() error = %v", err)
			}
		}()
	}
	wg.Wait()

	delivered := bob.delivered()
	if len(delivered) != total {
		t.Fatalf("expected %d messages, got %d", total, len(delivered))
	}
}
