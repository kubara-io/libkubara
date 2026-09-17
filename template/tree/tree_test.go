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

func TestRendererCreationErrors(t *testing.T) {
	engine, err := libtemplate.New()
	if err != nil {
		t.Fatal(err)
	}

	// Nil engine
	if _, err := tree.New(nil); err == nil {
		t.Fatal("expected error for nil engine")
	}

	// No sources
	if _, err := tree.New(engine); err == nil {
		t.Fatal("expected error for no sources")
	}

	// Source with nil FS
	if _, err := tree.New(engine, tree.WithSources(tree.Source{FS: nil})); err == nil {
		t.Fatal("expected error for nil source FS")
	}
}

func TestRendererOptions(t *testing.T) {
	engine, err := libtemplate.New()
	if err != nil {
		t.Fatal(err)
	}

	renderer, err := tree.New(
		engine,
		tree.WithSources(tree.Source{
			Name: "test-src",
			FS: fstest.MapFS{
				"file1.custom": {Data: []byte("val1")},
				"skip-me.txt":  {Data: []byte("skipped")},
			},
		}),
		tree.WithTemplateMatcher(tree.Suffix(".custom")),
		tree.WithPredicate(func(e tree.Entry) bool {
			return e.Path != "skip-me.txt"
		}),
		tree.WithPathFunc(func(e tree.Entry) (string, error) {
			return "out/" + e.Path, nil
		}),
		tree.WithCollisionPolicy(tree.CollisionPreferLast),
	)
	if err != nil {
		t.Fatalf("New renderer failed: %v", err)
	}

	results, err := renderer.Render(context.Background(), nil)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if len(results) != 1 || results[0].Path != "out/file1.custom" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestCollisionPolicies(t *testing.T) {
	engine, _ := libtemplate.New()
	fs := fstest.MapFS{
		"a.txt": {Data: []byte("first")},
		"b.txt": {Data: []byte("second")},
	}

	// CollisionPreferFirst
	rendererKeepFirst, err := tree.New(
		engine,
		tree.WithSources(tree.Source{FS: fs}),
		tree.WithKeyFunc(func(tree.Entry) (string, error) { return "target", nil }),
		tree.WithCollisionPolicy(tree.CollisionPreferFirst),
	)
	if err != nil {
		t.Fatal(err)
	}
	resKeepFirst, err := rendererKeepFirst.Render(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resKeepFirst) != 1 || string(resKeepFirst[0].Content) != "first" {
		t.Fatalf("expected 'first', got %q", string(resKeepFirst[0].Content))
	}

	// CollisionPreferLast
	rendererReplace, err := tree.New(
		engine,
		tree.WithSources(tree.Source{FS: fs}),
		tree.WithKeyFunc(func(tree.Entry) (string, error) { return "target", nil }),
		tree.WithCollisionPolicy(tree.CollisionPreferLast),
	)
	if err != nil {
		t.Fatal(err)
	}
	resReplace, err := rendererReplace.Render(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resReplace) != 1 || string(resReplace[0].Content) != "second" {
		t.Fatalf("expected 'second', got %q", string(resReplace[0].Content))
	}

	// CollisionError
	rendererError, err := tree.New(
		engine,
		tree.WithSources(tree.Source{FS: fs}),
		tree.WithKeyFunc(func(tree.Entry) (string, error) { return "target", nil }),
		tree.WithCollisionPolicy(tree.CollisionError),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rendererError.Render(context.Background(), nil); err == nil {
		t.Fatal("expected collision error")
	}
}
