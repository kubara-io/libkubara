package crdvalidate

import (
	"fmt"
	"io"

	"github.com/kubara-io/libkubara/manifest"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type Definition struct {
	object *manifest.Object
}

func NewDefinition(object *manifest.Object) (*Definition, error) {
	if object == nil {
		return nil, fmt.Errorf("CRD object is nil")
	}
	if object.APIVersion() != apiextensionsv1.SchemeGroupVersion.String() {
		return nil, fmt.Errorf("CRD apiVersion must be %q, got %q", apiextensionsv1.SchemeGroupVersion.String(), object.APIVersion())
	}
	if object.Kind() != "CustomResourceDefinition" {
		return nil, fmt.Errorf("CRD kind must be %q, got %q", "CustomResourceDefinition", object.Kind())
	}
	return &Definition{object: object}, nil
}

func DecodeCRD(reader io.Reader) (*Definition, error) {
	object, err := manifest.DecodeOne(reader)
	if err != nil {
		return nil, fmt.Errorf("decode CRD manifest: %w", err)
	}
	return NewDefinition(object)
}

func (d *Definition) typed() (*apiextensionsv1.CustomResourceDefinition, error) {
	if d == nil || d.object == nil {
		return nil, fmt.Errorf("CRD definition is nil")
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(d.object.Data(), &crd); err != nil {
		return nil, fmt.Errorf("convert CRD manifest: %w", err)
	}
	return &crd, nil
}
