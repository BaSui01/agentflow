// =============================================================================
// AgentFlow Configuration HTTP API
// =============================================================================
// Provides HTTP endpoints for configuration management:
// - GET /api/v1/config - Get current configuration (sanitized)
// - PUT /api/v1/config - Update configuration fields
// - POST /api/v1/config/reload - Reload configuration from file
// - GET /api/v1/config/fields - Get hot reloadable fields
// - GET /api/v1/config/changes - Get configuration change history
// =============================================================================
package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// =============================================================================
// API Types
// =============================================================================

// ConfigAPIHandler handles configuration API requests
type ConfigAPIHandler struct {
	manager *HotReloadManager
}

// ConfigResponse represents the configuration API response
type ConfigResponse struct {
	// Success indicates if the operation was successful
	Success bool `json:"success"`

	// Message provides additional information
	Message string `json:"message,omitempty"`

	// Config is the current configuration (sanitized)
	Config map[string]interface{} `json:"config,omitempty"`

	// Fields lists hot reloadable fields
	Fields map[string]FieldInfo `json:"fields,omitempty"`

	// Changes lists configuration changes
	Changes []ConfigChange `json:"changes,omitempty"`

	// Error provides error details
	Error string `json:"error,omitempty"`

	// RequiresRestart indicates if restart is needed
	RequiresRestart bool `json:"requires_restart,omitempty"`

	// Timestamp of the response
	Timestamp time.Time `json:"timestamp"`
}

// FieldInfo provides information about a configuration field
type FieldInfo struct {
	// Path is the field path
	Path string `json:"path"`

	// Description of the field
	Description string `json:"description"`

	// RequiresRestart indicates if changing requires restart
	RequiresRestart bool `json:"requires_restart"`

	// Sensitive indicates if the field is sensitive
	Sensitive bool `json:"sensitive"`

	// CurrentValue is the current value (redacted if sensitive)
	CurrentValue interface{} `json:"current_value,omitempty"`
}

// ConfigUpdateRequest represents a configuration update request
type ConfigUpdateRequest struct {
	// Updates is a map of field paths to new values
	Updates map[string]interface{} `json:"updates"`
}

// =============================================================================
// API Handler Implementation
// =============================================================================

// NewConfigAPIHandler creates a new configuration API handler
func NewConfigAPIHandler(manager *HotReloadManager) *ConfigAPIHandler {
	return &ConfigAPIHandler{
		manager: manager,
	}
}

// RegisterRoutes registers the configuration API routes
func (h *ConfigAPIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/config", h.handleConfig)
	mux.HandleFunc("/api/v1/config/reload", h.handleReload)
	mux.HandleFunc("/api/v1/config/fields", h.handleFields)
	mux.HandleFunc("/api/v1/config/changes", h.handleChanges)
}

// handleConfig handles GET and PUT requests for configuration
func (h *ConfigAPIHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getConfig(w, r)
	case http.MethodPut:
		h.updateConfig(w, r)
	case http.MethodOptions:
		h.handleCORS(w, r)
	default:
		h.methodNotAllowed(w, r)
	}
}

