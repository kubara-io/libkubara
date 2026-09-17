// Package crdvalidate compiles Kubernetes CustomResourceDefinitions and performs local,
// in-process validation, defaulting, structural pruning, and CEL rule evaluation on
// custom resources with full upstream Kubernetes API server parity.
//
// # Overview
//
// CustomResourceDefinitions (CRDs) define the schema contract for Kubernetes custom
// resources. Evaluating custom resources outside a cluster normally requires running
// an API server with etcd.
//
// crdvalidate runs the official upstream Kubernetes validation engine
// (k8s.io/apiextensions-apiserver and k8s.io/apiserver) in-process. It executes the same
// schema validation, defaulting, and CEL rules as kube-apiserver, with no cluster,
// etcd, or container runtime required.
//
// # Validation Pipeline
//
// When validating a custom resource, crdvalidate runs the full API server admission pipeline:
//  1. Group, Kind, and API Version matching against served versions in the CRD.
//  2. Structural pruning of unknown fields (either rejected or tracked via [UnknownFieldMode]).
//  3. Structural field defaulting and pruning of non-nullable nulls without defaults.
//  4. Metadata accessor validation (DNS subdomain name rules, namespace scope rules).
//  5. OpenAPI v3 schema validation (required fields, regex patterns, formats, ranges).
//  6. List set and map set key uniqueness validation.
//  7. Common Expression Language (CEL) validation rules declared in x-kubernetes-validations,
//     evaluated with standard API server per-call cost budgets.
//
// # Validation Lifecycle
//
// Three validation methods support different stages of resource lifecycles:
//   - [Validator.ValidateCreate]: Validates a new custom resource creation.
//   - [Validator.ValidateUpdate]: Validates an update against an existing resource. Both
//     objects must specify metadata.resourceVersion, matching strict API server update requirements.
//   - [Validator.ValidateTransition]: Validates proposed changes in GitOps or CLI workflows
//     where metadata.resourceVersion is omitted.
//
// # Example
//
//	validator, err := crdvalidate.Compile(bytes.NewReader(crdYAML))
//	if err != nil {
//		return err
//	}
//
//	rawObj, err := manifest.DecodeOneBytes(crYAML)
//	if err != nil {
//		return err
//	}
//
//	result := validator.ValidateCreate(ctx, rawObj, crdvalidate.RejectUnknown)
//	if err := result.Err(); err != nil {
//		return fmt.Errorf("invalid CR: %w", err)
//	}
//
//	// Unmarshal the defaulted and validated resource directly into a typed struct:
//	var issuer certmanagerv1.Issuer
//	if err := result.Into(&issuer); err != nil {
//		return err
//	}
//
// # Concurrency
//
// A compiled [Validator] is immutable and safe for concurrent use by multiple goroutines.
package crdvalidate

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/kubara-io/libkubara/diagnostic"
	"github.com/kubara-io/libkubara/manifest"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	structuraldefaulting "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	structurallisttype "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/listtype"
	schemaobjectmeta "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/objectmeta"
	structuralpruning "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	apiextensionsvalidation "k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
)

// UnknownFieldMode specifies how unknown or undeclared fields in a custom resource are handled.
type UnknownFieldMode int

const (
	// PruneUnknown silently prunes undeclared fields, matching the API server's storage behavior.
	PruneUnknown UnknownFieldMode = iota
	// RejectUnknown reports every pruned or undeclared field path as a validation error.
	RejectUnknown
)

// Validator executes in-process schema validation, defaulting, and CEL rule checks
// for custom resources of a compiled CustomResourceDefinition.
type Validator struct {
	group      string
	kind       string
	namespaced bool
	versions   map[string]*versionValidator
}

type versionValidator struct {
	structural *structuralschema.Structural
	openAPI    apiextensionsvalidation.SchemaValidator
	cel        *cel.Validator
}

