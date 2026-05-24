BACKEND_DIR := backend
INFRA_DIR   := infra

.PHONY: run
run:
	cd $(BACKEND_DIR) && go run ./cmd/server/*.go

.PHONY: generate
generate:
	cd $(BACKEND_DIR) && go generate ./...

.PHONY: up
up:
	cd $(INFRA_DIR) && docker compose up -d

.PHONY: down
down:
	cd $(INFRA_DIR) && docker compose down

