package swarm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const maxAgentMessageBytes = 8 << 20 // 8 MiB per framed agent protocol message.

type agentDecoder interface {
	Decode(v any) error
}

type agentProtocolDecoder struct {
	reader *bufio.Reader
}

func newAgentProtocolDecoder(r io.Reader) *agentProtocolDecoder {
	if br, ok := r.(*bufio.Reader); ok {
		return &agentProtocolDecoder{reader: br}
	}
	return &agentProtocolDecoder{reader: bufio.NewReader(r)}
}

func (d *agentProtocolDecoder) Decode(v any) error {
	frame, err := d.readFrame()
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(frame))
	if err := dec.Decode(v); err != nil {
		return err
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("agent protocol frame contains trailing JSON")
	}
	return nil
}

func (d *agentProtocolDecoder) readFrame() ([]byte, error) {
	var frame []byte
	for {
		part, err := d.reader.ReadSlice('\n')
		if len(part) > 0 {
			if len(frame)+len(part) > maxAgentMessageBytes {
				return nil, fmt.Errorf("agent protocol message exceeds %d bytes", maxAgentMessageBytes)
			}
			frame = append(frame, part...)
		}

		switch {
		case err == nil:
			return bytes.TrimSpace(frame), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(frame) > 0:
			return bytes.TrimSpace(frame), nil
		default:
			return nil, err
		}
	}
}
