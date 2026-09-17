package protocol

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	frameHeaderSize = 4

	// MaxFrameSize is the largest message payload SKALL will read from the wire.
	// A remote peer claiming a frame larger than this is either buggy or
	// malicious; reject it immediately without allocating the buffer.
	// 1 MiB is ample for any legitimate chat message.
	MaxFrameSize = 1 << 20 // 1 MiB
)

// ErrFrameTooLarge is returned when an incoming frame header declares a
// payload that exceeds MaxFrameSize.
var ErrFrameTooLarge = fmt.Errorf("frame exceeds maximum allowed size (%d bytes)", MaxFrameSize)

type FrameWriter struct {
	w io.Writer
}

type FrameReader struct {
	r *bufio.Reader
}

func NewFrameWriter(w io.Writer) *FrameWriter {
	return &FrameWriter{w: w}
}

func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: bufio.NewReader(r)}
}

func (w *FrameWriter) WriteMessage(message Message) error {
	payload, err := MarshalMessage(message)
	if err != nil {
		return err
	}

	var header [frameHeaderSize]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))

	if err := writeAll(w.w, header[:]); err != nil {
		return err
	}
	return writeAll(w.w, payload)
}

func (r *FrameReader) ReadMessage() (Message, error) {
	var header [frameHeaderSize]byte
	if _, err := io.ReadFull(r.r, header[:]); err != nil {
		return Message{}, err
	}

	length := binary.BigEndian.Uint32(header[:])
	if length == 0 {
		return Message{}, fmt.Errorf("%w: empty frame", ErrInvalidJSON)
	}
	// Security: reject oversized frames before allocating. Without this cap a
	// malicious peer can trigger make([]byte, 4 GiB) → OOM / panic.
	if length > MaxFrameSize {
		return Message{}, fmt.Errorf("%w: declared %d bytes", ErrFrameTooLarge, length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r.r, payload); err != nil {
		return Message{}, err
	}

	return UnmarshalMessage(payload)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}
