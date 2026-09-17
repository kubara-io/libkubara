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
  name: widgets.example.com
spec:
  group: example.com
  names:
    kind: Widget
    listKind: WidgetList
    plural: widgets
    singular: widget
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
                - count
              properties:
                count:
                  type: integer
                active:
                  type: boolean
                  default: true
`

type widgetSpec struct {
	Count  int  `json:"count"`
	Active bool `json:"active"`
}

type widgetResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              widgetSpec `json:"spec"`
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
kind: Widget
metadata:
  name: widget-1
  namespace: default
spec:
  count: 42
`
	validObj, err := manifest.DecodeOneString(validCR)
	if err != nil {
		t.Fatalf("DecodeOneString: %v", err)
	}

	validResult := validator.ValidateCreate(ctx, validObj, crdvalidate.RejectUnknown)
	if !validResult.Valid() {
		t.Fatalf("expected valid result, got: %v", validResult.Err())
	}

	var w widgetResource
	if err := validResult.Into(&w); err != nil {
		t.Fatalf("validResult.Into failed: %v", err)
	}
	if w.Name != "widget-1" || w.Spec.Count != 42 || !w.Spec.Active {
		t.Fatalf("unexpected widget struct after Into: %+v", w)
	}

	// 4. Invalid resource scenario: missing required 'count'
	invalidCR := `
apiVersion: example.com/v1
kind: Widget
metadata:
  name: widget-2
  namespace: default
spec:
  active: false
`
	invalidObj, err := manifest.DecodeOneString(invalidCR)
	if err != nil {
		t.Fatalf("DecodeOneString: %v", err)
	}

	invalidResult := validator.ValidateCreate(ctx, invalidObj, crdvalidate.RejectUnknown)
	if invalidResult.Valid() {
		t.Fatal("expected invalid result to fail validation")
	}

	var w2 widgetResource
	if err := invalidResult.Into(&w2); err == nil {
		t.Fatal("expected invalidResult.Into to fail with validation error")
	}
}
