DBMATE_VOLUME=$(shell pwd)/internal/data/pgschema/migrations
DBMATE_IMAGE=ghcr.io/amacneil/dbmate

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
