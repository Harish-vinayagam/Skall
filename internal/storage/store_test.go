package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

func TestSchemaInitialization(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	expected := []string{"schema_migrations", "identities", "peers", "groups", "group_memberships", "messages"}
	for _, table := range expected {
		var name string
		err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?;`, table).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %s to exist: %v", table, err)
		}
	}
}

func TestInsertAndRetrieveMessages(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	ts1 := time.Unix(1710000000, 0).UTC()
	ts2 := ts1.Add(2 * time.Second)
	outbound := protocol.Message{
		Version:     protocol.Version,
		ID:          "m-out-1",
		Type:        protocol.TypeChat,
		SenderID:    "alice",
		RecipientID: "bob",
		Timestamp:   ts1,
		Body:        "hello",
	}
	inbound := protocol.Message{
		Version:     protocol.Version,
		ID:          "m-in-1",
		Type:        protocol.TypeChat,
		SenderID:    "bob",
		RecipientID: "alice",
		Timestamp:   ts2,
		Body:        "hi",
	}

	if err := store.InsertMessage(outbound, DirectionOutbound, StatusSent); err != nil {
		t.Fatalf("InsertMessage(outbound) error = %v", err)
	}
	if err := store.InsertMessage(inbound, DirectionInbound, StatusReceived); err != nil {
		t.Fatalf("InsertMessage(inbound) error = %v", err)
	}

	history, err := store.ListDirectConversation("alice", "bob", 50)
	if err != nil {
		t.Fatalf("ListDirectConversation() error = %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(history))
	}
	if history[0].Message.ID != outbound.ID || history[1].Message.ID != inbound.ID {
		t.Fatalf("unexpected conversation order: %+v", history)
	}

	stored, err := store.GetMessage(outbound.ID)
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}
	if stored.Status != StatusSent || stored.Direction != DirectionOutbound {
		t.Fatalf("stored metadata mismatch: %+v", stored)
	}
}

func TestPeerPersistence(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	seenAt := time.Unix(1710001000, 0).UTC()
	if err := store.UpsertPeer("peer-1", "Alice", seenAt); err != nil {
		t.Fatalf("UpsertPeer() error = %v", err)
	}

	peers, err := store.ListPeers()
	if err != nil {
		t.Fatalf("ListPeers() error = %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].PeerID != "peer-1" || peers[0].DisplayName != "Alice" {
		t.Fatalf("peer mismatch: %+v", peers[0])
	}
}

func TestGroupPersistence(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	created := time.Unix(1710002000, 0).UTC()
	if err := store.UpsertGroup("g-1", "Friends", created); err != nil {
		t.Fatalf("UpsertGroup() error = %v", err)
	}
	if err := store.SetGroupMembership("g-1", "alice", created, true, created); err != nil {
		t.Fatalf("SetGroupMembership() error = %v", err)
	}

	groups, err := store.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	if len(groups) != 1 || groups[0].GroupID != "g-1" {
		t.Fatalf("group mismatch: %+v", groups)
	}

	members, err := store.ListGroupMembers("g-1")
	if err != nil {
		t.Fatalf("ListGroupMembers() error = %v", err)
	}
	if len(members) != 1 || members[0].PeerID != "alice" || !members[0].Active {
		t.Fatalf("group membership mismatch: %+v", members)
	}
}

func TestDuplicateMessageHandling(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	msg := protocol.Message{
		Version:     protocol.Version,
		ID:          "dupe-1",
		Type:        protocol.TypeChat,
		SenderID:    "alice",
		RecipientID: "bob",
		Timestamp:   time.Unix(1710003000, 0).UTC(),
		Body:        "first",
	}
	if err := store.InsertMessage(msg, DirectionOutbound, StatusSent); err != nil {
		t.Fatalf("InsertMessage() initial error = %v", err)
	}
	if err := store.InsertMessage(msg, DirectionOutbound, StatusSent); !errors.Is(err, ErrDuplicateMessage) {
		t.Fatalf("expected ErrDuplicateMessage, got %v", err)
	}
}

func TestMigrationBehavior(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "migration.db")

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);`); err != nil {
		t.Fatalf("create schema_migrations error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, name, applied_at) VALUES (0, 'bootstrap', ?);`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert baseline migration error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close preseed db error = %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	var version int
	if err := store.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations;`).Scan(&version); err != nil {
		t.Fatalf("query migrated version error = %v", err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("unexpected schema version: got %d want %d", version, currentSchemaVersion)
	}
}

func TestCorruptedDatabaseHandling(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "corrupt.db")
	if err := os.WriteFile(path, []byte("not-a-real-sqlite-db"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Open(path)
	if !errors.Is(err, ErrCorruptedDatabase) {
		t.Fatalf("expected ErrCorruptedDatabase, got %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "skall-test.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return store
}

func TestGetMessage_NotFound(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetMessage("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent message")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestUpdateMessageStatus_NotFound(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	err := store.UpdateMessageStatus("nonexistent-id", StatusSent)
	if err == nil {
		t.Fatal("expected error for nonexistent message status update")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestUpdateMessageStatus_Valid(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	msg := protocol.Message{
		Version:     protocol.Version,
		ID:          "m-update-1",
		Type:        protocol.TypeChat,
		SenderID:    "alice",
		RecipientID: "bob",
		Timestamp:   time.Now().UTC(),
		Body:        "update me",
	}

	if err := store.InsertMessage(msg, DirectionOutbound, StatusPending); err != nil {
		t.Fatalf("InsertMessage() error = %v", err)
	}

	if err := store.UpdateMessageStatus(msg.ID, StatusDelivered); err != nil {
		t.Fatalf("UpdateMessageStatus() error = %v", err)
	}

	stored, err := store.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}
	if stored.Status != StatusDelivered {
		t.Fatalf("expected status %s, got %s", StatusDelivered, stored.Status)
	}
}

func TestListGroupConversation_Basic(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	t0 := time.Unix(1710000000, 0).UTC()
	for i := 1; i <= 3; i++ {
		msg := protocol.Message{
			Version:   protocol.Version,
			ID:        fmt.Sprintf("grp-msg-%d", i),
			Type:      protocol.TypeChat,
			SenderID:  fmt.Sprintf("member-%d", i),
			GroupID:   "group-42",
			Timestamp: t0.Add(time.Duration(i) * time.Second),
			Body:      fmt.Sprintf("hello from %d", i),
		}
		if err := store.InsertMessage(msg, DirectionInbound, StatusReceived); err != nil {
			t.Fatalf("InsertMessage(grp-msg-%d) error = %v", i, err)
		}
	}

	history, err := store.ListGroupConversation("group-42", 10)
	if err != nil {
		t.Fatalf("ListGroupConversation() error = %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 group messages, got %d", len(history))
	}
	for i, sm := range history {
		expectedID := fmt.Sprintf("grp-msg-%d", i+1)
		if sm.Message.ID != expectedID {
			t.Errorf("history[%d] ID=%s, want %s", i, sm.Message.ID, expectedID)
		}
	}
}

func TestConcurrentInserts(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	const count = 30
	var wg sync.WaitGroup
	wg.Add(count)

	t0 := time.Now().UTC()
	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			msg := protocol.Message{
				Version:     protocol.Version,
				ID:          fmt.Sprintf("concurrent-msg-%d", idx),
				Type:        protocol.TypeChat,
				SenderID:    "alice",
				RecipientID: "bob",
				Timestamp:   t0.Add(time.Duration(idx) * time.Millisecond),
				Body:        fmt.Sprintf("concurrent payload %d", idx),
			}
			if err := store.InsertMessage(msg, DirectionOutbound, StatusSent); err != nil {
				t.Errorf("goroutine %d InsertMessage failed: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	history, err := store.ListDirectConversation("alice", "bob", 100)
	if err != nil {
		t.Fatalf("ListDirectConversation error: %v", err)
	}
	if len(history) != count {
		t.Fatalf("expected %d messages, got %d", count, len(history))
	}
}

func TestConcurrentPeerUpserts(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	const count = 20
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			peerID := fmt.Sprintf("peer-%d", idx%5) // high collision to test ON CONFLICT
			name := fmt.Sprintf("Peer %d", idx)
			if err := store.UpsertPeer(peerID, name, time.Now().UTC()); err != nil {
				t.Errorf("UpsertPeer goroutine %d error: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	peers, err := store.ListPeers()
	if err != nil {
		t.Fatalf("ListPeers error: %v", err)
	}
	if len(peers) != 5 {
		t.Fatalf("expected 5 unique peers, got %d", len(peers))
	}
}

func TestStore_CloseIdempotent(t *testing.T) {
	store := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestOpen_EmptyPath(t *testing.T) {
	if _, err := Open("   "); err == nil {
		t.Fatal("expected error for empty database path")
	}
}

func TestUpsertIdentityMetadata(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if err := store.UpsertIdentityMetadata(id); err != nil {
		t.Fatalf("UpsertIdentityMetadata() error = %v", err)
	}
	// Upsert again to test update path
	id.DisplayName = "Updated Name"
	if err := store.UpsertIdentityMetadata(id); err != nil {
		t.Fatalf("second UpsertIdentityMetadata() error = %v", err)
	}
}

// TestGetGroup verifies that a persisted group can be fetched by ID.
func TestGetGroup(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	created := time.Unix(1710010000, 0).UTC()
	if err := store.UpsertGroup("grp-get-1", "Get Me", created); err != nil {
		t.Fatalf("UpsertGroup() error = %v", err)
	}

	g, err := store.GetGroup("grp-get-1")
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if g.GroupID != "grp-get-1" || g.Name != "Get Me" {
		t.Fatalf("GetGroup() mismatch: %+v", g)
	}
}

// TestGetGroup_NotFound verifies sql.ErrNoRows is returned for missing groups.
func TestGetGroup_NotFound(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetGroup("does-not-exist")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

// TestDeleteGroup verifies that deleting a group removes it and cascades
// to the group_memberships table.
func TestDeleteGroup(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	created := time.Unix(1710011000, 0).UTC()
	if err := store.UpsertGroup("grp-del-1", "Delete Me", created); err != nil {
		t.Fatalf("UpsertGroup() error = %v", err)
	}
	if err := store.SetGroupMembership("grp-del-1", "alice", created, true, created); err != nil {
		t.Fatalf("SetGroupMembership() error = %v", err)
	}

	if err := store.DeleteGroup("grp-del-1"); err != nil {
		t.Fatalf("DeleteGroup() error = %v", err)
	}

	// Group must be gone.
	if _, err := store.GetGroup("grp-del-1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows after delete, got %v", err)
	}

	// Memberships must be gone (CASCADE).
	members, err := store.ListGroupMembers("grp-del-1")
	if err != nil {
		t.Fatalf("ListGroupMembers() after delete error = %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected 0 members after group delete, got %d", len(members))
	}
}

// TestDeleteGroup_NotFound verifies sql.ErrNoRows is returned when the group
// does not exist.
func TestDeleteGroup_NotFound(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	err := store.DeleteGroup("never-existed")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

// TestPaginatedMessages verifies that the limit parameter is honoured for both
// direct and group conversation queries.
func TestPaginatedMessages(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	t0 := time.Unix(1710020000, 0).UTC()

	// Insert 10 direct messages between alice and bob.
	for i := 1; i <= 10; i++ {
		msg := protocol.Message{
			Version:     protocol.Version,
			ID:          fmt.Sprintf("direct-page-%d", i),
			Type:        protocol.TypeChat,
			SenderID:    "alice",
			RecipientID: "bob",
			Timestamp:   t0.Add(time.Duration(i) * time.Second),
			Body:        fmt.Sprintf("direct msg %d", i),
		}
		if err := store.InsertMessage(msg, DirectionOutbound, StatusSent); err != nil {
			t.Fatalf("InsertMessage(direct-%d) error = %v", i, err)
		}
	}

	// Fetch with limit=5.
	page, err := store.ListDirectConversation("alice", "bob", 5)
	if err != nil {
		t.Fatalf("ListDirectConversation(limit=5) error = %v", err)
	}
	if len(page) != 5 {
		t.Fatalf("expected 5 messages with limit=5, got %d", len(page))
	}
	// Must be in chronological order (oldest first).
	if page[0].Message.ID != "direct-page-1" || page[4].Message.ID != "direct-page-5" {
		t.Fatalf("unexpected ordering: first=%s last=%s", page[0].Message.ID, page[4].Message.ID)
	}

	// Insert 8 group messages.
	for i := 1; i <= 8; i++ {
		msg := protocol.Message{
			Version:   protocol.Version,
			ID:        fmt.Sprintf("group-page-%d", i),
			Type:      protocol.TypeChat,
			SenderID:  fmt.Sprintf("member-%d", i),
			GroupID:   "grp-page",
			Timestamp: t0.Add(time.Duration(i) * time.Second),
			Body:      fmt.Sprintf("group msg %d", i),
		}
		if err := store.InsertMessage(msg, DirectionInbound, StatusReceived); err != nil {
			t.Fatalf("InsertMessage(group-%d) error = %v", i, err)
		}
	}

	// Fetch with limit=3.
	gpage, err := store.ListGroupConversation("grp-page", 3)
	if err != nil {
		t.Fatalf("ListGroupConversation(limit=3) error = %v", err)
	}
	if len(gpage) != 3 {
		t.Fatalf("expected 3 group messages with limit=3, got %d", len(gpage))
	}
	if gpage[0].Message.ID != "group-page-1" || gpage[2].Message.ID != "group-page-3" {
		t.Fatalf("unexpected group ordering: first=%s last=%s", gpage[0].Message.ID, gpage[2].Message.ID)
	}
}

// TestDeleteGroup_EmptyID verifies that an empty group ID is rejected.
func TestDeleteGroup_EmptyID(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	if err := store.DeleteGroup("  "); err == nil {
		t.Fatal("expected error for empty group id")
	}
}