// Result holds the outcome of a validation operation, including the normalized
// object (with schema defaults applied and unknown fields pruned), any diagnostics,
// and paths pruned during validation.
type Result struct {
	// Object is the validated, defaulted, and pruned manifest object.
	Object *manifest.Object
	// Diagnostics contains any validation errors or warnings.
	Diagnostics diagnostic.List
	// PrunedPaths lists the field paths that were pruned from the object.
	PrunedPaths []string
}

// Valid returns true if the validation result contains no errors.
func (r Result) Valid() bool { return !r.Diagnostics.HasErrors() }

// Err returns the combined error list if validation failed, or nil if valid.
func (r Result) Err() error { return r.Diagnostics.Err() }

// Into unmarshals the validated, defaulted, and pruned object into target.
//
// Target must be a non-nil pointer to a Go struct or map. If validation produced
// any diagnostic errors (that is, r.Valid() is false), Into returns r.Err()
// immediately without modifying target.
//
// Example:
//
//	var issuer certmanagerv1.Issuer
//	if err := result.Into(&issuer); err != nil {
//		return fmt.Errorf("invalid issuer: %w", err)
//	}
//
// It returns an error if validation failed, if target is nil or not a pointer,
// or if unmarshaling fails.
func (r Result) Into(target any) error {
	if err := r.Err(); err != nil {
		return err
	}
	if r.Object == nil {
		return fmt.Errorf("validated object is nil")
	}
	return r.Object.Into(target)
}

// Compile decodes a CustomResourceDefinition from reader and compiles its OpenAPI v3
// structural schemas, field defaulting trees, list and map constraints, and CEL
// validation rules into a reusable, thread-safe Validator.
//
// Example:
//
//	validator, err := crdvalidate.Compile(bytes.NewReader(crdBytes))
//	if err != nil {
//		log.Fatalf("failed to compile CRD: %v", err)
//	}
//
// It returns an error if the reader contains invalid YAML/JSON, does not represent
// an apiextensions.k8s.io/v1 CustomResourceDefinition, or contains no served versions
// with an OpenAPI schema.
func Compile(reader io.Reader) (*Validator, error) {
	definition, err := DecodeCRD(reader)
	if err != nil {
		return nil, err
	}
	return definition.Compile()
}

// Compile compiles the Definition's OpenAPI v3 structural schemas, defaulting trees,
// and CEL rules into a reusable, thread-safe Validator.
//
// It returns an error if d is nil, or if schema compilation fails.
func (d *Definition) Compile() (*Validator, error) {
	if d == nil {
		return nil, fmt.Errorf("CRD definition is nil")
	}
	crd, err := d.typed()
	if err != nil {
		return nil, err
	}
	if crd.Spec.Group == "" || crd.Spec.Names.Kind == "" {
		return nil, fmt.Errorf("CRD group and kind are required")
	}
	compiled := &Validator{
		group:      crd.Spec.Group,
		kind:       crd.Spec.Names.Kind,
		namespaced: crd.Spec.Scope == apiextensionsv1.NamespaceScoped,
		versions:   map[string]*versionValidator{},
	}
	for _, version := range crd.Spec.Versions {
		if !version.Served || version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
			continue
		}
		internalSchema := &apiextensions.JSONSchemaProps{}
		if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(
			version.Schema.OpenAPIV3Schema, internalSchema, nil,
		); err != nil {
			return nil, fmt.Errorf("convert schema for version %q: %w", version.Name, err)
		}
		structural, err := structuralschema.NewStructural(internalSchema)
		if err != nil {
			return nil, fmt.Errorf("build structural schema for version %q: %w", version.Name, err)
		}
		openAPI, _, err := apiextensionsvalidation.NewSchemaValidator(internalSchema)
		if err != nil {
			return nil, fmt.Errorf("compile OpenAPI schema for version %q: %w", version.Name, err)
		}
		compiled.versions[version.Name] = &versionValidator{
			structural: structural,
			openAPI:    openAPI,
			cel:        cel.NewValidator(structural, true, celconfig.PerCallLimit),
		}
	}
	if len(compiled.versions) == 0 {
		return nil, fmt.Errorf("CRD has no served version with an OpenAPI schema")
	}
	return compiled, nil
}

