# Multi-stage build for the curator backend.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server/
RUN CGO_ENABLED=0 go build -o /out/seed-curator ./cmd/seed-curator/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/server /usr/local/bin/curator-server
COPY --from=build /out/seed-curator /usr/local/bin/seed-curator
EXPOSE 8082
CMD ["/usr/local/bin/curator-server"]
