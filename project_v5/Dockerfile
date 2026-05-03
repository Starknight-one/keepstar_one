# V5 chat engine — full stack (widget frontend IIFE bundle + Go backend)
# Build context: repo root (Railway service config: rootDirectory="/")
# Mirrors project_v4/Dockerfile so deploy parity stays simple.

# Stage 1: build widget.js (Vite IIFE bundle, ~206 KB / 64 KB gz)
FROM node:22-alpine AS frontend
WORKDIR /build
COPY project_v5/frontend/package*.json ./
RUN npm ci
COPY project_v5/frontend/ ./
RUN npm run build

# Stage 2: build Go binary
FROM golang:1.24-alpine AS backend
WORKDIR /build
COPY project_v5/backend/go.mod project_v5/backend/go.sum ./
RUN go mod download
COPY project_v5/backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# Stage 3: alpine runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=backend /build/server .
COPY --from=frontend /build/dist ./static
EXPOSE 8084
CMD ["./server"]
