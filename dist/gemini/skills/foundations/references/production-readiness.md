# Production Readiness

Pre-deployment checklist for production systems. Use this before any production release.

## Security

| Check | Status |
|-------|--------|
| Secrets in vault/secrets manager, not env vars or code | ☐ |
| No hardcoded credentials in repository | ☐ |
| Dependencies scanned for vulnerabilities | ☐ |
| Input validation on all external data | ☐ |
| Authentication/authorization implemented | ☐ |
| HTTPS enforced, secure headers set | ☐ |

## Observability

| Check | Status |
|-------|--------|
| Structured logging configured | ☐ |
| Health endpoints (/health, /ready) responding | ☐ |
| Metrics exposed for scraping | ☐ |
| Distributed tracing configured | ☐ |
| Alerts configured for key metrics | ☐ |
| Dashboards created for service | ☐ |

## Reliability

| Check | Status |
|-------|--------|
| Graceful shutdown implemented | ☐ |
| Circuit breakers for external dependencies | ☐ |
| Retry logic with exponential backoff | ☐ |
| Timeouts configured for all external calls | ☐ |
| Rate limiting in place | ☐ |
| Database connection pooling configured | ☐ |

## Testing

| Check | Status |
|-------|--------|
| Unit test coverage meets threshold | ☐ |
| Integration tests passing | ☐ |
| Load tested at expected scale | ☐ |
| Failure scenarios tested | ☐ |
| Rollback tested | ☐ |

## Operations

| Check | Status |
|-------|--------|
| Runbooks documented | ☐ |
| On-call rotation defined | ☐ |
| Rollback plan documented | ☐ |
| Database migrations tested | ☐ |
| Feature flags for risky changes | ☐ |
| Deployment pipeline tested | ☐ |

## Data

| Check | Status |
|-------|--------|
| Backup strategy implemented | ☐ |
| Data retention policy defined | ☐ |
| PII handling compliant | ☐ |
| Audit logging for sensitive operations | ☐ |

## Performance

| Check | Status |
|-------|--------|
| Response time SLOs defined | ☐ |
| Resource limits configured (CPU, memory) | ☐ |
| Caching strategy implemented | ☐ |
| Database queries optimized | ☐ |

## Critical Rules

| Always | Never |
|--------|-------|
| Complete this checklist before production deploy | Skip items for "quick fixes" |
| Get sign-off from relevant team members | Deploy without rollback plan |
| Test rollback procedure | Assume it works because it worked in staging |
| Monitor closely after deploy | Deploy on Friday afternoon |
