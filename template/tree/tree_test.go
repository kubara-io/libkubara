package tree_test

import (
	"context"
	"testing"
	"testing/fstest"

	libtemplate "github.com/kubara-io/libkubara/template"
	"github.com/kubara-io/libkubara/template/tree"
)

func TestRenderTemplateAndCopyStaticFile(t *testing.T) {
	engine, err := libtemplate.New()
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := tree.New(engine, tree.WithSources(tree.Source{Name: "base", FS: fstest.MapFS{
		"hello.txt.tplt": {Data: []byte("Hello {{ .name }}")},
		"static.txt":     {Data: []byte("static")},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	results, err := renderer.Render(context.Background(), map[string]any{"name": "Kubara"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Path != "hello.txt" || string(results[0].Content) != "Hello Kubara" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestRenderAllKeepsTemplateErrorsWithTheirResult(t *testing.T) {
	engine, err := libtemplate.New()
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := tree.New(engine, tree.WithSources(tree.Source{Name: "base", FS: fstest.MapFS{
		"broken.txt.tplt": {Data: []byte("{{ .missing }}")},
		"static.txt":      {Data: []byte("static")},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	results, err := renderer.RenderAll(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("RenderAll() error = nil, want template error")
	}
	if len(results) != 2 || results[0].Error == nil || string(results[1].Content) != "static" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestCollisionResolverControlsSelection(t *testing.T) {
	engine, err := libtemplate.New()
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := tree.New(
		engine,
		tree.WithSources(tree.Source{Name: "base", FS: fstest.MapFS{
			"a.txt": {Data: []byte("first")},
			"b.txt": {Data: []byte("second")},
		}}),
		tree.WithKeyFunc(func(tree.Entry) (string, error) { return "shared", nil }),
		tree.WithCollisionResolver(func(current, next tree.Entry) (bool, error) {
			return next.Path > current.Path, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	results, err := renderer.Render(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || string(results[0].Content) != "second" {
		t.Fatalf("unexpected results: %#v", results)
	}
}
