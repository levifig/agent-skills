# Docker Patterns

Best practices for building production-ready container images.

## Multi-Stage Builds

Separate build dependencies from runtime to minimize image size and attack surface.

```dockerfile
# Stage 1: Build
FROM python:3.12-slim AS builder
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential && rm -rf /var/lib/apt/lists/*
COPY requirements.txt .
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

# Stage 2: Runtime
FROM python:3.12-slim
RUN useradd --uid 1000 --create-home appuser
COPY --from=builder /install /usr/local
WORKDIR /app
COPY --chown=appuser:appuser . .
USER appuser
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1
ENTRYPOINT ["python", "-m", "app"]
```

## Layer Optimization

Order instructions from least to most frequently changing:

```dockerfile
# Good: Dependencies before code
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY . .

# Bad: Busts cache on every code change
COPY . .
RUN pip install -r requirements.txt
```

Combine RUN commands and clean up:

```dockerfile
# Good: Single RUN with cleanup
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/*
```

## Non-Root User

```dockerfile
# Create user with specific UID for K8s compatibility
RUN useradd --uid 1000 --create-home --shell /bin/bash appuser
COPY --chown=appuser:appuser . .
USER appuser
```

**Why UID 1000?** Matches Kubernetes `runAsUser: 1000`, avoids volume mount permissions issues.

## Health Checks

```dockerfile
# HTTP health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# Process health check
HEALTHCHECK --interval=30s --timeout=3s \
    CMD pgrep -x python || exit 1
```

| Parameter | Value | Purpose |
|-----------|-------|---------|
| `--interval` | 30s | Time between checks |
| `--timeout` | 3s | Max time for check |
| `--start-period` | 5s | Grace period for startup |
| `--retries` | 3 | Failures before unhealthy |

## Base Images

| Image | Size | Use Case |
|-------|------|----------|
| `scratch` | 0MB | Static Go binaries |
| `distroless` | ~2MB | Compiled languages |
| `alpine` | ~5MB | Need shell/packages |
| `*-slim` | ~50MB | Need glibc |

```dockerfile
# For static binaries
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/server /
USER nonroot:nonroot
ENTRYPOINT ["/server"]
```

## Secrets Management

Never bake secrets into images:

```dockerfile
# BuildKit secret mount (build-time only)
RUN --mount=type=secret,id=pip_conf,target=/etc/pip.conf \
    pip install --no-cache-dir -r requirements.txt

# Runtime: docker run -e DATABASE_PASSWORD="$SECRET" app
```

## Environment Variables

```dockerfile
# Build-time only (not in final image)
ARG BUILD_VERSION

# Runtime (available in container)
ENV APP_VERSION=${BUILD_VERSION}
ENV LOG_LEVEL=INFO
ENV PYTHONUNBUFFERED=1
```

## .dockerignore

```
.git
__pycache__
*.pyc
.pytest_cache
.mypy_cache
.venv
.env
*.local.*
Dockerfile*
docker-compose*
```

## Checklist

- [ ] Multi-stage build used
- [ ] Non-root user (UID 1000)
- [ ] Health check defined
- [ ] No secrets in image
- [ ] Specific version tags
- [ ] .dockerignore present
- [ ] Layer order optimized
- [ ] apt lists cleaned up
- [ ] PYTHONUNBUFFERED=1 set
