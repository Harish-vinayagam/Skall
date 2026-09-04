package network

import (
    "sync/atomic"
    "testing"
    "time"

    "github.com/Harish-vinayagam/Skall/internal/protocol"
)

func makeFakeClient(server *Server, id, remote string) *clientConn {
    return &clientConn{
        server: server,
        conn:   nil,
        remote: remote,
        peerID: id,
        writeCh: make(chan protocol.Message, 8),
        closed:  make(chan struct{}),
    }
}

func TestDirectRouting(t *testing.T) {
    s := NewServer("test")

    a := makeFakeClient(s, "alice", "a")
    b := makeFakeClient(s, "bob", "b")

    s.addClient(a)
    s.addClient(b)
    s.registerPeer(a, a.peerID)
    s.registerPeer(b, b.peerID)

    msg := protocol.Message{
        Version:     protocol.Version,
        ID:          "1",
        Type:        protocol.TypeChat,
        SenderID:    a.peerID,
        RecipientID: b.peerID,
        Timestamp:   time.Now().UTC(),
        Body:        "hello",
    }

    s.broadcast(a, msg)

    select {
    case m := <-b.writeCh:
        if m.Body != "hello" {
            t.Fatalf("unexpected body: %s", m.Body)
        }
    case <-time.After(100 * time.Millisecond):
        t.Fatal("message not delivered to recipient")
    }
}

func TestUnknownPeerDelivery(t *testing.T) {
    s := NewServer("test")

    a := makeFakeClient(s, "alice", "a")
    s.addClient(a)
    s.registerPeer(a, a.peerID)

    msg := protocol.Message{
        Version:     protocol.Version,
        ID:          "1",
        Type:        protocol.TypeChat,
        SenderID:    a.peerID,
        RecipientID: "doesnotexist",
        Timestamp:   time.Now().UTC(),
        Body:        "hello",
    }

    s.broadcast(a, msg)

    select {
    case m := <-a.writeCh:
        if m.Type != protocol.TypeSystem {
            t.Fatalf("expected system message, got %v", m.Type)
        }
    case <-time.After(100 * time.Millisecond):
        t.Fatal("expected delivery failure message to sender")
    }
}

func TestDisconnectedPeerDelivery(t *testing.T) {
    s := NewServer("test")

    a := makeFakeClient(s, "alice", "a")
    b := makeFakeClient(s, "bob", "b")

    s.addClient(a)
    s.addClient(b)
    s.registerPeer(a, a.peerID)
    s.registerPeer(b, b.peerID)

    // simulate b closed
    atomic.StoreInt32(&b.closedN, 1)

    msg := protocol.Message{
        Version:     protocol.Version,
        ID:          "1",
        Type:        protocol.TypeChat,
        SenderID:    a.peerID,
        RecipientID: b.peerID,
        Timestamp:   time.Now().UTC(),
        Body:        "hello",
    }

    s.broadcast(a, msg)

    select {
    case m := <-a.writeCh:
        if m.Type != protocol.TypeSystem {
            t.Fatalf("expected system message, got %v", m.Type)
        }
    case <-time.After(100 * time.Millisecond):
        t.Fatal("expected delivery failure message to sender")
    }
}

func TestConcurrentMessages(t *testing.T) {
    s := NewServer("test")

    a := makeFakeClient(s, "alice", "a")
    b := makeFakeClient(s, "bob", "b")

    s.addClient(a)
    s.addClient(b)
    s.registerPeer(a, a.peerID)
    s.registerPeer(b, b.peerID)

    done := make(chan struct{})

    go func() {
        for i := 0; i < 50; i++ {
            msg := protocol.Message{
                Version:     protocol.Version,
                ID:          "1",
                Type:        protocol.TypeChat,
                SenderID:    a.peerID,
                RecipientID: b.peerID,
                Timestamp:   time.Now().UTC(),
                Body:        "x",
            }
            s.broadcast(a, msg)
        }
        close(done)
    }()

    // ensure some messages arrive
    received := 0
    timeout := time.After(500 * time.Millisecond)
LOOP:
    for {
        select {
        case <-b.writeCh:
            received++
            if received >= 10 {
                break LOOP
            }
        case <-timeout:
            t.Fatalf("only received %d messages", received)
        }
    }

    <-done
}