// ValidateCreate validates a new custom resource creation against the CRD's structural schema.
//
// The validation process:
//   - Verifies kind, group, and served API version match the CRD definition.
//   - Prunes undeclared fields according to mode ([PruneUnknown] or [RejectUnknown]).
//   - Applies schema field defaults and strips non-nullable nulls without defaults.
//   - Validates metadata (name DNS subdomain format, namespace presence matching scope).
//   - Validates OpenAPI schema constraints (required fields, patterns, ranges).
//   - Validates list set and map set key uniqueness.
//   - Evaluates CEL validation rules with API server per-call cost budgets.
//
// If ctx is nil, context.Background() is used.
func (v *Validator) ValidateCreate(ctx context.Context, object *manifest.Object, mode UnknownFieldMode) Result {
	return v.validate(ctx, toUnstructured(object), nil, mode).publicResult()
}

// ValidateUpdate validates an update from oldObject to object against schema update rules,
// immutable field constraints, and transition CEL rules.
//
// Both object and oldObject must specify metadata.resourceVersion, matching the strict
// behavior of kube-apiserver update admission. For GitOps workflows where resourceVersion
// is omitted, use [Validator.ValidateTransition] instead.
func (v *Validator) ValidateUpdate(ctx context.Context, object, oldObject *manifest.Object, mode UnknownFieldMode) Result {
	return v.validate(ctx, toUnstructured(object), toUnstructured(oldObject), mode).publicResult()
}

// ValidateTransition validates an update between two resources where metadata.resourceVersion
// may be omitted, such as when comparing GitOps proposed changes against stored resources.
//
// If neither resource specifies a resourceVersion, ValidateTransition assigns temporary
// version markers to satisfy apiserver update checks, runs full transition validation,
// and clears the markers before returning.
func (v *Validator) ValidateTransition(ctx context.Context, object, oldObject *manifest.Object, mode UnknownFieldMode) Result {
	newObject := toUnstructured(object)
	old := toUnstructured(oldObject)
	if newObject == nil || old == nil || newObject.GetResourceVersion() != "" || old.GetResourceVersion() != "" {
		return v.validate(ctx, newObject, old, mode).publicResult()
	}
	newObject.SetResourceVersion("local-validation")
	old.SetResourceVersion("local-validation")
	internal := v.validate(ctx, newObject, old, mode)
	if internal.object != nil {
		internal.object.SetResourceVersion("")
	}
	return internal.publicResult()
}

type internalResult struct {
	object      *unstructured.Unstructured
	errors      field.ErrorList
	prunedPaths []string
}

func (r internalResult) publicResult() Result {
	result := Result{PrunedPaths: append([]string(nil), r.prunedPaths...)}
	if r.object != nil {
		object, err := manifest.New(r.object.Object)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, diagnostic.Diagnostic{
				Severity: diagnostic.SeverityError,
				Code:     "Internal",
				Message:  "convert normalized custom resource",
				Cause:    err,
			})
		} else {
			result.Object = object
		}
	}
	for _, validationError := range r.errors {
		result.Diagnostics = append(result.Diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Path:     validationError.Field,
			Code:     string(validationError.Type),
			Message:  validationError.Error(),
		})
	}
	return result
}

func toUnstructured(object *manifest.Object) *unstructured.Unstructured {
	if object == nil {
		return nil
	}
	return &unstructured.Unstructured{Object: object.Data()}
}

