FROM golang:alpine AS builder

ARG XCADDY_VERSION=v0.4.5

ARG CADDY_VERSION=v2.11.4

RUN apk add --update git

# Install xcaddy
RUN go install github.com/caddyserver/xcaddy/cmd/xcaddy@${XCADDY_VERSION}

COPY ./go.mod ./go.sum /plugin/
COPY ./pkg /plugin/pkg
COPY ./internal /plugin/internal
COPY ./plugins /plugin/plugins

# Build caddy
RUN xcaddy build ${CADDY_VERSION} \
    --with github.com/caddy-dns/cloudflare \
    --with github.com/tmacro/cadet/plugins/caddy \
    --replace github.com/tmacro/cadet=/plugin

FROM alpine

ENV XDG_DATA_HOME=/data

ENV XDG_CONFIG_HOME=/config

COPY --from=builder /go/caddy /usr/local/bin/caddy

CMD ["/usr/local/bin/caddy", "run"]
