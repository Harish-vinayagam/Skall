package discovery

import (
	"testing"
)

func TestPeerInfoValidation(t *testing.T) {
	cases := []struct {
		name   string
		info   PeerInfo
		wantOK bool
	}{
		{
			name:   "valid",
			info:   PeerInfo{PeerID: "abc", Host: "192.0.2.1", Port: 4000},
			wantOK: true,
		},
		{
			name:   "missing peer id",
			info:   PeerInfo{Host: "192.0.2.1", Port: 4000},
			wantOK: false,
		},
		{
			name:   "missing host",
			info:   PeerInfo{PeerID: "abc", Port: 4000},
			wantOK: false,
		},
		{
			name:   "invalid port",
			info:   PeerInfo{PeerID: "abc", Host: "192.0.2.1", Port: 0},
			wantOK: false,
		},
		{
			name:   "port too large",
			info:   PeerInfo{PeerID: "abc", Host: "192.0.2.1", Port: 70000},
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.info.IsValid(); got != tc.wantOK {
				t.Fatalf("IsValid() = %v, want %v", got, tc.wantOK)
			}
		})
	}
}

func TestPeerInfoEndpoint(t *testing.T) {
	info := PeerInfo{PeerID: "abc", Host: "192.0.2.1", Port: 4000}
	if got := info.Endpoint(); got != "192.0.2.1:4000" {
		t.Fatalf("Endpoint() = %q, want %q", got, "192.0.2.1:4000")
	}
}

func TestPeerInfoKey(t *testing.T) {
	withID := PeerInfo{PeerID: "abc", Host: "192.0.2.1", Port: 4000}
	if got := withID.Key(); got != "abc" {
		t.Fatalf("Key() = %q, want %q", got, "abc")
	}
	withoutID := PeerInfo{Host: "192.0.2.1", Port: 4000}
	if got := withoutID.Key(); got != "192.0.2.1:4000" {
		t.Fatalf("Key() = %q, want %q", got, "192.0.2.1:4000")
	}
}

func TestParsePort(t *testing.T) {
	if _, err := ParsePort(""); err == nil {
		t.Fatal("expected error for empty port")
	}
	if _, err := ParsePort("abc"); err == nil {
		t.Fatal("expected error for non-numeric port")
	}
	if _, err := ParsePort("0"); err == nil {
		t.Fatal("expected error for zero port")
	}
	if _, err := ParsePort("70000"); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
	port, err := ParsePort("4000")
	if err != nil {
		t.Fatalf("ParsePort(4000) unexpected error: %v", err)
	}
	if port != 4000 {
		t.Fatalf("ParsePort(4000) = %d, want 4000", port)
	}
}
