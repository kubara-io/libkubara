package crdvalidate_test

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/kubara-io/libkubara/crdvalidate"
	"github.com/kubara-io/libkubara/manifest"
)

func ExampleCompile() {
	crdYAML := `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: databases.example.com
spec:
  group: example.com
  names:
    kind: Database
    plural: databases
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          required:
            - spec
          properties:
            spec:
              type: object
              required:
                - engine
              properties:
                engine:
                  type: string
                replicas:
                  type: integer
                  default: 1
`

	validator, err := crdvalidate.Compile(strings.NewReader(crdYAML))
	if err != nil {
		log.Fatal(err)
	}

	crYAML := `
apiVersion: example.com/v1
kind: Database
metadata:
  name: my-db
  namespace: default
spec:
  engine: postgres
`

	obj, err := manifest.DecodeOneString(crYAML)
	if err != nil {
		log.Fatal(err)
	}

	result := validator.ValidateCreate(context.Background(), obj, crdvalidate.RejectUnknown)
	if !result.Valid() {
		log.Fatalf("validation failed: %v", result.Err())
	}

	// Schema default of replicas=1 was applied automatically
	replicas, _, _ := result.Object.NestedInt64("spec", "replicas")
	fmt.Printf("Database %s/%s validated with replicas=%d\n", result.Object.Namespace(), result.Object.Name(), replicas)

	// Output:
	// Database default/my-db validated with replicas=1
}

func ExampleResult_Into() {
	crdYAML := `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: databases.example.com
spec:
  group: example.com
  names:
    kind: Database
    plural: databases
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                engine:
                  type: string
                replicas:
                  type: integer
                  default: 2
`

	validator, err := crdvalidate.Compile(strings.NewReader(crdYAML))
	if err != nil {
		log.Fatal(err)
	}

	crYAML := `
apiVersion: example.com/v1
kind: Database
metadata:
  name: analytics-db
  namespace: default
spec:
  engine: clickhouse
`

	obj, _ := manifest.DecodeOneString(crYAML)
	result := validator.ValidateCreate(context.Background(), obj, crdvalidate.RejectUnknown)

	type databaseSpec struct {
		Engine   string `json:"engine"`
		Replicas int    `json:"replicas"`
	}
	type databaseResource struct {
		Spec databaseSpec `json:"spec"`
	}

	var db databaseResource
	if err := result.Into(&db); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Engine: %s, Replicas: %d\n", db.Spec.Engine, db.Spec.Replicas)

	// Output:
	// Engine: clickhouse, Replicas: 2
}

func ExampleValidator_ValidateTransition() {
	crdYAML := `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: databases.example.com
spec:
  group: example.com
  names:
    kind: Database
    plural: databases
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                replicas:
                  type: integer
`

	validator, _ := crdvalidate.Compile(strings.NewReader(crdYAML))

	currentCR, _ := manifest.DecodeOneString(`
apiVersion: example.com/v1
kind: Database
metadata:
  name: prod-db
  namespace: default
spec:
  replicas: 1
`)

	proposedCR, _ := manifest.DecodeOneString(`
apiVersion: example.com/v1
kind: Database
metadata:
  name: prod-db
  namespace: default
spec:
  replicas: 3
`)

	// Validate GitOps transition without needing metadata.resourceVersion
	result := validator.ValidateTransition(context.Background(), proposedCR, currentCR, crdvalidate.RejectUnknown)
	if !result.Valid() {
		log.Fatal(result.Err())
	}

	replicas, _, _ := result.Object.NestedInt64("spec", "replicas")
	fmt.Printf("Transition valid: scaled to %d replicas\n", replicas)

	// Output:
	// Transition valid: scaled to 3 replicas
}
