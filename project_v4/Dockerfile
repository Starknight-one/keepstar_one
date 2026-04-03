# V4 Pencil Engine — backend only (no frontend bundled)
# Frontend connects via API URL

# Stage 1: Build Go binary
FROM golang:1.24-alpine AS backend
WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# Stage 2: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=backend /build/server .
EXPOSE 8082
CMD ["./server"]
