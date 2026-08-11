.PHONY: build build-frontend run test specs specs-use specs-current clean docker docker-up docker-down lint tidy dev-frontend setup-hooks

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

# The spec tree's own gate: ownership, house shape, and that every test a spec cites exists.
# CI runs it too, as part of `go test ./...`.
specs:
	go test ./internal/speckit/ -count=1

# Selects the spec that /speckit-plan, -tasks, -analyze, -checklist and -implement act on, by
# writing the same .specify/feature.json those commands read. Per-machine and gitignored: two
# people sharing a checkout do not overwrite each other's selection.
#
#   make specs-use SPEC=004
#   make specs-use SPEC=004-carddav-service
#
# A prefix is enough as long as it names exactly one spec. Ambiguity is an error rather than a
# guess: silently planning against the wrong spec is the failure this target exists to prevent.
specs-use:
	@test -n "$(SPEC)" || { echo "usage: make specs-use SPEC=<number-or-slug>" >&2; exit 2; }
	@matches=$$(ls -d specs/$(SPEC)*/ 2>/dev/null | sed 's:/$$::'); \
	count=$$(printf '%s' "$$matches" | grep -c . || true); \
	if [ "$$count" -eq 0 ]; then echo "no spec matches '$(SPEC)':" >&2; ls -d specs/*/ >&2; exit 1; fi; \
	if [ "$$count" -gt 1 ]; then echo "'$(SPEC)' matches more than one spec:" >&2; echo "$$matches" >&2; exit 1; fi; \
	printf '{"feature_directory":"%s"}\n' "$$matches" > .specify/feature.json; \
	echo "selected $$matches"

# Prints the current selection. State you can set and cannot see is state you will get wrong.
specs-current:
	@if [ -f .specify/feature.json ]; then \
		sed -n 's/.*"feature_directory":"\([^"]*\)".*/\1/p' .specify/feature.json; \
	else \
		echo "no spec selected — run: make specs-use SPEC=<number-or-slug>"; \
	fi

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
