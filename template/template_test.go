package template_test

import (
	"context"
	"errors"
	"testing"

	libtemplate "github.com/kubara-io/libkubara/template"
)

func TestEngineHermeticSprigAndYAML(t *testing.T) {
	engine, err := libtemplate.New()
	if err != nil {
		t.Fatal(err)
	}
	output, err := engine.Render(context.Background(), libtemplate.Input{
		Name:   "test",
		Source: []byte(`{{ .name | upper }}: {{ toYaml .items | nindent 2 }}`),
	}, map[string]any{"name": "demo", "items": []string{"one", "two"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "DEMO: \n  - one\n  - two" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestEngineMissingKeyAndOutputLimit(t *testing.T) {
	engine, err := libtemplate.New(libtemplate.WithMaxOutput(2))
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Render(context.Background(), libtemplate.Input{Name: "missing", Source: []byte(`{{ .missing }}`)}, map[string]any{})
	if err == nil {
		t.Fatal("expected missing-key error")
	}
	_, err = engine.Render(context.Background(), libtemplate.Input{Name: "large", Source: []byte(`abc`)}, nil)
	if !errors.Is(err, libtemplate.ErrOutputLimit) {
		t.Fatalf("expected output-limit error, got %v", err)
	}
}
