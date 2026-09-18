package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NetworkConfig controls p2p connectivity and discovery.
type NetworkConfig struct {
	// ListenAddrs are the multiaddresses the libp2p node will bind to.
	ListenAddrs []string `json:"listen_addresses"`

	// EnableMDNS enables local network peer discovery via multicast DNS.
	EnableMDNS bool `json:"enable_mdns"`

	// BootstrapPeers is a list of multiaddresses to connect to on startup.
	BootstrapPeers []string `json:"bootstrap_peers,omitempty"`
}

// UIConfig controls TUI visual preferences.
type UIConfig struct {
	// Theme selects the visual color palette ("default", "minimal", "vibrant").
	Theme string `json:"theme"`

	// ShowTimestamps controls message timestamp display in chat views.
	ShowTimestamps bool `json:"show_timestamps"`
}

// Config represents the top-level SKALL configuration.
type Config struct {
	// DataDir is the directory where identities, history, and stores reside.
	DataDir string `json:"data_dir"`

	// Username is the default handle used for direct messaging.
	Username string `json:"username,omitempty"`

	// DisplayName is the human-friendly name shown to peers.
	DisplayName string `json:"display_name,omitempty"`

	// Network contains p2p listening and discovery settings.
	Network NetworkConfig `json:"network"`

	// UI contains visual display preferences.
	UI UIConfig `json:"ui"`
}

// DefaultConfig returns a sensible, non-root default configuration.
func DefaultConfig() Config {
	dataDir := strings.TrimSpace(os.Getenv("SKALL_DATA_DIR"))
	if dataDir == "" {
		configDir, _ := os.UserConfigDir()
		dataDir = filepath.Join(configDir, "skall")
		if configDir == "" {
			dataDir = ".skall"
		}
	}

	return Config{
		DataDir: dataDir,
		Network: NetworkConfig{
			ListenAddrs:    []string{"/ip4/0.0.0.0/tcp/0"},
			EnableMDNS:     true,
			BootstrapPeers: []string{},
		},
		UI: UIConfig{
			Theme:          "default",
			ShowTimestamps: true,
		},
	}
}

// DefaultConfigPath resolves the canonical path to the config file following XDG conventions.
func DefaultConfigPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("SKALL_CONFIG_PATH")); override != "" {
		return override, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return filepath.Join(configDir, "skall", "config.json"), nil
}

// Load reads and parses a Config from the given filesystem path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config json: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Save writes the given Config to the specified path with secure permissions (0600).
func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	if err := os.Chmod(path, 0o600); err != nil && !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("set config permissions: %w", err)
	}

	return nil
}

// LoadOrCreate loads existing configuration or initializes and saves default config.
func LoadOrCreate() (Config, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return DefaultConfig(), nil
	}

	cfg, err := Load(path)
	if err == nil {
		return cfg, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		// If the file exists but has invalid JSON or permissions, return the error
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
			// fall through to create
		} else if !strings.Contains(err.Error(), "no such file") {
			return Config{}, err
		}
	}

	defaultCfg := DefaultConfig()
	if err := Save(path, defaultCfg); err != nil {
		// Non-fatal if read-only filesystem; return default config in-memory
		return defaultCfg, nil
	}

	return defaultCfg, nil
}

// Validate checks for structural validity and sanitizes values.
func (c *Config) Validate() error {
	if override := strings.TrimSpace(os.Getenv("SKALL_DATA_DIR")); override != "" {
		c.DataDir = override
	}
	c.DataDir = strings.TrimSpace(c.DataDir)
	if c.DataDir == "" {
		c.DataDir = filepath.Join(".", ".skall")
	}

	if len(c.Network.ListenAddrs) == 0 {
		c.Network.ListenAddrs = []string{"/ip4/0.0.0.0/tcp/0"}
	}

	if c.UI.Theme == "" {
		c.UI.Theme = "default"
	}

	return nil
}
