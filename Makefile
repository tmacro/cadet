UPSTREAMS := upstreams
BIN      = bin
DIST     = dist
GO       = go

XCADDY := $(BIN)/xcaddy

CADDY_VERSION := v2.10.0
XCADDY_VERSION := v0.4.5

$(DIST):
	@mkdir -p $@

$(BIN):
	@mkdir -p $@

$(BIN)/%: | $(BIN)
	env GOBIN=$(abspath $(BIN)) $(GO) install $(PACKAGE)

$(XCADDY): PACKAGE=github.com/caddyserver/xcaddy/cmd/xcaddy@$(XCADDY_VERSION)

.PHONY: dashboard
dashboard: | $(DIST)
	@$(GO) build -o dist/cadet-dashboard ./cmd/dashboard

.PHONY: caddy
caddy: | $(XCADDY) $(DIST)
	 $(XCADDY) build $(CADDY_VERSION) \
	 	--output $(DIST)/caddy \
	    --with github.com/caddy-dns/cloudflare \
	    --with github.com/tmacro/cadet/plugins/caddy \
	    --replace github.com/tmacro/cadet=.

.PHONY: build-css
build-css:
	@docker buildx build --no-cache -t tailwind-builder:dev images/tailwind
	docker run --rm -v $(PWD):/build -v $(PWD)/internal/webui/static/css:/output tailwind-builder:dev
	@sudo chown $(USER):$(USER) $(PWD)/internal/webui/static/css/main.css

.PHONY: watch-css
watch-css:
	@docker buildx build -t tailwind-builder:dev images/tailwind
	docker run --rm -v $(PWD):/build -v $(PWD)/internal/webui/static/css:/output tailwind-builder:dev tailwindcss -i /config/tailwind.css -o /output/main.css --watch=always
	@sudo chown $(USER):$(USER) $(PWD)/internal/webui/static/css/main.css

CADDY_DOCKER_IMAGE := tmacro/cadet-proxy
CADDY_DOCKER_TAG := latest

.PHONY: caddy-docker
caddy-docker:
	docker buildx build -t $(CADDY_DOCKER_IMAGE):$(CADDY_DOCKER_TAG) -f images/caddy/Dockerfile .

COREDNS_DOCKER_IMAGE := tmacro/cadet-dns
COREDNS_DOCKER_TAG := latest

.PHONY: coredns-docker
coredns-docker:
	docker buildx build -t $(COREDNS_DOCKER_IMAGE):$(COREDNS_DOCKER_TAG) -f images/coredns/Dockerfile .

WHOAMI_DOCKER_IMAGE := tmacro/cadet-whoami
WHOAMI_DOCKER_TAG := latest

.PHONY: whoami-docker
whoami-docker:
	docker buildx build -t $(WHOAMI_DOCKER_IMAGE):$(WHOAMI_DOCKER_TAG) -f images/whoami/Dockerfile .

.PHONY: e2e
e2e: | caddy-docker coredns-docker whoami-docker
	go test ./e2e
