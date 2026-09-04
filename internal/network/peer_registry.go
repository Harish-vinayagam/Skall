package network

import (
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

var (
	ErrInvalidPeerID     = errors.New("invalid peer id")
	ErrPeerIDAlreadyUsed = errors.New("peer id already connected")
)

type peerConnection interface {
	enqueue(protocol.Message) bool
}

type PeerRegistry struct {
	mu         sync.RWMutex
	byPeerID   map[string]peerConnection
	byConn     map[peerConnection]string
	registered map[peerConnection]struct{}
}

func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{
		byPeerID:   make(map[string]peerConnection),
		byConn:     make(map[peerConnection]string),
		registered: make(map[peerConnection]struct{}),
	}
}

func (r *PeerRegistry) Register(peerID string, conn peerConnection) error {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return ErrInvalidPeerID
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byPeerID[peerID]; ok && existing != conn {
		return ErrPeerIDAlreadyUsed
	}

	if previousPeerID, ok := r.byConn[conn]; ok && previousPeerID != peerID {
		delete(r.byPeerID, previousPeerID)
	}

	r.byPeerID[peerID] = conn
	r.byConn[conn] = peerID
	r.registered[conn] = struct{}{}
	return nil
}

func (r *PeerRegistry) Lookup(peerID string) (peerConnection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn, ok := r.byPeerID[peerID]
	return conn, ok
}

func (r *PeerRegistry) PeerID(conn peerConnection) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peerID, ok := r.byConn[conn]
	return peerID, ok
}

func (r *PeerRegistry) Unregister(conn peerConnection) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	peerID, ok := r.byConn[conn]
	if !ok {
		delete(r.registered, conn)
		return "", false
	}

	delete(r.byConn, conn)
	delete(r.byPeerID, peerID)
	delete(r.registered, conn)
	return peerID, true
}

func (r *PeerRegistry) IsRegistered(conn peerConnection) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.byConn[conn]
	return ok
}

func (r *PeerRegistry) List(excludePeerID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peers := make([]string, 0, len(r.byPeerID))
	for peerID := range r.byPeerID {
		if peerID == excludePeerID {
			continue
		}
		peers = append(peers, peerID)
	}
	sort.Strings(peers)
	return peers
}
