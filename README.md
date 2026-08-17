# libkubara (proof of concept)

`libkubara` explores reusable building blocks extracted from Kubara's rendering
and local CustomResourceDefinition validation logic.

> **Status:** This repository is under active development. Its APIs are still
> proof-of-concept and may change. Do not use it as a stable rollout validation
> boundary yet.

## Core model

A consumer's configuration is a **Kubernetes Custom Resource**, not a Go config
struct with a separately generated JSON Schema.

The CRD is the single source of truth for:

- config shape and required properties
- defaults
- OpenAPI constraints
- Kubernetes list/map semantics
- `x-kubernetes-validations` CEL rules
- create/update transition rules
- IDE schema integration in future tooling

This allows a GitOps repository to validate and normalize proposed Custom
Resources without a Kubernetes runtime before committing or rolling them out.

```text
project-crd.yaml       schema, defaults, and CEL policy
project-config.yaml    current Project custom resource
project-update.yaml    proposed GitOps change
          │
          ▼
 decode CRD → compile local validator → decode config resource
          │
          ├─ ValidateCreate(config)
          ├─ ValidateTransition(proposed, current)
          └─ normalized/defaulted resource → template context
```

The module currently targets **Go 1.24** and pins Kubernetes libraries to the
**v0.34.x / Kubernetes 1.34** API baseline. An importing project may select a
newer compatible Kubernetes module through Go minimal version selection.

## Packages

| Package | PoC purpose |
|---|---|
| `manifest` | Decode Kubernetes-style YAML/JSON streams into dependency-neutral `manifest.Object` facades. |
| `crdvalidate` | Decode opaque CRD definitions, then normalize and validate Custom Resource creates and transitions locally. |
| `kubernetes` | Optional interoperability for consumers that intentionally use typed Kubernetes CRD or Unstructured values. |
| `template` | Render one `text/template` with hermetic Sprig, YAML helpers, strict missing keys, cancellation, and an output limit. |
| `template/tree` | Discover ordered `fs.FS` sources, resolve collisions, render `.tplt` files, and copy static files. `RenderAll` retains per-file errors for compatibility adapters. |
| `diagnostic` | Initial shared diagnostic values; not integrated across every PoC package yet. |

## Importing-project example

[`examples/consumer`](examples/consumer) is a separate Go module demonstrating
the intended external call structure. It contains:

- [`project-crd.yaml`](examples/consumer/project-crd.yaml)
- [`project-config.yaml`](examples/consumer/project-config.yaml)
- [`project-config-update.yaml`](examples/consumer/project-config-update.yaml)

The normal consumer imports no `k8s.io` packages.

Run it with:

```bash
cd examples/consumer
go run .
```

Expected output:

```text
CRD create: platforms/storefront stage=dev replicas=1
generated/project.txt: Project storefront runs in dev with 1 replica(s)
generated/static.txt: copied unchanged
```

## Consumer call structure

```go
// Load the schema directly from the CRD YAML kept in the repository.
crd, err := crdvalidate.DecodeCRD(crdReader)
validator, err := crdvalidate.Compile(crd)

// Load the configuration as a Kubernetes Custom Resource.
current, err := manifest.DecodeOne(configReader)
created := validator.ValidateCreate(ctx, current, crdvalidate.RejectUnknown)
if err := created.Err(); err != nil {
    return err
}

// Validate a proposed GitOps change locally when the old object is available.
proposed, err := manifest.DecodeOne(updateReader)
updated := validator.ValidateTransition(ctx, proposed, created.Object, crdvalidate.RejectUnknown)
if err := updated.Err(); err != nil {
    return err
}

// Render from a detached copy of the validated/defaulted resource.
dataBuilder := template.NewData()
_ = dataBuilder.Namespace("config", created.Object.Data())
data, _ := dataBuilder.Build()

engine, _ := template.New(template.WithMissingKeyError())
renderer, _ := tree.New(engine,
    tree.WithSources(tree.Source{Name: "base", FS: sourceFS}),
)
results, err := renderer.Render(ctx, data)
```

`ValidateTransition` is intended for old/proposed GitOps resources that omit
API-server-managed `metadata.resourceVersion`. `ValidateUpdate` retains stricter
API-style update metadata behavior.

## Kubernetes dependency isolation

The normal public workflow exposes `crdvalidate.Definition`, `manifest.Object`,
and `diagnostic.List`; it does not expose `apiextensionsv1`, `unstructured`, or
`field.ErrorList`. Consumers that already use Kubernetes types may explicitly
opt into the adapter:

```go
import kubeadapter "github.com/kubara-io/libkubara/kubernetes"

definition, err := kubeadapter.Definition(typedCRD)
object, err := kubeadapter.Object(typedUnstructured)
typedCopy := kubeadapter.Unstructured(object)
```

This facade reduces source/API coupling and keeps ordinary consumer code clear
of Kubernetes dependencies. It does **not** create binary-level isolation: the
validator still uses Kubernetes modules internally, and Go minimal version
selection chooses one version for the final process. Libkubara therefore still
needs a documented supported range and CI against both its minimum dependency
and Kubara's selected version. Complete version isolation would require a
separate validation process or service.

## Determinism and safety defaults

- Hermetic Sprig is enabled by default. Environment, clock, random, UUID, and
  DNS functions require explicit `template.WithFullSprig()` opt-in.
- Missing template map keys fail by default.
- Templates have a default 16 MiB output limit.
- Tree rendering returns data and does not write to the host filesystem.
- Manifest decoding requires no REST client or Kubernetes runtime.
- CRD validation works on a deep copy and returns the normalized object.
- `RejectUnknown` makes locally pruned fields fail validation instead of being
  silently lost during a rollout.

These APIs are not a security sandbox. Templates and schemas should still be
treated as trusted input during this PoC.

## What deliberately remains in Kubara

The library does not define Kubara's catalog or platform policy:

- OCI catalog pulling, caching, and precedence
- `.cluster`, `.env`, `.catalog`, and `.spokes` context conventions
- Helm/Terraform provider selection
- `platform-components` and `platform-configs`
- enabled-service filtering and per-cluster output layout
- Kubara-specific migration and provider policy

Kubara's own future config can become a Kubara Custom Resource and use the same
manifest/CRD validation pipeline before Kubara builds its rendering context.

## Development

```bash
make check
```

Run `make help` to list individual test, formatting, dependency, and vetting
targets.

This repository was created as a local PoC and has not been published or tagged.
