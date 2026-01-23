# Security Patterns

Security mindset, threat modeling, and compliance patterns.

## Security Mindset

For every feature, ask:

1. **How could this be exploited?**
2. **What happens if input is malicious?**
3. **What if authenticated but not authorized?**
4. **What if the system is partially compromised?**

## STRIDE Threat Model

| Threat | Definition | Example |
|--------|------------|---------|
| **S**poofing | Impersonating something/someone | Stolen API key |
| **T**ampering | Modifying data or code | SQL injection |
| **R**epudiation | Denying actions taken | Missing audit logs |
| **I**nfo Disclosure | Exposing information | Error stack traces |
| **D**enial of Service | Disrupting service | Resource exhaustion |
| **E**levation of Privilege | Gaining unauthorized access | Broken access control |

## Input Validation

Validate all inputs at trust boundaries:

```python
from pydantic import BaseModel, Field, field_validator

class MeasurementInput(BaseModel):
    tower_id: UUID
    value: float = Field(ge=-1000, le=10000)
    timestamp: datetime

    @field_validator("timestamp")
    @classmethod
    def timestamp_not_future(cls, v):
        if v > datetime.now(timezone.utc):
            raise ValueError("Timestamp cannot be in future")
        return v
```

## Error Handling

Never expose internal details:

```python
# Good: Generic error to user
return {"error": "Invalid request"}

# Bad: Internal details exposed
return {"error": f"SQL error: {e}"}
```

## Secret Management

### Principles

1. **Never in code** - Secrets don't belong in source
2. **Never in logs** - Redact secrets from logging
3. **Least privilege** - Only access what you need
4. **Rotation** - Rotatable without downtime
5. **Encryption** - At rest and in transit

### Environment Variables

```python
from pydantic_settings import BaseSettings

class Settings(BaseSettings):
    database_url: str
    api_key: str
    debug: bool = False

    class Config:
        env_file = ".env"
```

### Log Redaction

```python
import structlog

def redact_secrets(_, __, event_dict):
    sensitive_keys = {"password", "secret", "token", "key"}
    for key in list(event_dict.keys()):
        if any(s in key.lower() for s in sensitive_keys):
            event_dict[key] = "[REDACTED]"
    return event_dict
```

### Never Log

- Passwords and credentials
- API keys and tokens
- Personal identifiable information (PII)
- Session tokens
- Encryption keys

## Common Vulnerabilities

### SQL Injection

```python
# Vulnerable
query = f"SELECT * FROM users WHERE id = '{user_id}'"

# Safe: Parameterized
query = "SELECT * FROM users WHERE id = :id"
result = session.execute(text(query), {"id": user_id})
```

### Broken Access Control

```python
# Vulnerable: Missing authorization
@app.get("/projects/{project_id}")
async def get_project(project_id: UUID):
    return await Project.get(project_id)

# Safe: With authorization
@app.get("/projects/{project_id}")
async def get_project(project_id: UUID, user: User = Depends(get_current_user)):
    project = await Project.get(project_id)
    if project.owner_id != user.id:
        raise HTTPException(403, "Access denied")
    return project
```

## OWASP Top 10 Checklist

### A01: Broken Access Control
- [ ] Deny by default
- [ ] Record access control failures
- [ ] Rate limit API access
- [ ] JWT tokens short-lived

### A02: Cryptographic Failures
- [ ] TLS 1.2+ for all connections
- [ ] Encryption for sensitive data at rest
- [ ] No sensitive data in URLs

### A03: Injection
- [ ] Parameterized queries
- [ ] Input validation on all fields
- [ ] ORM for database access

### A05: Security Misconfiguration
- [ ] Unnecessary features disabled
- [ ] Default credentials changed
- [ ] Error handling doesn't expose details
- [ ] Security headers configured

### A09: Logging & Monitoring
- [ ] Login failures logged
- [ ] Access control failures logged
- [ ] Logs protected from tampering
- [ ] Logs don't contain sensitive data

## Security Headers

```python
@app.middleware("http")
async def add_security_headers(request, call_next):
    response = await call_next(request)
    response.headers["X-Content-Type-Options"] = "nosniff"
    response.headers["X-Frame-Options"] = "DENY"
    response.headers["X-XSS-Protection"] = "1; mode=block"
    response.headers["Strict-Transport-Security"] = "max-age=31536000"
    response.headers["Content-Security-Policy"] = "default-src 'self'"
    return response
```

## Container Security

| Check | Requirement |
|-------|-------------|
| Non-root user | UID 1000 |
| Read-only root filesystem | Yes |
| Capabilities dropped | Yes |
| `allowPrivilegeEscalation` | `false` |
| Image vulnerability scanning | Required |
| Minimal base images | Updated |

## Critical Rules

### Always

- Validate all input at trust boundaries
- Log security events (without secrets)
- Fail securely (deny by default)
- Encrypt in transit (TLS) and at rest
- Use parameterized queries
- Assume breach - defense in depth

### Never

- Trust external input without validation
- Log secrets, credentials, or PII
- Use default credentials
- Expose detailed errors to users
- Store secrets in code or version control
- Skip security scanning
