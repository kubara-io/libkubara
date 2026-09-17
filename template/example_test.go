package template_test

import (
	"context"
	"fmt"
	"log"

	libtemplate "github.com/kubara-io/libkubara/template"
)

func ExampleEngine_Render() {
	engine, err := libtemplate.New(libtemplate.WithMissingKeyError())
	if err != nil {
		log.Fatal(err)
	}

	input := libtemplate.Input{
		Name:   "greeting.txt",
		Source: []byte("Hello {{ .name | upper }}! Items: {{ toYaml .items }}"),
	}

	data := map[string]any{
		"name":  "world",
		"items": []string{"first", "second"},
	}

	output, err := engine.Render(context.Background(), input, data)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(output))

	// Output:
	// Hello WORLD! Items: - first
	// - second
}

func ExampleData() {
	dataBuilder := libtemplate.NewData()
	_ = dataBuilder.Namespace("app", map[string]any{"name": "storefront"})
	_ = dataBuilder.Namespace("cluster", map[string]any{"region": "eu-central-1"})

	contextData, err := dataBuilder.Build()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("App: %v, Region: %v\n",
		contextData["app"].(map[string]any)["name"],
		contextData["cluster"].(map[string]any)["region"],
	)

	// Output:
	// App: storefront, Region: eu-central-1
}
