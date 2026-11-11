VERSION ?= dev
REGISTRY ?= ghcr.io/kayamii

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-18s %s\n", $$1, $$2}'

.PHONY: install
install: ## Install frontend dependencies
	cd frontend && npm ci

.PHONY: dev-backend
dev-backend: ## Run the backend on :8080
	cd backend && go run ./cmd/podora

.PHONY: dev-frontend
dev-frontend: ## Run the frontend dev server on :3000 (proxies /api to :8080)
	cd frontend && npm run dev

.PHONY: build
build: build-backend build-frontend ## Build backend and frontend

.PHONY: build-backend
build-backend:
	cd backend && go build -trimpath -ldflags "-X github.com/Kayamii/Podora/backend/internal/api.Version=$(VERSION)" -o bin/podora ./cmd/podora

.PHONY: build-frontend
build-frontend:
	cd frontend && npm run build

.PHONY: test
test: ## Run backend tests
	cd backend && go test ./...

.PHONY: lint
lint: ## Vet the backend, lint the frontend and the chart
	cd backend && go vet ./...
	cd frontend && npm run lint
	helm lint charts/podora

.PHONY: docker
docker: ## Build both container images
	docker build --build-arg VERSION=$(VERSION) -t $(REGISTRY)/podora-backend:$(VERSION) backend
	docker build -t $(REGISTRY)/podora-frontend:$(VERSION) frontend

.PHONY: up
up: ## Run the full stack with docker compose
	docker compose up --build
