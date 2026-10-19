SHELL         := bash
.SHELLFLAGS   := -eu -o pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS     += --no-builtin-rules

GO    ?= go
BIN   := $(CURDIR)/bin
DIST  := $(CURDIR)/dist
BUILD := $(CURDIR)/build

PROJECT_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Target platform for the binaries in ./dist; defaults to the host.
GOOS   ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)

gomod_version = $(shell $(GO) list -m -f '{{.Version}}' $(1))

CADDY_VERSION   ?= $(call gomod_version,github.com/caddyserver/caddy/v2)
COREDNS_VERSION ?= $(call gomod_version,github.com/coredns/coredns)

XCADDY_VERSION        := v0.4.5
DEADCODE_VERSION      := v0.52.0
STATICCHECK_VERSION   := v0.8.2
GOLANGCI_LINT_VERSION := v2.15.0
GOIMPORTS_VERSION     := v0.51.0
TAILWIND_VERSION      := 4.1.12
GORELEASER_VERSION    := v2.18.3
GINKGO_VERSION        := $(call gomod_version,github.com/onsi/ginkgo/v2)

XCADDY        := $(BIN)/xcaddy
DEADCODE      := $(BIN)/deadcode
STATICCHECK   := $(BIN)/staticcheck
GOLANGCI_LINT := $(BIN)/golangci-lint
GOIMPORTS     := $(BIN)/goimports
GINKGO        := $(BIN)/ginkgo
TAILWIND      := $(BIN)/tailwindcss
GORELEASER    := $(BIN)/goreleaser

##@ Build

.PHONY: all
all:

.PHONY: build
build: caddy coredns dashboard ## Build all binaries into ./dist

.PHONY: caddy
caddy: | $(XCADDY) ## Build ./dist/caddy with the cadet plugin
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 $(XCADDY) build $(CADDY_VERSION) \
		--output $(DIST)/caddy \
		--with github.com/caddy-dns/cloudflare \
		--with github.com/tmacro/cadet/plugins/caddy \
		--replace github.com/tmacro/cadet=.

COREDNS_SRC := $(BUILD)/coredns-$(COREDNS_VERSION)
COREDNS_TARBALL := $(COREDNS_SRC).tar.gz
COREDNS_TOOLCHAIN = go$$(cat .go-version)+auto

$(COREDNS_TARBALL): | $(BUILD)
	curl -sSfL -o $@.tmp https://api.github.com/repos/coredns/coredns/tarball/$(COREDNS_VERSION)
	mv $@.tmp $@

.PHONY: coredns
coredns: $(COREDNS_TARBALL) | $(DIST) ## Build ./dist/coredns with the cadet plugin
	rm -rf $(COREDNS_SRC) && mkdir -p $(COREDNS_SRC)
	tar -xzf $(COREDNS_TARBALL) -C $(COREDNS_SRC) --strip-components=1
	echo "replace github.com/tmacro/cadet => $(CURDIR)" >> $(COREDNS_SRC)/go.mod
	cp images/coredns/plugin.cfg $(COREDNS_SRC)/plugin.cfg
	cd $(COREDNS_SRC) && env -u GOOS -u GOARCH GOTOOLCHAIN=$(COREDNS_TOOLCHAIN) $(GO) generate coredns.go
	cd $(COREDNS_SRC) && env -u GOOS -u GOARCH GOTOOLCHAIN=$(COREDNS_TOOLCHAIN) $(GO) get
	$(MAKE) -C $(COREDNS_SRC) coredns BINARY=$(DIST)/coredns GOFLAGS=-buildvcs=false \
		GOLANG_VERSION="$$(cat $(COREDNS_SRC)/.go-version)+auto" \
		SYSTEM="GOOS=$(GOOS) GOARCH=$(GOARCH)"

.PHONY: dashboard
dashboard: | $(DIST) ## Build ./dist/cadet-dashboard
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 $(GO) build -o $(DIST)/cadet-dashboard ./cmd/dashboard

