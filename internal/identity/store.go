package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func DefaultPath() (string, error) {
	if override := os.Getenv("SKALL_IDENTITY_PATH"); override != "" {
		return override, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "skall", "identity.json"), nil
}

func DefaultStore() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}

	return NewStore(path), nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() (Identity, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Identity{}, err
	}

	var identity Identity
	if err := json.Unmarshal(data, &identity); err != nil {
		return Identity{}, fmt.Errorf("decode identity: %w", err)
	}
	if err := identity.Validate(); err != nil {
		return Identity{}, err
	}

	return identity, nil
}

func (s *Store) Save(identity Identity) error {
	if err := identity.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return err
	}

	if err := os.Chmod(s.path, 0o600); err != nil && !errors.Is(err, os.ErrPermission) {
		return err
	}

	return nil
}

func (s *Store) LoadOrCreate() (Identity, error) {
	identity, err := s.Load()
	if err == nil {
		return identity, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return Identity{}, err
	}

	identity, err = Generate()
	if err != nil {
		return Identity{}, err
	}

	if err := s.Save(identity); err != nil {
		return Identity{}, err
	}

	return identity, nil
}
