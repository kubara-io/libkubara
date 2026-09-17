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

type UnknownFieldMode int

const (
	// PruneUnknown matches the API server's persisted-object behavior.
	PruneUnknown UnknownFieldMode = iota
	// RejectUnknown reports every pruned path as a validation error.
	RejectUnknown
)

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

type Result struct {
	Object      *manifest.Object
	Diagnostics diagnostic.List
	PrunedPaths []string
}

func (r Result) Valid() bool { return !r.Diagnostics.HasErrors() }
func (r Result) Err() error  { return r.Diagnostics.Err() }

func (r Result) Into(target any) error {
	if err := r.Err(); err != nil {
		return err
	}
	if r.Object == nil {
		return fmt.Errorf("validated object is nil")
	}
	return r.Object.Into(target)
}

func Compile(reader io.Reader) (*Validator, error) {
	definition, err := DecodeCRD(reader)
	if err != nil {
		return nil, err
	}
	return definition.Compile()
}

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

func (v *Validator) ValidateCreate(ctx context.Context, object *manifest.Object, mode UnknownFieldMode) Result {
	return v.validate(ctx, toUnstructured(object), nil, mode).publicResult()
}

func (v *Validator) ValidateUpdate(ctx context.Context, object, oldObject *manifest.Object, mode UnknownFieldMode) Result {
	return v.validate(ctx, toUnstructured(object), toUnstructured(oldObject), mode).publicResult()
}

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
