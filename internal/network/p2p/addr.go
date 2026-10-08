package p2p

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// ValidateMultiaddr checks if raw is a syntactically valid libp2p multiaddress.
func ValidateMultiaddr(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("multiaddress cannot be empty")
	}
	_, err := ma.NewMultiaddr(trimmed)
	if err != nil {
		return fmt.Errorf("invalid multiaddress %q: %w", raw, err)
	}
	return nil
}

// ParsePeerAddr parses a target peer string and optional peer ID into a peer.AddrInfo.
//
// Supported input formats for target:
//  1. Full multiaddr containing /p2p/<peerID>:
//     e.g. "/ip4/198.51.100.1/tcp/9001/p2p/12D3KooW..."
//  2. Multiaddr without peer ID, paired with optPeerID:
//     e.g. "/ip4/198.51.100.1/tcp/9001" and "12D3KooW..."
//  3. Bare peer ID:
//     e.g. "12D3KooW..." (addresses can be resolved via DHT or peerstore)
//  4. Host:port endpoint paired with optPeerID:
//     e.g. "198.51.100.1:9001" and "12D3KooW..."
func ParsePeerAddr(target string, optPeerID string) (peer.AddrInfo, error) {
	target = strings.TrimSpace(target)
	optPeerID = strings.TrimSpace(optPeerID)

	if target == "" {
		return peer.AddrInfo{}, errors.New("target peer address or ID cannot be empty")
	}

	// 1. Try parsing as a multiaddr
	if maddr, err := ma.NewMultiaddr(target); err == nil {
		// Check if it includes a /p2p/ or /ipfs/ peer ID component
		info, err := peer.AddrInfoFromP2pAddr(maddr)
		if err == nil {
			// Multiaddr contains peer ID
			if optPeerID != "" {
				parsedOpt, err := peer.Decode(optPeerID)
				if err != nil {
					return peer.AddrInfo{}, fmt.Errorf("invalid optional peer ID %q: %w", optPeerID, err)
				}
				if parsedOpt != info.ID {
					return peer.AddrInfo{}, fmt.Errorf("peer ID in multiaddress (%s) does not match provided peer ID (%s)", info.ID, parsedOpt)
				}
			}
			return *info, nil
		}

		// Multiaddr without peer ID — require optPeerID
		if optPeerID == "" {
			return peer.AddrInfo{}, fmt.Errorf("multiaddress %q does not contain a peer ID; append /p2p/<peerID> or supply the peer ID", target)
		}

		pid, err := peer.Decode(optPeerID)
		if err != nil {
			return peer.AddrInfo{}, fmt.Errorf("invalid peer ID %q: %w", optPeerID, err)
		}

		return peer.AddrInfo{
			ID:    pid,
			Addrs: []ma.Multiaddr{maddr},
		}, nil
	}

	// 2. Check if target is a bare peer ID
	if pid, err := peer.Decode(target); err == nil {
		return peer.AddrInfo{
			ID: pid,
		}, nil
	}

	// 3. Try parsing as host:port
	if host, port, err := net.SplitHostPort(target); err == nil {
		if optPeerID == "" {
			return peer.AddrInfo{}, fmt.Errorf("endpoint %q requires a peer ID to connect via libp2p", target)
		}
		pid, err := peer.Decode(optPeerID)
		if err != nil {
			return peer.AddrInfo{}, fmt.Errorf("invalid peer ID %q: %w", optPeerID, err)
		}

		// Build multiaddr for host:port
		var maddr ma.Multiaddr
		ip := net.ParseIP(host)
		if ip != nil {
			if ip.To4() != nil {
				maddr, err = ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%s", host, port))
			} else {
				maddr, err = ma.NewMultiaddr(fmt.Sprintf("/ip6/%s/tcp/%s", host, port))
			}
		} else {
			maddr, err = ma.NewMultiaddr(fmt.Sprintf("/dns4/%s/tcp/%s", host, port))
		}
		if err != nil {
			return peer.AddrInfo{}, fmt.Errorf("construct multiaddr from %q: %w", target, err)
		}

		return peer.AddrInfo{
			ID:    pid,
			Addrs: []ma.Multiaddr{maddr},
		}, nil
	}

	return peer.AddrInfo{}, fmt.Errorf("invalid peer address %q: must be a libp2p multiaddress (/ip4/.../p2p/...) or a peer ID", target)
}

// IsRelayAddr returns true if the multiaddress routes through a circuit relay (/p2p-circuit).
func IsRelayAddr(maddr ma.Multiaddr) bool {
	if maddr == nil {
		return false
	}
	for _, p := range maddr.Protocols() {
		if p.Name == "p2p-circuit" {
			return true
		}
	}
	return strings.Contains(maddr.String(), "/p2p-circuit")
}

// IsRelayConn returns true if the connection routes through a circuit relay.
func IsRelayConn(conn network.Conn) bool {
	if conn == nil {
		return false
	}
	return IsRelayAddr(conn.RemoteMultiaddr())
}
