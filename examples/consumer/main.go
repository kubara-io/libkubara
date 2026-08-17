package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"testing/fstest"

	"github.com/kubara-io/libkubara/crdvalidate"
	"github.com/kubara-io/libkubara/manifest"
	libtemplate "github.com/kubara-io/libkubara/template"
	"github.com/kubara-io/libkubara/template/tree"
)

//go:embed project-crd.yaml
var projectCRDYAML []byte

//go:embed project-config.yaml
var projectConfigYAML []byte

//go:embed project-config-update.yaml
var projectConfigUpdateYAML []byte

func main() {
	ctx := context.Background()

	definition, _ := crdvalidate.DecodeCRD(bytes.NewReader(projectCRDYAML))
	validator, _ := crdvalidate.Compile(definition)

	current, _ := manifest.DecodeOne(bytes.NewReader(projectConfigYAML))
	config := validator.ValidateCreate(ctx, current, crdvalidate.RejectUnknown)
	fmt.Printf("CRD create: %s/%s stage=%s replicas=%d\n",
		config.Object.Namespace(),
		config.Object.Name(),
		mustNestedString(config.Object, "spec", "stage"),
		mustNestedInt64(config.Object, "spec", "replicas"),
	)

	dataBuilder := libtemplate.NewData()
	dataBuilder.Namespace("config", config.Object.Data())
	data, _ := dataBuilder.Build()

	engine, _ := libtemplate.New(
		libtemplate.WithHermeticSprig(),
		libtemplate.WithMissingKeyError(),
	)

	templates := fstest.MapFS{
		"generated/project.txt.tplt": {Data: []byte(
			`Project {{ .config.metadata.name }} runs in {{ .config.spec.stage }} with {{ .config.spec.replicas }} replica(s)`,
		)},
		"generated/static.txt": {Data: []byte("copied unchanged")},
	}

	renderer, _ := tree.New(engine,
		tree.WithSources(tree.Source{Name: "application", FS: templates}),
	)

	results, _ := renderer.Render(ctx, data)
	for _, result := range results {
		fmt.Printf("%s: %s\n", result.Path, result.Content)
	}
}

func validateConfigLifecycle(ctx context.Context, definition *crdvalidate.Definition) *manifest.Object {
	validator, err := crdvalidate.Compile(definition)
	check(err)

	current, err := manifest.DecodeOne(bytes.NewReader(projectConfigYAML))
	check(err)
	created := validator.ValidateCreate(ctx, current, crdvalidate.RejectUnknown)
	check(created.Err())
	fmt.Printf("CRD create: %s/%s stage=%s replicas=%d\n",
		created.Object.Namespace(),
		created.Object.Name(),
		mustNestedString(created.Object, "spec", "stage"),
		mustNestedInt64(created.Object, "spec", "replicas"),
	)

	proposed, err := manifest.DecodeOne(bytes.NewReader(projectConfigUpdateYAML))
	check(err)
	updated := validator.ValidateTransition(ctx, proposed, created.Object, crdvalidate.RejectUnknown)
	check(updated.Err())
	fmt.Printf("CRD update: %s/%s stage=%s replicas=%d\n",
		updated.Object.Namespace(),
		updated.Object.Name(),
		mustNestedString(updated.Object, "spec", "stage"),
		mustNestedInt64(updated.Object, "spec", "replicas"),
	)

	return created.Object
}

func mustNestedString(object *manifest.Object, fields ...string) string {
	value, found, err := object.NestedString(fields...)
	check(err)
	if !found {
		panic(fmt.Sprintf("missing string field %v", fields))
	}
	return value
}

func mustNestedInt64(object *manifest.Object, fields ...string) int64 {
	value, found, err := object.NestedInt64(fields...)
	check(err)
	if !found {
		panic(fmt.Sprintf("missing integer field %v", fields))
	}
	return value
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
