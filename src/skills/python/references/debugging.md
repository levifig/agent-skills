# Python Debugging

Debug patterns and tools for Python.

## Interactive Debugging with pdb

```python
# Insert breakpoint in code
import pdb; pdb.set_trace()

# Python 3.7+ built-in
breakpoint()

# Conditional breakpoint
if suspicious_condition:
    breakpoint()
```

**Common pdb commands:**

| Command | Action |
|---------|--------|
| `n` (next) | Execute next line |
| `s` (step) | Step into function |
| `c` (continue) | Continue to next breakpoint |
| `p expr` | Print expression value |
| `pp expr` | Pretty-print expression |
| `l` (list) | Show current code context |
| `w` (where) | Print stack trace |
| `u` (up) | Move up stack frame |
| `d` (down) | Move down stack frame |
| `q` (quit) | Exit debugger |

## Structured Logging

```python
import structlog

logger = structlog.get_logger()

# Add context that persists across log calls
logger = logger.bind(request_id=request.id, user_id=user.id)

# Log with structured data
logger.info("order_created", order_id=order.id, total=order.total)
logger.error("payment_failed",
    order_id=order.id,
    error_code=e.code,
    error_message=str(e)
)
```

## Traceback Analysis

```python
import traceback

try:
    risky_operation()
except Exception as e:
    # Full traceback as string
    tb_str = traceback.format_exc()
    logger.error("operation_failed", traceback=tb_str)

    # Extract specific frame info
    tb = traceback.extract_tb(e.__traceback__)
    for frame in tb:
        logger.debug("frame",
            filename=frame.filename,
            line=frame.lineno,
            function=frame.name
        )
```

## pytest Debugging

```bash
# Drop into debugger on failure
pytest --pdb

# Drop into debugger on first failure, then exit
pytest -x --pdb

# Show local variables in tracebacks
pytest -l

# Verbose output with print statements
pytest -v -s

# Run specific test
pytest tests/test_orders.py::test_create_order -v
```

## Remote Debugging

```python
# Using debugpy (VS Code compatible)
import debugpy
debugpy.listen(5678)
debugpy.wait_for_client()  # Pause until debugger attaches
breakpoint()
```
