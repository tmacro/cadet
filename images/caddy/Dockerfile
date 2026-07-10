FROM golang:alpine as builder

ARG XCADDY_VERSION=v0.4.5

ARG CADDY_VERSION=v2.10.0

RUN apk add --update git

# Install xcaddy
RUN go install github.com/caddyserver/xcaddy/cmd/xcaddy@${XCADDY_VERSION}

COPY . /plugin

# Build caddy
RUN xcaddy build ${CADDY_VERSION} \
    --with github.com/caddy-dns/cloudflare \
    --with github.com/tmacro/cadet/plugins/caddy \
    --replace github.com/tmacro/cadet=/plugin

FROM alpine

ENV XDG_DATA_HOME /data

ENV XDG_CONFIG_HOME /config

COPY --from=builder /go/caddy /usr/local/bin/caddy

CMD ["/usr/local/bin/caddy", "run"]
