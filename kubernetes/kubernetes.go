// Package kubernetes provides interoperability adapters between libkubara types
// and upstream Kubernetes apiextensions and unstructured types.
//
// By default, libkubara applications and CLIs do not need to import any k8s.io packages.
// This package is an optional adapter layer for projects (such as controllers, webhooks,
// or tests) that already work with typed [*apiextensionsv1.CustomResourceDefinition]
// or [*unstructured.Unstructured] objects.
package kubernetes

import (
	"fmt"

	"github.com/kubara-io/libkubara/crdvalidate"
	"github.com/kubara-io/libkubara/manifest"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Definition converts a typed Kubernetes CustomResourceDefinition struct into an
// uncompiled [crdvalidate.Definition].
//
// If the input CRD omits TypeMeta (apiVersion or kind), Definition automatically
// populates the standard apiextensions.k8s.io/v1 TypeMeta values on a deep copy.
//
// Example:
//
//	definition, err := kubernetes.Definition(typedCRD)
//	if err != nil {
//		return err
//	}
//	validator, err := definition.Compile()
//
// It returns an error if crd is nil or cannot be converted to a manifest object.
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

// Object converts a Kubernetes [*unstructured.Unstructured] into a [manifest.Object].
//
// It returns an error if object is nil or cannot be normalized.
func Object(object *unstructured.Unstructured) (*manifest.Object, error) {
	if object == nil {
		return nil, fmt.Errorf("unstructured object is nil")
	}
	return manifest.New(object.Object)
}

// Unstructured converts a [manifest.Object] into a Kubernetes [*unstructured.Unstructured].
// If object is nil, it returns nil.
func Unstructured(object *manifest.Object) *unstructured.Unstructured {
	if object == nil {
		return nil
	}
	return &unstructured.Unstructured{Object: object.Data()}
}
