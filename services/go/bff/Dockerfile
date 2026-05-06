# Multi-stage build for Go BFF service
FROM golang:1.26-alpine AS builder
WORKDIR /app

# Copy go mod files
COPY services/go/go.mod services/go/go.sum ./

# Download dependencies
RUN go mod download

# Copy source
COPY services/go ./

# Build the BFF service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /app/bin/bff ./bff/cmd/bff

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/bin/bff .

EXPOSE 8080
CMD ["./bff"]
