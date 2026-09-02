package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const Version = 1

type Identity struct {
	Version     int                `json:"version"`
	PeerID      string             `json:"peer_id"`
	Username    string             `json:"username"`
	DisplayName string             `json:"display_name"`
	CreatedAt   time.Time          `json:"created_at"`
	PublicKey   ed25519.PublicKey  `json:"public_key"`
	PrivateKey  ed25519.PrivateKey `json:"private_key"`
}

var (
	ErrMissingRequired = errors.New("identity is missing required fields")
	ErrInvalidKeyPair  = errors.New("identity key pair is invalid")
	ErrPeerIDMismatch  = errors.New("identity peer id does not match public key")
	ErrUnsupportedVer  = errors.New("unsupported identity version")
)

func Generate() (Identity, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, err
	}

	peerID := PeerIDFromPublicKey(publicKey)
	shortID := shortPeerID(peerID)
	now := time.Now().UTC()

	identity := Identity{
		Version:     Version,
		PeerID:      peerID,
		Username:    "user-" + shortID,
		DisplayName: "SKALL-" + shortID,
		CreatedAt:   now,
		PublicKey:   publicKey,
		PrivateKey:  privateKey,
	}

	return identity, identity.Validate()
}

func (i Identity) Validate() error {
	if i.Version != Version {
		return fmt.Errorf("%w: %d", ErrUnsupportedVer, i.Version)
	}
	if strings.TrimSpace(i.PeerID) == "" || strings.TrimSpace(i.Username) == "" || strings.TrimSpace(i.DisplayName) == "" {
		return ErrMissingRequired
	}
	if i.CreatedAt.IsZero() {
		return ErrMissingRequired
	}
	if len(i.PublicKey) != ed25519.PublicKeySize || len(i.PrivateKey) != ed25519.PrivateKeySize {
		return ErrInvalidKeyPair
	}
	if !i.PrivateKey.Public().(ed25519.PublicKey).Equal(i.PublicKey) {
		return ErrInvalidKeyPair
	}
	if PeerIDFromPublicKey(i.PublicKey) != i.PeerID {
		return ErrPeerIDMismatch
	}
	return nil
}

func (i Identity) Summary() string {
	return fmt.Sprintf("Peer ID: %s\nUsername: %s\nDisplay Name: %s\nCreated At: %s", i.PeerID, i.Username, i.DisplayName, i.CreatedAt.Format(time.RFC3339))
}

func PeerIDFromPublicKey(publicKey ed25519.PublicKey) string {
	hash := sha256.Sum256(publicKey)
	return hex.EncodeToString(hash[:])
}

func shortPeerID(peerID string) string {
	if len(peerID) <= 8 {
		return peerID
	}
	return peerID[:8]
}
