package protocol_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

// longString returns a string of n 'a' bytes.
func longString(n int) string {
	return strings.Repeat("a", n)
}

// baseValidMessage returns a protocol.Message with every required field set.
func baseValidMessage() protocol.Message {
	return protocol.Message{
		Version:   protocol.Version,
		ID:        "msg-id-1",
		Type:      protocol.TypeChat,
		SenderID:  "sender-id",
		Timestamp: time.Now().UTC(),
		Body:      "hello",
	}
}

// marshalRaw encodes m to JSON without running Validate(), so tests can craft
// messages with invalid fields.
func marshalRaw(m protocol.Message) ([]byte, error) {
	return json.Marshal(m)
}

// TestValidate_BodyTooLong verifies that bodies exceeding MaxBodyLen are rejected.
func TestValidate_BodyTooLong(t *testing.T) {
	msg := baseValidMessage()
	msg.Body = longString(protocol.MaxBodyLen + 1)
	if err := msg.Validate(); err == nil {
		t.Fatal("expected error for body exceeding MaxBodyLen, got nil")
	} else if !errors.Is(err, protocol.ErrFieldTooLong) {
		t.Errorf("expected ErrFieldTooLong, got: %v", err)
	}
}

// TestValidate_BodyAtMaxAccepted verifies that exactly MaxBodyLen bytes is valid.
func TestValidate_BodyAtMaxAccepted(t *testing.T) {
	msg := baseValidMessage()
	msg.Body = longString(protocol.MaxBodyLen)
	if err := msg.Validate(); err != nil {
		t.Errorf("body at MaxBodyLen should be valid, got: %v", err)
	}
}

// TestValidate_IDTooLong verifies that IDs exceeding MaxIDLen are rejected.
func TestValidate_IDTooLong(t *testing.T) {
	msg := baseValidMessage()
	msg.ID = longString(protocol.MaxIDLen + 1)
	if err := msg.Validate(); err == nil {
		t.Fatal("expected error for ID exceeding MaxIDLen, got nil")
	} else if !errors.Is(err, protocol.ErrFieldTooLong) {
		t.Errorf("expected ErrFieldTooLong, got: %v", err)
	}
}

// TestValidate_SenderIDTooLong verifies that SenderID exceeding MaxSenderIDLen
// is rejected without panic.
func TestValidate_SenderIDTooLong(t *testing.T) {
	msg := baseValidMessage()
	msg.SenderID = longString(protocol.MaxSenderIDLen + 1)
	if err := msg.Validate(); err == nil {
		t.Fatal("expected error for SenderID exceeding MaxSenderIDLen, got nil")
	} else if !errors.Is(err, protocol.ErrFieldTooLong) {
		t.Errorf("expected ErrFieldTooLong, got: %v", err)
	}
}

// TestValidate_RecipientIDTooLong verifies that an oversized RecipientID is
// rejected.
func TestValidate_RecipientIDTooLong(t *testing.T) {
	msg := baseValidMessage()
	msg.RecipientID = longString(protocol.MaxSenderIDLen + 1)
	if err := msg.Validate(); err == nil {
		t.Fatal("expected error for RecipientID exceeding limit, got nil")
	} else if !errors.Is(err, protocol.ErrFieldTooLong) {
		t.Errorf("expected ErrFieldTooLong, got: %v", err)
	}
}

// TestValidate_GroupIDTooLong verifies that an oversized GroupID is rejected.
func TestValidate_GroupIDTooLong(t *testing.T) {
	msg := baseValidMessage()
	msg.GroupID = longString(protocol.MaxSenderIDLen + 1)
	if err := msg.Validate(); err == nil {
		t.Fatal("expected error for GroupID exceeding limit, got nil")
	} else if !errors.Is(err, protocol.ErrFieldTooLong) {
		t.Errorf("expected ErrFieldTooLong, got: %v", err)
	}
}

// TestUnmarshalMessage_OversizedBodyRejected verifies that deserialising a
// message with an oversized body returns ErrFieldTooLong (not a store-write).
func TestUnmarshalMessage_OversizedBodyRejected(t *testing.T) {
	msg := baseValidMessage()
	msg.Body = longString(protocol.MaxBodyLen + 1)
	// Marshal manually (bypassing Validate) to get raw bytes with oversize body.
	raw, err := marshalRaw(msg)
	if err != nil {
		t.Fatalf("raw marshal: %v", err)
	}
	_, err = protocol.UnmarshalMessage(raw)
	if err == nil {
		t.Fatal("expected error from UnmarshalMessage with oversized body")
	}
	if !errors.Is(err, protocol.ErrFieldTooLong) {
		t.Errorf("expected ErrFieldTooLong, got: %v", err)
	}
}
