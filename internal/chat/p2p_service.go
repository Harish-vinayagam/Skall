package chat

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/Harish-vinayagam/Skall/internal/groups"
	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/network/p2p"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
	"github.com/Harish-vinayagam/Skall/internal/storage"
)

const (
	subscriberBuf = 64 // per-subscriber channel buffer
	maxSubs       = 16 // maximum concurrent subscribers

	// maxPeerIDLen is the maximum byte length we accept for a peer ID string
	// written into the local store. Protects against absurdly long IDs from
	// malicious peers polluting the database.
	maxPeerIDLen = 512
)

// ErrTooManySubscribers is returned by Subscribe when the subscriber cap has
// been reached. Callers should close an existing subscription before opening
// a new one.
var ErrTooManySubscribers = errors.New("too many subscribers: limit reached")

// P2PService is the production implementation of ChatService.
// It wires together the libp2p host, SQLite store, and groups manager.
type P2PService struct {
	local  identity.Identity
	host   p2p.Host
	store  *storage.Store
	groups *groups.Manager
	ctx    context.Context
	cancel context.CancelFunc

	// peer display-name cache (peerID → displayName)
	peerNames   map[string]string
	peerNamesMu sync.RWMutex

	// subscriber fan-out
	subsMu sync.Mutex
	subs   []chan Event

	statusMu sync.RWMutex
	status   ConnectionStatus
}

// NewP2PService creates a service, attaches the inbound message handler to
// host, and starts the background peer-monitor goroutine.
func NewP2PService(ctx context.Context, local identity.Identity, host p2p.Host, store *storage.Store, grpMgr *groups.Manager) *P2PService {
	ctx, cancel := context.WithCancel(ctx)
	svc := &P2PService{
		local:     local,
		host:      host,
		store:     store,
		groups:    grpMgr,
		ctx:       ctx,
		cancel:    cancel,
		peerNames: make(map[string]string),
		subs:      make([]chan Event, 0, 4),
		status:    StatusConnecting,
	}

	// Attach inbound message handler
	host.SetMessageHandler(svc.handleInbound)

	// Monitor peer connectivity
	go svc.monitorPeers()

	return svc
}

// --- ChatService implementation ---

func (s *P2PService) LocalIdentity() identity.Identity { return s.local }

func (s *P2PService) ConnectionStatus() ConnectionStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.status
}

func (s *P2PService) ConnectedPeerCount() int {
	return len(s.host.ConnectedPeers())
}

func (s *P2PService) ListConversations() ([]Conversation, error) {
	peers, err := s.store.ListPeers()
	if err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}

	myLibPID := ""
	if lp2pID, err := s.local.LibP2PPeerID(); err == nil {
		myLibPID = lp2pID.String()
	}

	convs := make([]Conversation, 0, len(peers))
	for _, p := range peers {
		if p.PeerID == s.local.PeerID || (myLibPID != "" && p.PeerID == myLibPID) {
			continue
		}
		msgs, err := s.store.ListDirectConversation(myLibPID, p.PeerID, 1)
		if (err != nil || len(msgs) == 0) && myLibPID != s.local.PeerID {
			msgs, _ = s.store.ListDirectConversation(s.local.PeerID, p.PeerID, 1)
		}
		lastMsg := ""
		lastAt := p.LastSeen
		if len(msgs) > 0 {
			lastMsg = msgs[0].Message.Body
			lastAt = msgs[0].Message.Timestamp
		}
		convs = append(convs, Conversation{
			ID:          p.PeerID,
			Name:        displayName(p.PeerID, p.DisplayName),
			Type:        ConversationDirect,
			LastMessage: lastMsg,
			LastAt:      lastAt,
		})
	}

	// Append group conversations
	grpIDs := s.groups.ListGroups()
	for _, gID := range grpIDs {
		g, ok := s.groups.GetGroup(gID)
		if !ok {
			continue
		}
		msgs, err := s.store.ListGroupConversation(gID, 1)
		lastMsg := ""
		lastAt := g.CreatedAt
		if err == nil && len(msgs) > 0 {
			lastMsg = msgs[0].Message.Body
			lastAt = msgs[0].Message.Timestamp
		}
		convs = append(convs, Conversation{
			ID:          gID,
			Name:        g.Name,
			Type:        ConversationGroup,
			LastMessage: lastMsg,
			LastAt:      lastAt,
		})
	}

	return convs, nil
}

