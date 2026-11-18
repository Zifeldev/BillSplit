# syntax=docker/dockerfile:1
# One image with all Go service binaries; docker-compose picks the binary per service.

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    for s in gateway auth social split finance notify ocr-worker; do \
      CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/$s ./cmd/$s || exit 1; \
    done

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
COPY --from=build /out/ /usr/local/bin/
USER app
EXPOSE 8088 50051 9090
