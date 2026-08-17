package kubernetes_test

import (
	"testing"

	kubeadapter "github.com/kubara-io/libkubara/kubernetes"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
}
