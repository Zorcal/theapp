# First slice: building electricity dashboard

## Goal

Prove the complete product and architecture with the smallest useful energy domain. A tenant can
define buildings and meters in PostgreSQL, ingest interval readings into Parquet, map source meter
identifiers, and query a real-time electricity dashboard grouped by building.

This slice demonstrates:

- transactional creation and editing of domain objects;
- tenant-scoped ingestion into columnar object storage;
- unresolved external identifiers and transactional mappings;
- a federated join between PostgreSQL and Parquet;
- interactive filtering and aggregation;
- an immediately visible result after changing a mapping.

It is intentionally not a general dashboard builder or a complete energy-management platform.

## Domain model

### PostgreSQL

`Building`

- organization ID
- project ID
- ID
- name
- optional description
- time zone
- timestamps and ETag

`Meter`

- organization ID
- project ID
- ID
- name
- building ID
- measurement kind, initially electricity consumption only
- unit, initially kWh only
- timestamps and ETag

`MeterMapping`

- organization ID
- project ID
- source name, such as `utility-export` or `demo-bms`
- external meter ID
- meter ID
- unique constraint on organization, project, source, and external meter ID

`Ingestion`

- organization ID
- project ID
- ID
- source name
- original filename and object location
- status and failure message
- accepted and rejected row counts
- creation and completion times
- idempotency identity or content checksum

The organization is the tenant and the project is the selected workspace and authorization scope.
Project already determines organization in the existing schema; retaining both values on energy
records and facts makes tenant enforcement explicit and supports safe composite joins. Constraints
must keep them consistent, and related foreign keys must prove that referenced records share both
scopes. These records are transactional: creating a meter and its initial mapping, moving a meter
between buildings, and replacing a mapping must either complete consistently or have no effect.

### Parquet

`IntervalReading`

- organization ID
- project ID
- ingestion ID
- source name
- external meter ID
- interval start as an unambiguous UTC instant
- interval duration in minutes
- consumption value
- unit

The natural fact identity is organization, project, source, external meter ID, interval start, and
interval duration. Consumption uses a fixed-precision representation rather than floating point.

Files are partitioned for pruning, conceptually:

```text
electricity/organization_id=<organization>/project_id=<project>/year=<yyyy>/month=<mm>/part-*.parquet
```

Organization ID, project ID, and interval start remain columns inside every file. File publication
must be atomic from a query's perspective so dashboards never read partial ingestion output.

## App flow

### 1. Define buildings and meters

The tenant creates buildings such as `Stockholm Office` and `Gothenburg Warehouse`, then creates
their electricity meters. These operations use the existing authorization, validation,
transactional storage, and optimistic-concurrency patterns.

### 2. Upload interval readings

The user selects a source, creates an ingestion, uploads a CSV file to the configured object store,
and tells the application to process that object. In deployed environments this can use a
short-lived cloud upload URL. Local development uses a local HTTP upload endpoint backed by files
under a configured development directory. The application validates the schema and units, converts
accepted rows to Parquet, and atomically publishes the files under the organization's project
prefix. The UI reports progress, accepted rows, rejected rows, and useful validation failures.

The file does not pass through gRPC: the server currently limits received messages to 2 MiB, while
the product intentionally demonstrates larger datasets. The API carries ingestion metadata and
object references only. Starting processing accepts an `x-idempotency-key` and runs as a durable
DBOS workflow so retries do not publish duplicate facts.

CSV is the first input format because it is easy to demonstrate and test. The stored analytical
format is always Parquet; API and building-management-system connectors can later reuse the same
normalized measurement schema.

### 3. Resolve imported meters

The mapping screen lists external meter IDs observed in Parquet and whether each resolves through
`MeterMapping`. The user can create and map a meter or connect the identifier to an existing meter.
Unmapped readings remain queryable under an `Unmapped` group and are never silently dropped.

### 4. Explore the dashboard

The building electricity dashboard provides:

- a date-range filter;
- summary metrics for total consumption, average interval consumption, and peak interval demand;
- a building breakdown for consumption and peak demand;
- an unmapped-meter count and link to the mapping workflow;
- drill-down from building to meter and interval.

Total consumption is `sum(consumption_kwh)`. Peak interval demand is initially the largest interval
consumption normalized to kW using its duration. This is an analytical approximation, not utility
billing demand, which may use rolling or coincident windows.

### 5. Demonstrate live reclassification

The user moves a meter to another building or changes an external-meter mapping, returns to the
dashboard, and refreshes it. The same Parquet readings join to updated PostgreSQL rows and the
historical aggregates change immediately. No CDC, warehouse synchronization, or Parquet rewrite is
involved.

## Federated query boundary

