package connection

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDialer struct {
	mu       sync.Mutex
	calls    int32
	listener net.Listener
	failNext int32
}

func (f *fakeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	atomic.AddInt32(&f.calls, 1)
	if atomic.LoadInt32(&f.failNext) > 0 {
		return nil, errors.New("dial failure")
	}
	if f.listener == nil {
		return nil, errors.New("no listener")
	}
	return net.Dial(network, f.listener.Addr().String())
}

func newFakeDialer(t *testing.T) *fakeDialer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return &fakeDialer{listener: listener}
}

func TestManagerConnectSuccess(t *testing.T) {
	dialer := newFakeDialer(t)
	m := NewManager(dialer)

	if err := m.Connect(context.Background(), "peer-a", dialer.listener.Addr().String()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, ok := m.Status("peer-a")
		if ok && status.State == StateConnected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, _ := m.Status("peer-a")
	t.Fatalf("expected connected state, got %v", status)
}

func TestManagerConnectFailure(t *testing.T) {
	dialer := newFakeDialer(t)
	atomic.StoreInt32(&dialer.failNext, 1)
	m := NewManager(dialer)

	if err := m.Connect(context.Background(), "peer-a", "127.0.0.1:1"); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, ok := m.Status("peer-a")
		if ok && status.State == StateFailed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, _ := m.Status("peer-a")
	t.Fatalf("expected failed state, got %v", status)
}

func TestManagerDuplicateConnect(t *testing.T) {
	dialer := newFakeDialer(t)
	m := NewManager(dialer)

	endpoint := dialer.listener.Addr().String()
	for i := 0; i < 5; i++ {
		if err := m.Connect(context.Background(), "peer-a", endpoint); err != nil {
			t.Fatalf("Connect() iteration %d error = %v", i, err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, ok := m.Status("peer-a")
		if ok && status.State == StateConnected {
			if atomic.LoadInt32(&dialer.calls) > 1 {
				t.Fatalf("expected single dial attempt, got %d", dialer.calls)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected connected state")
}

func TestManagerDisconnect(t *testing.T) {
	dialer := newFakeDialer(t)
	m := NewManager(dialer)

	if err := m.Connect(context.Background(), "peer-a", dialer.listener.Addr().String()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if err := m.Disconnect("peer-a"); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if _, ok := m.Status("peer-a"); ok {
		t.Fatal("expected peer to be forgotten after disconnect")
	}
}

func TestManagerForget(t *testing.T) {
	dialer := newFakeDialer(t)
	m := NewManager(dialer)

	if err := m.Connect(context.Background(), "peer-a", dialer.listener.Addr().String()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	m.Forget("peer-a")
	if _, ok := m.Status("peer-a"); ok {
		t.Fatal("expected peer to be forgotten")
	}
}

func TestManagerSnapshot(t *testing.T) {
	dialer := newFakeDialer(t)
	m := NewManager(dialer)

	if err := m.Connect(context.Background(), "peer-a", dialer.listener.Addr().String()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if err := m.Connect(context.Background(), "peer-b", dialer.listener.Addr().String()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := m.Snapshot()
		if len(snapshot) == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected 2 peers in snapshot, got %d", len(m.Snapshot()))
}

func TestManagerValidation(t *testing.T) {
	m := NewManager(nil)
	if err := m.Connect(context.Background(), "", "127.0.0.1:4000"); err == nil {
		t.Fatal("expected error for empty peer id")
	}
	if err := m.Connect(context.Background(), "peer-a", ""); err == nil {
		t.Fatal("expected error for empty endpoint")
	}
	if err := m.Disconnect(""); err == nil {
		t.Fatal("expected error for empty peer id on disconnect")
	}
}

func TestValidateEndpoint(t *testing.T) {
	if err := ValidateEndpoint(""); err == nil {
		t.Fatal("expected error for empty endpoint")
	}
	if err := ValidateEndpoint("127.0.0.1"); err == nil {
		t.Fatal("expected error for missing port")
	}
	if err := ValidateEndpoint(":4000"); err == nil {
		t.Fatal("expected error for missing host")
	}
	if err := ValidateEndpoint("127.0.0.1:4000"); err != nil {
		t.Fatalf("ValidateEndpoint() unexpected error: %v", err)
	}
}