// getConfig returns the current configuration (sanitized)
// @Summary Get current configuration
// @Description Returns the current configuration with sensitive fields redacted
// @Tags Configuration
// @Accept json
// @Produce json
// @Success 200 {object} ConfigResponse "Current configuration"
// @Failure 500 {object} ConfigResponse "Internal server error"
// @Router /api/v1/config [get]
func (h *ConfigAPIHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	config := h.manager.SanitizedConfig()

	resp := ConfigResponse{
		Success:   true,
		Message:   "Configuration retrieved successfully",
		Config:    config,
		Timestamp: time.Now(),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// updateConfig updates configuration fields
// @Summary Update configuration
// @Description Updates one or more configuration fields dynamically
// @Tags Configuration
// @Accept json
// @Produce json
// @Param request body ConfigUpdateRequest true "Configuration updates"
// @Success 200 {object} ConfigResponse "Configuration updated"
// @Failure 400 {object} ConfigResponse "Invalid request"
// @Failure 500 {object} ConfigResponse "Internal server error"
// @Router /api/v1/config [put]
func (h *ConfigAPIHandler) updateConfig(w http.ResponseWriter, r *http.Request) {
	var req ConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSON(w, http.StatusBadRequest, ConfigResponse{
			Success:   false,
			Error:     fmt.Sprintf("Invalid request body: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	if len(req.Updates) == 0 {
		h.writeJSON(w, http.StatusBadRequest, ConfigResponse{
			Success:   false,
			Error:     "No updates provided",
			Timestamp: time.Now(),
		})
		return
	}

	var errors []string
	var requiresRestart bool

	for path, value := range req.Updates {
		// Check if field is known
		field, known := hotReloadableFields[path]
		if !known {
			errors = append(errors, fmt.Sprintf("Unknown field: %s", path))
			continue
		}

		if field.RequiresRestart {
			requiresRestart = true
		}

		if err := h.manager.UpdateField(path, value); err != nil {
			errors = append(errors, fmt.Sprintf("Failed to update %s: %v", path, err))
		}
	}

	if len(errors) > 0 {
		h.writeJSON(w, http.StatusBadRequest, ConfigResponse{
			Success:         false,
			Error:           fmt.Sprintf("Some updates failed: %v", errors),
			RequiresRestart: requiresRestart,
			Timestamp:       time.Now(),
		})
		return
	}

	h.writeJSON(w, http.StatusOK, ConfigResponse{
		Success:         true,
		Message:         "Configuration updated successfully",
		Config:          h.manager.SanitizedConfig(),
		RequiresRestart: requiresRestart,
		Timestamp:       time.Now(),
	})
}

// handleReload handles POST requests to reload configuration from file
// @Summary Reload configuration from file
// @Description Reloads the configuration from the configuration file
// @Tags Configuration
// @Accept json
// @Produce json
// @Success 200 {object} ConfigResponse "Configuration reloaded"
// @Failure 500 {object} ConfigResponse "Reload failed"
// @Router /api/v1/config/reload [post]
func (h *ConfigAPIHandler) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.handleCORS(w, r)
		return
	}

	if r.Method != http.MethodPost {
		h.methodNotAllowed(w, r)
		return
	}

	if err := h.manager.ReloadFromFile(); err != nil {
		h.writeJSON(w, http.StatusInternalServerError, ConfigResponse{
			Success:   false,
			Error:     fmt.Sprintf("Failed to reload configuration: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	h.writeJSON(w, http.StatusOK, ConfigResponse{
		Success:   true,
		Message:   "Configuration reloaded successfully",
		Config:    h.manager.SanitizedConfig(),
		Timestamp: time.Now(),
	})
}

// handleFields returns the list of hot reloadable fields
// @Summary Get hot reloadable fields
// @Description Returns the list of configuration fields that can be hot reloaded
// @Tags Configuration
// @Accept json
// @Produce json
// @Success 200 {object} ConfigResponse "Hot reloadable fields"
// @Router /api/v1/config/fields [get]
func (h *ConfigAPIHandler) handleFields(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.handleCORS(w, r)
		return
	}

	if r.Method != http.MethodGet {
		h.methodNotAllowed(w, r)
		return
	}

	fields := make(map[string]FieldInfo)
	for path, field := range hotReloadableFields {
		info := FieldInfo{
			Path:            path,
			Description:     field.Description,
			RequiresRestart: field.RequiresRestart,
			Sensitive:       field.Sensitive,
		}

		// Get current value if not sensitive
		if !field.Sensitive {
			if value, err := h.manager.getFieldValue(path); err == nil {
				info.CurrentValue = value
			}
		}

		fields[path] = info
	}

	h.writeJSON(w, http.StatusOK, ConfigResponse{
		Success:   true,
		Message:   "Hot reloadable fields retrieved",
		Fields:    fields,
		Timestamp: time.Now(),
	})
}

// handleChanges returns the configuration change history
// @Summary Get configuration change history
// @Description Returns the history of configuration changes
// @Tags Configuration
// @Accept json
// @Produce json
// @Param limit query int false "Maximum number of changes to return" default(50)
// @Success 200 {object} ConfigResponse "Configuration changes"
// @Router /api/v1/config/changes [get]
func (h *ConfigAPIHandler) handleChanges(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.handleCORS(w, r)
		return
	}

	if r.Method != http.MethodGet {
		h.methodNotAllowed(w, r)
		return
	}

	// Parse limit parameter
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	changes := h.manager.GetChangeLog(limit)

	h.writeJSON(w, http.StatusOK, ConfigResponse{
		Success:   true,
		Message:   fmt.Sprintf("Retrieved %d configuration changes", len(changes)),
		Changes:   changes,
		Timestamp: time.Now(),
	})
}

// =============================================================================
// Helper Methods
// =============================================================================

// writeJSON writes a JSON response
func (h *ConfigAPIHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// handleCORS handles CORS preflight requests
func (h *ConfigAPIHandler) handleCORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// methodNotAllowed returns a 405 Method Not Allowed response
func (h *ConfigAPIHandler) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusMethodNotAllowed, ConfigResponse{
		Success:   false,
		Error:     fmt.Sprintf("Method %s not allowed", r.Method),
		Timestamp: time.Now(),
	})
}

// =============================================================================
// Middleware
// =============================================================================

// ConfigAPIMiddleware provides middleware for the configuration API
type ConfigAPIMiddleware struct {
	handler *ConfigAPIHandler
	apiKey  string
}

// NewConfigAPIMiddleware creates a new configuration API middleware
func NewConfigAPIMiddleware(handler *ConfigAPIHandler, apiKey string) *ConfigAPIMiddleware {
	return &ConfigAPIMiddleware{
		handler: handler,
		apiKey:  apiKey,
	}
}

// RequireAuth wraps a handler with API key authentication
func (m *ConfigAPIMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for OPTIONS requests (CORS preflight)
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		// Check API key if configured
		if m.apiKey != "" {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				apiKey = r.URL.Query().Get("api_key")
			}

			if apiKey != m.apiKey {
				m.handler.writeJSON(w, http.StatusUnauthorized, ConfigResponse{
					Success:   false,
					Error:     "Invalid or missing API key",
					Timestamp: time.Now(),
				})
				return
			}
		}

		next(w, r)
	}
}

// LogRequests wraps a handler with request logging
func (m *ConfigAPIMiddleware) LogRequests(next http.HandlerFunc, logger func(method, path string, status int, duration time.Duration)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next(wrapped, r)

		if logger != nil {
			logger(r.Method, r.URL.Path, wrapped.status, time.Since(start))
		}
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