func (v *Validator) validate(ctx context.Context, object, oldObject *unstructured.Unstructured, mode UnknownFieldMode) internalResult {
	if ctx == nil {
		ctx = context.Background()
	}
	result := internalResult{}
	if object == nil {
		result.errors = append(result.errors, field.Required(field.NewPath(""), "custom resource is required"))
		return result
	}
	result.object = object.DeepCopy()
	version, errors := v.versionFor(result.object)
	result.errors = append(result.errors, errors...)
	if version == nil {
		return result
	}

	pruneOptions := structuralschema.UnknownFieldPathOptions{TrackUnknownFieldPaths: true}
	result.prunedPaths = structuralpruning.PruneWithOptions(result.object.Object, version.structural, true, pruneOptions)
	structuraldefaulting.PruneNonNullableNullsWithoutDefaults(result.object.Object, version.structural)
	structuraldefaulting.Default(result.object.Object, version.structural)
	if mode == RejectUnknown {
		for _, path := range result.prunedPaths {
			result.errors = append(result.errors, field.Forbidden(field.NewPath(path), "field is not declared by the CRD schema"))
		}
	}

	if oldObject == nil {
		result.errors = append(result.errors, validation.ValidateObjectMetaAccessor(
			result.object, v.namespaced, validation.NameIsDNSSubdomain, field.NewPath("metadata"),
		)...)
		result.errors = append(result.errors, apiextensionsvalidation.ValidateCustomResource(nil, result.object.Object, version.openAPI)...)
	} else {
		oldCopy := oldObject.DeepCopy()
		oldVersion, oldErrors := v.versionFor(oldCopy)
		result.errors = append(result.errors, oldErrors...)
		if oldVersion == nil {
			return result
		}
		structuralpruning.Prune(oldCopy.Object, oldVersion.structural, true)
		structuraldefaulting.PruneNonNullableNullsWithoutDefaults(oldCopy.Object, oldVersion.structural)
		structuraldefaulting.Default(oldCopy.Object, oldVersion.structural)
		result.errors = append(result.errors, validation.ValidateObjectMetaAccessorUpdate(result.object, oldCopy, field.NewPath("metadata"))...)
		result.errors = append(result.errors, apiextensionsvalidation.ValidateCustomResourceUpdate(nil, result.object.Object, oldCopy.Object, version.openAPI)...)
		if version.cel != nil {
			celErrors, _ := version.cel.Validate(ctx, nil, version.structural, result.object.Object, oldCopy.Object, celconfig.RuntimeCELCostBudget)
			result.errors = append(result.errors, celErrors...)
		}
	}

	result.errors = append(result.errors, schemaobjectmeta.Validate(nil, result.object.Object, version.structural, true)...)
	result.errors = append(result.errors, structurallisttype.ValidateListSetsAndMaps(nil, version.structural, result.object.Object)...)
	if oldObject == nil && version.cel != nil {
		celErrors, _ := version.cel.Validate(ctx, nil, version.structural, result.object.Object, nil, celconfig.RuntimeCELCostBudget)
		result.errors = append(result.errors, celErrors...)
	}
	return result
}

func (v *Validator) versionFor(object *unstructured.Unstructured) (*versionValidator, field.ErrorList) {
	var errors field.ErrorList
	if object.GetKind() != v.kind {
		errors = append(errors, field.Invalid(field.NewPath("kind"), object.GetKind(), fmt.Sprintf("must be %q", v.kind)))
	}
	groupVersion := object.GroupVersionKind().GroupVersion()
	if groupVersion.Group != v.group {
		errors = append(errors, field.Invalid(field.NewPath("apiVersion"), object.GetAPIVersion(), fmt.Sprintf("group must be %q", v.group)))
		return nil, errors
	}
	version := v.versions[groupVersion.Version]
	if version == nil {
		errors = append(errors, field.NotSupported(field.NewPath("apiVersion"), object.GetAPIVersion(), v.servedAPIVersions()))
	}
	return version, errors
}

func (v *Validator) servedAPIVersions() []string {
	versions := make([]string, 0, len(v.versions))
	for version := range v.versions {
		versions = append(versions, v.group+"/"+version)
	}
	sort.Strings(versions)
	return versions
}