CSS_IN  := internal/webui/tailwind.css
CSS_OUT := internal/webui/static/css/main.css

.PHONY: css
css: | $(TAILWIND) ## Regenerate the dashboard stylesheet
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT)

.PHONY: css-watch
css-watch: | $(TAILWIND) ## Regenerate the dashboard stylesheet on change
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --watch=always

##@ Images

DOCKER_PLATFORMS    ?= linux/amd64
IMAGE_REGISTRY      ?= ghcr.io/tmacro

CADDY_DOCKER_IMAGE := $(IMAGE_REGISTRY)/cadet-proxy
CADDY_DOCKER_TAG := $(PROJECT_VERSION)

.PHONY: caddy-docker
caddy-docker: ## Build caddy docker image
	docker buildx build \
	    --platform $(DOCKER_PLATFORMS) \
		--build-arg CADDY_VERSION=$(CADDY_VERSION) \
		--build-arg XCADDY_VERSION=$(XCADDY_VERSION) \
		--tag $(CADDY_DOCKER_IMAGE):$(CADDY_DOCKER_TAG) \
		-f $(CURDIR)/images/caddy/Dockerfile .

COREDNS_DOCKER_IMAGE := $(IMAGE_REGISTRY)/cadet-dns
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

$(BUILD):
	@mkdir -p $@

$(BIN):
	@mkdir -p $@

$(BIN)/%: | $(BIN)
	env -u GOOS -u GOARCH GOBIN=$(abspath $(BIN)) $(GO) install $(PACKAGE)

$(XCADDY):        PACKAGE=github.com/caddyserver/xcaddy/cmd/xcaddy@$(XCADDY_VERSION)
$(DEADCODE):      PACKAGE=golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION)
$(STATICCHECK):   PACKAGE=honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
$(GOLANGCI_LINT): PACKAGE=github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
$(GOIMPORTS):     PACKAGE=golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
$(GINKGO):        PACKAGE=github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

TOOLS := $(XCADDY) $(DEADCODE) $(STATICCHECK) $(GOLANGCI_LINT) $(GOIMPORTS) $(GINKGO) $(TAILWIND) $(GORELEASER)

TAILWIND_OS   := $(if $(filter Darwin,$(shell uname -s)),macos,linux)
TAILWIND_ARCH := $(if $(filter arm64 aarch64,$(shell uname -m)),arm64,x64)

$(TAILWIND): | $(BIN)
	curl -sSfL -o $@.tmp https://github.com/tailwindlabs/tailwindcss/releases/download/v$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_OS)-$(TAILWIND_ARCH)
	chmod +x $@.tmp
	mv $@.tmp $@

GORELEASER_OS    := $(if $(filter Darwin,$(shell uname -s)),Darwin,Linux)
GORELEASER_ARCH  := $(if $(filter arm64 aarch64,$(shell uname -m)),arm64,x86_64)
GORELEASER_URL   := https://github.com/goreleaser/goreleaser/releases/download/$(GORELEASER_VERSION)
GORELEASER_ASSET := goreleaser_$(GORELEASER_OS)_$(GORELEASER_ARCH).tar.gz
SHA256SUM        := $(if $(filter Darwin,$(shell uname -s)),shasum -a 256,sha256sum)

$(GORELEASER): | $(BIN)
	@tmp=$$(mktemp -d); trap 'rm -rf $$tmp' EXIT; \
	curl -sSfL -o $$tmp/$(GORELEASER_ASSET) $(GORELEASER_URL)/$(GORELEASER_ASSET); \
	curl -sSfL -o $$tmp/checksums.txt $(GORELEASER_URL)/checksums.txt; \
	(cd $$tmp && grep " $(GORELEASER_ASSET)$$" checksums.txt | $(SHA256SUM) -c --quiet -); \
	tar -xzf $$tmp/$(GORELEASER_ASSET) -C $$tmp goreleaser; \
	mv $$tmp/goreleaser $@

##@ Tools

.PHONY: tools
tools: | $(TOOLS) ## Install pinned dev tools into ./bin

