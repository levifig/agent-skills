---
name: python
description: >-
  Use for all Python 3.12+ development. Covers project setup with uv, FastAPI
  web services, Pydantic validation, async/await patterns, type hints with mypy,
  pytest testing, SQLAlchemy database operations, Polars data processing, httpx
  API clients, and Docker deployment. Activate when working with .py files,
  pyproject.toml, or Python frameworks.
---

# Python Development

Comprehensive guide for modern Python 3.12+ development with FastAPI ecosystem.

## When to Use This Skill

- Starting or configuring Python projects
- Building REST APIs with FastAPI
- Defining data models with Pydantic
- Implementing async/await patterns
- Adding type hints and mypy configuration
- Writing tests with pytest
- Working with databases (SQLAlchemy 2.0)
- Building data pipelines with Polars
- Integrating external APIs
- Deploying to production with Docker

## Stack Overview

| Layer | Default | Alternatives |
|-------|---------|--------------|
| Runtime | Python 3.12+ | - |
| Package Manager | uv | rye, poetry |
| Linter/Formatter | ruff | black + flake8 |
| Type Checker | mypy | pyright |
| Web Framework | FastAPI | Flask, Django |
| Validation | Pydantic v2 | - |
| ORM | SQLAlchemy 2.0 | - |
| Data Processing | Polars | Pandas |
| HTTP Client | httpx | aiohttp |
| Testing | pytest | - |
| Containerization | Docker | - |

## Core Philosophy

Follows [foundations principles](../foundations/SKILL.md). Python-specific emphasis:

- **Async by default** — non-blocking I/O for web services
- **Strict typing** — catch errors at development time with mypy
- **Pydantic everywhere** — validate at trust boundaries
- **12-factor methodology** — environment-based configuration

## Quick Reference

### Project Setup with uv

```bash
uv init my-project --python 3.12
uv add fastapi uvicorn pydantic-settings
uv add --dev pytest ruff mypy
uv run pytest
```

### pyproject.toml Essentials

```toml
[project]
name = "my-project"
version = "0.1.0"
requires-python = ">=3.12"

[tool.ruff]
target-version = "py312"
select = ["E", "F", "I", "N", "W", "UP"]

[tool.mypy]
python_version = "3.12"
strict = true
```

### FastAPI Endpoint

```python
from fastapi import FastAPI, Depends, HTTPException, status
from pydantic import BaseModel, EmailStr, Field

app = FastAPI()

class UserCreate(BaseModel):
    email: EmailStr
    username: str = Field(..., min_length=3, max_length=50)

class UserResponse(BaseModel):
    id: int
    email: EmailStr
    username: str
    model_config = {"from_attributes": True}

@app.post("/users", response_model=UserResponse, status_code=status.HTTP_201_CREATED)
async def create_user(user: UserCreate, db: AsyncSession = Depends(get_db)):
    db_user = User(**user.model_dump())
    db.add(db_user)
    await db.commit()
    await db.refresh(db_user)
    return db_user
```

### Pydantic Model with Validation

```python
from pydantic import BaseModel, Field, field_validator

class UserRegistration(BaseModel):
    email: EmailStr
    password: str = Field(..., min_length=8)

    @field_validator("password")
    @classmethod
    def password_strength(cls, v: str) -> str:
        if not any(c.isupper() for c in v):
            raise ValueError("Must contain uppercase")
        return v
```

### Async Database Query

```python
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

async def get_user(session: AsyncSession, user_id: int) -> User | None:
    result = await session.execute(select(User).where(User.id == user_id))
    return result.scalar_one_or_none()
```

### pytest Test

```python
@pytest.mark.asyncio
async def test_create_user(client: AsyncClient):
    response = await client.post("/users", json={"email": "test@example.com", "username": "test"})
    assert response.status_code == 201
    assert response.json()["email"] == "test@example.com"
```

## Topics

| Topic | Use For |
|-------|---------|
| [Core](reference/core.md) | Project setup, pyproject.toml, modern Python features |
| [FastAPI](reference/fastapi.md) | REST APIs, routing, dependency injection, middleware |
| [Pydantic](reference/pydantic.md) | Data models, validation, settings management |
| [Async](reference/async.md) | async/await, TaskGroup, context managers |
| [Types](reference/types.md) | Type hints, mypy, Protocol, generics |
| [Testing](reference/testing.md) | pytest, fixtures, mocking, async tests |
| [Database](reference/database.md) | SQLAlchemy 2.0, Alembic migrations, transactions |
| [Data](reference/data.md) | Polars, ETL pipelines, schema validation |
| [API Clients](reference/api.md) | httpx, retries, rate limiting, error handling |
| [Deployment](reference/deployment.md) | Docker, logging, OpenTelemetry, health checks |

## Critical Rules

### Always

- Use `async def` for I/O-bound operations
- Use Pydantic models for external input validation
- Use `pathlib.Path` for file operations
- Run mypy in CI/CD pipeline

### Never

- Block the event loop with sync I/O in async code
- Use mutable default arguments (`def foo(items=[])`)
- Skip validation on external input
- Hardcode configuration values (use pydantic-settings)
