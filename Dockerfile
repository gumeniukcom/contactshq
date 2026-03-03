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

RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app
COPY --from=builder /app/contactshq .
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/migrations ./migrations

RUN mkdir -p /app/backups

EXPOSE 8080

CMD ["./contactshq"]
