package manifest_test

import (
	"strings"
	"testing"

	"github.com/kubara-io/libkubara/manifest"
)

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
