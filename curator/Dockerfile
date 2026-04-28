# One-binary curator deployment: SPA + Go backend behind a single port.
# Mirrors project_admin/Dockerfile so Railway treats curator as one service.
#
# Build context must be the curator/ directory (set Root Directory in Railway).

# Stage 1: build frontend
FROM node:22-alpine AS frontend
WORKDIR /build
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# Stage 2: build Go server
FROM golang:1.24-alpine AS backend
WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# Stage 3: runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=backend /build/server .
COPY --from=frontend /build/dist ./static
EXPOSE 8082
CMD ["./server"]
