# Python Testing with pytest

Follows [foundations testing principles](../../foundations/reference/code-style.md#test-patterns).

## Test Organization

```
tests/
├── conftest.py           # Shared fixtures
├── unit/
│   ├── test_models.py
│   └── test_services.py
├── integration/
│   └── test_api.py
└── e2e/
    └── test_workflows.py
```

## Basic Test Structure

```python
import pytest
from my_app.models import User

def test_user_creation():
    # Arrange
    email = "test@example.com"
    username = "testuser"

    # Act
    user = User(email=email, username=username)

    # Assert
    assert user.email == email
    assert user.username == username

def test_user_validation_fails():
    with pytest.raises(ValueError, match="Invalid email"):
        User(email="not_an_email", username="test")
```

## Fixtures

```python
@pytest.fixture
def user_data() -> dict:
    return {"email": "test@example.com", "username": "testuser", "password": "pass123"}

@pytest.fixture
async def db_session() -> AsyncSession:
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    async with async_session_maker() as session:
        yield session
        await session.rollback()

@pytest.fixture(scope="session")
def config():
    return load_test_config()
```

## Parametrized Tests

```python
@pytest.mark.parametrize("email,expected_valid", [
    ("user@example.com", True),
    ("invalid.email", False),
    ("@example.com", False),
])
def test_email_validation(email: str, expected_valid: bool):
    is_valid = validate_email(email)
    assert is_valid == expected_valid
```

## Mocking

```python
from unittest.mock import AsyncMock, patch

@pytest.fixture
def mock_email_service():
    mock = AsyncMock()
    mock.send_email.return_value = True
    return mock

async def test_registration_sends_email(user_service, mock_email_service):
    user_service.email_service = mock_email_service
    await user_service.register(email="test@example.com")
    mock_email_service.send_email.assert_called_once()

@patch("my_app.services.external_api.fetch_data")
async def test_with_external_api(mock_fetch):
    mock_fetch.return_value = {"status": "success"}
    result = await process_external_data()
    assert result["status"] == "success"
```

## Async Testing

```python
@pytest.mark.asyncio
async def test_async_function():
    result = await async_operation()
    assert result == expected_value

@pytest.mark.asyncio
async def test_concurrent_operations():
    results = await asyncio.gather(async_op1(), async_op2())
    assert all(r is not None for r in results)
```

## Testing FastAPI

```python
from httpx import AsyncClient
from fastapi import status

@pytest.fixture
async def client(app) -> AsyncClient:
    async with AsyncClient(app=app, base_url="http://test") as ac:
        yield ac

@pytest.mark.asyncio
async def test_create_user_endpoint(client: AsyncClient):
    response = await client.post("/users", json={"email": "test@example.com", "username": "test"})
    assert response.status_code == status.HTTP_201_CREATED
    assert response.json()["email"] == "test@example.com"
```

## Coverage

```toml
[tool.pytest.ini_options]
testpaths = ["tests"]
addopts = "--cov=src --cov-report=term-missing --cov-fail-under=80 -v"
markers = ["unit", "integration", "slow"]
```

## pytest-specific Rules

### Always
- Name tests: `test_<unit>_<scenario>` or `test_<unit>_<scenario>_<result>`
- Use `@pytest.fixture` for setup/teardown
- Use `conftest.py` for shared fixtures across modules
- Use `@pytest.mark.asyncio` for async test functions
- Use `@pytest.mark.parametrize` for input variations

### Never
- Use `time.sleep()` — use `pytest-asyncio` or mock time
- Skip `await` in async fixtures (causes silent failures)
