# CI/CD Troubleshooting

Debugging CI failures, version conflicts, environment differences, and cache issues.

## Triage Decision Tree

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
# Check local versions
python --version
pip freeze | grep -E "(pytest|mypy|black|ruff)"
node --version && npm --version

# Compare lockfile changes
git diff origin/main -- package-lock.json requirements*.txt
```

## Common Failure Categories

| Symptom | Likely Cause | First Check |
|---------|--------------|-------------|
| Tests pass locally, fail in CI | Environment difference | Python/Node version, env vars |
| Linter passes locally, fails in CI | Version mismatch | Lock file, tool version |
| Random failures | Flaky test or race condition | Run tests multiple times |
| Timeout | Resource limits or hanging test | CI runner specs |
| Permission denied | File permissions or docker | Check container user |

## Version Conflicts

### Diagnosis

```yaml
# Add to workflow for debugging
- name: Show versions
  run: |
    python --version
    pip freeze
    node --version
    npm list
```

### Common Issues

**mypy/black/ruff version mismatch:**
```toml
# Pin exact versions in pyproject.toml
[tool.poetry.dev-dependencies]
mypy = "1.8.0"
black = "24.1.1"
ruff = "0.1.14"
```

**ESLint v8 to v9:** Different config format (flat config)

### Lockfile Best Practices

```bash
# Python: Use exact versions
pip-compile --generate-hashes requirements.in
pip install -r requirements.txt --no-deps

# Node: Use lockfile exactly
npm ci  # Not npm install
```

## Environment Differences

### Common Missing Variables

| Variable | Local | CI | Impact |
|----------|-------|-----|--------|
| `TZ` | Local timezone | UTC | Time-dependent tests |
| `PYTHONPATH` | Project root | Not set | Import errors |
| `HOME` | Your home | `/home/runner` | Config file paths |

### Fix: Set Consistent Environment

```yaml
env:
  TZ: UTC
  PYTHONPATH: ${{ github.workspace }}
  PYTHONUNBUFFERED: 1
  NODE_ENV: test
```

### OS Differences (macOS vs Linux)

| Aspect | macOS | Linux CI |
|--------|-------|----------|
| File system | Case-insensitive | Case-sensitive |
| Temp dir | /var/folders/... | /tmp |
| Home dir | /Users/you | /home/runner |

```python
# This works on macOS, fails on Linux:
from MyModule import MyClass  # File is mymodule.py
```

## Cache Issues

### Common Problems

**Stale cache:**
```yaml
# Force invalidation with version suffix
key: ${{ runner.os }}-pip-${{ hashFiles('**/requirements.txt') }}-v2
```

**Cache miss when expected hit:**
- Hash includes files that changed
- Branch scope prevents matching
- Cache evicted (7 day TTL)

**Wrong dependencies despite cache hit:**
```yaml
# Include all dependency files in hash
key: ${{ runner.os }}-pip-${{ hashFiles('**/requirements*.txt', '**/pyproject.toml') }}
```

### Debug Cache

```yaml
- uses: actions/cache@v4
  id: cache
  with:
    path: ~/.cache/pip
    key: ${{ runner.os }}-pip-${{ hashFiles('**/requirements.txt') }}

- run: echo "Cache hit: ${{ steps.cache.outputs.cache-hit }}"

- name: Check cache contents
  if: steps.cache.outputs.cache-hit == 'true'
  run: ls -la ~/.cache/pip/ && du -sh ~/.cache/pip/
```

### Force Cache Invalidation

```bash
# Via API
gh api repos/{owner}/{repo}/actions/caches
gh api -X DELETE repos/{owner}/{repo}/actions/caches/{cache_id}
```

## GitHub Actions Debugging

### Add Debug Output

```yaml
- name: Debug Environment
  run: |
    echo "=== Environment ==="
    env | sort
    echo "=== Python ==="
    python --version && which python
    echo "=== Directory ==="
    pwd && ls -la
```

### Enable Debug Logging

Set repository secrets:
- `ACTIONS_RUNNER_DEBUG: true`
- `ACTIONS_STEP_DEBUG: true`

Or re-run with "Enable debug logging" checked.

### SSH Access for Debugging

```yaml
- name: Setup tmate session
  if: ${{ failure() }}
  uses: mxschmitt/action-tmate@v3
  with:
    limit-access-to-actor: true
```

## Resolution Checklist

- [ ] Python/Node version matches
- [ ] All dependencies at same version
- [ ] Environment variables set
- [ ] Working directory correct
- [ ] File permissions correct
- [ ] Services running (database, etc.)
- [ ] Cache not stale
- [ ] Tests run in same order
- [ ] No local uncommitted changes affecting test
