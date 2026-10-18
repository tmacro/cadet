SHELL         := bash
.SHELLFLAGS   := -eu -o pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS     += --no-builtin-rules

GO   ?= go
BIN  := $(CURDIR)/bin
DIST := $(CURDIR)/dist

PROJECT_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

gomod_version = $(shell $(GO) list -m -f '{{.Version}}' $(1))

CADDY_VERSION   ?= $(call gomod_version,github.com/caddyserver/caddy/v2)
COREDNS_VERSION ?= $(call gomod_version,github.com/coredns/coredns)

XCADDY_VERSION        := v0.4.5
DEADCODE_VERSION      := v0.52.0
STATICCHECK_VERSION   := v0.8.2
GOLANGCI_LINT_VERSION := v2.15.0
GOIMPORTS_VERSION     := v0.51.0
TAILWIND_VERSION      := 4.1.12
GINKGO_VERSION        := $(call gomod_version,github.com/onsi/ginkgo/v2)

##@ Build

.PHONY: all
all:

.PHONY: caddy
caddy: | $(XCADDY) ## Build ./dist/caddy with the cadet plugin
	$(XCADDY) build $(CADDY_VERSION) \
		--output $(DIST)/caddy \
		--with github.com/caddy-dns/cloudflare \
		--with github.com/tmacro/cadet/plugins/caddy \
		--replace github.com/tmacro/cadet=.

.PHONY: coredns
coredns:

.PHONY: dashboard
dashboard: ## Build ./dist/cadet-dashboard

CSS_IN  := internal/webui/tailwind.css
CSS_OUT := internal/webui/static/css/main.css

.PHONY: css
css: ## Regenerate the dashboard stylesheet
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT)

.PHONY: css-watch
css-watch: ## Regenerate the dashboard stylesheet on change
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --watch=always

##@ Images

DOCKER_PLATFORMS    ?= linux/amd64

CADDY_DOCKER_IMAGE := tmacro/cadet-proxy
CADDY_DOCKER_TAG := $(PROJECT_VERSION)

.PHONY: caddy-docker
caddy-docker: ## Build caddy docker image
	docker buildx build \
	    --platform $(DOCKER_PLATFORMS) \
		--build-arg CADDY_VERSION=$(CADDY_VERSION) \
		--build-arg XCADDY_VERSION=$(XCADDY_VERSION) \
		--tag $(CADDY_DOCKER_IMAGE):$(CADDY_DOCKER_TAG) \
		-f $(CURDIR)/images/caddy/Dockerfile .

COREDNS_DOCKER_IMAGE := tmacro/cadet-dns
COREDNS_DOCKER_TAG := $(PROJECT_VERSION)

.PHONY: coredns-docker
coredns-docker: ## Build coredns docker image
	docker buildx build \
        --platform $(DOCKER_PLATFORMS) \
		--build-arg COREDNS_VERSION=$(COREDNS_VERSION) \
		--tag $(COREDNS_DOCKER_IMAGE):$(COREDNS_DOCKER_TAG) \
		-f $(CURDIR)/images/coredns/Dockerfile .

$(DIST):
	@mkdir -p $@

$(BIN):
	@mkdir -p $@

$(BIN)/%: | $(BIN)
	env GOBIN=$(abspath $(BIN)) $(GO) install $(PACKAGE)

XCADDY        := $(BIN)/xcaddy
DEADCODE      := $(BIN)/deadcode
STATICCHECK   := $(BIN)/staticcheck
GOLANGCI_LINT := $(BIN)/golangci-lint
GOIMPORTS     := $(BIN)/goimports
GINKGO        := $(BIN)/ginkgo
TAILWIND      := $(BIN)/tailwindcss

$(XCADDY):        PACKAGE=github.com/caddyserver/xcaddy/cmd/xcaddy@$(XCADDY_VERSION)
$(DEADCODE):      PACKAGE=golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION)
$(STATICCHECK):   PACKAGE=honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
$(GOLANGCI_LINT): PACKAGE=github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
$(GOIMPORTS):     PACKAGE=golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
$(GINKGO):        PACKAGE=github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

