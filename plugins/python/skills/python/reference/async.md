# Python Async Patterns

Modern asynchronous programming with asyncio and async/await.

## Basic Async Patterns

```python
import asyncio
import httpx

async def fetch_data(url: str) -> dict:
    async with httpx.AsyncClient() as client:
        response = await client.get(url)
        return response.json()

async def main():
    result = await fetch_data("https://api.example.com/data")
    print(result)

if __name__ == "__main__":
    asyncio.run(main())
```

## Concurrent Operations

```python
# TaskGroup (Python 3.11+) - Structured concurrency
async def fetch_multiple_urls(urls: list[str]) -> list[dict]:
    async with asyncio.TaskGroup() as tg:
        tasks = [tg.create_task(fetch_data(url)) for url in urls]
    return [task.result() for task in tasks]

# Using gather
async def fetch_with_gather(urls: list[str]) -> list[dict]:
    results = await asyncio.gather(
        *[fetch_data(url) for url in urls],
        return_exceptions=True
    )
    return [r for r in results if not isinstance(r, Exception)]

# With timeout
async def fetch_with_timeout(url: str, timeout: float = 10.0) -> dict:
    async with asyncio.timeout(timeout):
        return await fetch_data(url)
```

## Async Context Managers

```python
from contextlib import asynccontextmanager
from typing import AsyncIterator

class AsyncDatabase:
    async def connect(self):
        await asyncio.sleep(0.1)

    async def disconnect(self):
        await asyncio.sleep(0.1)

    async def __aenter__(self):
        await self.connect()
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.disconnect()

@asynccontextmanager
async def get_session() -> AsyncIterator[AsyncSession]:
    session = AsyncSession()
    try:
        yield session
        await session.commit()
    except Exception:
        await session.rollback()
        raise
    finally:
        await session.close()
```

## Async Iterators

```python
from typing import AsyncIterator

class AsyncDataStream:
    def __init__(self, count: int):
        self.count = count
        self.current = 0

    def __aiter__(self):
        return self

    async def __anext__(self) -> int:
        if self.current >= self.count:
            raise StopAsyncIteration
        await asyncio.sleep(0.1)
        value = self.current
        self.current += 1
        return value

# Usage
async for item in AsyncDataStream(5):
    print(item)

# Async comprehension
items = [item async for item in AsyncDataStream(5)]
```

## Sync/Async Bridge

```python
from functools import partial

# Run sync code in thread pool
async def run_sync_in_executor(func, *args):
    loop = asyncio.get_event_loop()
    return await loop.run_in_executor(None, partial(func, *args))

# Run async code from sync context
def sync_wrapper():
    return asyncio.run(async_function())
```

## Common Pitfalls

```python
# WRONG: Blocking the event loop
async def bad():
    import time
    time.sleep(1)  # BLOCKS!

# CORRECT: Use async sleep
async def good():
    await asyncio.sleep(1)

# WRONG: Forgetting await
result = fetch_data(url)  # Returns coroutine, doesn't execute!

# CORRECT: Always await
result = await fetch_data(url)
```

## Critical Rules

### Always
- Use async with for async context managers
- Track created tasks or use TaskGroup
- Handle exceptions in concurrent tasks
- Type hint as Awaitable[T] or Coroutine

### Never
- Block the event loop with sync I/O
- Forget to await coroutines
- Use async for CPU-bound work
- Create tasks without tracking references
