# infra

Local dev stack: Postgres + telemetry pipeline (app → Collector → Tempo/Loki → Grafana).

## Files

### `docker-compose.yml`
Brings up five services on the `theapp` bridge network:

| Service          | Image                                          | Host port      | Role                                  |
| ---------------- | ---------------------------------------------- | -------------- | ------------------------------------- |
| `postgres`       | `postgres:17`                                  | `5433→5432`    | App database (`theapp`, trust auth)   |
| `otel-collector` | `otel/opentelemetry-collector-contrib:0.91.0`  | `4317`, `4318` | Receives OTLP from the app            |
| `tempo`          | `grafana/tempo:2.3.1`                          | `3200`         | Trace storage                         |
| `loki`           | `grafana/loki:3.3.2`                           | `3100`         | Log storage                           |
| `grafana`        | `grafana/grafana:10.2.3`                       | `3000`         | UI (admin/admin)                      |

Named volumes `tempo-data`, `loki-data`, `grafana-data` persist across restarts.

### `otel-collector-config.yml`
Collector pipeline. Receives OTLP (gRPC `:4317`, HTTP `:4318`), tags signals with `deployment.environment=local`, batches them, and exports traces to `tempo:4317` and logs to `loki:3100` (OTLP native).

### `tempo.yml`
Tempo config. Accepts OTLP from the Collector, stores traces on the local filesystem (`/var/tempo`), and retains them for 1 hour.

### `grafana-datasources.yml`
Provisioned at startup. Wires Grafana to Tempo (default) and Loki. Trace and log views cross-link by `trace_id`.

## Flow

```
                  ┌─OTLP traces─▶ tempo (:4317)
app ──OTLP──▶ otel-collector ─┤                      ◀── grafana
              (:4317/:4318)   └─OTLP logs───▶ loki (:3100)   (:3000)
```

App sends telemetry to `localhost:4317` (or `:4318`). Open Grafana at <http://localhost:3000> to query traces and logs. At the sign-in prompt, log in with `admin`/`admin` (skip the password-change step).
