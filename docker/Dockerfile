# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies including make
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build using make
RUN make build

# Runtime stage
FROM alpine:latest

# Install runtime dependencies including su-exec for user switching
RUN apk add --no-cache ca-certificates tzdata wget su-exec

# Create non-root user
RUN addgroup -g 1000 ergs && \
    adduser -D -u 1000 -G ergs ergs

# Create directories with proper permissions
RUN mkdir -p /data /config && \
    chown -R ergs:ergs /data /config && \
    chmod 755 /data /config

# Copy binary from builder
COPY --from=builder /build/bin/ergs /usr/local/bin/ergs

# Copy default config template
COPY --from=builder /build/pkg/config/config.toml.sample /config/config.toml.sample

# Copy entrypoint script
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Set environment variables
ENV ERGS_CONFIG=/config/config.toml
ENV ERGS_DATA=/data

# Expose ports
# 8080 for web interface
EXPOSE 8080

# Use entrypoint to fix permissions before running command
ENTRYPOINT ["/entrypoint.sh"]

# Default command (can be overridden in docker-compose)
CMD ["ergs", "web"]

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1
