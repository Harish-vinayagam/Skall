package identity

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestLibP2PPrivKeyConversion(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	privKey, err := id.LibP2PPrivKey()
	if err != nil {
		t.Fatalf("LibP2PPrivKey() error = %v", err)
	}
	if privKey == nil {
		t.Fatal("LibP2PPrivKey() returned nil")
	}
}

func TestLibP2PPeerIDDerived(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	pid, err := id.LibP2PPeerID()
	if err != nil {
		t.Fatalf("LibP2PPeerID() error = %v", err)
	}
	if pid == "" {
		t.Fatal("LibP2PPeerID() returned empty peer.ID")
	}
	// Must be deterministic: same key → same peer.ID every time.
	pid2, err := id.LibP2PPeerID()
	if err != nil {
		t.Fatalf("LibP2PPeerID() second call error = %v", err)
	}
	if pid != pid2 {
		t.Fatalf("LibP2PPeerID() not deterministic: %q != %q", pid, pid2)
	}
}

func TestLibP2PPeerIDUniquePerIdentity(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatalf("Generate() a error = %v", err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatalf("Generate() b error = %v", err)
	}
	pidA, err := a.LibP2PPeerID()
	if err != nil {
		t.Fatalf("a.LibP2PPeerID() error = %v", err)
	}
	pidB, err := b.LibP2PPeerID()
	if err != nil {
		t.Fatalf("b.LibP2PPeerID() error = %v", err)
	}
	if pidA == pidB {
		t.Fatal("distinct identities must produce distinct libp2p peer IDs")
	}
}

func TestLibP2PPrivKeyEmptyIdentity(t *testing.T) {
	var empty Identity
	if _, err := empty.LibP2PPrivKey(); err == nil {
		t.Fatal("expected error for identity with no private key")
	}
}

func TestValidate_TamperedPublicKey(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	other, err := Generate()
	if err != nil {
		t.Fatalf("Generate() other error = %v", err)
	}

	// Tamper: swap public key with other's public key (keeps same peerID and privateKey)
	id.PublicKey = other.PublicKey
	err = id.Validate()
	if err == nil {
		t.Fatal("expected error when validating identity with mismatched public key")
	}
	if !errors.Is(err, ErrInvalidKeyPair) && !errors.Is(err, ErrPeerIDMismatch) {
		t.Fatalf("expected ErrInvalidKeyPair or ErrPeerIDMismatch, got %v", err)
	}
}

func TestValidate_EmptyFields(t *testing.T) {
	var empty Identity
	err := empty.Validate()
	if err == nil {
		t.Fatal("expected error for empty identity")
	}
	if !errors.Is(err, ErrUnsupportedVer) {
		t.Fatalf("expected ErrUnsupportedVer for version=0, got %v", err)
	}

	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	id.Username = ""
	if err := id.Validate(); !errors.Is(err, ErrMissingRequired) {
		t.Fatalf("expected ErrMissingRequired for empty Username, got %v", err)
	}

	id, _ = Generate()
	id.DisplayName = "   "
	if err := id.Validate(); !errors.Is(err, ErrMissingRequired) {
		t.Fatalf("expected ErrMissingRequired for whitespace DisplayName, got %v", err)
	}

	id, _ = Generate()
	id.CreatedAt = time.Time{}
	if err := id.Validate(); !errors.Is(err, ErrMissingRequired) {
		t.Fatalf("expected ErrMissingRequired for zero CreatedAt, got %v", err)
	}
}

func TestValidate_UnsupportedVersion(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	id.Version = 99
	if err := id.Validate(); !errors.Is(err, ErrUnsupportedVer) {
		t.Fatalf("expected ErrUnsupportedVer, got %v", err)
	}
}

func TestValidate_PeerIDMismatch(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	id.PeerID = "1234567890abcdef"
	if err := id.Validate(); !errors.Is(err, ErrPeerIDMismatch) {
		t.Fatalf("expected ErrPeerIDMismatch, got %v", err)
	}
}

func TestSave_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping read-only test when running as root")
	}
	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0o500); err != nil {
		t.Fatalf("Mkdir error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(readOnlyDir, 0o755)
	})

	store := NewStore(filepath.Join(readOnlyDir, "sub", "identity.json"))
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := store.Save(id); err == nil {
		t.Fatal("expected error saving to read-only directory")
	}
}

func TestLoadOrCreate_IsIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(filepath.Join(tempDir, "identity.json"))

	first, err := store.LoadOrCreate()
	if err != nil {
		t.Fatalf("first LoadOrCreate() error = %v", err)
	}

	second, err := store.LoadOrCreate()
	if err != nil {
		t.Fatalf("second LoadOrCreate() error = %v", err)
	}

	if first.PeerID != second.PeerID {
		t.Fatalf("expected identical PeerID on subsequent LoadOrCreate calls: %s != %s", first.PeerID, second.PeerID)
	}
}

func TestSummary(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	summary := id.Summary()
	if summary == "" {
		t.Fatal("Summary() returned empty string")
	}
}
