# Multi-stage build for Go services (backend, bff)
# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app

# Copy go mod files
COPY services/go/go.mod services/go/go.sum ./

# Download dependencies
RUN go mod download

# Copy source
COPY services/go ./

# Build the application (ARG to specify which service)
ARG SERVICE_NAME=backend
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /app/bin/service ./cmd/${SERVICE_NAME}

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/bin/service .

EXPOSE 8080
CMD ["./service"]
