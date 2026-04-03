# V4 Pencil Engine — backend + frontend
# Railway root must be set to "/" (repo root) for this to work

# Stage 1: Build frontend (from project/frontend/)
FROM node:22-alpine AS frontend
WORKDIR /build
COPY project/frontend/package*.json ./
RUN npm ci
COPY project/frontend/ ./
ENV VITE_API_URL=/api/v1
RUN npm run build

# Stage 2: Build Go binary (from project_v4/backend/)
FROM golang:1.24-alpine AS backend
WORKDIR /build
COPY project_v4/backend/go.mod project_v4/backend/go.sum ./
RUN go mod download
COPY project_v4/backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=backend /build/server .
COPY --from=frontend /build/dist ./static
EXPOSE 8082
CMD ["./server"]
