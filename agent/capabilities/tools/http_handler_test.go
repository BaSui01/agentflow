package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupProtocolWithAgent(t *testing.T) *DiscoveryProtocol {
	t.Helper()
	reg := newCovTestRegistry(t)
	config := &ProtocolConfig{EnableLocal: true}
	proto := NewDiscoveryProtocol(config, reg, zap.NewNop())

	card := a2a.NewAgentCard("agent1", "Agent1", "http://localhost", "1.0")
	info := &AgentInfo{
		Card:    card,
		Status:  AgentStatusOnline,
		IsLocal: true,
		Capabilities: []CapabilityInfo{
			{Capability: a2a.Capability{Name: "search"}, AgentID: "agent1", Status: CapabilityStatusActive, Score: 50},
		},
	}
	require.NoError(t, proto.Announce(context.Background(), info))
	return proto
}

func TestHandleListAgents(t *testing.T) {
	proto := setupProtocolWithAgent(t)

	t.Run("GET returns agents", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/discovery/agents", nil)
		w := httptest.NewRecorder()
		proto.handleListAgents(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	})

	t.Run("GET with capabilities filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/discovery/agents?capabilities=search", nil)
		w := httptest.NewRecorder()
		proto.handleListAgents(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POST not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/discovery/agents", nil)
		w := httptest.NewRecorder()
		proto.handleListAgents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
func TestHandleGetAgent(t *testing.T) {
	proto := setupProtocolWithAgent(t)

	t.Run("GET existing agent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/discovery/agents/agent1", nil)
		w := httptest.NewRecorder()
		proto.handleGetAgent(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET nonexistent agent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/discovery/agents/nonexistent", nil)
		w := httptest.NewRecorder()
		proto.handleGetAgent(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET empty agent ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/discovery/agents/", nil)
		w := httptest.NewRecorder()
		proto.handleGetAgent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/discovery/agents/agent1", nil)
		w := httptest.NewRecorder()
		proto.handleGetAgent(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestHandleAnnounce(t *testing.T) {
	proto := setupProtocolWithAgent(t)

	t.Run("POST valid agent", func(t *testing.T) {
		card := a2a.NewAgentCard("agent2", "Agent2", "http://localhost", "1.0")
		info := &AgentInfo{Card: card, Status: AgentStatusOnline, IsLocal: true}
		body, _ := json.Marshal(info)

		req := httptest.NewRequest(http.MethodPost, "/discovery/announce", bytes.NewReader(body))
		w := httptest.NewRecorder()
		proto.handleAnnounce(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POST invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/discovery/announce", bytes.NewReader([]byte("invalid")))
		w := httptest.NewRecorder()
		proto.handleAnnounce(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST body too large", func(t *testing.T) {
		oversized := bytes.Repeat([]byte("a"), maxAnnounceBodyBytes+1)
		req := httptest.NewRequest(http.MethodPost, "/discovery/announce", bytes.NewReader(oversized))
		w := httptest.NewRecorder()
		proto.handleAnnounce(w, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})

	t.Run("POST internal error is sanitized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/discovery/announce", bytes.NewReader([]byte(`{}`)))
		w := httptest.NewRecorder()
		proto.handleAnnounce(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), internalProtocolErrMsg)
		assert.NotContains(t, w.Body.String(), "invalid agent info")
	})

	t.Run("GET not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/discovery/announce", nil)
		w := httptest.NewRecorder()
		proto.handleAnnounce(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestHandleHealth(t *testing.T) {
	proto := setupProtocolWithAgent(t)

	req := httptest.NewRequest(http.MethodGet, "/discovery/health", nil)
	w := httptest.NewRecorder()
	proto.handleHealth(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthy")
}

func TestProcessMulticastAnnouncement(t *testing.T) {
	reg := newCovTestRegistry(t)
	proto := NewDiscoveryProtocol(&ProtocolConfig{EnableLocal: true}, reg, zap.NewNop())

	t.Run("nil info", func(t *testing.T) {
		proto.processMulticastAnnouncement(nil)
	})

	t.Run("nil card", func(t *testing.T) {
		proto.processMulticastAnnouncement(&AgentInfo{})
	})

	t.Run("valid announcement", func(t *testing.T) {
		card := a2a.NewAgentCard("remote-agent", "Remote", "http://remote", "1.0")
		proto.processMulticastAnnouncement(&AgentInfo{Card: card, Status: AgentStatusOnline})
	})
}

func TestCapabilityMatcher_CalculateMatchScore_ExcludedAgent(t *testing.T) {
	reg := newCovTestRegistry(t)
	registerCovTestAgent(t, reg, "agent1", []string{"search"})
	matcher := NewCapabilityMatcher(reg, nil, zap.NewNop())

	ctx := context.Background()
	results, err := matcher.Match(ctx, &MatchRequest{
		RequiredCapabilities: []string{"search"},
		ExcludedAgents:       []string{"agent1"},
	})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestCapabilityMatcher_CalculateMatchScore_MaxLoad(t *testing.T) {
	reg := newCovTestRegistry(t)
	registerCovTestAgent(t, reg, "agent1", []string{"search"})
	matcher := NewCapabilityMatcher(reg, nil, zap.NewNop())

	ctx := context.Background()
	results, err := matcher.Match(ctx, &MatchRequest{
		RequiredCapabilities: []string{"search"},
		MaxLoad:              0.01, // very low max load
	})
	require.NoError(t, err)
	// Agent has 0 load, should still match
	assert.NotEmpty(t, results)
}

func TestCapabilityMatcher_CapabilityMatches_Fuzzy(t *testing.T) {
	reg := newCovTestRegistry(t)
	registerCovTestAgent(t, reg, "agent1", []string{"code_review"})
	matcher := NewCapabilityMatcher(reg, nil, zap.NewNop())

	ctx := context.Background()
	// Test fuzzy matching with partial name
	results, err := matcher.Match(ctx, &MatchRequest{
		TaskDescription:      "review code",
		RequiredCapabilities: []string{"code_review"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}
