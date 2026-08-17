package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

type Object struct {
	data map[string]any
}

func New(data map[string]any) (*Object, error) {
	if data == nil {
		return nil, fmt.Errorf("manifest object data is nil")
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("encode manifest object: %w", err)
	}
	decoded, _, err := unstructured.UnstructuredJSONScheme.Decode(encoded, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("normalize manifest object: %w", err)
	}
	object, ok := decoded.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("manifest decoded as %T, expected an object", decoded)
	}
	return fromUnstructured(object), nil
}

func fromUnstructured(object *unstructured.Unstructured) *Object {
	return &Object{data: runtime.DeepCopyJSON(object.Object)}
}

func (o *Object) Data() map[string]any {
	if o == nil {
		return nil
	}
	return runtime.DeepCopyJSON(o.data)
}

func (o *Object) APIVersion() string { return o.stringField("apiVersion") }
func (o *Object) Kind() string       { return o.stringField("kind") }
func (o *Object) Name() string       { return o.nestedString("metadata", "name") }
func (o *Object) Namespace() string  { return o.nestedString("metadata", "namespace") }

func (o *Object) NestedString(fields ...string) (string, bool, error) {
	if o == nil {
		return "", false, nil
	}
	return unstructured.NestedString(o.data, fields...)
}

func (o *Object) NestedInt64(fields ...string) (int64, bool, error) {
	if o == nil {
		return 0, false, nil
	}
	return unstructured.NestedInt64(o.data, fields...)
}

func (o *Object) stringField(field string) string {
	if o == nil {
		return ""
	}
	value, _ := o.data[field].(string)
	return value
}

func (o *Object) nestedString(fields ...string) string {
	value, _, _ := o.NestedString(fields...)
	return value
}

func Decode(reader io.Reader) ([]*Object, error) {
	if reader == nil {
		return nil, fmt.Errorf("manifest reader is nil")
	}

	decoder := utilyaml.NewYAMLOrJSONDecoder(reader, 4096)
	var objects []*Object
	for document := 0; ; document++ {
		var raw runtime.RawExtension
		err := decoder.Decode(&raw)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode manifest document %d: %w", document, err)
		}
		if len(raw.Raw) == 0 || string(raw.Raw) == "null" {
			continue
		}

		decoded, _, err := unstructured.UnstructuredJSONScheme.Decode(raw.Raw, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("decode manifest document %d as a Kubernetes object: %w", document, err)
		}
		object, ok := decoded.(*unstructured.Unstructured)
		if !ok {
			return nil, fmt.Errorf("manifest document %d decoded as %T, expected an object", document, decoded)
		}
		if len(object.Object) == 0 {
			continue
		}
		objects = append(objects, fromUnstructured(object))
	}
	return objects, nil
}

func DecodeOne(reader io.Reader) (*Object, error) {
	objects, err := Decode(reader)
	if err != nil {
		return nil, err
	}
	if len(objects) != 1 {
		return nil, fmt.Errorf("expected exactly one manifest object, got %d", len(objects))
	}
	return objects[0], nil
}