func (s *P2PService) ListGroups() ([]Group, error) {
	ids := s.groups.ListGroups()
	out := make([]Group, 0, len(ids))
	for _, id := range ids {
		g, ok := s.groups.GetGroup(id)
		if !ok {
			continue
		}
		out = append(out, Group{
			GroupID:     g.ID,
			Name:        g.Name,
			MemberCount: len(g.Members),
			CreatedAt:   g.CreatedAt,
		})
	}
	return out, nil
}

func (s *P2PService) ListPeers() ([]Peer, error) {
	stored, err := s.store.ListPeers()
	if err != nil {
		return nil, err
	}

	// Build connected set from libp2p
	connected := s.connectedSet()

	out := make([]Peer, 0, len(stored))
	for _, p := range stored {
		if p.PeerID == s.local.PeerID {
			continue
		}
		_, isConn := connected[p.PeerID]
		out = append(out, Peer{
			PeerID:      p.PeerID,
			DisplayName: displayName(p.PeerID, p.DisplayName),
			Connected:   isConn,
			LastSeen:    p.LastSeen,
		})
	}
	return out, nil
}

func (s *P2PService) GetMessages(conversationID string, limit int) ([]DisplayMessage, error) {
	if limit <= 0 {
		limit = 100
	}

	// Try as a group first
	if _, ok := s.groups.GetGroup(conversationID); ok {
		msgs, err := s.store.ListGroupConversation(conversationID, limit)
		if err != nil {
			return nil, err
		}
		return s.toDisplayMessages(msgs), nil
	}

	// Otherwise direct conversation
	myID := s.local.PeerID
	if lp2pID, err := s.local.LibP2PPeerID(); err == nil {
		myID = lp2pID.String()
	}
	msgs, err := s.store.ListDirectConversation(myID, conversationID, limit)
	if (err != nil || len(msgs) == 0) && myID != s.local.PeerID {
		if msgs2, err2 := s.store.ListDirectConversation(s.local.PeerID, conversationID, limit); err2 == nil && len(msgs2) > 0 {
			msgs = msgs2
		}
	}
	if err != nil && len(msgs) == 0 {
		return nil, err
	}
	return s.toDisplayMessages(msgs), nil
}

func (s *P2PService) SendDirect(peerID, body string) error {
	senderID := s.local.PeerID
	if lp2pID, err := s.local.LibP2PPeerID(); err == nil {
		senderID = lp2pID.String()
	}
	msg := protocol.NewChatMessage(senderID, peerID, "", body)

	// Persist outbound
	if err := s.store.InsertMessage(msg, storage.DirectionOutbound, storage.StatusPending); err != nil && err != storage.ErrDuplicateMessage {
		log.Printf("chat: persist outbound: %v", err)
	}
	// Cache peer name
	s.setPeerName(peerID, peerID)

	// Resolve libp2p peer ID and send
	libID, err := s.skallToLibP2P(peerID)
	if err != nil {
		_ = s.store.UpdateMessageStatus(msg.ID, storage.StatusFailed)
		return fmt.Errorf("resolve peer: %w", err)
	}
	if err := s.host.SendMessage(s.ctx, libID, msg); err != nil {
		_ = s.store.UpdateMessageStatus(msg.ID, storage.StatusFailed)
		return fmt.Errorf("send message: %w", err)
	}
	_ = s.store.UpdateMessageStatus(msg.ID, storage.StatusSent)

	// Notify subscribers
	s.publish(Event{
		Kind:           EventNewMessage,
		ConversationID: peerID,
		Message:        s.storedToDisplay(storage.StoredMessage{Message: msg, Direction: storage.DirectionOutbound, Status: storage.StatusSent}),
	})
	return nil
}

