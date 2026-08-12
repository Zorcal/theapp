# Energy management analytics

## Product vision

Build a multi-tenant energy analytics application where organizations understand consumption and
demand across their buildings and sites. Each tenant owns its assets, meter mappings, reporting
structure, targets, and analytical views.

In the existing application, an `Organization` is the tenant and a `Project` is an authorization
and workspace scope within it. Energy resources belong to a project; organization- and
system-scoped role assignments continue to grant broader access through the existing RBAC model.

The application combines two kinds of data:

- PostgreSQL stores smaller, mutable domain objects requiring transactions and immediate
  consistency: sites, buildings, meters, meter groups, external-ID mappings, tariffs, targets,
  metric definitions, and dashboards.
- Object storage holds larger, append-oriented datasets as partitioned Parquet files: electricity,
  gas and water readings, equipment telemetry, weather observations, and emissions factors.

DuckDB, with MotherDuck as a possible deployment option, queries Parquet and joins it directly to
the current tenant-owned PostgreSQL data. This avoids copying transactional dimensions through CDC.
A transactional change, such as moving a meter to another building, can affect a newly executed
analytical query immediately.

## Product experience

An energy manager should be able to:

1. Create the sites, buildings, meters, and reporting groups it manages.
2. Map meter identifiers from source systems to those assets.
3. Upload or otherwise ingest high-volume interval readings.
4. See ingestion progress and resolve readings from unknown meters.
5. Explore consumption, peak demand, cost, and emissions by time and business dimension.
6. Change a mapping and immediately see the dashboard recomputed under the new classification.

A representative mature query answers:

> Show electricity consumption, peak demand, estimated cost, and target variance for the last 90
> days, grouped by region and building type and normalized by floor area and weather.

Measurements come from Parquet. Their asset identity, grouping, targets, and pricing come from live
PostgreSQL data.

## Architectural shape

```text
Transactional PostgreSQL             Parquet object storage
------------------------             ----------------------
Meters and mappings         ───┐      Interval readings
Sites and buildings         ───┤      Equipment telemetry
Groups and regions          ───┼────  Weather observations
Tariffs and targets         ───┤      Emissions factors
Dashboard definitions       ───┘      Derived measurements
                              │
                              ▼
                     DuckDB / MotherDuck
                              │
                              ▼
                    Interactive dashboard
```

Measurements retain source-system meter identifiers rather than PostgreSQL foreign keys.
Tenant-owned mappings connect those identifiers to application meters at query time. Unknown meters
remain visible, and mapping changes can reclassify historical readings without rewriting Parquet.

## Product principles

- Tenant identity is present in authorization, transactional rows, object paths, Parquet rows, and
  every federated join. A path partition alone is not a tenant-isolation boundary.
- PostgreSQL is authoritative for user-managed domain objects; Parquet is authoritative for
  ingested measurements.
- Dashboards execute against current transactional state and do not depend on CDC becoming current.
- Ingestion is asynchronous, observable, repeatable, and safe to retry without duplicating facts.
- Queries prune by tenant and time before reading Parquet and project only required columns.
- Timestamps have an unambiguous instant and meters have explicit units. Local time zones are
  presentation and calendar-grouping concerns.
- Local development works without internet access or cloud credentials. It uses embedded DuckDB
  and a filesystem-backed object store through the same application interfaces as deployed
  analytical and object-storage services.
- The first implementation uses current meter mappings: changing a mapping reinterprets history.
  Effective-dated mappings can be introduced when reporting must preserve historical ownership.

## Existing foundation and new capabilities

The repository already provides the application shell this product should extend:

- a layered Go backend with protobuf/gRPC APIs and an HTTP/JSON grpc-gateway;
- generated customer and internal OpenAPI specifications with Swagger UI;
- PostgreSQL migrations, typed pgx queries, cross-store transactions, audit logging, and ETags;
- organizations, projects, magic-link authentication, and project-, organization-, and
  system-scoped RBAC;
- DBOS workflows and request-scoped idempotency keys for durable asynchronous operations;
- OpenTelemetry logs, metrics, and traces with a local Grafana stack;
- unit tests with generated `moq` mocks and isolated PostgreSQL integration tests.

No frontend is present yet. Object storage, Parquet writing, DuckDB or MotherDuck, file-upload
support, and energy-specific domain packages also do not exist. They are additions to this
foundation, not assumed capabilities. The backend implementation should follow the existing `mdl`
→ core → pgstore and protobuf → validation → conversion → handler boundaries rather than
introducing a separate architecture for analytics.

Object storage is an injected application boundary, not a cloud SDK used throughout the domain. A
local implementation stores objects under a configured directory; deployed implementations can
use GCS or an S3-compatible service without changing ingestion or query orchestration. The interface
must cover the shared semantics the application needs—scoped object references, reading, atomic
publication, listing or discovery where required, and deletion—not provider-specific concepts.
Upload URL generation may be a separate capability because a local upload endpoint and a cloud
signed URL use different transport mechanisms.

## Extension path

Once the core pattern is established, the domain can grow without changing its architectural
boundary:

- gas, water, solar generation, batteries, and exported energy;
- campuses, regions, building types, tenants, and cost centers;
- tariffs, estimated cost, budgets, and targets;
- emissions factors and carbon reporting;
- peak-demand alerts, anomaly detection, and data-quality monitoring;
- weather normalization and energy-use intensity by floor area;
- equipment telemetry and submeter analysis;
- saved dashboards, custom metrics, and scheduled reports;
- automated ingestion from utilities and building-management systems;
- effective-dated mappings for historically accurate classification.

The initial product slice is defined separately in [first-slice/README.md](first-slice/README.md).
