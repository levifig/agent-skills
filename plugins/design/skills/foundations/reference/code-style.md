# Code Style

Coding conventions for Python and TypeScript development.

## Python

### Tooling

| Tool | Purpose | Config |
|------|---------|--------|
| **black** | Formatting | Line length: 100 |
| **isort** | Import sorting | Profile: black |
| **mypy** | Type checking | Strict mode |
| **ruff** | Linting | Replaces flake8 |
| **structlog** | Logging | JSON in production |

### Type Hints (Required)

```python
from typing import Optional
from uuid import UUID

async def get_measurements(
    db: AsyncSession,
    tower_id: UUID,
    start: datetime,
    end: datetime,
    limit: int = 1000,
) -> list[Measurement]:
    """Retrieve measurements within time range."""
    ...
```

### Structured Logging

```python
import structlog

logger = structlog.get_logger(__name__)

# Good: Structured context
logger.info(
    "measurement_processed",
    tower_id=str(tower_id),
    processing_time_ms=elapsed_ms,
)

# Bad: String interpolation
logger.info(f"Processed tower {tower_id}")  # Don't use
```

### Import Organization

```python
# Standard library
from datetime import datetime
from typing import Optional
from uuid import UUID

# Third-party
import structlog
from fastapi import APIRouter, Depends
from pydantic import BaseModel

# Local application
from myproject.core.database import get_db
from myproject.models.tower import Tower
```

### Error Handling

```python
# Define specific exceptions
class ThermalModelError(Exception):
    """Base exception for thermal model errors."""

class ConvergenceError(ThermalModelError):
    """Temperature solution did not converge."""

# Catch specific exceptions
try:
    result = await process_batch(batch)
except ConvergenceError as e:
    logger.warning("convergence_failed", error=str(e))
    result = create_fallback_result(batch)
except Exception as e:
    logger.error("unexpected_error", exc_info=True)
    raise ProcessingError(f"Batch processing failed: {e}") from e
```

## TypeScript

### Tooling

| Tool | Purpose | Config |
|------|---------|--------|
| **Prettier** | Formatting | 2-space, single quotes |
| **ESLint** | Linting | Strict mode |
| **TypeScript** | Type checking | Strict mode |

### Functional Components

```typescript
interface MeasurementCardProps {
  towerId: string;
  timestamp: Date;
  value: number;
  quality?: 'good' | 'degraded' | 'bad';
}

export function MeasurementCard({
  towerId,
  timestamp,
  value,
  quality = 'good',
}: MeasurementCardProps) {
  return (
    <div className={cn('measurement-card', quality)}>
      <span className="value">{value.toFixed(2)}</span>
    </div>
  );
}
```

### Type Definitions

```typescript
// Interface for object shapes
interface Tower {
  id: string;
  name: string;
  latitude: number;
  longitude: number;
}

// Type for unions
type MeasurementType = 'voltage' | 'current' | 'power';

// Generic types
interface ApiResponse<T> {
  data: T;
  meta: { total: number; page: number };
}
```

### Hooks Pattern

```typescript
function useMeasurements(towerId: string) {
  const [measurements, setMeasurements] = useState<Measurement[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function fetchData() {
      try {
        const data = await api.getMeasurements(towerId);
        if (!cancelled) setMeasurements(data);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err : new Error('Unknown'));
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    }
    fetchData();
    return () => { cancelled = true; };
  }, [towerId]);

  return { measurements, isLoading, error };
}
```

## Comment Policy

### Hierarchy

1. **Clear Code** - Self-documenting names and structure
2. **Contextual Comments** - Short docstrings, WHY comments
3. **Documentation Files** - ADRs, guides (last resort)

### Good Comments

```python
# Use synchronous I/O here because the async driver
# has a memory leak in high-throughput scenarios.
# See: https://github.com/lib/issues/1234
result = sync_query(data)

# Regulatory requirement: retain for 7 years
# per IEC 62443-2-1 section 4.3.3.6
AUDIT_RETENTION_DAYS = 2555

# TODO(PLT-456): Replace with batch API when available
```

### Bad Comments

```python
i = i + 1  # increment i (obvious)
# old_value = calculate_legacy()  # dead code
x = 86400  # seconds in a day (use constant name)
```

## Test Patterns

### AAA Pattern

```python
async def test_thermal_model_calculates_temperature():
    """Test thermal model returns valid temperature."""
    # Arrange
    input_data = ThermalInput(ambient_temp_c=25.0, wind_speed_mps=2.5)
    model = ThermalModel(conductor=ACSR_DRAKE)

    # Act
    result = await model.calculate(input_data)

    # Assert
    assert result.temperature_c == pytest.approx(75.2, rel=1e-2)
```

### Parameterized Tests

```python
@pytest.mark.parametrize("wind_speed,expected", [
    (0.0, "natural"),
    (0.5, "natural"),
    (0.51, "forced"),
    (10.0, "forced"),
])
def test_convection_type_selection(wind_speed, expected):
    assert select_convection_type(wind_speed) == expected
```

### Test Organization

```
tests/
├── unit/
│   └── services/
├── integration/
│   └── api/
├── e2e/
├── fixtures/
│   └── thermal.py  # *_perfect, *_degraded, *_chaos
└── conftest.py
```
