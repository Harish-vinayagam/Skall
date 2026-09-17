package chat

// p2p_service_security_test.go — security-focused tests for P2PService.
//
// These tests live in package "chat" (white-box) to share the test helpers
// defined in service_test.go.

import (
	"errors"
	"testing"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

// TestHandleInbound_SenderIDOverrideWithAuthenticatedID verifies that when a
// remote peer claims a SenderID that does not match its authenticated libp2p
// peer ID, the service ignores the claimed ID and uses the authenticated one.
//
// This prevents application-layer impersonation: even though libp2p Noise
// verifies the *transport* identity, the JSON SenderID field is supplied by
// the remote and could be set to any value.
func TestHandleInbound_SenderIDOverrideWithAuthenticatedID(t *testing.T) {
	svc, host, _ := makeTestService(t)
	ch := svc.Subscribe()

	authenticatedPeer := libp2ppeer.ID("QmAuthenticatedPeer001")
	// Attacker puts a different peer's ID in SenderID — impersonation attempt.
	spoofedMsg := protocol.NewChatMessage("victim-peer-id", svc.local.PeerID, "", "impersonated body")

	host.deliver(spoofedMsg, authenticatedPeer)

	select {
	case ev := <-ch:
		if ev.Kind != EventNewMessage {
			t.Fatalf("expected EventNewMessage, got %v", ev.Kind)
		}
		// The event must carry the authenticated peer ID, not the spoofed one.
		if ev.Message.SenderID == "victim-peer-id" {
			t.Errorf("SenderID spoofing not mitigated: got claimed ID %q instead of authenticated ID %q",
				ev.Message.SenderID, authenticatedPeer.String())
		}
		if ev.Message.SenderID != authenticatedPeer.String() {
			t.Errorf("SenderID = %q, want authenticated ID %q",
				ev.Message.SenderID, authenticatedPeer.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound event")
	}
}

// TestHandleInbound_SenderIDMatchedIsKept verifies that when the claimed
// SenderID already matches the authenticated transport peer ID, the message is
// passed through unchanged (no spurious log line or modification).
func TestHandleInbound_SenderIDMatchedIsKept(t *testing.T) {
	svc, host, _ := makeTestService(t)
	ch := svc.Subscribe()

	authenticatedPeer := libp2ppeer.ID("QmHonestPeer001")
	// Honest peer: SenderID == authenticated ID.
	msg := protocol.NewChatMessage(authenticatedPeer.String(), svc.local.PeerID, "", "honest message")

	host.deliver(msg, authenticatedPeer)

	select {
	case ev := <-ch:
		if ev.Message.SenderID != authenticatedPeer.String() {
			t.Errorf("honest SenderID should not be altered: got %q", ev.Message.SenderID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

// TestSubscribe_CapEnforced verifies that opening more than maxSubs subscribers
// is rejected (the returned channel is closed immediately to signal failure).
func TestSubscribe_CapEnforced(t *testing.T) {
	svc, _, _ := makeTestService(t)

	// Open maxSubs subscribers.
	channels := make([]<-chan Event, 0, maxSubs)
	for i := 0; i < maxSubs; i++ {
		ch := svc.Subscribe()
		// A properly-returned channel must be open (not closed).
		select {
		case _, ok := <-ch:
			if !ok {
				t.Fatalf("subscriber %d: channel closed immediately (cap hit too early)", i)
			}
		default:
			// Empty channel — good, it's open.
		}
		channels = append(channels, ch)
	}

	// The next Subscribe must be rejected (channel is closed).
	overflow := svc.Subscribe()
	select {
	case _, ok := <-overflow:
		if ok {
			t.Fatal("overflow channel should be closed, but received a value")
		}
		// Channel is closed — correctly rejected.
	default:
		t.Fatal("overflow channel should be closed (select default fired, channel is open)")
	}
}

// TestSubscribeE_CapEnforced verifies that SubscribeE returns ErrTooManySubscribers
// when the cap is hit.
func TestSubscribeE_CapEnforced(t *testing.T) {
	svc, _, _ := makeTestService(t)

	// Fill to cap.
	for i := 0; i < maxSubs; i++ {
		if _, err := svc.SubscribeE(); err != nil {
			t.Fatalf("SubscribeE[%d] failed prematurely: %v", i, err)
		}
	}

	// The next one must fail.
	_, err := svc.SubscribeE()
	if err == nil {
		t.Fatal("expected ErrTooManySubscribers, got nil")
	}
	if !errors.Is(err, ErrTooManySubscribers) {
		t.Errorf("expected ErrTooManySubscribers, got %v", err)
	}
}

// TestUnsubscribe_FreesSlot verifies that Unsubscribe removes the channel from
// the fan-out list so the slot can be reused.
func TestUnsubscribe_FreesSlot(t *testing.T) {
	svc, _, _ := makeTestService(t)

	// Fill to cap.
	first, err := svc.SubscribeE()
	if err != nil {
		t.Fatalf("SubscribeE: %v", err)
	}
	for i := 1; i < maxSubs; i++ {
		if _, err := svc.SubscribeE(); err != nil {
			t.Fatalf("SubscribeE[%d]: %v", i, err)
		}
	}

	// Unsubscribe the first channel — this should free one slot.
	svc.Unsubscribe(first)

	// Now a new subscription should succeed.
	if _, err := svc.SubscribeE(); err != nil {
		t.Errorf("expected SubscribeE to succeed after Unsubscribe, got: %v", err)
	}
}
