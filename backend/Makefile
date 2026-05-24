DBMATE_VOLUME=$(shell pwd)/core/data/schema/migrations
DBMATE_IMAGE=ghcr.io/amacneil/dbmate

.PHONY: new-migration
new-migration:
	@if [ -z "$(name)" ]; then \
		echo "Error: You must specify a migration name using 'make new-migration name=<migration_name>'"; \
		exit 1; \
	fi
	docker run --rm -it --network=host \
		-v "$(DBMATE_VOLUME):/db/migrations" \
		-e DBMATE_MIGRATIONS_DIR=/db/migrations \
		--user $(shell id -u):$(shell id -g) \
		$(DBMATE_IMAGE) new $(name)
