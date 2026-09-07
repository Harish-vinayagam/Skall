package discovery

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeManager is a test double for ConnectionManager that records calls and
// can be configured to return errors.
type fakeManager struct {
	mu sync.Mutex

	connects    []connectCall
	disconnects []string

	connectErr    error
	disconnectErr error
}

type connectCall struct {
	peerID   string
	endpoint string
}

func (f *fakeManager) Connect(_ context.Context, peerID, endpoint string) error {
	f.mu.Lock()
	f.connects = append(f.connects, connectCall{peerID: peerID, endpoint: endpoint})
	f.mu.Unlock()
	return f.connectErr
}

func (f *fakeManager) Disconnect(peerID string) error {
	f.mu.Lock()
	f.disconnects = append(f.disconnects, peerID)
	f.mu.Unlock()
	return f.disconnectErr
}

func (f *fakeManager) connectCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.connects)
}

func (f *fakeManager) disconnectCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.disconnects)
}

// sendAndWait sends ev on ch and waits up to deadline for cond to become true.
func sendAndWait(t *testing.T, ch chan<- Event, ev Event, cond func() bool, label string) {
	t.Helper()
	ch <- ev
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s: condition not met within deadline", label)
}

// runConnector starts a connector goroutine and returns (cancel, events chan).
func runConnector(mgr ConnectionManager) (context.CancelFunc, chan Event) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Event, 8)
	c := NewConnector(mgr)
	go c.Run(ctx, ch) //nolint:errcheck
	return cancel, ch
}

// errorOnFirstManager wraps fakeManager and returns an error only for the
// first Connect call. The flag is toggled atomically inside Connect (which is
// already holding fakeManager.mu), so there is no data race between the test
// goroutine and the connector goroutine.
type errorOnFirstManager struct {
	*fakeManager
	first *int32 // 1 = next call errors; 0 = success
}

func (e *errorOnFirstManager) Connect(ctx context.Context, peerID, endpoint string) error {
	e.fakeManager.mu.Lock()
	e.fakeManager.connects = append(e.fakeManager.connects, connectCall{peerID: peerID, endpoint: endpoint})
	wasFirst := atomic.SwapInt32(e.first, 0) == 1
	e.fakeManager.mu.Unlock()
	if wasFirst {
		return errors.New("refused")
	}
	return nil
}

// --- Tests ---

func TestConnectorPeerAdded(t *testing.T) {
	mgr := &fakeManager{}
	cancel, ch := runConnector(mgr)
	defer cancel()

	peer := PeerInfo{PeerID: "peer-a", Host: "192.0.2.10", Port: 4000}
	sendAndWait(t, ch, Event{Type: EventPeerAdded, Peer: peer},
		func() bool { return mgr.connectCount() == 1 }, "Connect not called")

	mgr.mu.Lock()
	got := mgr.connects[0]
	mgr.mu.Unlock()

	if got.peerID != "peer-a" {
		t.Errorf("Connect peerID = %q, want %q", got.peerID, "peer-a")
	}
	if got.endpoint != "192.0.2.10:4000" {
		t.Errorf("Connect endpoint = %q, want %q", got.endpoint, "192.0.2.10:4000")
	}
}

func TestConnectorPeerRemoved(t *testing.T) {
	mgr := &fakeManager{}
	cancel, ch := runConnector(mgr)
	defer cancel()

	peer := PeerInfo{PeerID: "peer-a", Host: "192.0.2.10", Port: 4000}
	sendAndWait(t, ch, Event{Type: EventPeerRemoved, Peer: peer},
		func() bool { return mgr.disconnectCount() == 1 }, "Disconnect not called")

	mgr.mu.Lock()
	got := mgr.disconnects[0]
	mgr.mu.Unlock()

	if got != "peer-a" {
		t.Errorf("Disconnect peerID = %q, want %q", got, "peer-a")
	}
}

func TestConnectorDuplicatePeerAdded(t *testing.T) {
	// The Connector itself forwards both events; de-duplication is the
	// ConnectionManager's responsibility. Verify the connector does not suppress
	// the second call (the real Manager handles it with no-op).
	mgr := &fakeManager{}
	cancel, ch := runConnector(mgr)
	defer cancel()

	peer := PeerInfo{PeerID: "peer-a", Host: "192.0.2.10", Port: 4000}
	ch <- Event{Type: EventPeerAdded, Peer: peer}
	ch <- Event{Type: EventPeerAdded, Peer: peer}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mgr.connectCount() >= 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected 2 Connect calls, got %d", mgr.connectCount())
}

