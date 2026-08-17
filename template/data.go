package template

import (
	"encoding/json"
	"fmt"
)

type Data struct {
	values map[string]any
}

func NewData() *Data { return &Data{values: map[string]any{}} }

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
