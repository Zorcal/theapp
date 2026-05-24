# infra

Local dev stack: Postgres + tracing pipeline (app → Collector → Tempo → Grafana).

## Files

### `docker-compose.yml`
Brings up four services on the `theapp` bridge network:

| Service          | Image                                          | Host port      | Role                                  |
| ---------------- | ---------------------------------------------- | -------------- | ------------------------------------- |
| `postgres`       | `postgres:17`                                  | `5433→5432`    | App database (`theapp`, trust auth)   |
| `otel-collector` | `otel/opentelemetry-collector-contrib:0.91.0`  | `4317`, `4318` | Receives OTLP from the app            |
| `tempo`          | `grafana/tempo:2.3.1`                          | `3200`         | Trace storage                         |
| `grafana`        | `grafana/grafana:10.2.3`                       | `3000`         | UI (admin/admin)                      |

Named volumes `tempo-data`, `grafana-data` persist across restarts.

### `otel-collector-config.yml`
Collector pipeline. Receives OTLP (gRPC `:4317`, HTTP `:4318`), tags spans with `deployment.environment=local`, batches them, and exports to `tempo:4317`.

### `tempo.yml`
Tempo config. Accepts OTLP from the Collector, stores traces on the local filesystem (`/var/tempo`), and retains them for 1 hour.

### `grafana-datasources.yml`
Provisioned at startup. Wires Grafana to Tempo at `http://tempo:3200` as the default datasource, with service map enabled.

## Flow

```
app ──OTLP──▶ otel-collector ──OTLP──▶ tempo ◀── grafana
              (:4317/:4318)            (:4317)     (:3000)
```

App sends traces to `localhost:4317` (or `:4318`). Open Grafana at <http://localhost:3000> to query them.
