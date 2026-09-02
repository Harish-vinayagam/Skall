package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateGeneratesIdentityOnFirstRun(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(filepath.Join(tempDir, "identity.json"))

	loaded, err := store.LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("generated identity invalid: %v", err)
	}

	if _, err := os.Stat(store.Path()); err != nil {
		t.Fatalf("expected identity file to exist: %v", err)
	}
}

func TestLoadExistingIdentity(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(filepath.Join(tempDir, "identity.json"))

	original, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := store.Save(original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.PeerID != original.PeerID || loaded.Username != original.Username || loaded.DisplayName != original.DisplayName {
		t.Fatalf("loaded identity does not match saved identity: %+v vs %+v", loaded, original)
	}
}

func TestIdentityPersistence(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(filepath.Join(tempDir, "identity.json"))

	original, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := store.Save(original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.PeerID != original.PeerID {
		t.Fatalf("persisted peer id mismatch: got %s want %s", loaded.PeerID, original.PeerID)
	}
}

func TestCorruptedIdentityHandling(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(filepath.Join(tempDir, "identity.json"))

	if err := os.WriteFile(store.Path(), []byte(`{"version":1,"peer_id":"broken"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := store.LoadOrCreate(); err == nil {
		t.Fatal("expected corrupted identity to return an error")
	}
}

func TestGenerateUniqueIdentity(t *testing.T) {
	one, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	two, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if one.PeerID == two.PeerID {
		t.Fatal("expected generated identities to be unique")
	}
}
