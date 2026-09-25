package groups

import (
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/Harish-vinayagam/Skall/internal/storage"
)

// openTestStore creates an isolated SQLite store in a temp directory.
func openTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("openTestStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestLoadFromStore_EmptyDB verifies that LoadFromStore is a no-op on an empty
// database and does not return an error.
func TestLoadFromStore_EmptyDB(t *testing.T) {
	store := openTestStore(t)
	mgr := NewManager()

	if err := LoadFromStore(store, mgr); err != nil {
		t.Fatalf("LoadFromStore on empty DB error = %v", err)
	}
	if ids := mgr.ListGroups(); len(ids) != 0 {
		t.Fatalf("expected 0 groups after empty-DB load, got %d", len(ids))
	}
}

// TestLoadFromStore_RestoresGroups verifies that groups and their active
// memberships are restored into a fresh Manager after a simulated restart.
func TestLoadFromStore_RestoresGroups(t *testing.T) {
	store := openTestStore(t)
	created := time.Unix(1710030000, 0).UTC()

	// Write two groups with members.
	if err := store.UpsertGroup("g-restore-1", "Alpha", created); err != nil {
		t.Fatalf("UpsertGroup g-restore-1: %v", err)
	}
	if err := store.SetGroupMembership("g-restore-1", "alice", created, true, created); err != nil {
		t.Fatalf("SetGroupMembership alice: %v", err)
	}
	if err := store.SetGroupMembership("g-restore-1", "bob", created, true, created); err != nil {
		t.Fatalf("SetGroupMembership bob: %v", err)
	}

	if err := store.UpsertGroup("g-restore-2", "Beta", created); err != nil {
		t.Fatalf("UpsertGroup g-restore-2: %v", err)
	}
	if err := store.SetGroupMembership("g-restore-2", "charlie", created, true, created); err != nil {
		t.Fatalf("SetGroupMembership charlie: %v", err)
	}

	// Simulate a restart with a fresh Manager.
	mgr := NewManager()
	if err := LoadFromStore(store, mgr); err != nil {
		t.Fatalf("LoadFromStore error = %v", err)
	}

	ids := mgr.ListGroups()
	if len(ids) != 2 {
		t.Fatalf("expected 2 groups after restore, got %d: %v", len(ids), ids)
	}

	// Verify group Alpha has alice and bob.
	g1, ok := mgr.GetGroup("g-restore-1")
	if !ok {
		t.Fatal("g-restore-1 not found after restore")
	}
	if g1.Name != "Alpha" {
		t.Fatalf("expected name Alpha, got %s", g1.Name)
	}
	if len(g1.Members) != 2 {
		t.Fatalf("expected 2 members in g-restore-1, got %d", len(g1.Members))
	}
	if !mgr.IsMember("g-restore-1", "alice") || !mgr.IsMember("g-restore-1", "bob") {
		t.Fatal("alice or bob missing from restored group")
	}

	// Verify group Beta has charlie.
	if !mgr.IsMember("g-restore-2", "charlie") {
		t.Fatal("charlie missing from restored group Beta")
	}
}

// TestLoadFromStore_InactiveMembers verifies that members with active=false are
// NOT restored into the Manager.
func TestLoadFromStore_InactiveMembers(t *testing.T) {
	store := openTestStore(t)
	created := time.Unix(1710031000, 0).UTC()

	if err := store.UpsertGroup("g-inactive", "Inactive Test", created); err != nil {
		t.Fatalf("UpsertGroup: %v", err)
	}
	// Add alice as active, bob as inactive.
	if err := store.SetGroupMembership("g-inactive", "alice", created, true, created); err != nil {
		t.Fatalf("SetGroupMembership alice: %v", err)
	}
	if err := store.SetGroupMembership("g-inactive", "bob", created, false, created); err != nil {
		t.Fatalf("SetGroupMembership bob: %v", err)
	}

	mgr := NewManager()
	if err := LoadFromStore(store, mgr); err != nil {
		t.Fatalf("LoadFromStore error = %v", err)
	}

	if !mgr.IsMember("g-inactive", "alice") {
		t.Fatal("alice (active) should be in restored manager")
	}
	if mgr.IsMember("g-inactive", "bob") {
		t.Fatal("bob (inactive) should NOT be in restored manager")
	}
}

// TestLoadFromStore_Idempotent verifies that calling LoadFromStore twice on the
// same Manager does not duplicate groups or members.
func TestLoadFromStore_Idempotent(t *testing.T) {
	store := openTestStore(t)
	created := time.Unix(1710032000, 0).UTC()

	if err := store.UpsertGroup("g-idem", "Idempotent", created); err != nil {
		t.Fatalf("UpsertGroup: %v", err)
	}
	if err := store.SetGroupMembership("g-idem", "alice", created, true, created); err != nil {
		t.Fatalf("SetGroupMembership: %v", err)
	}

	mgr := NewManager()
	for i := 0; i < 3; i++ {
		if err := LoadFromStore(store, mgr); err != nil {
			t.Fatalf("LoadFromStore call %d error = %v", i+1, err)
		}
	}

	ids := mgr.ListGroups()
	if len(ids) != 1 {
		t.Fatalf("expected 1 group after 3 LoadFromStore calls, got %d", len(ids))
	}
	members, err := mgr.ViewMembers("g-idem")
	if err != nil {
		t.Fatalf("ViewMembers error: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member after idempotent load, got %d", len(members))
	}
}

// TestLoadFromStore_PreservesTimestamps verifies that CreatedAt on restored
// groups matches what was stored, not time.Now().
func TestLoadFromStore_PreservesTimestamps(t *testing.T) {
	store := openTestStore(t)
	created := time.Unix(1600000000, 0).UTC() // A fixed timestamp in the past.

	if err := store.UpsertGroup("g-ts", "Timestamps", created); err != nil {
		t.Fatalf("UpsertGroup: %v", err)
	}

	mgr := NewManager()
	if err := LoadFromStore(store, mgr); err != nil {
		t.Fatalf("LoadFromStore error = %v", err)
	}

	g, ok := mgr.GetGroup("g-ts")
	if !ok {
		t.Fatal("group g-ts not found")
	}
	// Allow a 1-second tolerance for timestamp precision in SQLite storage.
	diff := g.CreatedAt.Sub(created)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Fatalf("CreatedAt mismatch: got %v want %v (diff %v)", g.CreatedAt, created, diff)
	}
}
