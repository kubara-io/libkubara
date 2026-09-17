package tree_test

import (
	"context"
	"fmt"
	"log"
	"testing/fstest"

	libtemplate "github.com/kubara-io/libkubara/template"
	"github.com/kubara-io/libkubara/template/tree"
)

func ExampleRenderer_Render() {
	engine, err := libtemplate.New()
	if err != nil {
		log.Fatal(err)
	}

	files := fstest.MapFS{
		"config.yaml.tplt": {Data: []byte("name: {{ .app.name }}\nreplicas: {{ .app.replicas }}")},
		"static.txt":       {Data: []byte("unprocessed static content")},
	}

	renderer, err := tree.New(
		engine,
		tree.WithSources(tree.Source{Name: "base", FS: files}),
	)
	if err != nil {
		log.Fatal(err)
	}

	data := map[string]any{
		"app": map[string]any{
			"name":     "payment-service",
			"replicas": 3,
		},
	}

	results, err := renderer.Render(context.Background(), data)
	if err != nil {
		log.Fatal(err)
	}

	for _, res := range results {
		fmt.Printf("%s:\n%s\n", res.Path, string(res.Content))
	}

	// Output:
	// config.yaml:
	// name: payment-service
	// replicas: 3
	// static.txt:
	// unprocessed static content
}
