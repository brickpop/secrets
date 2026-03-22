package agent

import (
	"encoding/binary"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

// Well-known error string used for client-side retry logic.
const ErrPassphraseRequired = "passphrase required"

// WriteMsg writes a 4-byte big-endian length-prefixed proto message.
func WriteMsg(w io.Writer, m proto.Message) error {
	b, err := proto.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("writing body: %w", err)
	}
	return nil
}

// ReadMsg reads a 4-byte big-endian length-prefixed proto message.
func ReadMsg(r io.Reader, m proto.Message) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return fmt.Errorf("reading header: %w", err)
	}

	size := binary.BigEndian.Uint32(hdr[:])
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("reading body: %w", err)
	}

	if err := proto.Unmarshal(buf, m); err != nil {
		return fmt.Errorf("unmarshaling message: %w", err)
	}
	return nil
}
