# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o goclaw .

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create app user
RUN addgroup -g 1000 goclaw && \
    adduser -D -u 1000 -G goclaw goclaw

# Set working directory
WORKDIR /home/goclaw

# Copy binary from builder
COPY --from=builder /app/goclaw .

# Create directories
RUN mkdir -p .goclaw/workspace .goclaw/sessions && \
    chown -R goclaw:goclaw /home/goclaw

# Switch to non-root user
USER goclaw

# Expose health check and webhooks
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./goclaw", "start"]