TOOLS := $(XCADDY) $(DEADCODE) $(STATICCHECK) $(GOLANGCI_LINT) $(GOIMPORTS) $(GINKGO) $(TAILWIND)

TAILWIND_OS   := $(if $(filter Darwin,$(shell uname -s)),macos,linux)
TAILWIND_ARCH := $(if $(filter arm64 aarch64,$(shell uname -m)),arm64,x64)

$(TAILWIND):
	@mkdir -p $(BIN)
	curl -sSfL -o $@.tmp https://github.com/tailwindlabs/tailwindcss/releases/download/v$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_OS)-$(TAILWIND_ARCH)
	chmod +x $@.tmp
	mv $@.tmp $@

##@ Tools

.PHONY: tools
tools: | $(TOOLS) ## Install pinned dev tools into ./bin

##@ Development

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	@$(GO) mod tidy

.PHONY: fmt
fmt: | $(GOIMPORTS) ## Format Go sources
	$(GOIMPORTS) -w .

.PHONY: vet
vet:
	@$(GO) vet ./...

.PHONY: deadcode
deadcode: | $(DEADCODE)
	@$(DEADCODE) ./...

.PHONY: staticcheck
staticcheck: | $(STATICCHECK)
	@$(STATICCHECK) ./...

##@ Checks

.PHONY: lint
lint: | $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) run ./...

.PHONY: check-fmt
check-fmt: | $(GOIMPORTS) ## Fail if Go sources need formatting
	@out=$$($(GOIMPORTS) -l .); \
	if [ -n "$$out" ]; then echo "needs formatting (run make fmt):"; echo "$$out"; exit 1; fi

.PHONY: check-tidy
check-tidy: ## Fail if go.mod or go.sum need tidying
	$(GO) mod tidy -diff

.PHONY: check-css
check-css: | $(TAILWIND) ## Fail if the committed stylesheet is stale
	@tmp=$$(mktemp); trap 'rm -f $$tmp' EXIT; \
	$(TAILWIND) -i $(CSS_IN) -o $$tmp 2>/dev/null; \
	cmp -s $$tmp $(CSS_OUT) || { echo "$(CSS_OUT) is stale (run make css)"; exit 1; }

.PHONY: check
check: check-fmt check-tidy lint check-css

##@ Tests

TEST_FLAGS   ?= -race
GINKGO_FLAGS ?= -v
E2E_COMPOSE  := docker compose -f e2e/docker-compose.yaml

.PHONY: test
test: ## Run unit tests (everything but ./e2e)
	$(GO) test $(TEST_FLAGS) $$($(GO) list ./... | grep -v '/e2e')

.PHONY: e2e
e2e: | $(GINKGO) ## Run the e2e suite
	$(MAKE) e2e-up
	status=0; $(GINKGO) run $(GINKGO_FLAGS) ./e2e || status=$$?; \
	$(MAKE) e2e-down; \
	exit $$status

.PHONY: e2e-up
e2e-up: ## Build and start the e2e stack
	$(E2E_COMPOSE) up --detach --build --wait

.PHONY: e2e-down
e2e-down: ## Stop and remove the e2e stack
	$(E2E_COMPOSE) down --volumes --remove-orphans

##@ Misc

.PHONY: ci
ci: check test build e2e ## Everything CI runs, in order

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BIN) $(DIST)

.PHONY: fclean
fclean: | clean ## Remove build output and tools
	rm -rf $(DIST)

.PHONY: help
help: ## Show this help
	@awk 'BEGIN { FS = ":.*## " } \
		/^##@/ { printf "\n%s\n", substr($$0, 5) } \
		/^[a-zA-Z0-9_-]+:.*## / { printf "  %-14s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
