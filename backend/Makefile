DBMATE_VOLUME=$(shell pwd)/internal/data/pgschema/migrations
DBMATE_SCHEMA_DIR=$(shell pwd)/internal/data/pgschema
DBMATE_IMAGE=ghcr.io/amacneil/dbmate
PG_IMAGE=postgres:17

.PHONY: new-pg-migration
new-pg-migration:
	@if [ -z "$(name)" ]; then \
		echo "Error: You must specify a migration name using 'make new-pg-migration name=<migration_name>'"; \
		exit 1; \
	fi
	docker run --rm -it --network=host \
		-v "$(DBMATE_VOLUME):/db/migrations" \
		-e DBMATE_MIGRATIONS_DIR=/db/migrations \
		--user $(shell id -u):$(shell id -g) \
		$(DBMATE_IMAGE) new $(name)

.PHONY: dump-pg-schema
dump-pg-schema:
	@set -e; \
	NETWORK=pgschema-dump-$$$$; \
	PG=$$NETWORK-postgres; \
	trap 'docker stop $$PG 2>/dev/null; docker network rm $$NETWORK 2>/dev/null' EXIT; \
	docker network create $$NETWORK; \
	docker run --rm -d --name $$PG --network $$NETWORK \
		-e POSTGRES_HOST_AUTH_METHOD=trust \
		$(PG_IMAGE); \
	docker run --rm --network $$NETWORK \
		-v "$(DBMATE_VOLUME):/db/migrations" \
		-e DBMATE_MIGRATIONS_DIR=/db/migrations \
		-e DBMATE_NO_DUMP_SCHEMA=true \
		-e DATABASE_URL="postgres://postgres@$$PG/theapp?sslmode=disable" \
		$(DBMATE_IMAGE) --wait up; \
	docker run --rm --network $$NETWORK \
		-v "$(DBMATE_VOLUME):/db/migrations" \
		-v "$(DBMATE_SCHEMA_DIR):/db/schema" \
		-e DBMATE_MIGRATIONS_DIR=/db/migrations \
		-e DBMATE_SCHEMA_FILE=/db/schema/schema.sql \
		-e DATABASE_URL="postgres://postgres@$$PG/theapp?sslmode=disable" \
		--user $(shell id -u):$(shell id -g) \
		$(DBMATE_IMAGE) dump