func (s *P2PService) SendGroup(groupID, body string) error {
	msg, err := s.groups.SendGroupMessage(groupID, s.local.PeerID, body)
	if err != nil {
		return err
	}

	// Persist
	if err := s.store.InsertMessage(msg, storage.DirectionOutbound, storage.StatusPending); err != nil && err != storage.ErrDuplicateMessage {
		log.Printf("chat: persist group outbound: %v", err)
	}

	// Fan out to connected members
	members, _ := s.groups.ViewMembers(groupID)
	connected := s.connectedSet()
	for _, memberID := range members {
		if memberID == s.local.PeerID {
			continue
		}
		libID, err := s.skallToLibP2P(memberID)
		if err != nil {
			continue
		}
		if _, ok := connected[memberID]; !ok {
			continue
		}
		if err := s.host.SendMessage(s.ctx, libID, msg); err != nil {
			log.Printf("chat: send group msg to %s: %v", memberID, err)
		}
	}
	_ = s.store.UpdateMessageStatus(msg.ID, storage.StatusSent)

	s.publish(Event{
		Kind:           EventNewMessage,
		ConversationID: groupID,
		Message:        s.storedToDisplay(storage.StoredMessage{Message: msg, Direction: storage.DirectionOutbound, Status: storage.StatusSent}),
	})
	return nil
}

// CreateGroup creates a new group in the in-memory manager and persists it to
// the SQLite store so it survives a restart.
func (s *P2PService) CreateGroup(groupID, name string) error {
	g, err := s.groups.CreateGroup(groupID, name)
	if err != nil {
		return err
	}
	if err := s.store.UpsertGroup(g.ID, g.Name, g.CreatedAt); err != nil {
		return fmt.Errorf("chat: persist group: %w", err)
	}
	return nil
}

// JoinGroup adds peerID as an active member of groupID in both the in-memory
// manager and the SQLite store.
// If peerID is already a member of the in-memory group the AddMember call
// returns ErrMemberAlreadyAdded; the store is still updated (upsert) so the
// membership is guaranteed to be active in the database.
func (s *P2PService) JoinGroup(groupID, peerID string) error {
	now := s.now()

	// Try to add in memory; ignore ErrMemberAlreadyAdded (idempotent).
	if err := s.groups.AddMember(groupID, peerID); err != nil && err.Error() != groups.ErrMemberAlreadyAdded.Error() {
		// Re-check with errors.Is since the error is wrapped with the peer ID.
		if !isErrMemberAlreadyAdded(err) {
			return err
		}
	}

	// Always persist — upsert ensures the membership is marked active.
	if err := s.store.SetGroupMembership(groupID, peerID, now, true, now); err != nil {
		return fmt.Errorf("chat: persist membership: %w", err)
	}
	return nil
}

// LeaveGroup removes peerID from the in-memory member set and marks the
// membership inactive in the SQLite store (active=false). The row is kept so
// that message history is retained.
func (s *P2PService) LeaveGroup(groupID, peerID string) error {
	// Remove from in-memory manager; ignore ErrMemberNotFound (idempotent).
	if err := s.groups.LeaveGroup(groupID, peerID); err != nil {
		if !isErrMemberNotFound(err) {
			return err
		}
	}

	// Mark inactive in the store (upsert with active=false).
	now := s.now()
	if err := s.store.SetGroupMembership(groupID, peerID, now, false, now); err != nil {
		return fmt.Errorf("chat: persist leave membership: %w", err)
	}
	return nil
}

// DeleteGroup removes a group from both the in-memory manager and the SQLite
// store. The store deletion cascades to group_memberships rows.
func (s *P2PService) DeleteGroup(groupID string) error {
	// Remove from memory first; ignore ErrGroupNotFound (idempotent).
	if err := s.groups.DeleteGroup(groupID); err != nil {
		if !isErrGroupNotFound(err) {
			return err
		}
	}
	if err := s.store.DeleteGroup(groupID); err != nil {
		return fmt.Errorf("chat: delete group from store: %w", err)
	}
	return nil
}

