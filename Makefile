SHELL := /bin/bash

GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/mrweez/iwmmd/internal/platform/version.Version=$(VERSION) \
	-X github.com/mrweez/iwmmd/internal/platform/version.Commit=$(COMMIT) \
	-X github.com/mrweez/iwmmd/internal/platform/version.Date=$(BUILD_DATE)

ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: help dev-api dev-web web-install web-build build test fmt vet lint key docker-build compose-up compose-build-up compose-down clean

help:
	@echo "Targets:"
	@echo "  dev-api           Run the Go API with local data and text logs"
	@echo "  dev-web           Run the Vite dev server"
	@echo "  web-install       Install frontend dependencies"
	@echo "  web-build         Build the embedded SPA"
	@echo "  build             Build bin/iwmmd with the SPA embedded"
	@echo "  test              Run Go tests"
	@echo "  lint              Run go vet and oxlint"
	@echo "  key               Generate a master key"
	@echo "  docker-build      Build the local multi-stage image"
	@echo "  compose-up        Start the published image with Compose"
	@echo "  compose-build-up  Build and start the local image with Compose"
	@echo "  compose-down      Stop Compose services"
	@echo "  clean             Remove local data and build output"

dev-api:
	IWMMD_DATA_DIR=./data IWMMD_LOG_FORMAT=text IWMMD_METRICS_ADDR=127.0.0.1:9090 $(GO) run ./cmd/iwmmd serve

dev-web:
	cd web && pnpm dev

web-install:
	cd web && pnpm install

web-build:
	cd web && pnpm build

build: web-build
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/iwmmd ./cmd/iwmmd

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint: vet
	cd web && pnpm lint

key:
	$(GO) run ./cmd/iwmmd gen-key

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t iwmmd:local .

compose-up:
	docker compose up -d

compose-build-up:
	docker compose -f compose.build.yaml up -d --build

compose-down:
	docker compose down

clean:
	rm -rf bin data
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
