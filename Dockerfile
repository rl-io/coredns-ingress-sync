# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

# Install ca-certificates in builder stage
RUN apk --no-cache add ca-certificates git

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Get build arguments for multi-platform builds
ARG TARGETOS
ARG TARGETARCH

# Build the binary with static linking and security flags using cross-compilation
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o controller main.go

# Final stage - minimal runtime image
FROM scratch

# Copy ca-certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /app/controller /controller

# Create a non-root user (we'll use numeric IDs since scratch has no /etc/passwd)
# The controller will run as user 65534 (nobody)
USER 65534:65534

# Set entrypoint
ENTRYPOINT ["/controller"]
