package crdvalidate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kubara-io/libkubara/crdvalidate"
	"github.com/kubara-io/libkubara/manifest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const sampleCRDYAML = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: databases.example.com
spec:
  group: example.com
  names:
    kind: Database
    listKind: DatabaseList
    plural: databases
    singular: database
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
            apiVersion:
              type: string
            kind:
              type: string
            metadata:
              type: object
            spec:
              type: object
              required:
                - replicas
              properties:
                replicas:
                  type: integer
                enabled:
                  type: boolean
                  default: true
`

type databaseSpec struct {
	Replicas int  `json:"replicas"`
	Enabled  bool `json:"enabled"`
}

type databaseResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              databaseSpec `json:"spec"`
}

func TestCompileAndValidate(t *testing.T) {
	ctx := context.Background()

	// 1. Compile from io.Reader
	validator, err := crdvalidate.Compile(strings.NewReader(sampleCRDYAML))
	if err != nil {
		t.Fatalf("Compile from reader: %v", err)
	}

	// 2. DecodeCRDBytes / DecodeCRDString and Definition.Compile()
	defBytes, err := crdvalidate.DecodeCRDBytes([]byte(sampleCRDYAML))
	if err != nil {
		t.Fatalf("DecodeCRDBytes: %v", err)
	}
	v2, err := defBytes.Compile()
	if err != nil {
		t.Fatalf("Definition.Compile: %v", err)
	}
	if v2 == nil {
		t.Fatal("expected non-nil validator")
	}

	defStr, err := crdvalidate.DecodeCRDString(sampleCRDYAML)
	if err != nil {
		t.Fatalf("DecodeCRDString: %v", err)
	}
	if defStr == nil {
		t.Fatal("expected non-nil definition")
	}

	// 3. Valid resource scenario
	validCR := `
apiVersion: example.com/v1
kind: Database
metadata:
  name: postgres-primary
  namespace: default
spec:
  replicas: 3
`
	validObj, err := manifest.DecodeOneString(validCR)
	if err != nil {
		t.Fatalf("DecodeOneString: %v", err)
	}

	validResult := validator.ValidateCreate(ctx, validObj, crdvalidate.RejectUnknown)
	if !validResult.Valid() {
		t.Fatalf("expected valid result, got: %v", validResult.Err())
	}

	var db databaseResource
	if err := validResult.Into(&db); err != nil {
		t.Fatalf("validResult.Into failed: %v", err)
	}
	if db.Name != "postgres-primary" || db.Spec.Replicas != 3 || !db.Spec.Enabled {
		t.Fatalf("unexpected database struct after Into: %+v", db)
	}

	// 4. Invalid resource scenario: missing required 'replicas'
	invalidCR := `
apiVersion: example.com/v1
kind: Database
metadata:
  name: broken-db
  namespace: default
spec:
  enabled: false
`
	invalidObj, err := manifest.DecodeOneString(invalidCR)
	if err != nil {
		t.Fatalf("DecodeOneString: %v", err)
	}

	invalidResult := validator.ValidateCreate(ctx, invalidObj, crdvalidate.RejectUnknown)
	if invalidResult.Valid() {
		t.Fatal("expected invalid result to fail validation")
	}

	var db2 databaseResource
	if err := invalidResult.Into(&db2); err == nil {
		t.Fatal("expected invalidResult.Into to fail with validation error")
	}
}

func TestValidateUpdateAndTransition(t *testing.T) {
	ctx := context.Background()
	validator, err := crdvalidate.Compile(strings.NewReader(sampleCRDYAML))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	oldCR := `
apiVersion: example.com/v1
kind: Database
metadata:
  name: db-lifecycle
  namespace: default
  resourceVersion: "1"
spec:
  replicas: 1
`
	oldObj, err := manifest.DecodeOneString(oldCR)
	if err != nil {
		t.Fatal(err)
	}

	newCR := `
apiVersion: example.com/v1
kind: Database
metadata:
  name: db-lifecycle
  namespace: default
  resourceVersion: "1"
spec:
  replicas: 3
`
	newObj, err := manifest.DecodeOneString(newCR)
	if err != nil {
		t.Fatal(err)
	}

	// ValidateUpdate
	updateResult := validator.ValidateUpdate(ctx, newObj, oldObj, crdvalidate.RejectUnknown)
	if !updateResult.Valid() {
		t.Fatalf("expected ValidateUpdate to succeed, got: %v", updateResult.Err())
	}
	var updatedDB databaseResource
	if err := updateResult.Into(&updatedDB); err != nil {
		t.Fatalf("updateResult.Into failed: %v", err)
	}
	if updatedDB.Spec.Replicas != 3 {
		t.Fatalf("expected replicas=3, got %d", updatedDB.Spec.Replicas)
	}

	// ValidateTransition with resourceVersion omitted
	unversionedOld, _ := manifest.DecodeOneString(`
apiVersion: example.com/v1
kind: Database
metadata:
  name: db-lifecycle
  namespace: default
spec:
  replicas: 1
`)
	unversionedNew, _ := manifest.DecodeOneString(`
apiVersion: example.com/v1
kind: Database
metadata:
  name: db-lifecycle
  namespace: default
spec:
  replicas: 3