func (s *P2PService) Subscribe() <-chan Event {
	ch := make(chan Event, subscriberBuf)
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	if len(s.subs) >= maxSubs {
		// Return a closed channel so callers can detect the failure without
		// blocking. The error is also logged; callers that need the error
		// value should use SubscribeE instead.
		log.Printf("chat: subscriber cap (%d) reached; rejecting new subscription", maxSubs)
		close(ch)
		return ch
	}
	s.subs = append(s.subs, ch)
	return ch
}

// SubscribeE is like Subscribe but returns an error when the subscriber cap has
// been reached instead of a closed channel.
func (s *P2PService) SubscribeE() (<-chan Event, error) {
	ch := make(chan Event, subscriberBuf)
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	if len(s.subs) >= maxSubs {
		return nil, ErrTooManySubscribers
	}
	s.subs = append(s.subs, ch)
	return ch, nil
}

// Unsubscribe removes the channel returned by Subscribe from the fan-out list
// and closes it. Safe to call multiple times; subsequent calls are no-ops.
func (s *P2PService) Unsubscribe(ch <-chan Event) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for i, sub := range s.subs {
		if sub == ch {
			s.subs = append(s.subs[:i], s.subs[i+1:]...)
			close(sub)
			return
		}
	}
}

func (s *P2PService) Close() error {
	s.cancel()
	s.subsMu.Lock()
	for _, ch := range s.subs {
		close(ch)
	}
	s.subs = nil
	s.subsMu.Unlock()
	return s.host.Close()
}

// --- internal helpers ---

// handleInbound is registered as the p2p.MessageHandler.
func (s *P2PService) handleInbound(msg protocol.Message, from libp2ppeer.ID) {
	// Security: always use the libp2p-authenticated peer ID as the canonical
	// sender identity. The msg.SenderID field is supplied by the remote peer
	// and must not be trusted for attribution — a connected peer could set it
	// to any value, including another peer's ID, to impersonate them at the
	// application layer. Libp2p Noise verifies the transport identity; we use
	// that as the ground truth and log a mismatch for auditability.
	authenticatedID := from.String()
	if msg.SenderID != authenticatedID {
		log.Printf("chat: peer %s claimed SenderID %q — overriding with authenticated ID",
			authenticatedID, msg.SenderID)
	}
	senderID := authenticatedID

	// Security: guard against absurdly long peer IDs before writing to the store.
	if len(senderID) > maxPeerIDLen {
		log.Printf("chat: rejecting inbound message: authenticated peer ID too long (%d bytes)", len(senderID))
		return
	}

	// Persist
	if err := s.store.UpsertPeer(senderID, senderID, time.Now()); err != nil {
		log.Printf("chat: upsert peer: %v", err)
	}
	if err := s.store.InsertMessage(msg, storage.DirectionInbound, storage.StatusReceived); err != nil && err != storage.ErrDuplicateMessage {
		log.Printf("chat: persist inbound: %v", err)
	}

	convID := senderID
	if msg.GroupID != "" {
		convID = msg.GroupID
	}

	dm := DisplayMessage{
		ID:         msg.ID,
		SenderID:   senderID,
		SenderName: s.getPeerName(senderID),
		Body:       msg.Body,
		Timestamp:  msg.Timestamp,
		IsOutbound: false,
	}

	s.publish(Event{
		Kind:           EventNewMessage,
		ConversationID: convID,
		Message:        dm,
	})
}

