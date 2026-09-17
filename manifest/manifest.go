// Package manifest provides parsing, inspection, serialization, and unmarshaling
// for Kubernetes manifests and custom resources without requiring a live cluster.
//
// The central abstraction is [Object], a dependency-neutral representation of a
// Kubernetes resource backed by a normalized data dictionary. Objects can be decoded
// from multi-document YAML or JSON streams, traversed using nested field accessors,
// serialized back to YAML or JSON, and unmarshaled directly into typed Go structs.
//
// # Decoding Manifests
//
// Manifests can be decoded from any [io.Reader], byte slice, or string:
//
//	// Decode a single document
//	obj, err := manifest.DecodeOneBytes(rawYAML)
//	if err != nil {
//		return err
//	}
//	fmt.Printf("Resource: %s/%s\n", obj.Kind(), obj.Name())
//
//	// Decode a multi-document stream
//	objs, err := manifest.DecodeString(multiDocYAML)
//	if err != nil {
//		return err
//	}
//
// Empty documents (including YAML document separators with only whitespace or comments)
// and null documents are automatically skipped during decoding.
//
// # Typed Unmarshaling
//
// An Object unmarshals directly into any typed struct:
//
//	var issuer certmanagerv1.Issuer
//	if err := obj.Into(&issuer); err != nil {
//		return err
//	}
//
// # Concurrency
//
// An Object is immutable after creation. All accessor, inspection, and serialization
// methods on an Object are safe for concurrent use by multiple goroutines.
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	k8syaml "sigs.k8s.io/yaml"
)

// Object represents a single Kubernetes manifest or custom resource, encapsulating
// its data map and providing typed accessors, serialization, and unmarshaling.
type Object struct {
	data map[string]any
}

// New creates a new Object from a raw map, normalizing it through the Kubernetes
// unstructured JSON scheme to ensure numeric types, boolean values, and map keys
// follow standard Kubernetes serialization rules.
//
// An error is returned if data is nil or cannot be encoded and decoded as a valid
// Kubernetes object.
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

// Data returns a detached deep copy of the object's underlying unstructured data map.
// Mutations made to the returned map do not affect the Object. If o is nil, Data returns nil.
func (o *Object) Data() map[string]any {
	if o == nil {
		return nil
	}
	return runtime.DeepCopyJSON(o.data)
}

