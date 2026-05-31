# infra

Local dev stack: Postgres + telemetry pipeline (app → Collector → Tempo/Loki/Mimir → Grafana). The LGTM observability backends — Loki (logs), Grafana, Tempo (traces), Mimir (metrics).

## Files

### `docker-compose.yml`
Brings up six services on the `theapp` bridge network:

| Service          | Image                                          | Host port      | Role                                  |
| ---------------- | ---------------------------------------------- | -------------- | ------------------------------------- |
| `postgres`       | `postgres:17`                                  | `5433→5432`    | App database (`theapp`, trust auth)   |
| `otel-collector` | `otel/opentelemetry-collector-contrib:0.91.0`  | `4317`, `4318` | Receives OTLP from the app            |
| `tempo`          | `grafana/tempo:2.3.1`                          | `3200`         | Trace storage                         |
| `loki`           | `grafana/loki:3.3.2`                           | `3100`         | Log storage                           |
| `mimir`          | `grafana/mimir:2.13.0`                         | `9009`         | Metric storage                        |
| `grafana`        | `grafana/grafana:10.2.3`                       | `3000`         | UI (admin/admin)                      |

Named volumes `tempo-data`, `loki-data`, `mimir-data`, `grafana-data` persist across restarts.

### `otel-collector-config.yml`
Collector pipeline. Receives OTLP (gRPC `:4317`, HTTP `:4318`), tags signals with `deployment.environment=local`, batches them, and exports traces to `tempo:4317`, logs to `loki:3100`, and metrics to `mimir:9009` (all OTLP native).

### `tempo.yml`
Tempo config. Accepts OTLP from the Collector, stores traces on the local filesystem (`/var/tempo`), and retains them for 1 hour.

### `mimir.yml`
Mimir config. Single-binary mode with multitenancy disabled, storing metrics on the local filesystem (`/data`). The single-instance settings (in-memory ring, replication factor 1) matter because Mimir otherwise assumes a cluster and blocks waiting for peers that never arrive.

### `grafana-datasources.yml`
Provisioned at startup. Wires Grafana to Tempo (default), Loki, and Mimir. Trace and log views cross-link by `trace_id`.

## Flow

```
                  ┌─OTLP traces──▶ tempo  (:4317)
app ──OTLP──▶ otel-collector ─┼─OTLP logs────▶ loki   (:3100)  ◀── grafana
              (:4317/:4318)   └─OTLP metrics─▶ mimir  (:9009)      (:3000)
```

App sends telemetry to `localhost:4317` (or `:4318`). Open Grafana at <http://localhost:3000> to query traces, logs, and metrics. At the sign-in prompt, log in with `admin`/`admin` (skip the password-change step).
