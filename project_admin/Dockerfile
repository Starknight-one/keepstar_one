# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /build
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.24-alpine AS backend
WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=backend /build/server .
COPY --from=frontend /build/dist ./static
EXPOSE 8081
CMD ["./server"]