// Into unmarshals the object into a target Go struct or map using Kubernetes
// unstructured conversion rules. The target must be a non-nil pointer.
//
// Target types typically include kubebuilder-generated API types (such as
// corev1.Pod or certmanagerv1.Issuer) or custom configuration structs with
// JSON struct tags.
//
// Example:
//
//	var deployment appsv1.Deployment
//	if err := obj.Into(&deployment); err != nil {
//		return fmt.Errorf("unmarshal deployment: %w", err)
//	}
//
// It returns an error if o or target is nil, if target is not a pointer, or if
// unmarshaling fails due to type mismatches.
func (o *Object) Into(target any) error {
	if o == nil {
		return fmt.Errorf("manifest object is nil")
	}
	if target == nil {
		return fmt.Errorf("target is nil")
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(o.data, target); err != nil {
		return fmt.Errorf("convert manifest object: %w", err)
	}
	return nil
}

// MarshalJSON implements json.Marshaler, encoding the object's underlying data.
// If o is nil, it returns the JSON null literal.
func (o *Object) MarshalJSON() ([]byte, error) {
	if o == nil {
		return []byte("null"), nil
	}
	return json.Marshal(o.data)
}

// MarshalYAML implements yaml.Marshaler, returning the raw data map for YAML encoders.
// If o is nil, it returns nil.
func (o *Object) MarshalYAML() (any, error) {
	if o == nil {
		return nil, nil
	}
	return o.data, nil
}

// JSON returns the JSON encoding of the object. It is a convenience wrapper for MarshalJSON.
func (o *Object) JSON() ([]byte, error) {
	return o.MarshalJSON()
}

// YAML returns the YAML encoding of the object, normalized using Kubernetes YAML rules
// (via sigs.k8s.io/yaml) so formatting and key ordering match kubectl and apiserver output.
// If o is nil, it returns "null\n".
func (o *Object) YAML() ([]byte, error) {
	if o == nil {
		return []byte("null\n"), nil
	}
	return k8syaml.Marshal(o.data)
}

// APIVersion returns the object's apiVersion, or empty string if not set.
func (o *Object) APIVersion() string { return o.stringField("apiVersion") }

// Kind returns the object's kind, or empty string if not set.
func (o *Object) Kind() string { return o.stringField("kind") }

// Name returns the object's metadata.name, or empty string if not set.
func (o *Object) Name() string { return o.nestedString("metadata", "name") }

// Namespace returns the object's metadata.namespace, or empty string if not set.
func (o *Object) Namespace() string { return o.nestedString("metadata", "namespace") }

// Labels returns a copy of the object's metadata.labels map, or nil if none exist.
func (o *Object) Labels() map[string]string { return o.metadataStringMap("labels") }

// Annotations returns a copy of the object's metadata.annotations map, or nil if none exist.
func (o *Object) Annotations() map[string]string { return o.metadataStringMap("annotations") }

// NestedString returns the string value located at the specified field path.
//
// The return values are:
//   - val: the string value if found and matching type
//   - found: true if the field path exists
//   - err: non-nil if a value exists at the path but is not a string
//
// If the receiver is nil or the path does not exist, it returns ("", false, nil).
func (o *Object) NestedString(fields ...string) (string, bool, error) {
	if o == nil {
		return "", false, nil
	}
	return unstructured.NestedString(o.data, fields...)
}

// NestedInt64 returns the int64 integer value located at the specified field path.
// Numeric values decoded from JSON or YAML that fit within int64 are converted.
//
// If the receiver is nil or the path does not exist, it returns (0, false, nil).
func (o *Object) NestedInt64(fields ...string) (int64, bool, error) {
	if o == nil {
		return 0, false, nil
	}
	return unstructured.NestedInt64(o.data, fields...)
}

// NestedBool returns the boolean value located at the specified field path.
//
// If the receiver is nil or the path does not exist, it returns (false, false, nil).
func (o *Object) NestedBool(fields ...string) (bool, bool, error) {
	if o == nil {
		return false, false, nil
	}
	return unstructured.NestedBool(o.data, fields...)
}

// NestedSlice returns a slice of elements located at the specified field path.
//
// If the receiver is nil or the path does not exist, it returns (nil, false, nil).
func (o *Object) NestedSlice(fields ...string) ([]any, bool, error) {
	if o == nil {
		return nil, false, nil
	}
	return unstructured.NestedSlice(o.data, fields...)
}

// NestedMap returns a map located at the specified field path.
//
// If the receiver is nil or the path does not exist, it returns (nil, false, nil).
func (o *Object) NestedMap(fields ...string) (map[string]any, bool, error) {
	if o == nil {
		return nil, false, nil
	}
	return unstructured.NestedMap(o.data, fields...)
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

func (o *Object) metadataStringMap(field string) map[string]string {
	if o == nil {
		return nil
	}
	m, _, _ := unstructured.NestedStringMap(o.data, "metadata", field)
	return m
}

// Decode reads a multi-document YAML or JSON stream from reader and returns all
// decoded Kubernetes manifest objects.
//
// Empty documents, comments-only blocks, and null documents are ignored. It returns
// an error if reader is nil, if YAML/JSON decoding fails, or if a document does not
// represent a Kubernetes object map.
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

// DecodeBytes decodes all Kubernetes manifest objects from byte slice data.
// It is a convenience wrapper for Decode(bytes.NewReader(data)).
func DecodeBytes(data []byte) ([]*Object, error) {
	return Decode(bytes.NewReader(data))
}

// DecodeString decodes all Kubernetes manifest objects from a string.
// It is a convenience wrapper for Decode(strings.NewReader(s)).
func DecodeString(s string) ([]*Object, error) {
	return Decode(strings.NewReader(s))
}

// DecodeOne reads exactly one Kubernetes manifest object from reader.
//
// It returns an error if reader contains zero objects, more than one object,
// or if decoding fails.
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

// DecodeOneBytes reads exactly one Kubernetes manifest object from byte slice data.
// It is a convenience wrapper for DecodeOne(bytes.NewReader(data)).
func DecodeOneBytes(data []byte) (*Object, error) {
	return DecodeOne(bytes.NewReader(data))
}

// DecodeOneString reads exactly one Kubernetes manifest object from a string.
// It is a convenience wrapper for DecodeOne(strings.NewReader(s)).
func DecodeOneString(s string) (*Object, error) {
	return DecodeOne(strings.NewReader(s))
}
