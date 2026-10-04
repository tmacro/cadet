ARG COREDNS_VERSION=v1.14.6

FROM golang:alpine AS builder

RUN apk add --update git make curl

RUN curl -L -o coredns.tar "https://api.github.com/repos/coredns/coredns/tarball/$COREDNS_VERSION" \
    && tar -xf coredns.tar \
    && mv coredns-coredns-* coredns

COPY go.mod go.sum /cadet/

COPY pkg /cadet/pkg

COPY plugins /cadet/plugins

COPY images/coredns/plugin.cfg /go/coredns/plugin.cfg

RUN cd coredns \
    && echo "replace github.com/tmacro/cadet => /cadet" >> go.mod \
    && make gen \
    && GOFLAGS="-buildvcs=false" SYSTEM="GOOS=linux GOARCH=amd64" BINARY="dist/coredns" make -e coredns

FROM alpine

COPY --from=builder /go/coredns/dist/coredns /usr/local/bin/coredns

CMD ["/usr/local/bin/coredns", "-conf", "/etc/coredns/Corefile"]
