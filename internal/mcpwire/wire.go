// Package mcpwire provides a local, application-injected MCP stdio engine.
// It contains no bank SDK, credentials, authenticated sessions or bank handlers.
package mcpwire

import (
	"encoding/json"
	"io"

	"github.com/vasyza/sber-go/internal/strictjson"
)

const CurrentVersion = "2026-07-28"
const LegacyVersion = "2025-11-25"

// Implementation is self-reported display information, never an identity proof.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func object(data json.RawMessage) bool {
	var value map[string]json.RawMessage
	return json.Unmarshal(data, &value) == nil && value != nil
}

func send(output io.Writer, id json.RawMessage, result map[string]any, code int, message string, data ...map[string]any) error {
	response := map[string]any{"jsonrpc": "2.0"}

	if code != 0 {
		response["error"] = map[string]any{"code": code, "message": message}
		if code == -32022 && len(data) == 1 && data[0] != nil {
			response["error"].(map[string]any)["data"] = data[0]
		}
	} else {
		response["result"] = result
	}
	frame, err := marshalResponse(response, id)
	if err != nil || len(frame) > MaxFrameBytes || strictjson.Validate(frame) != nil {
		delete(response, "result")
		response["error"] = map[string]any{"code": -32603, "message": staticMessage(-32603)}
		frame, err = marshalResponse(response, id)
		if err != nil || len(frame) > MaxFrameBytes || strictjson.Validate(frame) != nil {
			return ErrOutput
		}
	}
	line := append(frame, '\n')
	n, err := writeFrame(output, line)
	if err != nil || n != len(line) {
		return ErrOutput
	}
	return nil
}

func marshalResponse(response map[string]any, id json.RawMessage) ([]byte, error) {
	body, err := json.Marshal(response)
	if err != nil || id == nil {
		return body, err
	}
	// Reinsert the original strict, bounded token after encoding the result.
	// Even SetEscapeHTML(false) would normalize some Unicode ID spellings.
	frame := make([]byte, 0, len(body)+len(id)+6)
	frame = append(frame, `{"id":`...)
	frame = append(frame, id...)
	frame = append(frame, ',')
	frame = append(frame, body[1:]...)
	return frame, nil
}
