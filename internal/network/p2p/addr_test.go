package p2p

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func TestValidateMultiaddr(t *testing.T) {
	valid := []string{
		"/ip4/127.0.0.1/tcp/9001",
		"/ip4/0.0.0.0/tcp/0",
		"/ip6/::1/tcp/9001",
		"/dns4/bootstrap.skall.chat/tcp/9001",
		"/ip4/198.51.100.1/tcp/9001/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN",
		"/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN",
		"/p2p-circuit/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN",
	}

	for _, addr := range valid {
		if err := ValidateMultiaddr(addr); err != nil {
			t.Errorf("ValidateMultiaddr(%q) unexpected error: %v", addr, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"invalid",
		"127.0.0.1:9001",
		"/ip4/999.999.999.999/tcp/9001",
		"/ip4/127.0.0.1/tcp/invalidport",
	}

	for _, addr := range invalid {
		if err := ValidateMultiaddr(addr); err == nil {
			t.Errorf("ValidateMultiaddr(%q) expected error, got nil", addr)
		}
	}
}

func TestParsePeerAddr(t *testing.T) {
	pidStr := "12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN"
	expectedPID, err := peer.Decode(pidStr)
	if err != nil {
		t.Fatalf("decode test peer id: %v", err)
	}

	// 1. Full multiaddr with /p2p/
	fullMaddr := "/ip4/198.51.100.1/tcp/9001/p2p/" + pidStr
	info, err := ParsePeerAddr(fullMaddr, "")
	if err != nil {
		t.Fatalf("ParsePeerAddr full multiaddr failed: %v", err)
	}
	if info.ID != expectedPID {
		t.Errorf("got peer ID %s, want %s", info.ID, expectedPID)
	}
	if len(info.Addrs) != 1 || info.Addrs[0].String() != "/ip4/198.51.100.1/tcp/9001" {
		t.Errorf("got addrs %v, want /ip4/198.51.100.1/tcp/9001", info.Addrs)
	}

	// 2. Full multiaddr with matching optPeerID
	info, err = ParsePeerAddr(fullMaddr, pidStr)
	if err != nil {
		t.Fatalf("ParsePeerAddr with matching optPeerID failed: %v", err)
	}
	if info.ID != expectedPID {
		t.Errorf("got peer ID %s, want %s", info.ID, expectedPID)
	}

	// 3. Full multiaddr with conflicting optPeerID
	otherPID := "12D3KooWRByBsE7GskpS58G5g4373hQy3g54xS95oV5Z5J5qT99X"
	_, err = ParsePeerAddr(fullMaddr, otherPID)
	if err == nil {
		t.Fatal("expected error on peer ID mismatch, got nil")
	}

	// 4. Multiaddr without peer ID, paired with optPeerID
	noPIDMaddr := "/ip4/198.51.100.1/tcp/9001"
	info, err = ParsePeerAddr(noPIDMaddr, pidStr)
	if err != nil {
		t.Fatalf("ParsePeerAddr no PID + optPeerID failed: %v", err)
	}
	if info.ID != expectedPID {
		t.Errorf("got peer ID %s, want %s", info.ID, expectedPID)
	}
	if len(info.Addrs) != 1 || info.Addrs[0].String() != noPIDMaddr {
		t.Errorf("got addrs %v, want %s", info.Addrs, noPIDMaddr)
	}

	// 5. Multiaddr without peer ID and without optPeerID -> should error clearly
	_, err = ParsePeerAddr(noPIDMaddr, "")
	if err == nil {
		t.Fatal("expected error for multiaddr without peer ID, got nil")
	}

	// 6. Bare peer ID
	info, err = ParsePeerAddr(pidStr, "")
	if err != nil {
		t.Fatalf("ParsePeerAddr bare peer ID failed: %v", err)
	}
	if info.ID != expectedPID {
		t.Errorf("got peer ID %s, want %s", info.ID, expectedPID)
	}
	if len(info.Addrs) != 0 {
		t.Errorf("expected 0 addrs for bare peer ID, got %v", info.Addrs)
	}

	// 7. host:port with optPeerID
	info, err = ParsePeerAddr("198.51.100.1:9001", pidStr)
	if err != nil {
		t.Fatalf("ParsePeerAddr host:port with optPeerID failed: %v", err)
	}
	if info.ID != expectedPID {
		t.Errorf("got peer ID %s, want %s", info.ID, expectedPID)
	}
	if len(info.Addrs) != 1 || info.Addrs[0].String() != "/ip4/198.51.100.1/tcp/9001" {
		t.Errorf("got addrs %v, want /ip4/198.51.100.1/tcp/9001", info.Addrs)
	}

	// 8. host:port without optPeerID -> should error
	_, err = ParsePeerAddr("198.51.100.1:9001", "")
	if err == nil {
		t.Fatal("expected error for host:port without peer ID, got nil")
	}

	// 9. Empty target
	_, err = ParsePeerAddr("", "")
	if err == nil {
		t.Fatal("expected error for empty target, got nil")
	}
}

func TestIsRelayAddr(t *testing.T) {
	relayAddr, err := ma.NewMultiaddr("/ip4/1.2.3.4/tcp/9001/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN/p2p-circuit/p2p/12D3KooWRByBsE7GskpS58G5g4373hQy3g54xS95oV5Z5J5qT99X")
	if err != nil {
		t.Fatalf("NewMultiaddr: %v", err)
	}
	if !IsRelayAddr(relayAddr) {
		t.Errorf("expected IsRelayAddr to return true for %s", relayAddr)
	}

	directAddr, err := ma.NewMultiaddr("/ip4/1.2.3.4/tcp/9001/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	if err != nil {
		t.Fatalf("NewMultiaddr: %v", err)
	}
	if IsRelayAddr(directAddr) {
		t.Errorf("expected IsRelayAddr to return false for %s", directAddr)
	}

	if IsRelayAddr(nil) {
		t.Error("expected IsRelayAddr(nil) to return false")
	}
}
