# CI/CD Patterns

Best practices for continuous integration and deployment pipelines.

## GitHub Actions Workflow

```yaml
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"
      - run: pip install ruff mypy
      - run: ruff check . && ruff format --check .
      - run: mypy .

  test:
    runs-on: ubuntu-latest
    needs: lint
    services:
      postgres:
        image: postgres:17
        env:
          POSTGRES_PASSWORD: test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
        ports:
          - 5432:5432
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"
          cache: "pip"
      - run: pip install -e ".[test]"
      - run: pytest --cov --cov-report=xml
        env:
          DATABASE_URL: postgresql://postgres:test@localhost:5432/test

  build:
    runs-on: ubuntu-latest
    needs: [test]
    if: github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v5
        with:
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

## Pipeline Stages

```
Lint --> Test --> Security --> Build --> Deploy
```

| Stage | Purpose | Fail Fast |
|-------|---------|-----------|
| Lint | Code style, formatting | Yes |
| Test | Unit, integration tests | Yes |
| Security | Vulnerability scanning | Yes |
| Build | Container image creation | No (main only) |
| Deploy | Environment deployment | No (manual gate) |

## Caching Strategies

### Python Dependencies

```yaml
- uses: actions/setup-python@v5
  with:
    python-version: "3.12"
    cache: "pip"
    cache-dependency-path: |
      requirements.txt
      requirements-dev.txt
```

### Docker Layer Caching

```yaml
- uses: docker/build-push-action@v5
  with:
    cache-from: type=gha
    cache-to: type=gha,mode=max
```

### Custom Caching

```yaml
- uses: actions/cache@v4
  with:
    path: ~/.cache/pre-commit
    key: pre-commit-${{ hashFiles('.pre-commit-config.yaml') }}
```

## Secrets Management

```yaml
# Repository/environment secrets
env:
  DATABASE_URL: ${{ secrets.DATABASE_URL }}

# Environment-specific
deploy:
  environment: production  # Uses production secrets
```

| Type | Use Case |
|------|----------|
| Repository secrets | Shared across workflows |
| Environment secrets | Per-environment (staging/prod) |
| Organization secrets | Shared across repos |

## Matrix Builds

```yaml
test:
  strategy:
    fail-fast: false  # Run all combinations
    matrix:
      python-version: ["3.11", "3.12"]
      os: [ubuntu-latest, macos-latest]
  runs-on: ${{ matrix.os }}
  steps:
    - uses: actions/setup-python@v5
      with:
        python-version: ${{ matrix.python-version }}
```

## GitOps Deployment

```yaml
deploy:
  needs: build
  steps:
    - uses: actions/checkout@v4
      with:
        repository: org/gitops
        token: ${{ secrets.GITOPS_TOKEN }}
    - run: |
        yq -i '.spec.template.spec.containers[0].image = "app:${{ github.sha }}"' \
          apps/production/deployment.yaml
    - run: |
        git config user.name "GitHub Actions"
        git config user.email "actions@github.com"
        git commit -am "Deploy ${{ github.sha }}"
        git push
```

## Reusable Workflows

```yaml
# Call
jobs:
  call-workflow:
    uses: org/shared-workflows/.github/workflows/python-ci.yml@main
    with:
      python-version: "3.12"
    secrets: inherit

# Define
on:
  workflow_call:
    inputs:
      python-version:
        required: true
        type: string
```

## Debugging

```yaml
- name: Debug Environment
  run: |
    echo "Python: $(python --version)"
    echo "Working dir: $(pwd)"
    env | sort

# Enable debug logging
# Set repository secrets:
# ACTIONS_RUNNER_DEBUG: true
# ACTIONS_STEP_DEBUG: true
```

## Checklist

- [ ] Lint and type check before tests
- [ ] Security scanning in pipeline
- [ ] Tests run with real database
- [ ] Coverage reports uploaded
- [ ] Docker caching enabled
- [ ] Secrets not in workflow files
- [ ] Concurrency control
- [ ] Environment gates for production
