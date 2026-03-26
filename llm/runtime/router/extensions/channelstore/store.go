package channelstore

import (
	"context"
	"strings"
	"sync"
)

// Channel describes a reusable channel record supplied by an external store.
// It is intentionally storage-agnostic and does not assume any database schema.
type Channel struct {
	ID       string
	Provider string
	BaseURL  string
	Region   string
	Priority int
	Weight   int
	Disabled bool
	Metadata map[string]string
	Extra    map[string]any
}

// Key describes a reusable channel key record supplied by an external store.
type Key struct {
	ID        string
	ChannelID string
	BaseURL   string
	Region    string
	Priority  int
	Weight    int
	Disabled  bool
	Metadata  map[string]string
}

// ModelMapping describes a reusable channel model mapping record.
type ModelMapping struct {
	ID          string
	ChannelID   string
	PublicModel string
	RemoteModel string
	Provider    string
	BaseURL     string
	Region      string
	Priority    int
	Weight      int
	Disabled    bool
	Metadata    map[string]string
}

// Secret contains resolved secret material for a key.
type Secret struct {
	APIKey    string
	SecretKey string
	Headers   map[string]string
	Metadata  map[string]string
}

// ModelMappingSource provides mapping candidates without hardcoding storage details.
type ModelMappingSource interface {
	FindMappingsByModel(ctx context.Context, model string) ([]ModelMapping, error)
	FindMappingsByProvider(ctx context.Context, provider string) ([]ModelMapping, error)
}

// ChannelSource resolves channel metadata for candidate channel IDs.
type ChannelSource interface {
	GetChannels(ctx context.Context, channelIDs []string) ([]Channel, error)
}

// KeySource lists available keys for a selected channel.
type KeySource interface {
	ListKeys(ctx context.Context, channelID string) ([]Key, error)
}

// SecretSource resolves secret material for a selected key.
type SecretSource interface {
	GetSecret(ctx context.Context, keyID string) (*Secret, error)
}

// Store is the aggregate source interface for the channelstore extension.
type Store interface {
	ModelMappingSource
	ChannelSource
	KeySource
	SecretSource
}

// StaticStoreConfig describes the in-memory records for StaticStore.
type StaticStoreConfig struct {
	Channels []Channel
	Keys     []Key
	Mappings []ModelMapping
	Secrets  map[string]Secret
}

// StaticStore is a generic in-memory implementation useful for tests, demos,
// and simple integrations that do not need a dedicated database adapter.
type StaticStore struct {
	mu       sync.RWMutex
	channels map[string]Channel
	keys     map[string][]Key
	mappings []ModelMapping
	secrets  map[string]Secret
}

// NewStaticStore creates a generic in-memory channel store.
func NewStaticStore(cfg StaticStoreConfig) *StaticStore {
	store := &StaticStore{
		channels: make(map[string]Channel, len(cfg.Channels)),
		keys:     make(map[string][]Key),
		mappings: make([]ModelMapping, 0, len(cfg.Mappings)),
		secrets:  make(map[string]Secret, len(cfg.Secrets)),
	}
	for _, channel := range cfg.Channels {
		store.channels[normalize(channel.ID)] = cloneChannel(channel)
	}
	for _, key := range cfg.Keys {
		channelID := normalize(key.ChannelID)
		store.keys[channelID] = append(store.keys[channelID], cloneKey(key))
	}
	for _, mapping := range cfg.Mappings {
		store.mappings = append(store.mappings, cloneModelMapping(mapping))
	}
	for keyID, secret := range cfg.Secrets {
		store.secrets[normalize(keyID)] = cloneSecretValue(secret)
	}
	return store
}

// FindMappingsByModel returns all enabled mappings for the requested public model.
func (s *StaticStore) FindMappingsByModel(_ context.Context, model string) ([]ModelMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	model = normalize(model)
	if model == "" {
		return nil, nil
	}

	var mappings []ModelMapping
	for _, mapping := range s.mappings {
		if mapping.Disabled {
			continue
		}
		if normalize(mapping.PublicModel) != model {
			continue
		}
		mappings = append(mappings, cloneModelMapping(mapping))
	}
	return mappings, nil
}

// FindMappingsByProvider returns all enabled mappings for the requested provider.
func (s *StaticStore) FindMappingsByProvider(_ context.Context, provider string) ([]ModelMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	provider = normalize(provider)
	if provider == "" {
		return nil, nil
	}

	var mappings []ModelMapping
	for _, mapping := range s.mappings {
		if mapping.Disabled {
			continue
		}
		if normalize(mapping.Provider) != provider {
			continue
		}
		mappings = append(mappings, cloneModelMapping(mapping))
	}
	return mappings, nil
}

// GetChannels returns the enabled channel records for the provided IDs.
func (s *StaticStore) GetChannels(_ context.Context, channelIDs []string) ([]Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(channelIDs) == 0 {
		return nil, nil
	}

	result := make([]Channel, 0, len(channelIDs))
	for _, channelID := range uniqueNormalized(channelIDs) {
		channel, ok := s.channels[channelID]
		if !ok || channel.Disabled {
			continue
		}
		result = append(result, cloneChannel(channel))
	}
	return result, nil
}

// ListKeys returns enabled keys for a channel.
func (s *StaticStore) ListKeys(_ context.Context, channelID string) ([]Key, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channelID = normalize(channelID)
	if channelID == "" {
		return nil, nil
	}

	keys := s.keys[channelID]
	result := make([]Key, 0, len(keys))
	for _, key := range keys {
		if key.Disabled {
			continue
		}
		result = append(result, cloneKey(key))
	}
	return result, nil
}

// GetSecret returns secret material for a key ID.
func (s *StaticStore) GetSecret(_ context.Context, keyID string) (*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	secret, ok := s.secrets[normalize(keyID)]
	if !ok {
		return nil, nil
	}
	cloned := cloneSecretValue(secret)
	return &cloned, nil
}

func cloneChannel(channel Channel) Channel {
	channel.Metadata = cloneStringMap(channel.Metadata)
	channel.Extra = cloneAnyMap(channel.Extra)
	return channel
}

func cloneKey(key Key) Key {
	key.Metadata = cloneStringMap(key.Metadata)
	return key
}

func cloneModelMapping(mapping ModelMapping) ModelMapping {
	mapping.Metadata = cloneStringMap(mapping.Metadata)
	return mapping
}

func cloneSecretValue(secret Secret) Secret {
	secret.Headers = cloneStringMap(secret.Headers)
	secret.Metadata = cloneStringMap(secret.Metadata)
	return secret
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func mergeStringMaps(parts ...map[string]string) map[string]string {
	size := 0
	for _, part := range parts {
		size += len(part)
	}
	if size == 0 {
		return nil
	}
	merged := make(map[string]string, size)
	for _, part := range parts {
		for key, value := range part {
			merged[key] = value
		}
	}
	return merged
}

func mergeAnyMaps(parts ...map[string]any) map[string]any {
	size := 0
	for _, part := range parts {
		size += len(part)
	}
	if size == 0 {
		return nil
	}
	merged := make(map[string]any, size)
	for _, part := range parts {
		for key, value := range part {
			merged[key] = value
		}
	}
	return merged
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueNormalized(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := normalize(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
