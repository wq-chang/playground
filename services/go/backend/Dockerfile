# Multi-stage build for Go backend service
FROM golang:1.26-alpine AS builder
WORKDIR /app

# Copy go mod files
COPY services/go/go.mod services/go/go.sum ./

# Download dependencies
RUN go mod download

# Copy source
COPY services/go ./

# Build the backend service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /app/bin/backend ./backend

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/bin/backend .

EXPOSE 8080
CMD ["./backend"]
