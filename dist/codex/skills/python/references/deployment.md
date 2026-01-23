# Python Production Deployment

Production-ready deployment patterns for Python applications.

## Dockerfile Best Practices

```dockerfile
FROM python:3.12-slim as builder
WORKDIR /build
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev --no-install-project

FROM python:3.12-slim
RUN useradd -m -u 1000 appuser
WORKDIR /app
COPY --from=builder /build/.venv /app/.venv
COPY --chown=appuser:appuser src/ /app/src/
ENV PATH="/app/.venv/bin:$PATH" PYTHONUNBUFFERED=1
USER appuser
HEALTHCHECK --interval=30s --timeout=3s CMD python -c "import httpx; httpx.get('http://localhost:8000/health')"
EXPOSE 8000
CMD ["uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

## Production Settings

```python
from pydantic_settings import BaseSettings, SettingsConfigDict
from functools import lru_cache

class Settings(BaseSettings):
    app_name: str = "My Application"
    environment: str = "production"
    debug: bool = False
    database_url: str
    secret_key: str
    otlp_endpoint: str = "http://localhost:4317"

    model_config = SettingsConfigDict(env_file=".env", case_sensitive=False)

@lru_cache
def get_settings() -> Settings:
    return Settings()
```

## Structured Logging

```python
import structlog

def configure_logging(log_level: str = "INFO"):
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.JSONRenderer()
        ],
        wrapper_class=structlog.make_filtering_bound_logger(log_level),
    )

logger = structlog.get_logger()

async def process_order(order_id: int):
    log = logger.bind(order_id=order_id)
    log.info("processing_order_started")
    try:
        result = await process(order_id)
        log.info("processing_order_completed", duration_ms=result.duration)
    except Exception as e:
        log.error("processing_order_failed", error=str(e), exc_info=True)
        raise
```

## OpenTelemetry

```python
from opentelemetry import trace, metrics
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

def configure_telemetry(service_name: str, otlp_endpoint: str):
    trace_provider = TracerProvider()
    trace_provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=otlp_endpoint)))
    trace.set_tracer_provider(trace_provider)

tracer = trace.get_tracer(__name__)

async def fetch_user(user_id: int):
    with tracer.start_as_current_span("fetch_user") as span:
        span.set_attribute("user.id", user_id)
        return await db.get(User, user_id)
```

## Health Checks

```python
from fastapi import FastAPI
from pydantic import BaseModel

class HealthResponse(BaseModel):
    status: str
    version: str
    checks: dict[str, str]

@app.get("/health")
async def health_check():
    return HealthResponse(status="healthy", version="1.0.0", checks={})

@app.get("/health/ready")
async def readiness_check(db: AsyncSession = Depends(get_db)):
    checks = {}
    try:
        await db.execute(text("SELECT 1"))
        checks["database"] = "healthy"
    except Exception as e:
        return HealthResponse(status="unhealthy", version="1.0.0", checks={"database": str(e)}), 503
    return HealthResponse(status="healthy", version="1.0.0", checks=checks)
```

## Graceful Shutdown

```python
from contextlib import asynccontextmanager

@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("application_starting")
    await database.connect()
    yield
    logger.info("application_shutting_down")
    await database.disconnect()

app = FastAPI(lifespan=lifespan)
```

## Gunicorn Config

```python
# gunicorn_config.py
import multiprocessing

bind = "0.0.0.0:8000"
workers = multiprocessing.cpu_count() * 2 + 1
worker_class = "uvicorn.workers.UvicornWorker"
timeout = 30
graceful_timeout = 10
```

## Critical Rules

### Always
- Use multi-stage Docker builds
- Run as non-root user
- Implement health checks
- Use structured logging
- Configure graceful shutdown

### Never
- Run as root in containers
- Hardcode secrets in images
- Skip health checks
- Use development server in production
