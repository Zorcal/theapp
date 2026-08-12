# Product documentation

This directory holds durable product visions and scoped initiatives.

## Energy management analytics

A multi-tenant application for combining live energy-asset configuration with imported interval
measurements:

- [Product vision](energy-management/README.md)
- [First product slice and app flow](energy-management/first-slice/README.md)

The first slice deliberately limits the energy domain while retaining the architectural pattern the
product is intended to prove: transactional domain objects in PostgreSQL, analytical measurements
in Parquet object storage, and real-time federated queries through DuckDB or MotherDuck.
