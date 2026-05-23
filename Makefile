# VampiFox Makefile — "Mantra untuk membangunkan kerajaan"
.PHONY: help awaken build test test-cover lint docker-up docker-down docker-build tidy

BINARY=vampifox
VERSION=$(shell git describe --tags --always 2>/dev/null || echo "0.1.0-nightfall")
BUILD_FLAGS=-ldflags="-s -w -X main.Version=$(VERSION)"

help: ## Tampilkan daftar mantra
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

awaken: ## Jalankan VampiFox di mode development
	@echo "🦊🧛 VampiFox sedang terbangun..."
	go run ./cmd/vampifox

build: ## Build binary VampiFox
	@echo "🔨 Membangun kerajaan..."
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o bin/$(BINARY) ./cmd/vampifox

test: ## Jalankan semua test
	@echo "🧪 Menguji kekuatan..."
	go test ./... -v -race -coverprofile=coverage.out

test-cover: test ## Test + tampilkan coverage
	go tool cover -html=coverage.out

lint: ## Linting kode
	golangci-lint run ./...

docker-up: ## Jalankan stack development dengan Docker
	@echo "🏰 Membangun kerajaan lokal..."
	docker compose -f deploy/docker/docker-compose.yml up -d

docker-down: ## Matikan stack development
	docker compose -f deploy/docker/docker-compose.yml down

docker-build: ## Build Docker image
	docker build -f deploy/docker/Dockerfile -t vampifox:$(VERSION) .

tidy: ## Bersihkan dependencies
	go mod tidy

.DEFAULT_GOAL := help