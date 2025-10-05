# Legacy Dockerfile - Use Dockerfile.api, Dockerfile.processor, or Dockerfile.operator instead
# This file is kept for backward compatibility

# Multi-stage build for API server (default)
FROM golang:1.24-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /build/api \
    ./cmd/api

FROM alpine:latest

WORKDIR /app

COPY --from=builder /build/api /app/api
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8080

ENTRYPOINT ["/app/api"]
CMD ["--env=/app/.env"]
