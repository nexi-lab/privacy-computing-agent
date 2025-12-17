# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy source code
COPY . ./

RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod tidy
RUN go build -ldflags="-w -s" -o agent_proxy .

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata && \
    mkdir -p /app/config

# Copy binary from builder
COPY --from=builder /build/agent_proxy .

# Expose port
EXPOSE 2024

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:2024/ || exit 1

# Run the binary
CMD ["./agent_proxy"]

