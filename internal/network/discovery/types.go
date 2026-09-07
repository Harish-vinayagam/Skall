package discovery

import (
	"context"
	"net"
	"strconv"
	"strings"
)

type EventType int

const (
	EventPeerAdded EventType = iota + 1
	EventPeerRemoved
)

type PeerInfo struct {
	PeerID      string
	DisplayName string
	Host        string
	Port        int
}

func (p PeerInfo) IsValid() bool {
	if strings.TrimSpace(p.PeerID) == "" {
		return false
	}
	if strings.TrimSpace(p.Host) == "" {
		return false
	}
	if p.Port <= 0 || p.Port > 65535 {
		return false
	}
	return true
}

func (p PeerInfo) Endpoint() string {
	return net.JoinHostPort(strings.TrimSpace(p.Host), strconv.Itoa(p.Port))
}

func (p PeerInfo) Key() string {
	peerID := strings.TrimSpace(p.PeerID)
	if peerID != "" {
		return peerID
	}
	return p.Endpoint()
}

type Event struct {
	Type EventType
	Peer PeerInfo
}

type Discovery interface {
	Start(ctx context.Context) error
	Events() <-chan Event
	Close() error
}

type Scanner interface {
	Scan(ctx context.Context) ([]PeerInfo, error)
}
