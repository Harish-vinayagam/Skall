package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harish-vinayagam/Skall/internal/config"
	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/storage"
	"github.com/Harish-vinayagam/Skall/internal/version"
)

func TestCLIVersion(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	cfg := config.DefaultConfig()

	for _, flag := range []string{"version", "--version", "-v"} {
		var buf bytes.Buffer
		err := runCLI([]string{flag}, id, cfg, &buf)
		if err != nil {
			t.Fatalf("runCLI(%q) error = %v", flag, err)
		}
		out := buf.String()
		if !strings.Contains(out, "SKALL v"+version.Version) {
			t.Errorf("flag %q output = %q, want version %q", flag, out, version.Version)
		}
	}
}

func TestCLIHelp(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	cfg := config.DefaultConfig()

	for _, flag := range []string{"help", "--help", "-h"} {
		var buf bytes.Buffer
		err := runCLI([]string{flag}, id, cfg, &buf)
		if err != nil {
			t.Fatalf("runCLI(%q) error = %v", flag, err)
		}
		out := buf.String()
		if !strings.Contains(out, "Usage:") || !strings.Contains(out, "Commands:") {
			t.Errorf("flag %q output missing help text: %q", flag, out)
		}
	}
}

func TestCLIIdentity(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	cfg := config.DefaultConfig()

	var buf bytes.Buffer
	err = runCLI([]string{"identity"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(identity) error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, id.PeerID) {
		t.Errorf("output = %q missing peer ID %q", out, id.PeerID)
	}
	if !strings.Contains(out, "libp2p Peer ID:") {
		t.Errorf("output = %q missing libp2p Peer ID", out)
	}
}

func TestCLIIdentitySetName(t *testing.T) {
	tempDir := t.TempDir()
	idPath := filepath.Join(tempDir, "identity.json")
	t.Setenv("SKALL_IDENTITY_PATH", idPath)
	dbPath := filepath.Join(tempDir, "skall.db")
	t.Setenv("SKALL_DB_PATH", dbPath)

	store := identity.NewStore(idPath)
	id, err := store.LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}

	cfg := config.DefaultConfig()
	var buf bytes.Buffer
	err = runCLI([]string{"identity", "set-name", "Custom User"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(identity set-name) error = %v", err)
	}
	if !strings.Contains(buf.String(), "Display name updated to: Custom User") {
		t.Fatalf("unexpected output: %s", buf.String())
	}

	// Verify persistence in identity store
	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("reload identity: %v", err)
	}
	if reloaded.DisplayName != "Custom User" {
		t.Fatalf("expected DisplayName 'Custom User', got %q", reloaded.DisplayName)
	}
}

func TestCLIPeers(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "skall.db")
	t.Setenv("SKALL_DB_PATH", dbPath)

	id, _ := identity.Generate()
	cfg := config.DefaultConfig()

	// 1. Empty peers list
	var buf bytes.Buffer
	err := runCLI([]string{"peers"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(peers) empty error = %v", err)
	}
	if !strings.Contains(buf.String(), "No known peers found") {
		t.Fatalf("expected empty peers notice, got: %s", buf.String())
	}

	// 2. Insert peer into DB and list again
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_ = db.UpsertPeer("peer-1234567890abcdef", "Alice", time.Now().UTC())
	_ = db.Close()

	buf.Reset()
	err = runCLI([]string{"peers"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(peers) error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "peer-1234567890a") {
		t.Fatalf("peers output missing inserted peer: %s", out)
	}
}

func TestCLIGroups(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "skall.db")
	t.Setenv("SKALL_DB_PATH", dbPath)

	id, _ := identity.Generate()
	cfg := config.DefaultConfig()

	// 1. Empty groups list
	var buf bytes.Buffer
	err := runCLI([]string{"groups"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(groups) error = %v", err)
	}
	if !strings.Contains(buf.String(), "No groups found") {
		t.Fatalf("expected empty groups notice, got: %s", buf.String())
	}

	// 2. Create group
	buf.Reset()
	err = runCLI([]string{"groups", "create", "dev-team", "Developers"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(groups create) error = %v", err)
	}
	if !strings.Contains(buf.String(), "Created group \"Developers\"") {
		t.Fatalf("unexpected create output: %s", buf.String())
	}

	// 3. List groups
	buf.Reset()
	err = runCLI([]string{"groups", "list"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(groups list) error = %v", err)
	}
	if !strings.Contains(buf.String(), "dev-team") || !strings.Contains(buf.String(), "Developers") {
		t.Fatalf("unexpected group list output: %s", buf.String())
	}
}

func TestCLIConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "skall-config.json")
	t.Setenv("SKALL_CONFIG_PATH", configPath)

	id, _ := identity.Generate()
	cfg := config.DefaultConfig()

	// 1. config show
	var buf bytes.Buffer
	err := runCLI([]string{"config", "show"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(config show) error = %v", err)
	}
	if !strings.Contains(buf.String(), "\"data_dir\"") {
		t.Fatalf("config show output invalid: %s", buf.String())
	}

	// 2. config path
	buf.Reset()
	err = runCLI([]string{"config", "path"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(config path) error = %v", err)
	}
	if !strings.Contains(buf.String(), configPath) {
		t.Fatalf("config path output = %q, want %q", buf.String(), configPath)
	}

	// 3. config init
	buf.Reset()
	err = runCLI([]string{"config", "init"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(config init) error = %v", err)
	}
	if !strings.Contains(buf.String(), "Initialized configuration at") {
		t.Fatalf("config init output invalid: %s", buf.String())
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	id, _ := identity.Generate()
	cfg := config.DefaultConfig()

	var buf bytes.Buffer
	err := runCLI([]string{"unknown-command-xyz"}, id, cfg, &buf)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %v, expected unknown command message", err)
	}
}

func TestParseListenAddr(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "/ip4/0.0.0.0/tcp/0"},
		{"   ", "/ip4/0.0.0.0/tcp/0"},
		{"9001", "/ip4/0.0.0.0/tcp/9001"},
		{"/ip4/127.0.0.1/tcp/4000", "/ip4/127.0.0.1/tcp/4000"},
	}

	for _, c := range cases {
		got := parseListenAddr(c.input)
		if got != c.want {
			t.Errorf("parseListenAddr(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestCLISkallDataDir(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "custom-data")
	t.Setenv("SKALL_DATA_DIR", dataDir)
	t.Setenv("SKALL_DB_PATH", "")
	t.Setenv("SKALL_IDENTITY_PATH", "")

	cfg := config.DefaultConfig()
	cfg.DataDir = dataDir

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate identity: %v", err)
	}

	// 1. Create a group via CLI
	var buf bytes.Buffer
	err = runCLI([]string{"groups", "create", "test-grp", "Test Group"}, id, cfg, &buf)
	if err != nil {
		t.Fatalf("runCLI(groups create) error = %v", err)
	}

	// Verify skall.db was created in custom dataDir with 0600 permissions
	dbPath := filepath.Join(dataDir, "skall.db")
	fi, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("expected skall.db to exist in custom dataDir %q: %v", dataDir, err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected 0600 permissions on skall.db, got %o", perm)
	}
}
