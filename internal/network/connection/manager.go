package connection

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type State int

const (
	StateDisconnected State = iota
	StateConnecting
	StateConnected
	StateFailed
)

func (s State) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

type PeerStatus struct {
	PeerID    string
	Endpoint  string
	State     State
	UpdatedAt time.Time
}

type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type Manager struct {
	mu      sync.Mutex
	peers   map[string]*peerState
	dialer  Dialer
	timeout time.Duration
}

type peerState struct {
	status PeerStatus
	cancel context.CancelFunc
}

func NewManager(dialer Dialer) *Manager {
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 5 * time.Second}
	}
	return &Manager{
		peers:   make(map[string]*peerState),
		dialer:  dialer,
		timeout: 5 * time.Second,
	}
}

func (m *Manager) Connect(ctx context.Context, peerID, endpoint string) error {
	peerID = trim(peerID)
	endpoint = trim(endpoint)
	if peerID == "" {
		return errors.New("peer id is required")
	}
	if endpoint == "" {
		return errors.New("endpoint is required")
	}

	m.mu.Lock()
	if existing, ok := m.peers[peerID]; ok {
		switch existing.status.State {
		case StateConnected:
			m.mu.Unlock()
			return nil
		case StateConnecting:
			m.mu.Unlock()
			return nil
		}
	}
	connectCtx, cancel := context.WithCancel(ctx)
	state := &peerState{
		status: PeerStatus{
			PeerID:    peerID,
			Endpoint:  endpoint,
			State:     StateConnecting,
			UpdatedAt: time.Now().UTC(),
		},
		cancel: cancel,
	}
	m.peers[peerID] = state
	m.mu.Unlock()

	go m.run(connectCtx, peerID, endpoint, state)
	return nil
}

func (m *Manager) run(ctx context.Context, peerID, endpoint string, state *peerState) {
	dialCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	conn, err := m.dialer.DialContext(dialCtx, "tcp", endpoint)
	if err != nil {
		m.markState(peerID, StateFailed)
		_ = conn
		return
	}
	_ = conn.Close()

	m.markState(peerID, StateConnected)
}

func (m *Manager) markState(peerID string, next State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.peers[peerID]
	if !ok {
		return
	}
	state.status.State = next
	state.status.UpdatedAt = time.Now().UTC()
}

func (m *Manager) Disconnect(peerID string) error {
	peerID = trim(peerID)
	if peerID == "" {
		return errors.New("peer id is required")
	}

	m.mu.Lock()
	state, ok := m.peers[peerID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	if state.cancel != nil {
		state.cancel()
	}
	delete(m.peers, peerID)
	m.mu.Unlock()
	return nil
}

func (m *Manager) Status(peerID string) (PeerStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.peers[peerID]
	if !ok {
		return PeerStatus{}, false
	}
	return state.status, true
}

func (m *Manager) Snapshot() []PeerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PeerStatus, 0, len(m.peers))
	for _, state := range m.peers {
		out = append(out, state.status)
	}
	return out
}

func (m *Manager) Forget(peerID string) {
	m.mu.Lock()
	delete(m.peers, peerID)
	m.mu.Unlock()
}

func trim(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\t' || value[0] == '\n' || value[0] == '\r') {
		value = value[1:]
	}
	for len(value) > 0 {
		last := value[len(value)-1]
		if last != ' ' && last != '\t' && last != '\n' && last != '\r' {
			break
		}
		value = value[:len(value)-1]
	}
	return value
}

func ValidateEndpoint(endpoint string) error {
	if trim(endpoint) == "" {
		return errors.New("endpoint is required")
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint: %w", err)
	}
	if trim(host) == "" {
		return errors.New("endpoint host is required")
	}
	if trim(port) == "" {
		return errors.New("endpoint port is required")
	}
	return nil
}
