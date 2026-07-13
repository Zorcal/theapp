# pgschema

Contains the Postgres database schema and migrations for the application. Migrations are applied automatically at startup.

New migration files are created with `make new-pg-migration name=<name>` from `backend/`.

Never use `ON DELETE CASCADE` (or `CASCADE` on any other DDL statement) in a migration. Dependent-row cleanup is always explicit application code, not a database cascade.

## seed.sql

Seed data — hardcoded rows that need to exist as real data rather than being created through the application. Unlike a migration, it isn't tracked as applied-once, so every statement in it must be idempotent (`ON CONFLICT ... DO NOTHING`), safe to run again unchanged against a database that already has the data.

## schema.sql

A full snapshot of the current schema generated from the migration files. Useful for understanding the current database structure without having to read through individual migration files.

It is generated — do not edit it by hand. Regenerate it with `make dump-pg-schema` after adding migrations, and commit both files together.
