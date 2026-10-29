# Build stage: use a 1.27 image for availability; go.mod pins the patch for govulncheck.
# BUILDPLATFORM keeps the Go compile native while buildx targets arm64.
# Images are pinned by digest; Dependabot and Renovate move the digest when the tag moves.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with hardening flags. buildx sets TARGETOS and TARGETARCH per platform.
# Defaults keep a plain docker build on linux/amd64 working when they are unset.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -installsuffix cgo \
    -ldflags="-s -w" -o server ./cmd/server

# Final stage
FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

# Create a non-root user with a fixed numeric uid so Kubernetes runAsNonRoot can verify it
RUN addgroup -S -g 1000 snorlx && adduser -S -u 1000 -G snorlx snorlx

WORKDIR /app

# ca-certificates for HTTPS to GitHub, wget for the health check, then pull every
# package fix published since the base image was built (not only the OpenSSL ones).
RUN apk --no-cache add ca-certificates wget \
    && apk --no-cache upgrade

# Copy binary from builder
COPY --from=builder /app/server .

# Transfer ownership to the non-root user
RUN chown snorlx:snorlx /app/server

USER 1000:1000

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Run
CMD ["./server"]
