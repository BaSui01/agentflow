package declarative

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentLoader loads AgentDefinition from files or raw bytes.
type AgentLoader interface {
	// LoadFile reads a file and parses it into an AgentDefinition.
	// Format is auto-detected from the file extension (.yaml, .yml, .json).
	LoadFile(path string) (*AgentDefinition, error)

	// LoadBytes parses raw bytes into an AgentDefinition.
	// format must be "yaml" or "json".
	LoadBytes(data []byte, format string) (*AgentDefinition, error)
}

// YAMLLoader implements AgentLoader for YAML and JSON formats.
type YAMLLoader struct{}

// NewYAMLLoader creates a new YAMLLoader.
func NewYAMLLoader() *YAMLLoader {
	return &YAMLLoader{}
}

// LoadFile reads a file and parses it based on extension.
func (l *YAMLLoader) LoadFile(path string) (*AgentDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent definition file: %w", err)
	}

	format := detectFormat(path)
	if format == "" {
		return nil, fmt.Errorf("unsupported file extension: %s", filepath.Ext(path))
	}

	return l.LoadBytes(data, format)
}

// LoadBytes parses raw bytes in the given format ("yaml" or "json").
func (l *YAMLLoader) LoadBytes(data []byte, format string) (*AgentDefinition, error) {
	var def AgentDefinition

	switch strings.ToLower(format) {
	case "yaml", "yml":
		if err := yaml.Unmarshal(data, &def); err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
	case "json":
		if err := json.Unmarshal(data, &def); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported format %q, use \"yaml\" or \"json\"", format)
	}

	return &def, nil
}

// detectFormat returns "yaml" or "json" based on file extension, or "" if unknown.
func detectFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	default:
		return ""
	}
}
