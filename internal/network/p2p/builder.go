package p2p

import (
	"context"
	"fmt"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"

	"github.com/Harish-vinayagam/Skall/internal/identity"
)

// DefaultListenAddrs are the multiaddrs a SKALL node listens on by default.
// TCP on a random port on all interfaces (IPv4 + IPv6).
var DefaultListenAddrs = []string{
	"/ip4/0.0.0.0/tcp/0",
	"/ip6/::/tcp/0",
}

// MDNSServiceName is the mDNS service tag used for local SKALL peer discovery.
// Using a dedicated tag keeps SKALL nodes from connecting to arbitrary libp2p nodes.
const MDNSServiceName = "_skall._p2p._udp"

// BuildNode creates a fully initialised libp2p Node from a SKALL identity.
// listenAddrs may be nil to use DefaultListenAddrs.
//
// The returned Node:
//   - derives its keypair from id.PrivateKey (no second identity)
//   - listens on TCP (random port by default)
//   - uses Noise for transport security (libp2p default)
//   - uses yamux for stream multiplexing (libp2p default)
//   - enables NATPortMap for NAT traversal where available
//   - runs libp2p mDNS discovery for automatic peer finding on the LAN
func BuildNode(ctx context.Context, id identity.Identity, listenAddrs []string) (*Node, error) {
	if len(listenAddrs) == 0 {
		listenAddrs = DefaultListenAddrs
	}

	// Convert the SKALL ed25519 identity to a libp2p crypto key.
	// KeyPairFromStdKey wraps the existing key bytes — no new key material.
	lp2pPriv, err := id.LibP2PPrivKey()
	if err != nil {
		return nil, fmt.Errorf("p2p: build node: convert identity key: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(lp2pPriv),
		libp2p.ListenAddrStrings(listenAddrs...),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, fmt.Errorf("p2p: build node: create libp2p host: %w", err)
	}

	node := newNode(h)

	// Start libp2p mDNS discovery. HandlePeerFound on the notifee is called
	// whenever a peer is seen on the LAN; it calls node.Connect automatically.
	notifee := &mdnsNotifee{node: node, ctx: ctx}
	svc := mdns.NewMdnsService(h, MDNSServiceName, notifee)
	if err := svc.Start(); err != nil {
		// mDNS failure is non-fatal — the node still works for direct connections.
		// Log and continue rather than returning an error.
		_ = h.Close()
		return nil, fmt.Errorf("p2p: build node: start mDNS: %w", err)
	}
	node.mdnsSvc = svc

	return node, nil
}

// BuildNodeNoMDNS creates a Node without mDNS discovery.
// This is used in tests where real mDNS would be noise, and for cases where
// the caller manages peer discovery externally.
func BuildNodeNoMDNS(_ context.Context, id identity.Identity, listenAddrs []string) (*Node, error) {
	if len(listenAddrs) == 0 {
		listenAddrs = DefaultListenAddrs
	}

	lp2pPriv, err := id.LibP2PPrivKey()
	if err != nil {
		return nil, fmt.Errorf("p2p: build node (no mdns): convert identity key: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(lp2pPriv),
		libp2p.ListenAddrStrings(listenAddrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("p2p: build node (no mdns): create libp2p host: %w", err)
	}

	return newNode(h), nil
}
