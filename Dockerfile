# Multi-stage Dockerfile for Cloudflare R2 Uploader (Production 2026)
FROM golang:alpine AS builder

WORKDIR /src

# Install build dependencies
RUN apk --no-cache add ca-certificates git

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build statically linked binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /src/bin/r2-uploader ./cmd/server

# Production stage
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -g 10001 -S appuser \
    && adduser -u 10001 -S appuser -G appuser

WORKDIR /app

# Prepare data directory with proper ownership
RUN mkdir -p /app/data && chown -R appuser:appuser /app

# Copy binary from builder
COPY --from=builder /src/bin/r2-uploader /app/r2-uploader

USER appuser:appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/api/health || exit 1

CMD ["/app/r2-uploader"]
