package protocol

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

const frameHeaderSize = 4

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
