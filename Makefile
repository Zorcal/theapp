BACKEND_DIR := backend
INFRA_DIR   := infra

.PHONY: up
up:
	cd $(INFRA_DIR) && docker compose up -d

.PHONY: down
down:
	cd $(INFRA_DIR) && docker compose down

.PHONY: generate
generate: generate-go generate-proto

.PHONY: generate-go
generate-go:
	cd $(BACKEND_DIR) && go generate ./...

.PHONY: gen-proto
generate-proto:
	buf dep update
	buf generate
