LAST_COMMIT := $(or $(shell git rev-parse --short HEAD 2> /dev/null),$(shell head -n 1 VERSION | grep -oP -m 1 "^[a-z0-9]+$$"), "")

# Try to get the semver from 1) $CONECTDESK_VERSION 2) git tag 3) the VERSION file 4) fallback.
VERSION := $(or $(CONECTDESK_VERSION),$(shell git describe --tags --abbrev=0 2> /dev/null),$(shell grep -oP 'tag: \Kv\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?' VERSION),"v0.0.0")

BUILDSTR := ${VERSION} (\#${LAST_COMMIT} $(shell date -u +"%Y-%m-%dT%H:%M:%S%z"))

# Binary names and paths.
BIN := conectdesk
FRONTEND_DIR := frontend
FRONTEND_DIST := ${FRONTEND_DIR}/dist
STATIC := ${FRONTEND_DIST} i18n schema static
GOPATH ?= $(HOME)/go
STUFFBIN ?= $(GOPATH)/bin/stuffbin

LDFLAGS := -X 'main.buildString=${BUILDSTR}' \
	-X 'main.versionString=${VERSION}' \
	-X 'github.com/conectdesk/conectdesk/internal/version.Version=${VERSION}'

.DEFAULT_GOAL := build

.PHONY: help
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed -e 's/^## //' | awk -F ': ' '{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

$(STUFFBIN):
	@echo "→ Installing stuffbin..."
	@go install github.com/knadh/stuffbin/...

.PHONY: install-deps
install-deps: $(STUFFBIN)
	@echo "→ Installing frontend dependencies..."
	@cd ${FRONTEND_DIR} && pnpm install

.PHONY: frontend-build
frontend-build: frontend-build-main frontend-build-widget

.PHONY: frontend-build-main
frontend-build-main: install-deps
	@echo "→ Building main frontend app for production..."
	@cd ${FRONTEND_DIR} && VITE_APP_VERSION="${VERSION}" pnpm build:main

.PHONY: frontend-build-widget
frontend-build-widget: install-deps
	@echo "→ Building widget frontend app for production..."
	@cd ${FRONTEND_DIR} && VITE_APP_VERSION="${VERSION}" pnpm build:widget

.PHONY: run-backend
run-backend:
	@echo "→ Running backend..."
	CGO_ENABLED=0 go run -ldflags="-s -w ${LDFLAGS} -X 'main.frontendDir=frontend/dist'" ./cmd/

.PHONY: run-frontend
run-frontend: run-frontend-main

.PHONY: run-frontend-main
run-frontend-main:
	@echo "→ Installing frontend dependencies (if not already installed)..."
	@cd ${FRONTEND_DIR} && pnpm install
	@echo "→ Running main frontend app..."
	@cd ${FRONTEND_DIR} && VITE_APP_VERSION="${VERSION}" pnpm dev:main

.PHONY: run-frontend-widget
run-frontend-widget:
	@echo "→ Installing frontend dependencies (if not already installed)..."
	@cd ${FRONTEND_DIR} && pnpm install
	@echo "→ Running widget frontend app..."
	@cd ${FRONTEND_DIR} && VITE_APP_VERSION="${VERSION}" pnpm dev:widget

.PHONY: build-backend
build-backend: $(STUFFBIN)
	@echo "→ Building backend..."
	@CGO_ENABLED=0 go build -a -ldflags="${LDFLAGS} -s -w" -o ${BIN} cmd/*.go

.PHONY: stuff
stuff: $(STUFFBIN)
	@echo "→ Stuffing static assets into binary..."
	@$(STUFFBIN) -a stuff -in ${BIN} -out ${BIN} ${STATIC}

.PHONY: build
build: frontend-build build-backend stuff
	@echo "→ Build successful. Current version: $(VERSION)"

.PHONY: demo-build
demo-build:
	@echo "→ Building in demo mode..."
	@VITE_DEMO_BUILD="true" $(MAKE) build

.PHONY: test
test: test-go test-frontend

.PHONY: test-go
test-go:
	@echo "→ Running Go tests..."
	go test -count=1 ./...

.PHONY: test-frontend
test-frontend:
	@echo "→ Running frontend tests..."
	cd ${FRONTEND_DIR} && pnpm install --frozen-lockfile && pnpm test:run

.PHONY: test-db
test-db:
	docker run -d --name conectdesk-test-db -p 127.0.0.1:5433:5432 \
		-e POSTGRES_USER=conectdesk -e POSTGRES_PASSWORD=conectdesk -e POSTGRES_DB=conectdesk \
		postgres:17-alpine

.PHONY: test-db-down
test-db-down:
	docker rm -f conectdesk-test-db

.PHONY: clean
clean:
	rm -f ${BIN}
	rm -rf ${FRONTEND_DIST}

.PHONY: start-docker
start-docker:
	docker compose up -d