func TestConnectorInvalidPeerInfo(t *testing.T) {
	mgr := &fakeManager{}
	cancel, ch := runConnector(mgr)
	defer cancel()

	// Missing PeerID — IsValid() returns false.
	invalid := PeerInfo{Host: "192.0.2.10", Port: 4000}
	ch <- Event{Type: EventPeerAdded, Peer: invalid}

	// Give the goroutine time to process.
	time.Sleep(100 * time.Millisecond)

	if mgr.connectCount() != 0 {
		t.Fatalf("expected no Connect calls for invalid peer, got %d", mgr.connectCount())
	}
}

func TestConnectorAlreadyConnectedPeer(t *testing.T) {
	// Manager returns nil (already connected is fine) — connector must not
	// treat nil as an error.
	mgr := &fakeManager{connectErr: nil}
	cancel, ch := runConnector(mgr)
	defer cancel()

	peer := PeerInfo{PeerID: "peer-b", Host: "192.0.2.11", Port: 4001}
	sendAndWait(t, ch, Event{Type: EventPeerAdded, Peer: peer},
		func() bool { return mgr.connectCount() == 1 }, "Connect not called")
}

func TestConnectorConnectError(t *testing.T) {
	// A non-nil error from Connect should be logged but must not crash or stop
	// the connector from processing subsequent events.
	//
	// errOnFirst returns an error for the first call only; subsequent calls
	// succeed. The switch happens inside Connect (under the lock) to avoid a
	// data race between the test goroutine and the connector goroutine.
	mgr := &fakeManager{}
	var firstCall int32 = 1 // 1 = first call; 0 = subsequent
	mgr.connectErr = nil
	// Override Connect to error on the first call only.
	errOnFirstMgr := &errorOnFirstManager{fakeManager: mgr, first: &firstCall}

	cancel, ch := runConnector(errOnFirstMgr)
	defer cancel()

	peer := PeerInfo{PeerID: "peer-c", Host: "192.0.2.12", Port: 4002}
	sendAndWait(t, ch, Event{Type: EventPeerAdded, Peer: peer},
		func() bool { return errOnFirstMgr.connectCount() == 1 }, "Connect not called despite error")

	// Send another peer — connector should still be alive.
	peer2 := PeerInfo{PeerID: "peer-d", Host: "192.0.2.13", Port: 4003}
	sendAndWait(t, ch, Event{Type: EventPeerAdded, Peer: peer2},
		func() bool { return errOnFirstMgr.connectCount() == 2 }, "Connector stopped after error")
}

func TestConnectorDisconnectError(t *testing.T) {
	mgr := &fakeManager{disconnectErr: errors.New("already gone")}
	cancel, ch := runConnector(mgr)
	defer cancel()

	peer := PeerInfo{PeerID: "peer-e", Host: "192.0.2.14", Port: 4004}
	sendAndWait(t, ch, Event{Type: EventPeerRemoved, Peer: peer},
		func() bool { return mgr.disconnectCount() == 1 }, "Disconnect not called")
}

func TestConnectorContextCancellation(t *testing.T) {
	mgr := &fakeManager{}
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Event)
	c := NewConnector(mgr)

	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, ch) }()

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() returned %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestConnectorEventsChannelClosed(t *testing.T) {
	mgr := &fakeManager{}
	ctx := context.Background()
	ch := make(chan Event)
	c := NewConnector(mgr)

	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, ch) }()

	close(ch)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned %v, want nil", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() did not return after events channel closed")
	}
}

func TestConnectorConcurrentEvents(t *testing.T) {
	// Fire many events from multiple goroutines simultaneously and verify the
	// connector processes all of them without data races (run with -race).
	mgr := &fakeManager{}
	cancel, ch := runConnector(mgr)
	defer cancel()

	const goroutines = 10
	const eventsEach = 20
	var wg sync.WaitGroup
	var total int64

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < eventsEach; i++ {
				peer := PeerInfo{
					PeerID: "peer-concurrent",
					Host:   "192.0.2.50",
					Port:   4000 + id,
				}
				select {
				case ch <- Event{Type: EventPeerAdded, Peer: peer}:
					atomic.AddInt64(&total, 1)
				case <-time.After(200 * time.Millisecond):
					// Channel full — acceptable under load.
				}
			}
		}(g)
	}

	wg.Wait()

	// Allow goroutine to drain the channel.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if int64(mgr.connectCount()) >= total {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d Connect calls, got %d", total, mgr.connectCount())
}

func TestNewConnectorPanicsOnNilManager(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil manager")
		}
	}()
	NewConnector(nil)
}
