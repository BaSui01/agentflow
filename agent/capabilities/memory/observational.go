package memory

import (
	"context"
	"time"
)

// Observation is a single dated observation compressed from conversation history.
type Observation struct {
	ID        string         `json:"id"`
	AgentID   string         `json:"agent_id"`
	Date      string         `json:"date"`
	Content   string         `json:"content"`
	CreatedAt time.Time      `json:"created_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ObservationStore persists and retrieves observations.
type ObservationStore interface {
	Save(ctx context.Context, obs Observation) error
	LoadRecent(ctx context.Context, agentID string, limit int) ([]Observation, error)
	LoadByDateRange(ctx context.Context, agentID string, start, end time.Time) ([]Observation, error)
}

// InMemoryObservationStore is a simple in-memory implementation for dev/test.
type InMemoryObservationStore struct {
	observations []Observation
}

func NewInMemoryObservationStore() *InMemoryObservationStore {
	return &InMemoryObservationStore{}
}

func (s *InMemoryObservationStore) Save(_ context.Context, obs Observation) error {
	s.observations = append(s.observations, obs)
	return nil
}

func (s *InMemoryObservationStore) LoadRecent(_ context.Context, agentID string, limit int) ([]Observation, error) {
	var results []Observation
	for i := len(s.observations) - 1; i >= 0 && len(results) < limit; i-- {
		if s.observations[i].AgentID == agentID {
			results = append(results, s.observations[i])
		}
	}
	return results, nil
}

func (s *InMemoryObservationStore) LoadByDateRange(_ context.Context, agentID string, start, end time.Time) ([]Observation, error) {
	var results []Observation
	for _, obs := range s.observations {
		if obs.AgentID == agentID && !obs.CreatedAt.Before(start) && !obs.CreatedAt.After(end) {
			results = append(results, obs)
		}
	}
	return results, nil
}
