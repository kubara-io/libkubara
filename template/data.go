package template

import (
	"encoding/json"
	"fmt"
)

// Data constructs namespaced template data contexts from JSON-serializable values,
// manifest objects, or Go structs.
type Data struct {
	values map[string]any
}

// NewData creates an empty Data builder.
func NewData() *Data { return &Data{values: map[string]any{}} }

// Namespace registers a named top-level key with normalized, JSON-serializable data.
//
// Values can be maps, structs, primitives, or manifest objects (which implement json.Marshaler).
//
// Example:
//
//	dataBuilder := template.NewData()
//	_ = dataBuilder.Namespace("config", validatedManifestObj)
//	_ = dataBuilder.Namespace("cluster", map[string]any{"region": "us-east-1"})
//	contextMap, err := dataBuilder.Build()
//
// It returns an error if name is empty, if name is already registered, or if value
// cannot be normalized via JSON marshaling.
func (d *Data) Namespace(name string, value any) error {
	if name == "" {
		return fmt.Errorf("template namespace must not be empty")
	}
	if _, exists := d.values[name]; exists {
		return fmt.Errorf("template namespace %q already exists", name)
	}
	normalized, err := normalizeJSON(value)
	if err != nil {
		return fmt.Errorf("normalize template namespace %q: %w", name, err)
	}
	d.values[name] = normalized
	return nil
}

// Build finalizes and returns the complete namespaced template context map.
func (d *Data) Build() (map[string]any, error) {
	if d == nil {
		return map[string]any{}, nil
	}
	value, err := normalizeJSON(d.values)
	if err != nil {
		return nil, err
	}
	return value.(map[string]any), nil
}

func normalizeJSON(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}