// monitorPeers periodically checks connected peers, persists them into the
// store so they appear in ListPeers(), and emits status/peer events.
func (s *P2PService) monitorPeers() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastCount int
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			connectedPeers := s.host.ConnectedPeers()
			count := len(connectedPeers)

			// Upsert every connected libp2p peer into the store so they
			// show up in ListPeers() even before a message is exchanged.
			for _, pid := range connectedPeers {
				pidStr := pid.String()
				if err := s.store.UpsertPeer(pidStr, pidStr, time.Now()); err != nil {
					log.Printf("chat: monitorPeers upsert %s: %v", pidStr, err)
				}
			}

			newStatus := StatusConnecting
			if count > 0 {
				newStatus = StatusConnected
			}
			s.statusMu.Lock()
			changed := s.status != newStatus
			s.status = newStatus
			s.statusMu.Unlock()

			if changed {
				s.publish(Event{Kind: EventStatusChanged, Status: newStatus})
			}
			if count != lastCount {
				lastCount = count
				s.publish(Event{Kind: EventPeerConnected})
			}
		}
	}
}

func (s *P2PService) publish(ev Event) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- ev:
		default:
			// Drop rather than block
		}
	}
}

func (s *P2PService) connectedSet() map[string]struct{} {
	peers := s.host.ConnectedPeers()
	set := make(map[string]struct{}, len(peers))
	for _, p := range peers {
		set[p.String()] = struct{}{}
	}
	return set
}

// skallToLibP2P resolves a SKALL PeerID to a libp2p peer.ID by searching
// currently connected libp2p peers. This is a best-effort lookup.
func (s *P2PService) skallToLibP2P(skallPeerID string) (libp2ppeer.ID, error) {
	// Check if it is already a valid libp2p peer.ID string
	if id, err := libp2ppeer.Decode(skallPeerID); err == nil {
		return id, nil
	}
	return "", fmt.Errorf("cannot resolve skall peer id %q to libp2p peer.ID: no address book", skallPeerID)
}

func (s *P2PService) toDisplayMessages(msgs []storage.StoredMessage) []DisplayMessage {
	out := make([]DisplayMessage, 0, len(msgs))
	for _, sm := range msgs {
		out = append(out, s.storedToDisplay(sm))
	}
	return out
}

func (s *P2PService) storedToDisplay(sm storage.StoredMessage) DisplayMessage {
	name := s.getPeerName(sm.Message.SenderID)
	lp2pID, _ := s.local.LibP2PPeerID()
	if sm.Message.SenderID == s.local.PeerID || (lp2pID != "" && sm.Message.SenderID == lp2pID.String()) {
		name = "You"
	}
	return DisplayMessage{
		ID:         sm.Message.ID,
		SenderID:   sm.Message.SenderID,
		SenderName: name,
		Body:       sm.Message.Body,
		Timestamp:  sm.Message.Timestamp,
		IsOutbound: sm.Direction == storage.DirectionOutbound,
	}
}

func (s *P2PService) getPeerName(peerID string) string {
	s.peerNamesMu.RLock()
	name, ok := s.peerNames[peerID]
	s.peerNamesMu.RUnlock()
	if ok && name != "" {
		return name
	}
	return shortID(peerID)
}

func (s *P2PService) setPeerName(peerID, name string) {
	s.peerNamesMu.Lock()
	s.peerNames[peerID] = name
	s.peerNamesMu.Unlock()
}

func displayName(peerID, storedName string) string {
	if storedName != "" && storedName != peerID {
		return storedName
	}
	return shortID(peerID)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}

// now returns the current UTC time. It is a method so tests can override it
// via composition in the future; for now it simply wraps time.Now().
func (s *P2PService) now() time.Time {
	return time.Now().UTC()
}

// isErrMemberAlreadyAdded returns true when err is (or wraps) ErrMemberAlreadyAdded.
func isErrMemberAlreadyAdded(err error) bool {
	return errors.Is(err, groups.ErrMemberAlreadyAdded)
}

// isErrMemberNotFound returns true when err is (or wraps) ErrMemberNotFound.
func isErrMemberNotFound(err error) bool {
	return errors.Is(err, groups.ErrMemberNotFound)
}

// isErrGroupNotFound returns true when err is (or wraps) ErrGroupNotFound.
func isErrGroupNotFound(err error) bool {
	return errors.Is(err, groups.ErrGroupNotFound)
}