##@ Development

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	@$(GO) mod tidy

GO_DIRS = $$($(GO) list -f '{{.Dir}}' ./...)

.PHONY: fmt
fmt: | $(GOIMPORTS) ## Format Go sources
	$(GOIMPORTS) -w $(GO_DIRS)

.PHONY: vet
vet: ## Run go vet
	@$(GO) vet ./...

.PHONY: deadcode
deadcode: | $(DEADCODE) ## Report unreachable functions
	@$(DEADCODE) ./...

.PHONY: staticcheck
staticcheck: | $(STATICCHECK) ## Run staticcheck
	@$(STATICCHECK) ./...

##@ Checks

.PHONY: lint
lint: | $(GOLANGCI_LINT) ## Run golangci-lint
	@$(GOLANGCI_LINT) run ./...

.PHONY: check-fmt
check-fmt: | $(GOIMPORTS) ## Fail if Go sources need formatting
	@out=$$($(GOIMPORTS) -l $(GO_DIRS)); \
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
check: check-fmt check-tidy lint check-css ## Run all checks: formatting, go.mod, lint and stylesheet

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

##@ Release

RELEASE_PLATFORMS ?= linux/amd64
RELEASE_DIR       := $(BUILD)/release
RELEASE_ASSETS    := $(RELEASE_DIR)/assets

.PHONY: release-binaries
release-binaries: | $(XCADDY) ## Build binaries for each of RELEASE_PLATFORMS
	rm -rf $(RELEASE_DIR) && mkdir -p $(RELEASE_ASSETS)
	for platform in $(RELEASE_PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		out=$(RELEASE_DIR)/$${os}_$${arch}; \
		GOOS=$$os GOARCH=$$arch $(MAKE) build DIST=$$out; \
		for bin in $$out/*; do \
			cp $$bin $(RELEASE_ASSETS)/$$(basename $$bin)_$${os}_$${arch}; \
		done; \
	done

RELEASE_IMAGES := $(CADDY_DOCKER_IMAGE)=caddy $(COREDNS_DOCKER_IMAGE)=coredns

comma := ,
empty :=
space := $(empty) $(empty)

.PHONY: release-images
release-images: ## Build release images from the release binaries (PUSH=1 to push)
	for image in $(RELEASE_IMAGES); do \
		name=$${image%%=*}; dir=$${image#*=}; \
		docker buildx build \
			--platform $(subst $(space),$(comma),$(strip $(RELEASE_PLATFORMS))) \
			--tag $$name:$(PROJECT_VERSION) \
			--tag $$name:latest \
			$(if $(PUSH),--push) \
			-f images/$$dir/Dockerfile.release $(RELEASE_ASSETS); \
	done

.PHONY: release
release: | $(GORELEASER) ## Publish a GitHub release (HEAD must be a clean, tagged commit)
	@git describe --exact-match --tags HEAD >/dev/null 2>&1 || { echo "HEAD is not tagged"; exit 1; }
	@git diff --quiet HEAD || { echo "working tree is dirty"; exit 1; }
	$(GORELEASER) check
	$(MAKE) release-binaries
	$(GORELEASER) release --clean
	$(MAKE) release-images PUSH=1

.PHONY: release-snapshot
release-snapshot: | $(GORELEASER) ## Dry-run the release into ./build/goreleaser without publishing
	$(MAKE) release-binaries
	$(GORELEASER) release --clean --snapshot
	$(MAKE) release-images

##@ Misc

.PHONY: ci
ci: check test build e2e ## Everything CI runs, in order

.PHONY: clean
clean: ## Remove build output and downloaded sources
	rm -rf $(DIST) $(BUILD)

.PHONY: fclean
fclean: | clean ## Remove build output and tools
	rm -rf $(BIN)

.PHONY: help
help: ## Show this help
	@awk 'BEGIN { FS = ":.*## " } \
		/^##@/ { printf "\n%s\n", substr($$0, 5) } \
		/^[a-zA-Z0-9_-]+:.*## / { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
