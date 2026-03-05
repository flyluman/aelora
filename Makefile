.PHONY: fmt test run-server run-migrate run-migrate-local lint vet build up-local down-local reset-local ps-local logs-local image-server k8s-render k8s-apply k8s-delete test-integration bench cover wire-gen

ENV_FILE := $(if $(wildcard .env),.env,.env.example)
COMPOSE := docker compose --env-file $(ENV_FILE) -f infra/local/docker-compose.yml

fmt:
	go fmt ./...

test:
	GOCACHE=/tmp/go-build go test ./...

run-server:
	@set -a; . ./$(ENV_FILE); set +a; go run ./cmd/server

run-migrate:
	@set -a; . ./$(ENV_FILE); set +a; go run ./cmd/server migrate

build:
	GOCACHE=/tmp/go-build go build ./...

lint:
	golangci-lint run

vet:
	go vet ./...

up-local:
	$(COMPOSE) up -d --build --remove-orphans postgres valkey server

down-local:
	$(COMPOSE) down

reset-local:
	$(COMPOSE) down -v --remove-orphans

ps-local:
	$(COMPOSE) ps

logs-local:
	$(COMPOSE) logs -f --tail=150

run-migrate-local:
	$(COMPOSE) run --rm server migrate

image-server:
	docker build -f infra/docker/Dockerfile --target runtime-server -t aelora/server:latest .

k8s-render:
	kubectl kustomize infra/k8s/base

k8s-apply:
	kubectl apply -k infra/k8s/base

k8s-delete:
	kubectl delete -k infra/k8s/base --ignore-not-found=true

bench:
	go test ./internal/... -bench=. -benchmem

cover:
	go test ./internal/... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

test-integration:
	@echo "Run integration tests with: go test -tags=integration ./internal/..."

wire-gen:
	@echo "Wire generation not yet configured"
