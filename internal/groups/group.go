package groups

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

var (
	ErrGroupNotFound      = errors.New("group not found")
	ErrGroupAlreadyExists = errors.New("group already exists")
	ErrInvalidGroupID     = errors.New("group id is required")
	ErrInvalidGroupName   = errors.New("group name is required")
	ErrInvalidPeerID      = errors.New("peer id is required")
	ErrMemberAlreadyAdded = errors.New("member already added to group")
	ErrMemberNotFound     = errors.New("member not found in group")
	ErrDuplicateMessage   = errors.New("duplicate group message id")
)

// Group represents a decentralized chat group. Members are tracked by peer ID and
// are expected to be connected peers on the network. Group messages reuse the
// existing protocol.Message so there is no duplicate message schema.
type Group struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Members   map[string]GroupMember `json:"members"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// GroupMember is a lightweight view of a peer's membership in a group.
type GroupMember struct {
	PeerID   string    `json:"peer_id"`
	JoinedAt time.Time `json:"joined_at"`
	Active   bool      `json:"active"`
	LastSeen time.Time `json:"last_seen,omitempty"`
}

// GroupMembership is kept as a compatibility alias for callers that prefer the
// more explicit model name.
type GroupMembership = GroupMember

// GroupMessage wraps the message protocol while keeping the group context explicit.
type GroupMessage struct {
	GroupID string           `json:"group_id"`
	Message protocol.Message `json:"message"`
}

// Manager owns group state and deduplicates group message IDs per group.
type Manager struct {
	mu     sync.RWMutex
	groups map[string]*Group
	seen   map[string]map[string]struct{}
}

func NewManager() *Manager {
	return &Manager{
		groups: make(map[string]*Group),
		seen:   make(map[string]map[string]struct{}),
	}
}

func (m *Manager) CreateGroup(groupID, name string) (Group, error) {
	groupID = strings.TrimSpace(groupID)
	name = strings.TrimSpace(name)
	if groupID == "" {
		return Group{}, ErrInvalidGroupID
	}
	if name == "" {
		return Group{}, ErrInvalidGroupName
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.groups[groupID]; exists {
		return Group{}, fmt.Errorf("%w: %s", ErrGroupAlreadyExists, groupID)
	}

	now := time.Now().UTC()
	group := &Group{
		ID:        groupID,
		Name:      name,
		Members:   make(map[string]GroupMember),
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.groups[groupID] = group
	m.seen[groupID] = make(map[string]struct{})
	return *group, nil
}

func (m *Manager) GetGroup(groupID string) (Group, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	group, exists := m.groups[strings.TrimSpace(groupID)]
	if !exists || group == nil {
		return Group{}, false
	}
	clone := *group
	clone.Members = make(map[string]GroupMember, len(group.Members))
	for peerID, member := range group.Members {
		clone.Members[peerID] = member
	}
	return clone, true
}

func (m *Manager) AddMember(groupID, peerID string) error {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" {
		return ErrInvalidGroupID
	}
	if peerID == "" {
		return ErrInvalidPeerID
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	group, exists := m.groups[groupID]
	if !exists || group == nil {
		return fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
	}
	if _, exists = group.Members[peerID]; exists {
		return fmt.Errorf("%w: %s", ErrMemberAlreadyAdded, peerID)
	}

	now := time.Now().UTC()
	group.Members[peerID] = GroupMember{
		PeerID:   peerID,
		JoinedAt: now,
		Active:   true,
		LastSeen: now,
	}
	group.UpdatedAt = now
	return nil
}

func (m *Manager) RemoveMember(groupID, peerID string) error {
	return m.removeMember(groupID, peerID)
}

func (m *Manager) LeaveGroup(groupID, peerID string) error {
	return m.removeMember(groupID, peerID)
}

func (m *Manager) removeMember(groupID, peerID string) error {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" {
		return ErrInvalidGroupID
	}
	if peerID == "" {
		return ErrInvalidPeerID
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	group, exists := m.groups[groupID]
	if !exists || group == nil {
		return fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
	}
	if _, exists = group.Members[peerID]; !exists {
		return fmt.Errorf("%w: %s", ErrMemberNotFound, peerID)
	}
	delete(group.Members, peerID)
	group.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *Manager) ViewMembers(groupID string) ([]string, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, ErrInvalidGroupID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	group, exists := m.groups[groupID]
	if !exists || group == nil {
		return nil, fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
	}
	members := make([]string, 0, len(group.Members))
	for peerID := range group.Members {
		members = append(members, peerID)
	}
	sort.Strings(members)
	return members, nil
}

func (m *Manager) ListGroups() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	groups := make([]string, 0, len(m.groups))
	for groupID := range m.groups {
		groups = append(groups, groupID)
	}
	sort.Strings(groups)
	return groups
}

func (m *Manager) IsMember(groupID, peerID string) bool {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" || peerID == "" {
		return false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	group, exists := m.groups[groupID]
	if !exists || group == nil {
		return false
	}
	_, exists = group.Members[peerID]
	return exists
}

func (m *Manager) SendGroupMessage(groupID, senderID, body string) (protocol.Message, error) {
	groupID = strings.TrimSpace(groupID)
	senderID = strings.TrimSpace(senderID)
	if groupID == "" {
		return protocol.Message{}, ErrInvalidGroupID
	}
	if senderID == "" {
		return protocol.Message{}, ErrInvalidPeerID
	}

	m.mu.RLock()
	group, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists || group == nil {
		return protocol.Message{}, fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
	}
	if _, exists = group.Members[senderID]; !exists {
		return protocol.Message{}, fmt.Errorf("%w: %s", ErrMemberNotFound, senderID)
	}
	if strings.TrimSpace(body) == "" {
		return protocol.Message{}, errors.New("group message body is required")
	}

	return protocol.NewChatMessage(senderID, "", groupID, body), nil
}

// RouteGroupMessage sends a group chat message to connected members only.
// Unknown groups and duplicate message IDs are rejected to avoid repeated delivery.
func (m *Manager) RouteGroupMessage(message protocol.Message, connected map[string]bool, deliver func(protocol.Message, string) error) ([]string, error) {
	if strings.TrimSpace(message.GroupID) == "" {
		return nil, ErrInvalidGroupID
	}
	if strings.TrimSpace(message.ID) == "" {
		return nil, errors.New("message id is required")
	}
	if strings.TrimSpace(message.SenderID) == "" {
		return nil, ErrInvalidPeerID
	}

	m.mu.Lock()
	group, exists := m.groups[message.GroupID]
	if !exists || group == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrGroupNotFound, message.GroupID)
	}
	if _, seen := m.seen[message.GroupID][message.ID]; seen {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrDuplicateMessage, message.ID)
	}
	m.seen[message.GroupID][message.ID] = struct{}{}
	m.mu.Unlock()

	// Route only to remaining active members that are connected and are not the sender.
	members, err := m.ViewMembers(message.GroupID)
	if err != nil {
		return nil, err
	}

	delivered := make([]string, 0, len(members))
	for _, peerID := range members {
		if peerID == message.SenderID {
			continue
		}
		if connected != nil && !connected[peerID] {
			continue
		}
		if deliver == nil {
			continue
		}
		if err := deliver(message, peerID); err != nil {
			continue
		}
		delivered = append(delivered, peerID)
	}
	return delivered, nil
}

func (m *Manager) MarkMessageSeen(groupID, messageID string) error {
	groupID = strings.TrimSpace(groupID)
	messageID = strings.TrimSpace(messageID)
	if groupID == "" {
		return ErrInvalidGroupID
	}
	if messageID == "" {
		return errors.New("message id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.groups[groupID]; !exists {
		return fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
	}
	if m.seen[groupID] == nil {
		m.seen[groupID] = make(map[string]struct{})
	}
	m.seen[groupID][messageID] = struct{}{}
	return nil
}

func (m *Manager) IsDuplicate(groupID, messageID string) bool {
	groupID = strings.TrimSpace(groupID)
	messageID = strings.TrimSpace(messageID)
	if groupID == "" || messageID == "" {
		return false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.seen[groupID] == nil {
		return false
	}
	_, exists := m.seen[groupID][messageID]
	return exists
}
