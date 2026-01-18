# Python Core

Foundation for modern Python 3.12+ development.

## Python Stack

| Component | Default | Alternative |
|-----------|---------|-------------|
| Runtime | Python 3.12+ | - |
| Package Manager | uv | rye, poetry |
| Linter/Formatter | ruff | black + flake8 |
| Type Checker | mypy | pyright |
| Testing | pytest | unittest |

## Project Structure

```
my_project/
├── src/
│   └── my_project/
│       ├── __init__.py
│       ├── main.py
│       ├── models/
│       ├── services/
│       └── utils/
├── tests/
│   ├── conftest.py
│   └── test_*.py
├── pyproject.toml
└── uv.lock
```

## Dependency Management with uv

```bash
uv init my-project --python 3.12
uv add fastapi uvicorn pydantic-settings
uv add --dev pytest ruff mypy
uv run python -m my_project
uv sync
```

## pyproject.toml

```toml
[project]
name = "my-project"
version = "0.1.0"
requires-python = ">=3.12"
dependencies = [
    "fastapi>=0.110.0",
    "uvicorn[standard]>=0.27.0",
    "pydantic-settings>=2.2.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0.0",
    "pytest-cov>=4.1.0",
    "ruff>=0.3.0",
    "mypy>=1.8.0",
]

[tool.ruff]
target-version = "py312"
line-length = 100
select = ["E", "F", "I", "N", "W", "UP"]

[tool.mypy]
python_version = "3.12"
strict = true

[tool.pytest.ini_options]
testpaths = ["tests"]
addopts = "--cov=src --cov-report=term-missing"
```

## Modern Python Features

```python
# Type aliases (PEP 695)
type Point = tuple[float, float]
type Vector = list[Point]

# Pattern matching
match command:
    case ["quit"]:
        return
    case ["load", filename]:
        load_file(filename)
    case _:
        print("Unknown command")

# Exception groups
try:
    result = process_data()
except* ValueError as eg:
    for exc in eg.exceptions:
        log_error(exc)
```

## Code Style

```python
# Imports - standard, third-party, local
import os
from pathlib import Path

import httpx
from fastapi import FastAPI

from my_project.models import User

# Constants
MAX_RETRIES = 3
API_BASE_URL = "https://api.example.com"

# Classes (PascalCase)
class UserRepository:
    pass

# Functions and variables (snake_case)
def fetch_user_data(user_id: int) -> dict:
    return {}
```

## Critical Rules

### Always
- Use type hints for all function signatures
- Use pathlib.Path for file operations
- Use f-strings for string formatting
- Use context managers for resources

### Never
- Use mutable default arguments
- Import * from modules
- Use bare except clauses
- Mix tabs and spaces
