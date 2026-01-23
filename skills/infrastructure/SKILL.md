---
name: infrastructure
description: >-
  Use for DevOps and infrastructure work. Covers Docker multi-stage builds,
  Kubernetes manifests and Helm charts, GitOps with ArgoCD/Flux, GitHub Actions
  CI/CD pipelines, and container security. Activate when working with
  Dockerfile, k8s manifests, helm charts, CI workflows, or deployment
  configuration.
---

# Infrastructure

Infrastructure patterns for containerization, orchestration, CI/CD pipelines, and deployment automation.

## Stack Overview

| Layer | Technologies |
|-------|--------------|
| Containers | Docker, BuildKit, multi-stage builds |
| Orchestration | Kubernetes, Helm, Kustomize |
| GitOps | ArgoCD, Flux, Argo Rollouts |
| CI/CD | GitHub Actions, GitLab CI |
| Registries | GHCR, ECR, GCR, DockerHub |

## Philosophy

1. **Infrastructure as Code** - All configuration in version control
2. **GitOps** - Git as the single source of truth for deployments
3. **Security by Default** - Non-root, minimal images, no secrets in code
4. **Observability** - Health checks, probes, structured logging
5. **Reproducibility** - Pinned versions, lockfiles, deterministic builds

## Quick Reference

### Docker Essentials

```dockerfile
# Multi-stage build with non-root user
FROM python:3.12-slim AS builder
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

FROM python:3.12-slim
RUN useradd --uid 1000 --create-home appuser
COPY --from=builder /install /usr/local
COPY --chown=appuser:appuser . .
USER appuser
HEALTHCHECK --interval=30s --timeout=3s CMD curl -f http://localhost:8000/health || exit 1
ENTRYPOINT ["python", "-m", "app"]
```

### Kubernetes Pod Spec

```yaml
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
  containers:
    - name: app
      image: app:v1.0.0  # Never :latest
      resources:
        requests: {memory: "256Mi", cpu: "100m"}
        limits: {memory: "512Mi", cpu: "500m"}
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
      livenessProbe:
        httpGet: {path: /health, port: 8000}
        initialDelaySeconds: 10
      readinessProbe:
        httpGet: {path: /ready, port: 8000}
        initialDelaySeconds: 5
```

### GitHub Actions Cache

```yaml
- uses: actions/cache@v4
  with:
    path: ~/.cache/pip
    key: ${{ runner.os }}-pip-${{ hashFiles('**/requirements.txt') }}
    restore-keys: |
      ${{ runner.os }}-pip-
```

## Topics

| Topic | Reference File | Description |
|-------|----------------|-------------|
| Docker | `reference/docker.md` | Multi-stage builds, layers, security, health checks |
| Kubernetes | `reference/kubernetes.md` | Deployments, services, probes, resources, secrets |
| GitOps | `reference/gitops.md` | ArgoCD, Kustomize, sync policies, rollout strategies |
| CI/CD | `reference/ci-cd.md` | GitHub Actions workflows, caching, secrets, matrix builds |
| Troubleshooting | `reference/troubleshooting.md` | CI failures, version conflicts, cache issues |

## Available Scripts

| Script | Usage | Description |
|--------|-------|-------------|
| `scripts/check-dockerfile.sh` | `check-dockerfile.sh <file>` | Validate Dockerfile best practices |
| `scripts/validate-k8s-manifest.py` | `validate-k8s-manifest.py <file>` | Check K8s manifest for required fields |

## Critical Rules

### Always

- Use multi-stage builds to minimize image size
- Run containers as non-root user (UID 1000)
- Include health checks in all services
- Pin specific image versions (no `:latest`)
- Set resource requests AND limits
- Use `npm ci` / `pip-sync` in CI (not install)
- Commit lockfiles to version control

### Never

- Commit secrets to version control
- Use `:latest` tags in production
- Skip security scanning in CI
- Deploy without rollback capability
- Store state in containers
- Run as root in production

## CI Failure Triage

```
CI Failed
+-- Same code passes locally?
|   +-- YES --> Check environment differences
|   |   +-- Python/Node version
|   |   +-- Environment variables
|   |   +-- File permissions
|   |   +-- Installed dependencies
|   +-- NO --> Fix the actual bug
+-- Flaky (sometimes passes)?
|   +-- Check for race conditions, shared state, timeouts
+-- Always fails in CI?
    +-- Check runner resources (memory, timeout)
    +-- Check external service access
    +-- Check CI-specific config
```

## Quick Diagnostics

```bash
# Check local vs CI Python version
python --version

# Check installed package versions
pip freeze | grep -E "(pytest|mypy|black|ruff)"

# Check Node/npm versions
node --version && npm --version

# Compare lockfile changes
git diff origin/main -- package-lock.json requirements*.txt
```
