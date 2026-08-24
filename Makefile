GO ?= go
LOCALBIN ?= $(shell pwd)/bin
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.12.2

.PHONY: help test test-consumer test-cert-manager fmt fmt-check vet lint tidy check

##@ General

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Testing

test: ## Run tests for the library module.
	$(GO) test ./...

test-consumer: ## Run tests for the consumer example module.
	cd examples/consumer && $(GO) test ./...

test-cert-manager: ## Run tests for the cert-manager example module.
	cd examples/cert-manager && $(GO) test ./...

##@ Development

fmt: ## Format Go source files in all modules.
	$(GO) fmt ./...
	cd examples/consumer && $(GO) fmt ./...
	cd examples/cert-manager && $(GO) fmt ./...

fmt-check: ## Verify Go source files are formatted.
	@test -z "$$($(GO)fmt -l .)"
	@test -z "$$(cd examples/consumer && $(GO)fmt -l .)"
	@test -z "$$(cd examples/cert-manager && $(GO)fmt -l .)"

vet: ## Run go vet for the library module.
	$(GO) vet ./...

lint: $(GOLANGCI_LINT) ## Run golangci-lint for the library module.
	$(GOLANGCI_LINT) run

$(GOLANGCI_LINT): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

tidy: ## Tidy dependencies for all modules.
	$(GO) mod tidy
	cd examples/consumer && $(GO) mod tidy
	cd examples/cert-manager && $(GO) mod tidy

check: fmt-check vet lint test test-consumer test-cert-manager ## Run formatting, vetting, linting, and all tests.
