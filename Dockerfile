# Stage 1: Build frontend
FROM node:22-alpine AS frontend

WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG BUILD_TIME=unknown

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /app/internal/web/static/spa ./internal/web/static/spa
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o contactshq ./cmd/server

# Stage 3: Runtime
FROM alpine:3.21

# No sqlite-libs: the binary is CGO-free and links modernc.org/sqlite, a pure-Go driver.
# wget comes from busybox and is what the health check uses.
RUN apk add --no-cache ca-certificates

# Run unprivileged. The backups directory has to belong to that user.
RUN adduser -D -u 10001 contactshq

WORKDIR /app
COPY --from=builder /app/contactshq .
# Only the template. Shipping the whole directory risks baking a real configs/config.yaml
# into the image, which would leak its secret and quietly outrank the environment.
COPY --from=builder /app/configs/config.example.yaml ./configs/
# Migrations are embedded in the binary; there is nothing to copy.

RUN mkdir -p /app/backups && chown -R contactshq:contactshq /app

USER contactshq

EXPOSE 8080

# /health probes the database, so a container that cannot reach it is reported unhealthy
# rather than kept in service.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://127.0.0.1:8080/health || exit 1

CMD ["./contactshq"]
