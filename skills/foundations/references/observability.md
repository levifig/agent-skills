# Observability

Universal observability principles for production systems. For language-specific tooling, see language skills.

## Philosophy

**Observability enables understanding.** Systems should emit enough telemetry to diagnose issues without deploying new code. The three pillars—logs, metrics, traces—each serve distinct purposes.

## Logging

See [code-style.md](./code-style.md) for structured logging principles.

### Log Levels

| Level | Use For |
|-------|---------|
| ERROR | Actionable failures requiring attention |
| WARN | Degraded state, potential issues |
| INFO | Business events, state transitions |
| DEBUG | Development troubleshooting (off in prod) |

### Logging Guidelines

| Always | Never |
|--------|-------|
| Use structured JSON in production | Log sensitive data (PII, credentials, tokens) |
| Include correlation/request IDs | Use string interpolation for log messages |
| Log at service boundaries | Log inside tight loops |
| Include relevant context (user_id, resource_id) | Log entire request/response bodies |
| Rate-limit repetitive log messages | Use DEBUG level in production |

## Metrics

### Metric Types

| Type | Use For | Example |
|------|---------|---------|
| Counter | Cumulative totals | requests_total, errors_total |
| Gauge | Current value | active_connections, queue_depth |
| Histogram | Distributions | request_duration_seconds |

### Key Metrics (RED/USE)

**RED Method** (request-driven services):
- **Rate**: Requests per second
- **Errors**: Error rate/count
- **Duration**: Latency percentiles (p50, p95, p99)

**USE Method** (resources):
- **Utilization**: Resource usage percentage
- **Saturation**: Queue depth, backpressure indicators
- **Errors**: Resource-level failures

### Metrics Guidelines

| Always | Never |
|--------|-------|
| Use consistent naming conventions | Create high-cardinality labels |
| Include units in metric names (_seconds, _bytes) | Use unbounded label values (user IDs, URLs) |
| Expose p50, p95, p99 for latencies | Expose only averages for latency |
| Pre-aggregate where possible | Create metrics inside hot paths without caching |
| Document metric semantics | Change metric semantics without versioning |

## Distributed Tracing

### Core Concepts

- **Trace**: End-to-end request journey across services
- **Span**: Single operation within a trace
- **Context propagation**: Passing trace IDs across boundaries

### Span Naming

Use `<component>.<operation>` format:
- `http.request`, `db.query`, `cache.get`
- `service.method_name` for internal operations

### When to Create Spans

- External service calls (HTTP, gRPC, database)
- Significant internal operations (>10ms typical)
- Queue publish/consume operations
- Cache operations

### Tracing Guidelines

| Always | Never |
|--------|-------|
| Propagate context across service boundaries | Create spans for trivial operations |
| Include error details in span status | Log sensitive data in span attributes |
| Use semantic conventions (OpenTelemetry) | Create unbounded span attributes |
| Sample appropriately (1-10% in high-traffic) | Disable tracing entirely in production |
| Record span events for debugging | Use tracing as a replacement for logging |

## Health Endpoints

| Endpoint | Purpose | Response |
|----------|---------|----------|
| /health | Liveness probe | 200 if process running |
| /ready | Readiness probe | 200 if can serve traffic |
| /metrics | Prometheus scrape | Metrics in exposition format |

### Health Check Design

- **Liveness**: Minimal check—process is alive, not deadlocked
- **Readiness**: Dependencies available, can handle requests
- Include dependency health in readiness (database, cache, critical services)
- Return structured JSON with component status

## Alerting Principles

### Alert Design

- Alert on symptoms, not causes
- Every alert must be actionable
- Include runbook links in alert messages
- Use severity levels consistently

### Alerting Guidelines

| Always | Never |
|--------|-------|
| Alert on user-facing symptoms | Alert on every error |
| Include runbook/playbook links | Create alerts without owners |
| Set appropriate severity levels | Page for non-urgent issues |
| Test alert routing regularly | Alert on metrics you won't investigate |
| Review and tune alert thresholds | Ignore alert fatigue—fix the root cause |

### Severity Levels

| Severity | Response | Example |
|----------|----------|---------|
| Critical | Immediate page | Service down, data loss risk |
| High | Response within 1 hour | Significant degradation |
| Medium | Response within 1 day | Elevated errors, approaching limits |
| Low | Review in next sprint | Minor anomalies, optimization opportunities |

## Implementation

See language-specific skills for tooling:
- `python` skill: structlog, prometheus_client, opentelemetry-python
- `typescript` skill: pino, prom-client, @opentelemetry/*
- `rails` skill: Rails instrumentation, ActiveSupport::Notifications
