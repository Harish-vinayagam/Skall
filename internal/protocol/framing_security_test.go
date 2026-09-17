package protocol_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

// validMsg returns a minimal valid protocol.Message for use in tests.
func validMsg() protocol.Message {
	return protocol.Message{
		Version:   protocol.Version,
		ID:        "test-id-001",
		Type:      protocol.TypeChat,
		SenderID:  "sender-peer-id",
		Timestamp: time.Now().UTC(),
		Body:      "hello",
	}
}

// writeLengthPrefixedPayload writes a raw 4-byte big-endian length header
// followed by payload, bypassing MarshalMessage validation. Used to craft
// malformed frames in security tests.
func writeLengthPrefixedPayload(length uint32, payload []byte) io.Reader {
	buf := &bytes.Buffer{}
	hdr := make([]byte, 4)
	binary.BigEndian.PutUint32(hdr, length)
	buf.Write(hdr)
	buf.Write(payload)
	return buf
}

// TestReadMessage_OversizedFrameRejected verifies that a frame declaring more
// than MaxFrameSize bytes is rejected before any allocation occurs.
// Without this check a malicious peer could trigger make([]byte, 4 GiB).
func TestReadMessage_OversizedFrameRejected(t *testing.T) {
	// Craft a header claiming MaxFrameSize+1 bytes but with no actual payload.
	// ReadMessage must reject the frame at the length check, not try to read
	// the payload.
	claimed := uint32(protocol.MaxFrameSize + 1)
	buf := &bytes.Buffer{}
	hdr := make([]byte, 4)
	binary.BigEndian.PutUint32(hdr, claimed)
	buf.Write(hdr)
	// No payload bytes — if the reader tries to read `claimed` bytes it will
	// get io.ErrUnexpectedEOF, but we want to see ErrFrameTooLarge first.

	r := protocol.NewFrameReader(buf)
	_, err := r.ReadMessage()
	if err == nil {
		t.Fatal("expected error for oversized frame, got nil")
	}
	if !strings.Contains(err.Error(), "frame exceeds maximum") {
		t.Errorf("expected ErrFrameTooLarge, got: %v", err)
	}
}

// TestReadMessage_MaxSizeFrameAccepted verifies that a frame exactly at
// MaxFrameSize is NOT rejected by the size check (it may still fail JSON
// decode, but the size guard should pass).
func TestReadMessage_MaxSizeFrameAccepted(t *testing.T) {
	// A frame exactly at the cap: the length-guard should let it through.
	// Fill with non-JSON so we get a JSON error, not a size error.
	payload := bytes.Repeat([]byte{'x'}, protocol.MaxFrameSize)
	buf := &bytes.Buffer{}
	hdr := make([]byte, 4)
	binary.BigEndian.PutUint32(hdr, uint32(len(payload)))
	buf.Write(hdr)
	buf.Write(payload)

	r := protocol.NewFrameReader(buf)
	_, err := r.ReadMessage()
	if err == nil {
		t.Fatal("expected JSON error for garbage payload, got nil")
	}
	// Must NOT be a frame-too-large error.
	if strings.Contains(err.Error(), "frame exceeds maximum") {
		t.Errorf("frame at exact cap should not trigger size error, got: %v", err)
	}
}

// TestReadMessage_EmptyFrameRejected verifies that a zero-length frame
// (length = 0) is rejected without panicking.
func TestReadMessage_EmptyFrameRejected(t *testing.T) {
	buf := &bytes.Buffer{}
	hdr := make([]byte, 4) // all zeros → length == 0
	buf.Write(hdr)

	r := protocol.NewFrameReader(buf)
	_, err := r.ReadMessage()
	if err == nil {
		t.Fatal("expected error for zero-length frame, got nil")
	}
}

// TestReadMessage_ValidRoundTrip verifies that a well-formed message survives
// a write → read round-trip unchanged.
func TestReadMessage_ValidRoundTrip(t *testing.T) {
	msg := validMsg()
	buf := &bytes.Buffer{}
	w := protocol.NewFrameWriter(buf)
	if err := w.WriteMessage(msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	r := protocol.NewFrameReader(buf)
	got, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if got.ID != msg.ID || got.Body != msg.Body {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, msg)
	}
}
