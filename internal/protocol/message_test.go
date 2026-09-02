package protocol

import (
	"bytes"
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
