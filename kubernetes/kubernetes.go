package kubernetes

import (
	"fmt"

	"github.com/kubara-io/libkubara/crdvalidate"
	"github.com/kubara-io/libkubara/manifest"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func Definition(crd *apiextensionsv1.CustomResourceDefinition) (*crdvalidate.Definition, error) {
	if crd == nil {
		return nil, fmt.Errorf("CRD is nil")
	}
	copy := crd.DeepCopy()
	if copy.APIVersion == "" {
		copy.APIVersion = apiextensionsv1.SchemeGroupVersion.String()
	}
	if copy.Kind == "" {
		copy.Kind = "CustomResourceDefinition"
	}
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(copy)
	if err != nil {
		return nil, fmt.Errorf("convert typed CRD: %w", err)
	}
	object, err := manifest.New(data)
	if err != nil {
		return nil, err
	}
	return crdvalidate.NewDefinition(object)
}

func Object(object *unstructured.Unstructured) (*manifest.Object, error) {
	if object == nil {
		return nil, fmt.Errorf("unstructured object is nil")
	}
	return manifest.New(object.Object)
}

func Unstructured(object *manifest.Object) *unstructured.Unstructured {
	if object == nil {
		return nil
	}
	return &unstructured.Unstructured{Object: object.Data()}
}
