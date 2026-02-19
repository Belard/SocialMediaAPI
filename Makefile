.PHONY: help build run dev docker-up docker-down docker-logs clean test smoke-facebook-oauth smoke

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the application
	go build -o bin/multiplatform-app main.go

run: ## Run the application locally
	go run main.go

dev: ## Run with hot reload (requires air: go install github.com/cosmtrek/air@latest)
	air

docker-up: ## Start all services with Docker Compose
	docker-compose up -d

docker-down: ## Stop all Docker services
	docker-compose down

docker-logs: ## Show Docker logs
	docker-compose logs -f

docker-rebuild: ## Rebuild and restart Docker services
	docker-compose down
	docker-compose build --no-cache
	docker-compose up -d

clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf uploads/*

test: ## Run tests
	go test -v ./...

generate-cert: ## Generate self-signed TLS certificate for local HTTPS
	./scripts/generate-cert.sh

smoke-facebook-oauth: ## Run OAuth-only Facebook API smoke test (requires running API)
	bash ./test/smoke/facebook_oauth_smoke.sh

smoke: test smoke-facebook-oauth ## Run unit tests and smoke test suite

install: ## Install dependencies
	go mod download
	go mod tidy

db-only: ## Start only PostgreSQL
	docker-compose up -d postgres

db-connect: ## Connect to PostgreSQL CLI
	docker exec -it multiplatform-postgres psql -U postgres -d multiplatform