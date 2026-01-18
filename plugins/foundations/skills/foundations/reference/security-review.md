# Security Review Checklist

Security-focused code review checklist aligned with OWASP guidelines.

## Quick Security Check

For every PR:

- [ ] No hardcoded secrets, keys, or passwords
- [ ] No sensitive data in logs
- [ ] Input validation present at boundaries
- [ ] SQL/NoSQL queries are parameterized

## OWASP Top 10 Checklist

### A01: Broken Access Control

- [ ] Authorization checked on every endpoint
- [ ] Default deny - explicit allow
- [ ] CORS configured restrictively
- [ ] Directory traversal prevented (no user input in file paths)
- [ ] Rate limiting on sensitive operations
- [ ] JWT/session tokens validated server-side

```python
# BAD: No authorization check
@app.get("/users/{user_id}")
async def get_user(user_id: int):
    return await db.get_user(user_id)

# GOOD: Authorization verified
@app.get("/users/{user_id}")
async def get_user(user_id: int, current_user: User = Depends(get_current_user)):
    if current_user.id != user_id and not current_user.is_admin:
        raise HTTPException(status_code=403)
    return await db.get_user(user_id)
```

### A02: Cryptographic Failures

- [ ] Sensitive data encrypted at rest
- [ ] TLS for data in transit
- [ ] Strong password hashing (bcrypt, argon2)
- [ ] No deprecated crypto algorithms (MD5, SHA1 for security)
- [ ] Secrets in environment variables, not code

```python
# BAD: Weak hashing
import hashlib
password_hash = hashlib.md5(password.encode()).hexdigest()

# GOOD: Strong password hashing
import bcrypt
password_hash = bcrypt.hashpw(password.encode(), bcrypt.gensalt())
```

### A03: Injection

- [ ] SQL queries parameterized
- [ ] NoSQL queries use safe APIs
- [ ] OS commands avoid user input (use subprocess with list args)
- [ ] LDAP queries escaped
- [ ] XML parsing disables external entities

```python
# BAD: SQL injection
query = f"SELECT * FROM users WHERE id = {user_id}"

# GOOD: Parameterized query
from sqlalchemy import select
query = select(User).where(User.id == user_id)

# GOOD: Use subprocess with list args (never shell=True with user input)
import subprocess
subprocess.run(["ls", validated_path], check=True)
```

### A04: Insecure Design

- [ ] Threat modeling for new features
- [ ] Security requirements in acceptance criteria
- [ ] Defense in depth (multiple layers)
- [ ] Fail securely (deny by default)

### A05: Security Misconfiguration

- [ ] Debug mode disabled in production
- [ ] Default credentials changed
- [ ] Error messages don't leak info
- [ ] Security headers configured
- [ ] Unnecessary features disabled

```python
# BAD: Debug mode in production
app = FastAPI(debug=True)

# GOOD: Environment-based config
app = FastAPI(debug=settings.DEBUG and not settings.IS_PRODUCTION)

# BAD: Detailed error to client
raise HTTPException(detail=f"Database error: {e.args}")

# GOOD: Generic error, log details
logger.error(f"Database error: {e}")
raise HTTPException(detail="An error occurred")
```

### A06: Vulnerable Components

- [ ] Dependencies pinned to specific versions
- [ ] Dependency scanning in CI
- [ ] No known vulnerable versions
- [ ] Regular dependency updates

### A07: Authentication Failures

- [ ] Strong password requirements
- [ ] Account lockout after failures
- [ ] Secure session management
- [ ] Multi-factor authentication available
- [ ] Password reset secure

```python
# Password requirements
MIN_PASSWORD_LENGTH = 12
REQUIRE_UPPERCASE = True
REQUIRE_LOWERCASE = True
REQUIRE_DIGIT = True
REQUIRE_SPECIAL = True

# Account lockout
MAX_LOGIN_ATTEMPTS = 5
LOCKOUT_DURATION_MINUTES = 15
```

### A08: Software and Data Integrity

- [ ] CI/CD pipeline secured
- [ ] Dependencies from trusted sources
- [ ] Code signing where applicable
- [ ] Integrity checks for critical data

### A09: Logging and Monitoring

- [ ] Security events logged
- [ ] Logs don't contain sensitive data
- [ ] Log injection prevented
- [ ] Alerting for suspicious activity

```python
# BAD: Sensitive data in logs
logger.info(f"User login: {email}, password: {password}")

# GOOD: No sensitive data
logger.info(f"User login: {email}")

# BAD: Log injection possible
logger.info(f"User action: {user_input}")

# GOOD: Sanitize user input in logs
logger.info("User action", extra={"action": sanitize(user_input)})
```

### A10: Server-Side Request Forgery (SSRF)

- [ ] URLs validated before fetching
- [ ] Internal network access blocked
- [ ] Allowlist for external services
- [ ] No user-controlled redirects

```python
# BAD: SSRF vulnerability
@app.post("/fetch")
async def fetch_url(url: str):
    return await httpx.get(url)  # Can access internal services

# GOOD: URL validation
from urllib.parse import urlparse

ALLOWED_HOSTS = {"api.example.com", "cdn.example.com"}

@app.post("/fetch")
async def fetch_url(url: str):
    parsed = urlparse(url)
    if parsed.hostname not in ALLOWED_HOSTS:
        raise HTTPException(status_code=400, detail="Host not allowed")
    return await httpx.get(url)
```

## API Security

- [ ] Authentication on all non-public endpoints
- [ ] Rate limiting configured
- [ ] Request size limits
- [ ] Content-Type validation
- [ ] CORS restrictive by default

## Frontend Security

- [ ] XSS prevention (output encoding)
- [ ] CSP headers configured
- [ ] No inline scripts/styles (CSP)
- [ ] Cookies: HttpOnly, Secure, SameSite
- [ ] No sensitive data in localStorage

## Database Security

- [ ] Principle of least privilege
- [ ] Connections encrypted
- [ ] No raw SQL with user input
- [ ] Sensitive columns encrypted

## Secrets Management

- [ ] No secrets in code
- [ ] No secrets in logs
- [ ] Secrets rotated regularly
- [ ] Access to secrets audited

---

*Reference: [OWASP Top 10](https://owasp.org/Top10/), [OWASP Cheat Sheets](https://cheatsheetseries.owasp.org/)*
