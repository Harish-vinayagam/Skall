package discovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

// fakeResolver is a no-op mDNS resolver used in tests to avoid real network I/O.
type fakeResolver struct {
	entries []*zeroconf.ServiceEntry
	err     error
}

func (f *fakeResolver) Browse(_ context.Context, _, _ string, entries chan<- *zeroconf.ServiceEntry) error {
	if f.err != nil {
		return f.err
	}
	go func() {
		for _, e := range f.entries {
			entries <- e
		}
		close(entries)
	}()
	return nil
}

// fakeFactory returns a ResolverFactory that always produces the given fakeResolver.
func fakeFactory(r *fakeResolver) ResolverFactory {
	return func() (resolverIface, error) {
		return r, nil
	}
}

// failFactory returns a ResolverFactory that always returns an error.
func failFactory() ResolverFactory {
	return func() (resolverIface, error) {
		return nil, errors.New("resolver creation failed")
	}
}

// newTestConfig builds an MDNSConfig wired to the given factory with a very
// short poll interval so tests that need multiple scans run quickly.
func newTestConfig(factory ResolverFactory) MDNSConfig {
	cfg := DefaultMDNSConfig()
	cfg.ResolverFactory = factory
	cfg.Interval = 50 * time.Millisecond
	return cfg
}

// --- Start / lifecycle tests ---

func TestMDNSDiscoveryStartFactoryError(t *testing.T) {
	cfg := newTestConfig(failFactory())
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, cfg)
	if err := d.Start(context.Background()); err == nil {
		t.Fatal("expected error when resolver factory fails")
	}
}

func TestMDNSDiscoveryDoubleStart(t *testing.T) {
	cfg := newTestConfig(fakeFactory(&fakeResolver{}))
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, cfg)
	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("first Start() unexpected error: %v", err)
	}
	defer d.Close()
	if err := d.Start(ctx); err == nil {
		t.Fatal("expected error on double start")
	}
}

func TestMDNSDiscoveryStartOnClosed(t *testing.T) {
	cfg := newTestConfig(fakeFactory(&fakeResolver{}))
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, cfg)
	_ = d.Close()
	if err := d.Start(context.Background()); err == nil {
		t.Fatal("expected error when starting a closed discovery")
	}
}

func TestMDNSDiscoveryCloseIsIdempotent(t *testing.T) {
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, DefaultMDNSConfig())
	if err := d.Close(); err != nil {
		t.Fatalf("Close() unexpected error: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close() second call unexpected error: %v", err)
	}
}

func TestMDNSDiscoveryAnnounceRequiresPeerID(t *testing.T) {
	d := NewMDNSDiscovery(PeerInfo{}, DefaultMDNSConfig())
	if err := d.Announce(context.Background(), 4000); err == nil {
		t.Fatal("expected error when announcing without peer id")
	}
}

func TestMDNSDiscoveryAnnounceRequiresPort(t *testing.T) {
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, DefaultMDNSConfig())
	if err := d.Announce(context.Background(), 0); err == nil {
		t.Fatal("expected error when announcing with invalid port")
	}
}

func TestMDNSDiscoveryEventsChannel(t *testing.T) {
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, DefaultMDNSConfig())
	if d.Events() == nil {
		t.Fatal("Events() returned nil channel")
	}
}

// --- applyDiff white-box tests (no real network) ---

func TestMDNSDiscoveryDiffEmitsEvents(t *testing.T) {
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, DefaultMDNSConfig())

	added := PeerInfo{PeerID: "peer-a", Host: "192.0.2.10", Port: 4000}
	if err := d.applyDiff([]PeerInfo{added}); err != nil {
		t.Fatalf("applyDiff() unexpected error: %v", err)
	}

	select {
	case ev := <-d.Events():
		if ev.Type != EventPeerAdded || ev.Peer.PeerID != "peer-a" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected peer added event")
	}

	if err := d.applyDiff(nil); err != nil {
		t.Fatalf("applyDiff() unexpected error: %v", err)
	}

	select {
	case ev := <-d.Events():
		if ev.Type != EventPeerRemoved || ev.Peer.PeerID != "peer-a" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected peer removed event")
	}
}

func TestMDNSDiscoveryDiffIgnoresSelf(t *testing.T) {
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, DefaultMDNSConfig())
	if err := d.applyDiff([]PeerInfo{{PeerID: "self", Host: "127.0.0.1", Port: 4000}}); err != nil {
		t.Fatalf("applyDiff() unexpected error: %v", err)
	}
	select {
	case ev := <-d.Events():
		t.Fatalf("did not expect event for self: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMDNSDiscoveryDiffDuplicateEvents(t *testing.T) {
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, DefaultMDNSConfig())
	peer := PeerInfo{PeerID: "peer-a", Host: "192.0.2.10", Port: 4000}
	if err := d.applyDiff([]PeerInfo{peer}); err != nil {
		t.Fatalf("applyDiff() unexpected error: %v", err)
	}
	if err := d.applyDiff([]PeerInfo{peer}); err != nil {
		t.Fatalf("applyDiff() unexpected error: %v", err)
	}
	count := 0
	deadline := time.After(100 * time.Millisecond)
loop:
	for {
		select {
		case <-d.Events():
			count++
		case <-deadline:
			break loop
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 event, got %d", count)
	}
}

func TestMDNSDiscoveryDiffInvalidPeer(t *testing.T) {
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, DefaultMDNSConfig())
	if err := d.applyDiff([]PeerInfo{{PeerID: "", Host: "192.0.2.10", Port: 4000}}); err != nil {
		t.Fatalf("applyDiff() unexpected error: %v", err)
	}
	select {
	case ev := <-d.Events():
		t.Fatalf("did not expect event for invalid peer: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

// --- fakeResolver integration: Start with injected resolver ---

func TestMDNSDiscoveryStartWithFakeResolver(t *testing.T) {
	cfg := newTestConfig(fakeFactory(&fakeResolver{}))
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}
	_ = d.Close()
}

func TestMDNSDiscoveryStartEmitsPeerFromResolver(t *testing.T) {
	// Build a fake ServiceEntry for a remote peer.
	entry := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: "peer-b"},
		HostName:      "192.0.2.20",
		Port:          4001,
		Text:          []string{"peer_id=peer-b", "display_name=Bob"},
	}
	cfg := newTestConfig(fakeFactory(&fakeResolver{entries: []*zeroconf.ServiceEntry{entry}}))
	d := NewMDNSDiscovery(PeerInfo{PeerID: "self"}, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}
	defer d.Close()

	select {
	case ev := <-d.Events():
		if ev.Type != EventPeerAdded {
			t.Fatalf("expected PeerAdded, got %v", ev.Type)
		}
		if ev.Peer.PeerID != "peer-b" {
			t.Fatalf("unexpected peer id: %s", ev.Peer.PeerID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected peer added event after start")
	}
}
