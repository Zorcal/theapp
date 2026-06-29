# ClickHouse

ClickHouse is the analytical store. It is a first-class database, not a Postgres replica — it holds data that never exists in Postgres (calculated metrics, workflow events, ingested analytical records) as well as a replicated copy of Postgres application tables so that analytical queries can join, aggregate, sort, and group by them alongside the rest.

The two write paths are:

- **Direct ingestion** — background workflows write analytical data straight to ClickHouse. This data has no Postgres counterpart.
- **Postgres replication** — application data is created transactionally in Postgres and replicated into ClickHouse. Postgres is the source of truth for these rows; the ClickHouse copy is read-only.

## Setup

ClickHouse runs as a Docker Compose service in `infra/` alongside Postgres and the observability stack. The same `docker-compose.yml` is used locally and in production. The HTTP interface is on port 8123 and the native protocol on port 9000.

## Migrations

ClickHouse schema changes are managed with dbmate, mirroring the Postgres setup. Migrations live under `internal/data/chschema/migrations/` and the dumped schema at `internal/data/chschema/schema.sql`. The Makefile exposes `new-ch-migration` and `dump-ch-schema` targets that follow the same pattern as their `pg` counterparts.

## Replicating Postgres tables

When self-hosting, use ClickHouse's `MaterializedPostgreSQL` database engine to tail the Postgres WAL and replicate application tables in near-real time. Each replicated table uses `ReplacingMergeTree` to handle upserts from the replication stream. Schema changes in Postgres require a corresponding ClickHouse migration before the next replication cycle, or the replication slot will stall.

When using ClickHouse Cloud, `MaterializedPostgreSQL` is not supported. Use ClickPipes instead — it is the managed CDC connector in ClickHouse Cloud and provides equivalent WAL-based replication from Postgres.
