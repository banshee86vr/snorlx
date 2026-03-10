# Build stage — use the same Go version as go.mod
# Use 1.26 for image availability in CI; go.mod uses 1.26.1 for toolchain/govulncheck
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with hardening flags
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-s -w" -o server ./cmd/server

# Final stage (use 3.23 for CI image availability; 3.24 when zlib 1.3.2+ is needed)
FROM alpine:3.23.3

# Create a non-root user to run the server
RUN addgroup -S snorlx && adduser -S -G snorlx snorlx

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates wget

# Copy binary from builder
COPY --from=builder /app/server .

# Transfer ownership to the non-root user
RUN chown snorlx:snorlx /app/server

USER snorlx

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Run
CMD ["./server"]
