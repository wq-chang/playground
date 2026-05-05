# Multi-stage build for React frontend
FROM node:24-alpine AS builder
WORKDIR /app

# Copy package files
COPY frontend/package.json frontend/package-lock.json ./

# Install dependencies
RUN npm ci --prefer-offline

# Copy source
COPY frontend ./

# Build
RUN npm run build

# Runtime stage - serve with nginx
FROM nginx:alpine
WORKDIR /usr/share/nginx/html

# Copy built assets
COPY --from=builder /app/dist .

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