Conceptually, DuckDB reads organization, project, and date partitions from Parquet, scans the
required columns, and joins the readings to equivalently scoped PostgreSQL mappings, meters, and
buildings:

```sql
SELECT
    coalesce(building.name, 'Unmapped') AS building,
    sum(reading.consumption_kwh) AS consumption_kwh,
    max(reading.consumption_kwh * 60 / reading.interval_minutes) AS peak_kw
FROM read_parquet(<organization-project-and-date-scoped-files>) AS reading
LEFT JOIN meter_mapping AS mapping
  ON mapping.organization_id = reading.organization_id
 AND mapping.project_id = reading.project_id
 AND mapping.source_name = reading.source_name
 AND mapping.external_meter_id = reading.external_meter_id
LEFT JOIN meter
  ON meter.organization_id = mapping.organization_id
 AND meter.project_id = mapping.project_id
 AND meter.id = mapping.meter_id
LEFT JOIN building
  ON building.organization_id = meter.organization_id
 AND building.project_id = meter.project_id
 AND building.id = meter.building_id
WHERE reading.organization_id = <authorized-organization>
  AND reading.project_id = <authorized-project>
  AND reading.interval_start >= <start>
  AND reading.interval_start < <end>
GROUP BY building.id, building.name;
```

The SQL is illustrative rather than an implementation contract. The authenticated `x-project-id`
context resolves the project and organization; query construction must inject both scopes rather
than accepting them as ordinary user filters.

## Fit with the existing codebase

The first slice should reuse these established patterns:

- protobuf schemas, generated Go clients, grpc-gateway, OpenAPI annotations, and Swagger UI;
- handler validators and conversion packages at the transport boundary;
- `mdl` inputs with `Validate`, domain cores, pgstores, typed pgx queries, and dbmate migrations;
- ETags for mutable buildings and meters and `pgdb.Transactor` for multi-record mutations;
- new energy permissions registered in the fail-closed gRPC permission registry and assigned at
  project scope unless an operation genuinely spans the organization;
- `ErrorDetail` only for actionable distinctions not already expressed by gRPC status or standard
  `google.rpc` details;
- DBOS for ingestion orchestration, with external object writes implemented as retry-safe steps;
- OpenTelemetry instrumentation and the existing full-suite and PostgreSQL integration-test style.

The first slice must add an object-storage abstraction and local object-storage service, a Parquet
writer, and a DuckDB or MotherDuck query adapter. Those choices belong to the implementation plan.
They should be interfaced at the application boundary so local DuckDB and a hosted analytical
backend can be evaluated without changing the energy core.

## Offline local development

After dependencies and development images have been installed, the complete application flow must
work locally without internet access:

- an injected filesystem object store keeps uploaded CSV and generated Parquet files under a
  configured directory outside the PostgreSQL data model;
- a local upload handler streams request bodies to that store without passing large payloads
  through gRPC;
- embedded DuckDB reads the local Parquet files and federates with local PostgreSQL;
- staging and production inject a cloud object-store adapter, initially GCS or S3-compatible, and
  may inject MotherDuck without changing energy-domain or workflow code;
- tests use temporary directories through the same object-store interface and never require cloud
  credentials, emulators, or network access;
- required DuckDB extensions and native libraries are pinned and packaged at build or image-build
  time; application startup and query execution must not download extensions dynamically.

Local and cloud implementations must obey the same path scoping, atomic-publication, retry, and
error semantics. Provider-specific features stay behind adapters. Configuration at the composition
root selects implementations; environment checks must not spread through domain, workflow, or query
code.

## Explicitly out of scope

- Gas, water, generation, storage, cost, tariffs, emissions, or weather
- Campuses, regions, building types, meter groups, or cost centers
- Cumulative meter registers and conversion to interval consumption
- Mixed units or unit conversion
- Missing-interval estimation, anomaly correction, or billing-grade demand
- Effective-dated building or mapping history
- Arbitrary user-authored SQL, metrics, charts, or dashboards
- Streaming ingestion and third-party connectors
- Sending measurement files through gRPC or storing their contents in PostgreSQL
- Cross-tenant comparisons
- CDC or duplicated PostgreSQL dimensions in analytical storage

These omissions keep the first slice small while leaving a direct extension path to the broader
product vision.

## Success criteria

The slice has established the pattern when two isolated tenants can each:

1. manage buildings, meters, and mappings transactionally;
2. ingest a realistically large interval-reading file and observe its processing state;
3. query consumption and peak demand interactively without loading readings into PostgreSQL;
4. see unknown meter IDs and resolve them without re-ingestion;
5. change a mapping or building assignment and observe updated historical aggregates immediately;
   and
6. receive no data, identifiers, object paths, or aggregates belonging to another tenant.

Implementation phases, technical choices, performance budgets, and rollout order belong in a
separate plan created after this scope is accepted.
