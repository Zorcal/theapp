# pgschema

Contains the Postgres database schema and migrations for the application. Migrations are applied automatically at startup.

New migration files are created with `make new-pg-migration name=<name>` from `backend/`.

## schema.sql

A full snapshot of the current schema generated from the migration files. Useful for understanding the current database structure without having to read through individual migration files.

It is generated — do not edit it by hand. Regenerate it with `make dump-pg-schema` after adding migrations, and commit both files together.
