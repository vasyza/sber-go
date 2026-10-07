package mcpwire

import (
	"bytes"
	"errors"
)

// MaxFrameBytes is the maximum encoded document size, excluding the LF delimiter.
const MaxFrameBytes = 1 << 20

var (
	ErrInput          = errors.New("MCP input failure")
	ErrOutput         = errors.New("MCP output failure")
	ErrFrameTooLarge  = errors.New("MCP frame exceeds limit")
	ErrTruncatedFrame = errors.New("unterminated MCP frame")
)

func splitFrame(data []byte, atEOF bool) (int, []byte, error) {
	if end := bytes.IndexByte(data, '\n'); end >= 0 {
		if end > MaxFrameBytes {
			return 0, nil, ErrFrameTooLarge
		}
		return end + 1, data[:end], nil
	}
	if len(data) > MaxFrameBytes {
		return 0, nil, ErrFrameTooLarge
	}
	if atEOF && len(data) != 0 {
		return 0, nil, ErrTruncatedFrame
	}
	return 0, nil, nil
}
