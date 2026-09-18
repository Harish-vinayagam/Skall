package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DataDir == "" {
		t.Fatal("DefaultConfig().DataDir is empty")
	}
	if len(cfg.Network.ListenAddrs) == 0 {
		t.Fatal("DefaultConfig().Network.ListenAddrs is empty")
	}
	if !cfg.Network.EnableMDNS {
		t.Fatal("DefaultConfig().Network.EnableMDNS should be true by default")
	}
	if cfg.UI.Theme == "" {
		t.Fatal("DefaultConfig().UI.Theme is empty")
	}
}

func TestLoadAndSaveConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	cfg := DefaultConfig()
	cfg.DisplayName = "Alice"
	cfg.Username = "alice-user"
	cfg.Network.ListenAddrs = []string{"/ip4/127.0.0.1/tcp/9001"}
	cfg.Network.BootstrapPeers = []string{"/ip4/127.0.0.1/tcp/9002/p2p/QmPeer"}

	if err := Save(configPath, cfg); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if loaded.DisplayName != "Alice" {
		t.Fatalf("loaded.DisplayName = %q, want %q", loaded.DisplayName, "Alice")
	}
	if loaded.Username != "alice-user" {
		t.Fatalf("loaded.Username = %q, want %q", loaded.Username, "alice-user")
	}
	if len(loaded.Network.ListenAddrs) != 1 || loaded.Network.ListenAddrs[0] != "/ip4/127.0.0.1/tcp/9001" {
		t.Fatalf("loaded.Network.ListenAddrs mismatch: %+v", loaded.Network.ListenAddrs)
	}
	if len(loaded.Network.BootstrapPeers) != 1 {
		t.Fatalf("loaded.Network.BootstrapPeers count mismatch: %+v", loaded.Network.BootstrapPeers)
	}
}

func TestLoadCorruptedConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "corrupted.json")

	if err := os.WriteFile(configPath, []byte("{broken-json"), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatal("expected error loading corrupted config")
	}
}

func TestLoadOrCreateNew(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "sub", "config.json")
	t.Setenv("SKALL_CONFIG_PATH", configPath)

	cfg, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate error: %v", err)
	}

	if cfg.UI.Theme != "default" {
		t.Fatalf("expected default theme, got %s", cfg.UI.Theme)
	}

	// Verify file was created on disk
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
}

func TestDefaultConfigPathOverride(t *testing.T) {
	t.Setenv("SKALL_CONFIG_PATH", "/custom/path/config.json")
	p, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath error: %v", err)
	}
	if p != "/custom/path/config.json" {
		t.Fatalf("got %s, want /custom/path/config.json", p)
	}
}

func TestValidateSanitizesEmptyValues(t *testing.T) {
	cfg := Config{
		DataDir: "",
		Network: NetworkConfig{ListenAddrs: nil},
		UI:      UIConfig{Theme: ""},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if cfg.DataDir == "" {
		t.Fatal("expected non-empty DataDir after validation")
	}
	if len(cfg.Network.ListenAddrs) == 0 {
		t.Fatal("expected default ListenAddrs after validation")
	}
	if cfg.UI.Theme != "default" {
		t.Fatalf("expected default theme, got %s", cfg.UI.Theme)
	}
}

func TestDefaultConfigPathStandard(t *testing.T) {
	t.Setenv("SKALL_CONFIG_PATH", "")
	p, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath error: %v", err)
	}
	if !strings.HasSuffix(p, "config.json") {
		t.Fatalf("expected path ending in config.json, got %s", p)
	}
}
