# Python Type Safety

Advanced type hints and static type checking with mypy.

## Basic Type Hints

```python
from collections.abc import Sequence, Mapping

def greet(name: str) -> str:
    return f"Hello, {name}!"

def process_items(items: Sequence[int], multiplier: float = 1.0) -> list[int]:
    return [int(item * multiplier) for item in items]

# Optional types
def find_user(user_id: int) -> User | None:
    return db.get(User, user_id)
```

## Generic Types

```python
from typing import TypeVar, Generic

T = TypeVar("T")

def first(items: Sequence[T]) -> T | None:
    return items[0] if items else None

class Repository(Generic[T]):
    def __init__(self, model: type[T]):
        self.model = model

    async def get(self, id: int) -> T | None:
        return await db.get(self.model, id)

# Usage
user_repo: Repository[User] = Repository(User)
```

## Protocol (Structural Typing)

```python
from typing import Protocol, runtime_checkable

@runtime_checkable
class Drawable(Protocol):
    def draw(self) -> None:
        ...

def render_items(items: Sequence[Drawable]) -> None:
    for item in items:
        item.draw()

# Any class with draw() method is Drawable
class Circle:
    def draw(self) -> None:
        print("Drawing circle")

assert isinstance(Circle(), Drawable)  # True
```

## TypedDict

```python
from typing import TypedDict, Required, NotRequired

class UserProfile(TypedDict):
    id: Required[int]
    username: Required[str]
    bio: NotRequired[str]

def create_user(data: UserProfile) -> User:
    return User(**data)
```

## Advanced Patterns

```python
from typing import Annotated, TypeGuard, ParamSpec

# Annotated for metadata
from pydantic import Field
UserId = Annotated[int, Field(gt=0)]

# TypeGuard for runtime narrowing
def is_str_list(val: list) -> TypeGuard[list[str]]:
    return all(isinstance(x, str) for x in val)

# ParamSpec for decorator typing
P = ParamSpec("P")
R = TypeVar("R")

def log_call(func: Callable[P, R]) -> Callable[P, R]:
    def wrapper(*args: P.args, **kwargs: P.kwargs) -> R:
        print(f"Calling {func.__name__}")
        return func(*args, **kwargs)
    return wrapper
```

## Mypy Configuration

```toml
[tool.mypy]
python_version = "3.12"
strict = true
warn_return_any = true
warn_unused_configs = true

[[tool.mypy.overrides]]
module = "tests.*"
disallow_untyped_defs = false
```

## Type Narrowing

```python
from typing import assert_never

def handle_value(value: int | str | None) -> str:
    if value is None:
        return "none"
    elif isinstance(value, int):
        return str(value * 2)
    elif isinstance(value, str):
        return value.upper()
    else:
        assert_never(value)
```

## Critical Rules

### Always
- Annotate all public functions
- Use strict mypy configuration
- Prefer Protocol over ABC
- Use TypedDict for structured dicts
- Run mypy in CI/CD

### Never
- Use Any without justification
- Ignore mypy errors without # type: ignore
- Skip type hints on public APIs
