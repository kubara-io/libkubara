package template_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	texttemplate "text/template"

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

func TestTemplateData(t *testing.T) {
	data := libtemplate.NewData()

	// Empty namespace error
	if err := data.Namespace("", "value"); err == nil {
		t.Fatal("expected error for empty namespace name")
	}

	// Valid namespace
	if err := data.Namespace("app", map[string]any{"env": "prod"}); err != nil {
		t.Fatalf("Namespace failed: %v", err)
	}

	// Duplicate namespace error
	if err := data.Namespace("app", "other"); err == nil {
		t.Fatal("expected error for duplicate namespace")
	}

	// Unserializable value error
	if err := data.Namespace("broken", make(chan int)); err == nil {
		t.Fatal("expected error for unserializable value")
	}

	// Build
	built, err := data.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if built["app"].(map[string]any)["env"] != "prod" {
		t.Fatalf("unexpected built data: %v", built)
	}

	// Build on nil Data
	var nilData *libtemplate.Data
	nilBuilt, err := nilData.Build()
	if err != nil || len(nilBuilt) != 0 {
		t.Fatalf("unexpected nil Data Build result: %v, err=%v", nilBuilt, err)
	}
}

func TestEngineOptionsAndFunctions(t *testing.T) {
	// Custom functions & WithFullSprig & WithDelims & YAML functions
	engine, err := libtemplate.New(
		libtemplate.WithFuncs(texttemplate.FuncMap{
			"customGreet": func(name string) string { return "Hello " + name },
		}),
		libtemplate.WithFullSprig(),
		libtemplate.WithDelims("[[", "]]"),
		libtemplate.WithMissingKey(libtemplate.MissingKeyZero),
	)
	if err != nil {
		t.Fatalf("New engine failed: %v", err)
	}

	out, err := engine.Render(context.Background(), libtemplate.Input{
		Source: []byte(`[[ customGreet .name ]] - [[ toYaml .map ]] - [[ (fromYaml "a: 1").a ]]`),
	}, map[string]any{"name": "World", "map": map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(string(out), "Hello World") || !strings.Contains(string(out), "k: v") {
		t.Fatalf("unexpected output: %s", string(out))
	}

	// WithoutSprig & WithoutYAMLFunctions
	bareEngine, err := libtemplate.New(
		libtemplate.WithoutSprig(),
		libtemplate.WithoutYAMLFunctions(),
		libtemplate.WithHermeticSprig(),
		libtemplate.WithMissingKeyError(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if bareEngine == nil {
		t.Fatal("expected non-nil bare engine")
	}

	// Engine creation errors:
	// 1. Invalid missing-key mode
	if _, err := libtemplate.New(libtemplate.WithMissingKey("invalid-mode")); err == nil {
		t.Fatal("expected error for invalid missing-key mode")
	}
	// 2. Mismatched delimiters (one empty)
	if _, err := libtemplate.New(libtemplate.WithDelims("[[", "")); err == nil {
		t.Fatal("expected error for mismatched delims")
	}
	// 3. Invalid function name
	if _, err := libtemplate.New(libtemplate.WithFuncs(texttemplate.FuncMap{"bad-func-name!": func() {}})); err == nil {
		t.Fatal("expected error for invalid function name")
	}
	// 4. Invalid function (nil func)
	if _, err := libtemplate.New(libtemplate.WithFuncs(texttemplate.FuncMap{"validName": nil})); err == nil {
		t.Fatal("expected error for nil function")
	}

	// Render on nil engine
	var nilEngine *libtemplate.Engine
	if _, err := nilEngine.Render(context.Background(), libtemplate.Input{Source: []byte("hi")}, nil); err == nil {
		t.Fatal("expected error rendering nil engine")
	}

	// Render with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.Render(canceledCtx, libtemplate.Input{Source: []byte("hi")}, nil); err == nil {
		t.Fatal("expected error rendering with canceled context")
	}

	// Render with parse error
	if _, err := engine.Render(context.Background(), libtemplate.Input{Source: []byte("[[ unclosed")}, nil); err == nil {
		t.Fatal("expected parse error")
	}
}
