package kubernetes_test

import (
	"fmt"
	"log"

	kubeadapter "github.com/kubara-io/libkubara/kubernetes"
	"github.com/kubara-io/libkubara/manifest"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ExampleDefinition() {
	// Programmatic CRD struct created in code or tests
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "databases.example.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "Database",
				Plural: "databases",
			},
			Scope: apiextensionsv1.NamespaceScoped,
		},
	}

	def, err := kubeadapter.Definition(crd)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(def != nil)

	// Output:
	// true
}

func ExampleObject() {
	unstr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "tls-cert",
				"namespace": "ingress",
			},
		},
	}

	// Adapt unstructured into libkubara manifest.Object
	obj, err := kubeadapter.Object(unstr)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s: %s/%s\n", obj.Kind(), obj.Namespace(), obj.Name())

	// Output:
	// Secret: ingress/tls-cert
}

func ExampleUnstructured() {
	obj, err := manifest.DecodeOneString(`
apiVersion: v1
kind: Namespace
metadata:
  name: staging
`)
	if err != nil {
		log.Fatal(err)
	}

	// Adapt libkubara manifest.Object back to k8s unstructured.Unstructured
	unstr := kubeadapter.Unstructured(obj)
	fmt.Printf("Unstructured Kind: %s, Name: %s\n", unstr.GetKind(), unstr.GetName())

	// Output:
	// Unstructured Kind: Namespace, Name: staging
}
