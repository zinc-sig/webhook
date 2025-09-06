# syntax=docker/dockerfile:1.4

# -- Build stage: Use Chainguard Go builder image, reproducible and minimal
FROM --platform=$BUILDPLATFORM cgr.dev/chainguard/go:latest-dev AS builder

# Optional: Specify target OS/arch for cross-platform builds
ARG TARGETOS
ARG TARGETARCH

WORKDIR /work

# Enable Go modules, prefer static binary for a distroless/static runtime
ENV CGO_ENABLED=0

# Leverage Docker layer caching by copying go.mod/sum first
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the Go binary (adjust ./cmd/server as your package main)
RUN go build -trimpath -ldflags="-s -w" -o /out/webhook ./cmd

# -- Production stage: Use Chainguard minimal static image, non-root
FROM cgr.dev/chainguard/static:latest AS runtime

# Copy only the binary from the builder, place at fixed path
COPY --from=builder /out/webhook /webhook

# Run as non-root (default in Chainguard static), working directory optional
USER nonroot:nonroot
WORKDIR /

# Optionally, restrict image capabilities with an explicit instruction
# (Can also be managed by orchestrator/podspec)
# RUN chmod 755 /app

# Entrypoint as exec form for signal handling
ENTRYPOINT ["/webhook"]
