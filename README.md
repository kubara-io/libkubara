# libkubara

[![Go Reference](https://pkg.go.dev/badge/github.com/kubara-io/libkubara.svg)](https://pkg.go.dev/github.com/kubara-io/libkubara)
[![CI](https://github.com/kubara-io/libkubara/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/kubara-io/libkubara/actions/workflows/ci.yaml)

In-process Kubernetes CRD validation, structural defaulting, and manifest handling without a running cluster.

Validating Kubernetes Custom Resources usually requires a full running cluster. Developers often set up `envtest`, spin up local kind or minikube clusters, or maintain duplicate Go structs and JSON Schemas that drift from the real CRD definition.

The Kubernetes API servers code is open and already provides the validation engine: OpenAPI v3 structural schemas, structural pruning, field defaulting, list and map set validation, and Common Expression Language (CEL) rules (`x-kubernetes-validations`). However, using `k8s.io/apiextensions-apiserver` and `k8s.io/apimachinery` directly as a library can be a real struggle and requires wiring a lot of internal logic: schema compilers, managing raw unstructured conversions, and handling verbose internal APIs.

`libkubara` does not re-implement Kubernetes validation logic. Instead, it embeds and wraps the official upstream `k8s.io` packages behind a more consumer friendly interface. You get the exact same validation, defaulting, and CEL evaluation that runs inside the Kubernetes API server, accessible directly in your code in milliseconds with zero external dependencies, no etcd, and no container runtimes.

## Quick start

Validate a Custom Resource against an operator CRD (e.g.: cert-manager `Issuer`) and parse the defaulted output directly into a typed struct:

```go
package main

import (
    "bytes"
    "context"
    _ "embed"
    "fmt"
    "log"

    certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
    "github.com/kubara-io/libkubara/crdvalidate"
    "github.com/kubara-io/libkubara/manifest"
)

//go:embed issuer-crd.yaml
var issuerCRD []byte

//go:embed issuer.yaml
var issuerYAML []byte

func main() {
    ctx := context.Background()

    // Compile schema and CEL rules directly from CRD YAML
    validator, err := crdvalidate.Compile(bytes.NewReader(issuerCRD))
    if err != nil {
        log.Fatalf("compile CRD: %v", err)
    }

    // Decode the input manifest
    rawObj, err := manifest.DecodeOneBytes(issuerYAML)
    if err != nil {
        log.Fatalf("decode manifest: %v", err)
    }

    // Run structural schema validation, defaulting, and CEL rules
    result := validator.ValidateCreate(ctx, rawObj, crdvalidate.RejectUnknown)

    // Unmarshal the defaulted and validated object into a typed struct
    var issuer certmanagerv1.Issuer
    if err := result.Into(&issuer); err != nil {
        log.Fatalf("validation failed:\n%v", err)
    }

    fmt.Printf("Validated Issuer: %s/%s\n", issuer.Namespace, issuer.Name)
    fmt.Printf("ACME Server: %s\n", issuer.Spec.ACME.Server)
}
```

## Key capabilities

### 1. In-process validation without a cluster
- Directly executes upstream `k8s.io/apiextensions-apiserver` structural schema compilation and defaulting logic.
- Evaluates `x-kubernetes-validations` CEL rules using the upstream CEL runtime with standard API server cost limits.
- Matches production `kube-apiserver` validation behavior without mocking or custom schema approximations.
- Supports strict rejection of undeclared fields via `crdvalidate.RejectUnknown`.

### 2. GitOps lifecycle transitions
`ValidateTransition` allows GitOps pipelines and CLI tools to validate proposed changes against current resources when API-server-assigned metadata like `metadata.resourceVersion` is absent:

```go
// Validate an update from current state to proposed state
result := validator.ValidateTransition(ctx, proposedObj, currentObj, crdvalidate.RejectUnknown)
if err := result.Err(); err != nil {
    return fmt.Errorf("proposed change rejected: %w", err)
}
```

### 3. Clean manifest abstractions
- `manifest.Decode` and `manifest.DecodeOne` parse multi-document YAML or JSON streams from `io.Reader`, `[]byte`, or `string`.
- Implements `json.Marshaler` and `yaml.Marshaler`, with `.JSON()` and `.YAML()` methods for serialization.
- Provides standard metadata and nested field accessors: `Labels()`, `Annotations()`, `NestedString()`, `NestedInt64()`, `NestedBool()`, `NestedSlice()`, and `NestedMap()`.
- Unmarshals into any target struct using `.Into(&target)`.

### 4. Hermetic file-tree generation
The `template` and `template/tree` packages provide a pipeline for tools that generate configuration files or manifests from validated custom resources:

- Hermetic Sprig functions by default. Non-deterministic functions like environment variables, clocks, or random numbers require explicit opt-in.
- Strict missing-key checking by default.
- Default 16 MiB output limit per template to protect memory.
- In-memory tree rendering across multiple `fs.FS` sources with configurable file matchers and collision policies.

```go
dataBuilder := template.NewData()
_ = dataBuilder.Namespace("config", result.Object)
data, _ := dataBuilder.Build()

engine, _ := template.New(template.WithMissingKeyError())
renderer, _ := tree.New(engine,
    tree.WithSources(tree.Source{Name: "templates", FS: templateFS}),
)
renderedFiles, err := renderer.Render(ctx, data)
```

## Packages

| Package | Purpose |
|---|---|
| `manifest` | Decode, manipulate, and serialize Kubernetes manifests without requiring apiserver dependencies. |
| `crdvalidate` | Drive upstream API server schema validation, defaulting, and GitOps transitions locally. |
| `kubernetes` | Adapters for codebases that already work with `apiextensionsv1` or `unstructured.Unstructured`. |
| `diagnostic` | Structured diagnostic list and severity reporting for validation failures. |
| `template` | Single template execution with hermetic Sprig, strict missing keys, and output limits. |
| `template/tree` | Multi-source `fs.FS` template and static file discovery, collision handling, and rendering. |

## Examples

- [`examples/cert-manager`](examples/cert-manager): Compiling a third-party operator CRD and validating manifests into kubebuilder-generated Go types.
- [`examples/consumer`](examples/consumer): Validating a custom resource lifecycle and rendering a template tree.

Run them locally:

```bash
cd examples/cert-manager && go run .
cd examples/consumer && go run .
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and testing instructions.

```bash
make check
```

## Background

`libkubara` originated from the manifest validation and template rendering core of [Kubara](https://github.com/kubara-io/kubara). It was extracted to provide a standalone library for any CLI, operator, testing suite, or GitOps workflow that needs Kubernetes schema validation without a running cluster.
