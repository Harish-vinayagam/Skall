package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMarshalAndUnmarshalMessage(t *testing.T) {
	message := Message{
		Version:     Version,
		ID:          "msg-1",
		Type:        TypeChat,
		SenderID:    "alice",
		RecipientID: "bob",
		Timestamp:   time.Unix(1700000000, 0).UTC(),
		Body:        "hello",
	}

	data, err := MarshalMessage(message)
	if err != nil {
		t.Fatalf("MarshalMessage() error = %v", err)
	}

	decoded, err := UnmarshalMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalMessage() error = %v", err)
	}

	if decoded.ID != message.ID || decoded.Type != message.Type || decoded.SenderID != message.SenderID || decoded.RecipientID != message.RecipientID || decoded.Body != message.Body {
		t.Fatalf("decoded message mismatch: %+v", decoded)
	}
}

func TestUnmarshalInvalidJSON(t *testing.T) {
	if _, err := UnmarshalMessage([]byte("not-json")); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidateMissingRequiredFields(t *testing.T) {
	message := Message{Version: Version, Type: TypeChat}
	if err := message.Validate(); err != ErrMissingRequired {
		t.Fatalf("expected ErrMissingRequired, got %v", err)
	}
}

func TestFrameRoundTripMultipleMessages(t *testing.T) {
	var buffer bytes.Buffer
	writer := NewFrameWriter(&buffer)
	reader := NewFrameReader(&buffer)

	first := Message{Version: Version, ID: "1", Type: TypeSystem, SenderID: "server", Timestamp: time.Unix(1, 0).UTC(), Body: "first"}
	second := Message{Version: Version, ID: "2", Type: TypeChat, SenderID: "alice", RecipientID: "bob", Timestamp: time.Unix(2, 0).UTC(), Body: "second"}

	if err := writer.WriteMessage(first); err != nil {
		t.Fatalf("WriteMessage(first) error = %v", err)
	}
	if err := writer.WriteMessage(second); err != nil {
		t.Fatalf("WriteMessage(second) error = %v", err)
	}

	decodedFirst, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage(first) error = %v", err)
	}
	decodedSecond, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage(second) error = %v", err)
	}

	if decodedFirst.ID != first.ID || decodedSecond.ID != second.ID {
		t.Fatalf("frame round trip mismatch: %+v %+v", decodedFirst, decodedSecond)
	}
}

func TestNewChatMessage_FieldsPopulated(t *testing.T) {
	msg := NewChatMessage("alice", "bob", "grp-1", "hello world")
	if msg.Version != Version {
		t.Errorf("expected Version=%d, got %d", Version, msg.Version)
	}
	if msg.Type != TypeChat {
		t.Errorf("expected Type=%s, got %s", TypeChat, msg.Type)
	}
	if msg.SenderID != "alice" || msg.RecipientID != "bob" || msg.GroupID != "grp-1" || msg.Body != "hello world" {
		t.Errorf("fields mismatch: %+v", msg)
	}
	if msg.ID == "" {
		t.Error("expected non-empty message ID")
	}
	if msg.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if err := msg.Validate(); err != nil {
		t.Fatalf("NewChatMessage produced invalid message: %v", err)
	}
}

func TestValidate_ZeroTimestamp(t *testing.T) {
	msg := Message{
		Version:  Version,
		ID:       "id-1",
		Type:     TypeChat,
		SenderID: "alice",
		Body:     "hi",
	}
	if err := msg.Validate(); !errors.Is(err, ErrMissingRequired) {
		t.Fatalf("expected ErrMissingRequired for zero timestamp, got %v", err)
	}
}

func TestValidate_UnknownType(t *testing.T) {
	msg := Message{
		Version:   Version,
		ID:        "id-1",
		Type:      Type("unknown-type"),
		SenderID:  "alice",
		Timestamp: time.Now().UTC(),
		Body:      "hi",
	}
	if err := msg.Validate(); !errors.Is(err, ErrUnknownType) {
		t.Fatalf("expected ErrUnknownType, got %v", err)
	}
}

func TestValidate_UnsupportedVersion(t *testing.T) {
	msg := Message{
		Version:   0,
		ID:        "id-1",
		Type:      TypeChat,
		SenderID:  "alice",
		Timestamp: time.Now().UTC(),
		Body:      "hi",
	}
	if err := msg.Validate(); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestValidate_FieldLengthBounds(t *testing.T) {
	now := time.Now().UTC()
	// Test oversized body
	longBody := strings.Repeat("a", MaxBodyLen+1)
	msg := Message{
		Version:   Version,
		ID:        "id-1",
		Type:      TypeChat,
		SenderID:  "alice",
		Timestamp: now,
		Body:      longBody,
	}
	if err := msg.Validate(); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("expected ErrFieldTooLong for long body, got %v", err)
	}

	// Test oversized ID
	msg = Message{
		Version:   Version,
		ID:        strings.Repeat("x", MaxIDLen+1),
		Type:      TypeChat,
		SenderID:  "alice",
		Timestamp: now,
		Body:      "normal",
	}
	if err := msg.Validate(); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("expected ErrFieldTooLong for long ID, got %v", err)
	}
}

func TestFrameReader_EmptyFrame(t *testing.T) {
	var buf bytes.Buffer
	var header [4]byte // length 0
	buf.Write(header[:])

	reader := NewFrameReader(&buf)
	if _, err := reader.ReadMessage(); !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("expected ErrInvalidJSON for empty frame, got %v", err)
	}
}

func TestFrameReader_OversizedFrame(t *testing.T) {
	var buf bytes.Buffer
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], MaxFrameSize+10)
	buf.Write(header[:])

	reader := NewFrameReader(&buf)
	if _, err := reader.ReadMessage(); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestFrameReader_TruncatedPayload(t *testing.T) {
	var buf bytes.Buffer
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], 100)
	buf.Write(header[:])
	buf.Write([]byte("short-data")) // only 10 bytes instead of 100

	reader := NewFrameReader(&buf)
	if _, err := reader.ReadMessage(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF for truncated payload, got %v", err)
	}
}

func TestConcurrentMarshalUnmarshal(t *testing.T) {
	const count = 30
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			msg := NewChatMessage(
				fmt.Sprintf("sender-%d", idx),
				fmt.Sprintf("recipient-%d", idx),
				"",
				fmt.Sprintf("body payload %d", idx),
			)
			data, err := MarshalMessage(msg)
			if err != nil {
				t.Errorf("goroutine %d: MarshalMessage failed: %v", idx, err)
				return
			}
			unmarshaled, err := UnmarshalMessage(data)
			if err != nil {
				t.Errorf("goroutine %d: UnmarshalMessage failed: %v", idx, err)
				return
			}
			if unmarshaled.SenderID != msg.SenderID || unmarshaled.Body != msg.Body {
				t.Errorf("goroutine %d: unmarshaled mismatch", idx)
			}
		}(i)
	}
	wg.Wait()
}
