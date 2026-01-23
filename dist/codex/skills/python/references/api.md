# Python API Integration

Building reliable API clients with httpx.

## Basic HTTP Operations

```python
import httpx

async def fetch_user(user_id: int) -> dict:
    async with httpx.AsyncClient() as client:
        response = await client.get(
            f"https://api.example.com/users/{user_id}",
            headers={"Authorization": "Bearer token"},
            timeout=10.0
        )
        response.raise_for_status()
        return response.json()
```

## API Client Pattern

```python
from httpx import AsyncClient
from pydantic import BaseModel

class APIClient:
    def __init__(self, base_url: str, api_key: str, timeout: float = 30.0):
        self.base_url = base_url
        self.api_key = api_key
        self.timeout = timeout
        self._client: AsyncClient | None = None

    async def __aenter__(self):
        self._client = AsyncClient(
            base_url=self.base_url,
            headers={"Authorization": f"Bearer {self.api_key}"},
            timeout=self.timeout
        )
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        if self._client:
            await self._client.aclose()

    async def get(self, endpoint: str, response_model: type[T]) -> T:
        response = await self._client.get(endpoint)
        response.raise_for_status()
        return response_model(**response.json())

# Usage
async with APIClient(BASE_URL, API_KEY) as client:
    user = await client.get("/users/1", User)
```

## Retry Logic

```python
from tenacity import retry, stop_after_attempt, wait_exponential, retry_if_exception_type
import httpx

@retry(
    stop=stop_after_attempt(3),
    wait=wait_exponential(multiplier=1, min=4, max=10),
    retry=retry_if_exception_type((httpx.TimeoutException, httpx.NetworkError)),
)
async def fetch_with_retry(url: str) -> dict:
    async with httpx.AsyncClient() as client:
        response = await client.get(url, timeout=10.0)
        response.raise_for_status()
        return response.json()
```

## Rate Limiting

```python
import asyncio
from datetime import datetime

class RateLimiter:
    def __init__(self, rate: int, per: float):
        self.rate = rate
        self.per = per
        self.allowance = rate
        self.last_check = datetime.now()

    async def acquire(self):
        current = datetime.now()
        elapsed = (current - self.last_check).total_seconds()
        self.allowance += elapsed * (self.rate / self.per)
        self.last_check = current

        if self.allowance < 1.0:
            await asyncio.sleep((1.0 - self.allowance) * (self.per / self.rate))
            self.allowance = 0.0
        else:
            self.allowance -= 1.0
```

## Error Handling

```python
from httpx import HTTPStatusError, TimeoutException

class APIError(Exception):
    pass

class NotFoundError(APIError):
    pass

async def safe_api_call(func, default=None):
    try:
        return await func()
    except HTTPStatusError as e:
        if e.response.status_code == 404:
            raise NotFoundError(f"Resource not found")
        elif e.response.status_code == 429:
            raise APIError("Rate limit exceeded")
        raise APIError(f"HTTP error: {e.response.status_code}")
    except TimeoutException:
        if default is not None:
            return default
        raise APIError("Request timeout")
```

## Pagination

```python
from collections.abc import AsyncIterator

async def paginate_cursor(client: APIClient, endpoint: str) -> AsyncIterator[dict]:
    cursor = None
    while True:
        params = {"limit": 100}
        if cursor:
            params["cursor"] = cursor
        response = await client.get(endpoint, params=params)
        for item in response["data"]:
            yield item
        if not response.get("has_more"):
            break
        cursor = response["next_cursor"]
```

## Critical Rules

### Always
- Use async HTTP clients (httpx)
- Implement retry logic
- Validate responses with Pydantic
- Handle rate limiting
- Use context managers

### Never
- Use requests library (sync, blocking)
- Ignore HTTP error status codes
- Skip timeout configuration
- Hardcode API credentials
