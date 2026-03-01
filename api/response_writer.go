package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// WriteJSONResponse writes a canonical API response using marshal-first pattern.
func WriteJSONResponse(w http.ResponseWriter, status int, data any) {
	buf, err := json.Marshal(data)
	if err != nil {
		fallback := Response{
			Success: false,
			Error: &ErrorInfo{
				Code:    "INTERNAL_ERROR",
				Message: "failed to encode response",
			},
			Timestamp: time.Now().UTC(),
		}
		buf, _ = json.Marshal(fallback)
		status = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}
