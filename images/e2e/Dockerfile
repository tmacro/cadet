FROM golang:alpine AS builder

COPY ./go.mod ./go.sum /app/
COPY ./e2e /app/e2e
COPY ./pkg /app/pkg

WORKDIR /app

RUN go build -o whoami ./e2e/cmd/whoami \
    &&  go build -o driver ./e2e/cmd/driver

FROM alpine

COPY --from=builder /app/whoami /usr/local/bin/whoami
COPY --from=builder /app/driver /usr/local/bin/driver

CMD ["/usr/local/bin/whoami"]
