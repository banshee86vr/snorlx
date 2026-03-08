# Build stage — use the same Go version as go.mod
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

# Final stage
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
