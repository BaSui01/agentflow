package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

func pathUintID(r *http.Request, name string, fallbackIndex int) (uint, bool) {
	idStr := strings.TrimSpace(r.PathValue(name))
	if idStr == "" {
		idStr = fallbackPathSegment(r, fallbackIndex)
	}
	if idStr == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

func pathStringValue(r *http.Request, name string, fallbackIndex int) string {
	value := strings.TrimSpace(r.PathValue(name))
	if value != "" {
		return value
	}
	return fallbackPathSegment(r, fallbackIndex)
}

func fallbackPathSegment(r *http.Request, index int) string {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if index < 0 || index >= len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[index])
}

func parsePositiveQueryInt(raw string, field string) (int, *types.Error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, types.NewInvalidRequestError(field + " must be a positive integer")
	}
	return parsed, nil
}

func parseNonNegativeQueryInt(raw string, field string) (int, *types.Error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, types.NewInvalidRequestError(field + " must be a non-negative integer")
	}
	return parsed, nil
}

func boundedOrDefault(value int, defaultValue int, maxValue int) int {
	if value <= 0 {
		value = defaultValue
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}