`)
	transitionResult := validator.ValidateTransition(ctx, unversionedNew, unversionedOld, crdvalidate.RejectUnknown)
	if !transitionResult.Valid() {
		t.Fatalf("expected ValidateTransition without resourceVersion to succeed, got: %v", transitionResult.Err())
	}
	if transitionResult.Object.Name() != "db-lifecycle" {
		t.Fatalf("unexpected transition object name: %s", transitionResult.Object.Name())
	}
}

func TestPruningModes(t *testing.T) {
	ctx := context.Background()
	validator, err := crdvalidate.Compile(strings.NewReader(sampleCRDYAML))
	if err != nil {
		t.Fatal(err)
	}

	crWithUnknown := `
apiVersion: example.com/v1
kind: Database
metadata:
  name: db-unknown
  namespace: default
spec:
  replicas: 2
  unexpectedField: "foo"
`
	obj, err := manifest.DecodeOneString(crWithUnknown)
	if err != nil {
		t.Fatal(err)
	}

	// PruneUnknown mode allows unknown fields and tracks pruned paths
	prunedResult := validator.ValidateCreate(ctx, obj, crdvalidate.PruneUnknown)
	if !prunedResult.Valid() {
		t.Fatalf("expected PruneUnknown to be valid, got: %v", prunedResult.Err())
	}
	if len(prunedResult.PrunedPaths) == 0 {
		t.Fatal("expected pruned paths for unexpectedField")
	}

	// RejectUnknown mode fails validation
	rejectedResult := validator.ValidateCreate(ctx, obj, crdvalidate.RejectUnknown)
	if rejectedResult.Valid() {
		t.Fatal("expected RejectUnknown to fail for unexpectedField")
	}
}

func TestValidationKindGroupVersionMismatches(t *testing.T) {
	ctx := context.Background()
	validator, err := crdvalidate.Compile(strings.NewReader(sampleCRDYAML))
	if err != nil {
		t.Fatal(err)
	}

	// Mismatched Kind
	wrongKindCR := `
apiVersion: example.com/v1
kind: Cluster
metadata:
  name: test
  namespace: default
spec:
  replicas: 1
`
	objKind, _ := manifest.DecodeOneString(wrongKindCR)
	res := validator.ValidateCreate(ctx, objKind, crdvalidate.RejectUnknown)
	if res.Valid() {
		t.Fatal("expected error for mismatched kind")
	}

	// Mismatched Group
	wrongGroupCR := `
apiVersion: other.com/v1
kind: Database
metadata:
  name: test
  namespace: default
spec:
  replicas: 1
`
	objGroup, _ := manifest.DecodeOneString(wrongGroupCR)
	resGroup := validator.ValidateCreate(ctx, objGroup, crdvalidate.RejectUnknown)
	if resGroup.Valid() {
		t.Fatal("expected error for mismatched group")
	}

	// Mismatched Version (triggers servedAPIVersions)
	wrongVerCR := `
apiVersion: example.com/v2
kind: Database
metadata:
  name: test
  namespace: default
spec:
  replicas: 1
`
	objVer, _ := manifest.DecodeOneString(wrongVerCR)
	resVer := validator.ValidateCreate(ctx, objVer, crdvalidate.RejectUnknown)
	if resVer.Valid() {
		t.Fatal("expected error for unsupported version")
	}
}

func TestDefinitionAndCompilationErrors(t *testing.T) {
	// NewDefinition nil
	if _, err := crdvalidate.NewDefinition(nil); err == nil {
		t.Fatal("expected error for NewDefinition(nil)")
	}

	// NewDefinition wrong apiVersion
	wrongAPIVersionObj, _ := manifest.DecodeOneString(`
apiVersion: v1
kind: CustomResourceDefinition
metadata:
  name: test
`)
	if _, err := crdvalidate.NewDefinition(wrongAPIVersionObj); err == nil {
		t.Fatal("expected error for wrong apiVersion in CRD")
	}

	// NewDefinition wrong kind
	wrongKindObj, _ := manifest.DecodeOneString(`
apiVersion: apiextensions.k8s.io/v1
kind: Pod
metadata:
  name: test
`)
	if _, err := crdvalidate.NewDefinition(wrongKindObj); err == nil {
		t.Fatal("expected error for wrong kind in CRD")
	}

	// (*Definition)(nil).Compile()
	var nilDef *crdvalidate.Definition
	if _, err := nilDef.Compile(); err == nil {
		t.Fatal("expected error compiling nil definition")
	}

	// Compile nil reader
	if _, err := crdvalidate.Compile(nil); err == nil {
		t.Fatal("expected error for Compile(nil)")
	}

	// Compile CRD missing served versions with schema
	incompleteCRD := `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: broken.example.com
spec:
  group: example.com
  names:
    kind: Broken
    plural: brokens
  scope: Namespaced
  versions:
    - name: v1
      served: false
`
	if _, err := crdvalidate.Compile(strings.NewReader(incompleteCRD)); err == nil {
		t.Fatal("expected error compiling CRD without served version schemas")
	}

	// ValidateCreate with nil object
	ctx := context.Background()
	validVal, _ := crdvalidate.Compile(strings.NewReader(sampleCRDYAML))
	nilObjResult := validVal.ValidateCreate(ctx, nil, crdvalidate.RejectUnknown)
	if nilObjResult.Valid() {
		t.Fatal("expected ValidateCreate with nil object to fail")
	}

	// Result.Into with nil Object when Valid is true
	emptyResult := crdvalidate.Result{}
	var target databaseResource
	if err := emptyResult.Into(&target); err == nil {
		t.Fatal("expected error on Result.Into with nil Object")
	}
}
