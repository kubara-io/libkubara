package kubernetes_test

import (
	"testing"

	kubeadapter "github.com/kubara-io/libkubara/kubernetes"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDefinitionAddsMissingTypeMetaForProgrammaticCRD(t *testing.T) {
	definition, err := kubeadapter.Definition(&apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "examples.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Example", Plural: "examples"},
			Scope: apiextensionsv1.NamespaceScoped,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if definition == nil {
		t.Fatal("expected a definition")
	}

	// nil CRD
	if _, err := kubeadapter.Definition(nil); err == nil {
		t.Fatal("expected error for nil CRD")
	}

	// CRD with explicit TypeMeta
	def2, err := kubeadapter.Definition(&apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "explicit.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Explicit", Plural: "explicits"},
			Scope: apiextensionsv1.ClusterScoped,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if def2 == nil {
		t.Fatal("expected non-nil definition")
	}
}

func TestObjectAndUnstructuredConversion(t *testing.T) {
	// Object(nil)
	if _, err := kubeadapter.Object(nil); err == nil {
		t.Fatal("expected error converting nil unstructured object")
	}

	// Unstructured(nil)
	if u := kubeadapter.Unstructured(nil); u != nil {
		t.Fatalf("expected nil for nil manifest object, got %v", u)
	}

	// Valid roundtrip: unstructured -> manifest.Object -> unstructured
	orig := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "app-config",
				"namespace": "production",
			},
			"data": map[string]any{
				"key": "value",
			},
		},
	}

	manifestObj, err := kubeadapter.Object(orig)
	if err != nil {
		t.Fatalf("Object conversion failed: %v", err)
	}
	if manifestObj.Name() != "app-config" || manifestObj.Namespace() != "production" {
		t.Fatalf("unexpected metadata: %s/%s", manifestObj.Namespace(), manifestObj.Name())
	}

	roundtrip := kubeadapter.Unstructured(manifestObj)
	if roundtrip == nil {
		t.Fatal("expected non-nil unstructured roundtrip")
	}
	if roundtrip.GetName() != "app-config" || roundtrip.GetNamespace() != "production" {
		t.Fatalf("unexpected roundtrip metadata: %s/%s", roundtrip.GetNamespace(), roundtrip.GetName())
	}
}
