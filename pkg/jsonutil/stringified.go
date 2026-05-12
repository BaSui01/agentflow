// Package jsonutil contains JSON normalization helpers shared across adapters.
package jsonutil

import (
	"bytes"
	"encoding/json"
)

// UnwrapStringifiedRawMessage unwraps a double-serialized JSON object/array.
// It preserves normal JSON objects/arrays and malformed or non-JSON strings.
func UnwrapStringifiedRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return raw
	}
	var strVal string
	if err := json.Unmarshal(raw, &strVal); err == nil && len(strVal) > 0 {
		inner := bytes.TrimSpace([]byte(strVal))
		if len(inner) > 0 && (inner[0] == '{' || inner[0] == '[') && json.Valid(inner) {
			return json.RawMessage(inner)
		}
	}
	return raw
}
