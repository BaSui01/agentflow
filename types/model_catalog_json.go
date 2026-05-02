package types

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// LoadModelDescriptorsJSON decodes model descriptors from JSON. The accepted
// payload is either a bare []ModelDescriptor or an object with a "models" array,
// which keeps generated snapshots easy to version with extra metadata.
func LoadModelDescriptorsJSON(r io.Reader) ([]ModelDescriptor, error) {
	if r == nil {
		return nil, fmt.Errorf("model catalog json reader cannot be nil")
	}
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode model catalog json: %w", err)
	}
	if decoder.More() {
		return nil, fmt.Errorf("model catalog json must contain a single top-level value")
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("model catalog json is empty")
	}

	var models []ModelDescriptor
	if err := json.Unmarshal(raw, &models); err != nil {
		var snapshot modelCatalogSnapshot
		if objErr := json.Unmarshal(raw, &snapshot); objErr != nil {
			return nil, fmt.Errorf("decode model catalog json as array or object: %w", err)
		}
		models = snapshot.Models
	}
	if err := validateModelDescriptors(models); err != nil {
		return nil, err
	}
	return cloneModelDescriptors(models), nil
}

// LoadModelCatalogJSON returns a lookup catalog decoded from JSON.
func LoadModelCatalogJSON(r io.Reader) (*ModelCatalog, error) {
	models, err := LoadModelDescriptorsJSON(r)
	if err != nil {
		return nil, err
	}
	return NewModelCatalog(models), nil
}

// LoadModelCatalogFile loads a JSON model catalog snapshot from disk.
func LoadModelCatalogFile(path string) (*ModelCatalog, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("model catalog path cannot be empty")
	}
	file, err := os.Open(trimmed)
	if err != nil {
		return nil, fmt.Errorf("open model catalog %q: %w", trimmed, err)
	}
	defer file.Close()
	return LoadModelCatalogJSON(file)
}

type modelCatalogSnapshot struct {
	VerifiedAt string            `json:"verified_at,omitempty"`
	Source     string            `json:"source,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Models     []ModelDescriptor `json:"models"`
}

func validateModelDescriptors(models []ModelDescriptor) error {
	if len(models) == 0 {
		return fmt.Errorf("model catalog must contain at least one model")
	}
	seen := make(map[string]struct{}, len(models))
	for i, model := range models {
		provider := strings.TrimSpace(model.Provider)
		id := strings.TrimSpace(model.ID)
		if provider == "" {
			return fmt.Errorf("model catalog entry %d missing provider", i)
		}
		if id == "" {
			return fmt.Errorf("model catalog entry %d missing id", i)
		}
		key := modelCatalogKey(provider, id)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("model catalog duplicate provider/id %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}
