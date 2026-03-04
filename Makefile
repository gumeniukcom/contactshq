.PHONY: build build-frontend run test clean docker docker-up docker-down lint tidy dev-frontend setup-hooks

BINARY    = contactshq
VERSION   = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   = -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)

build-frontend:
	cd web && npm ci && npm run build

build: build-frontend
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/server

run: build
	./$(BINARY)

dev-frontend:
	cd web && npm run dev

test:
	go test ./... -v -count=1

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f $(BINARY) coverage.out coverage.html
	rm -f contactshq.db
	rm -rf internal/web/static/spa/*
	touch internal/web/static/spa/.gitkeep

tidy:
	go mod tidy

lint:
	golangci-lint run ./...

docker:
	docker build -t contactshq .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

setup-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks configured to use .githooks/"
