package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFromBytes parses a ResourceMap from raw YAML bytes.
func LoadFromBytes(data []byte) (*ResourceMap, error) {
	var rm ResourceMap
	if err := yaml.Unmarshal(data, &rm); err != nil {
		return nil, fmt.Errorf("failed to parse resource map YAML: %w", err)
	}
	if err := Validate(&rm); err != nil {
		return nil, err
	}
	return &rm, nil
}

// LoadFromFile reads a resource map YAML file from disk and parses it.
func LoadFromFile(path string) (*ResourceMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource map file %q: %w", path, err)
	}
	return LoadFromBytes(data)
}
