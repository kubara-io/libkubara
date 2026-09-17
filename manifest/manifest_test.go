package manifest_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kubara-io/libkubara/manifest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type sampleSpec struct {
	Replicas int      `json:"replicas"`
	Enabled  bool     `json:"enabled"`
	Tags     []string `json:"tags"`
}

type sampleResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              sampleSpec `json:"spec"`
}

func TestDecodeOnePreservesKubernetesIntegerTypes(t *testing.T) {
	object, err := manifest.DecodeOne(strings.NewReader(`
apiVersion: example.io/v1
kind: Example
metadata:
  name: demo
spec:
  replicas: 2
`))
	if err != nil {
		t.Fatal(err)
	}
	replicas, found, err := object.NestedInt64("spec", "replicas")
	if err != nil {
		t.Fatal(err)
	}
	if !found || replicas != 2 {
		t.Fatalf("expected int64 replicas=2, got %v", replicas)
	}
}

func TestDecodeHelpers(t *testing.T) {
	yamlContent := `
apiVersion: example.io/v1
kind: Example
metadata:
  name: doc1
---
apiVersion: example.io/v1
kind: Example
metadata:
  name: doc2
`
	objsFromBytes, err := manifest.DecodeBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("DecodeBytes: %v", err)
	}
	if len(objsFromBytes) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objsFromBytes))
	}

	objsFromString, err := manifest.DecodeString(yamlContent)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	if len(objsFromString) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objsFromString))
	}

	singleYAML := `
apiVersion: example.io/v1
kind: Example
metadata:
  name: single
  namespace: test-ns
  labels:
    app: my-app
  annotations:
    note: "important"
spec:
  replicas: 3
  enabled: true
  tags:
    - alpha
    - beta
`
	objFromBytes, err := manifest.DecodeOneBytes([]byte(singleYAML))
	if err != nil {
		t.Fatalf("DecodeOneBytes: %v", err)
	}
	if objFromBytes.Name() != "single" || objFromBytes.Namespace() != "test-ns" {
		t.Fatalf("unexpected metadata: %s/%s", objFromBytes.Namespace(), objFromBytes.Name())
	}

	objFromString, err := manifest.DecodeOneString(singleYAML)
	if err != nil {
		t.Fatalf("DecodeOneString: %v", err)
	}
	if objFromString.Name() != "single" {
		t.Fatalf("unexpected name: %s", objFromString.Name())
	}
}

func TestAccessorsAndInto(t *testing.T) {
	yamlDoc := `
apiVersion: example.io/v1
kind: Example
metadata:
  name: test-obj
  namespace: default
  labels:
    env: prod
    tier: frontend
  annotations:
    description: test description
spec:
  replicas: 5
  enabled: true
  tags:
    - web
    - secure
`
	obj, err := manifest.DecodeOneString(yamlDoc)
	if err != nil {
		t.Fatal(err)
	}

	// Labels and Annotations
	labels := obj.Labels()
	if labels["env"] != "prod" || labels["tier"] != "frontend" {
		t.Fatalf("unexpected labels: %v", labels)
	}
	annotations := obj.Annotations()
	if annotations["description"] != "test description" {
		t.Fatalf("unexpected annotations: %v", annotations)
	}

	// NestedBool
	enabled, found, err := obj.NestedBool("spec", "enabled")
	if err != nil || !found || !enabled {
		t.Fatalf("NestedBool failed: found=%v, val=%v, err=%v", found, enabled, err)
	}

	// NestedSlice
	tags, found, err := obj.NestedSlice("spec", "tags")
	if err != nil || !found || len(tags) != 2 {
		t.Fatalf("NestedSlice failed: found=%v, len=%d, err=%v", found, len(tags), err)
	}

	// NestedMap
	meta, found, err := obj.NestedMap("metadata")
	if err != nil || !found || meta["name"] != "test-obj" {
		t.Fatalf("NestedMap failed: found=%v, meta=%v, err=%v", found, meta, err)
	}

	// Into
	var res sampleResource
	if err := obj.Into(&res); err != nil {
		t.Fatalf("Into failed: %v", err)
	}
	if res.Name != "test-obj" || res.Spec.Replicas != 5 || !res.Spec.Enabled || len(res.Spec.Tags) != 2 {
		t.Fatalf("Into produced unexpected struct: %+v", res)
	}

	// Into with nil receiver or target
	var nilObj *manifest.Object
	if err := nilObj.Into(&res); err == nil {
		t.Fatal("expected error on nil object Into")
	}
	if err := obj.Into(nil); err == nil {
		t.Fatal("expected error on nil target Into")
	}
}

func TestMarshaling(t *testing.T) {
	yamlDoc := `
apiVersion: example.io/v1
kind: Example
metadata:
  name: marshal-test
spec:
  replicas: 1
`
	obj, err := manifest.DecodeOneString(yamlDoc)
	if err != nil {
		t.Fatal(err)
	}

	// JSON via json.Marshal (testing MarshalJSON)
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if !strings.Contains(string(jsonBytes), `"name":"marshal-test"`) {
		t.Fatalf("json.Marshal output missing name: %s", string(jsonBytes))
	}

	// JSON() method
	jsonDirect, err := obj.JSON()
	if err != nil {
		t.Fatalf("JSON() failed: %v", err)
	}
	if string(jsonDirect) != string(jsonBytes) {
		t.Fatalf("expected JSON() to match json.Marshal, got %s vs %s", string(jsonDirect), string(jsonBytes))
	}

	// YAML() method
	yamlDirect, err := obj.YAML()
	if err != nil {
		t.Fatalf("YAML() failed: %v", err)
	}
	if !strings.Contains(string(yamlDirect), "name: marshal-test") {
		t.Fatalf("YAML() output missing expected content: %s", string(yamlDirect))
	}

	// MarshalYAML
	yamlAny, err := obj.MarshalYAML()
	if err != nil || yamlAny == nil {
		t.Fatalf("MarshalYAML failed: val=%v, err=%v", yamlAny, err)
	}

	// Nil safety
	var nilObj *manifest.Object
	nilJSON, err := nilObj.JSON()
	if err != nil || string(nilJSON) != "null" {
		t.Fatalf("expected null for nil object JSON, got %s, err=%v", string(nilJSON), err)
	}
	nilYAML, err := nilObj.YAML()
	if err != nil || string(nilYAML) != "null\n" {
		t.Fatalf("expected null\\n for nil object YAML, got %s, err=%v", string(nilYAML), err)
	}
}
