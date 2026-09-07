package storage

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

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
