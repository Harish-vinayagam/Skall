package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxBodyLen is the maximum allowed length of a message body.
	// 64 KiB is more than enough for any chat message.
	MaxBodyLen = 64 << 10 // 64 KiB

	// MaxIDLen is the maximum allowed length of a message ID.
	MaxIDLen = 256

	// MaxSenderIDLen is the maximum allowed length of a sender / recipient /
	// group ID field. libp2p peer ID strings are ≤ 100 bytes; this gives
	// headroom while still bounding storage.
	MaxSenderIDLen = 512
)

const Version = 1

type Type string

const (
	TypeChat   Type = "chat"
	TypeJoin   Type = "join"
	TypeLeave  Type = "leave"
	TypeSystem Type = "system"
)

type Message struct {
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Type        Type      `json:"type"`
	SenderID    string    `json:"sender_id"`
	RecipientID string    `json:"recipient_id,omitempty"`
	GroupID     string    `json:"group_id,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	Body        string    `json:"body"`
}

var (
	ErrInvalidJSON        = errors.New("invalid message json")
	ErrMissingRequired    = errors.New("message is missing required fields")
	ErrUnsupportedVersion = errors.New("unsupported message version")
	ErrUnknownType        = errors.New("unknown message type")
	ErrFieldTooLong       = errors.New("message field exceeds maximum length")
)

func (m Message) Validate() error {
	if m.Version != Version {
		return fmt.Errorf("%w: %d", ErrUnsupportedVersion, m.Version)
	}
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.SenderID) == "" || strings.TrimSpace(m.Body) == "" {
		return ErrMissingRequired
	}
	if m.Type != TypeChat && m.Type != TypeJoin && m.Type != TypeLeave && m.Type != TypeSystem {
		return ErrUnknownType
	}
	if m.Timestamp.IsZero() {
		return ErrMissingRequired
	}
	// Security: bound field lengths to prevent store pollution and memory abuse.
	if len(m.ID) > MaxIDLen {
		return fmt.Errorf("%w: id exceeds %d bytes", ErrFieldTooLong, MaxIDLen)
	}
	if len(m.SenderID) > MaxSenderIDLen {
		return fmt.Errorf("%w: sender_id exceeds %d bytes", ErrFieldTooLong, MaxSenderIDLen)
	}
	if len(m.RecipientID) > MaxSenderIDLen {
		return fmt.Errorf("%w: recipient_id exceeds %d bytes", ErrFieldTooLong, MaxSenderIDLen)
	}
	if len(m.GroupID) > MaxSenderIDLen {
		return fmt.Errorf("%w: group_id exceeds %d bytes", ErrFieldTooLong, MaxSenderIDLen)
	}
	if len(m.Body) > MaxBodyLen {
		return fmt.Errorf("%w: body exceeds %d bytes", ErrFieldTooLong, MaxBodyLen)
	}
	return nil
}

func MarshalMessage(message Message) ([]byte, error) {
	if err := message.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(message)
}

func UnmarshalMessage(data []byte) (Message, error) {
	var message Message
	if err := json.Unmarshal(data, &message); err != nil {
		return Message{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if err := message.Validate(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func NewChatMessage(senderID, recipientID, groupID, body string) Message {
	now := time.Now().UTC()
	return Message{
		Version:     Version,
		ID:          strconv.FormatInt(now.UnixNano(), 10),
		Type:        TypeChat,
		SenderID:    senderID,
		RecipientID: recipientID,
		GroupID:     groupID,
		Timestamp:   now,
		Body:        body,
	}
}
