# Contributing

Thank you for contributing to libkubara. We track bugs, feature requests, and
other work in GitHub issues.

## Development setup

Fork the repository, clone your fork, create a feature branch, and download Go
dependencies:

```bash
git clone git@github.com:<YOUR_FORK>/libkubara.git
cd libkubara
git switch -c feature/<BRANCH_NAME>
go mod download
```

Install the Go version declared in the repository's `go.mod`. The
[Go installation guide](https://go.dev/doc/install) and
[Go downloads page](https://go.dev/dl/) explain how to install a specific
release.

We suggest [Visual Studio Code](https://code.visualstudio.com/docs/languages/go)
with the official
[Go extension](https://marketplace.visualstudio.com/items?itemName=golang.go).

### Testing and validation

The repository includes a `Makefile` with targets for testing, formatting,
vetting, and linting across the main library and example modules:

```bash
# Run unit tests for the core library
make test

# Run tests for example modules
make test-consumer
make test-cert-manager

# Run code formatters, vet, and linter
make fmt
make vet
make lint

# Run all checks (required before opening a pull request)
make check
```

## Pull requests and issues

If you find a bug, open an issue. To fix a bug or add a feature, create a
feature branch, make the change, and open a pull request against this repository.
Mention the related issue number in the pull request when applicable.

Make sure `make check` passes cleanly before submitting your pull request.

## AI use

libkubara is built by humans for humans. Software engineering and API design
depend on trust between contributors and maintainers. That trust also matters in
how we work together.

You may use AI tools when contributing, but YOU must not replace human
communication or judgment using those tools. You must understand, test, and
review every AI-assisted change yourself. Write a clear, concise pull request
description and respond to review comments yourself.

Listing AI tooling as a co-author, co-signing commits using an AI tool, or
using `assisted-by`, `co-developed`, or similar commit trailers is not allowed.

The project maintainers will review contributions regardless of their origin.
However, we may close issues or pull requests without comment when they appear to
be unreviewed automated output, low-quality slop, or contain essay-length
descriptions or comments that waste reviewer time.

If a contribution does not show the care needed for a high-quality change,
maintainers will not spend time reviewing it.
