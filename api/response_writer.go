package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
		buf, err = json.Marshal(fallback)
		if err != nil {
			// stderr fallback: WriteJSONResponse 为包级函数无 logger；
			// 此路径仅在 json.Marshal(Response{}) 本身失败时触发，概率极低。
			fmt.Fprintf(os.Stderr, "api: json.Marshal fallback failed: %v\n", err)
			buf = []byte(`{"success":false,"error":{"code":"INTERNAL_ERROR","message":"failed to encode response"},"timestamp":null}`)
		}
		status = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if _, err := w.Write(buf); err != nil {
		fmt.Fprintf(os.Stderr, "api: writeJSONResponse write failed: %v\n", err)
	}
}
