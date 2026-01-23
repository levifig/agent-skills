# Conductor Limits & Validation Bounds

Physical parameter constraints for overhead transmission line calculations.

## Physical Validation Bounds

Enforce these limits in ALL physics implementations:

| Parameter | Min | Max | Unit | Reason |
|-----------|-----|-----|------|--------|
| Conductor temperature | -40 | 250 | °C | Material limits (AAC/ACSR) |
| Ambient temperature | -50 | 60 | °C | Operational range |
| Wind speed | 0 | 50 | m/s | Extreme weather (hurricane) |
| Wind angle | 0 | 90 | ° | Perpendicular = max cooling |
| Solar radiation | 0 | 1400 | W/m² | Solar constant × atmosphere |
| Current | 0 | rated × 2 | A | Emergency overload |
| Magnetic flux density | 0 | ~2.0 | T | Steel core saturation |
| Relative humidity | 0 | 100 | % | Physical definition |
| Elevation | -500 | 8000 | m | Dead Sea to high mountains |

## Conductor Temperature Classes

Different conductor types have different thermal limits:

| Conductor Type | Normal Max | Emergency Max | Annealing Risk |
|----------------|------------|---------------|----------------|
| AAC (All Aluminum) | 75°C | 100°C | >150°C |
| ACSR (Steel Reinforced) | 75°C | 100°C | >150°C |
| AAAC (Aluminum Alloy) | 90°C | 120°C | >180°C |
| ACSS (Steel Supported) | 200°C | 250°C | N/A (pre-annealed) |
| HTLS (High-Temp Low-Sag) | 150-250°C | 200-300°C | Varies |

**Important**: Temperature limits are design-dependent. Always check conductor specifications.

## Validation Implementation

### Python Pattern

```python
from dataclasses import dataclass
from typing import Tuple

@dataclass
class PhysicalBounds:
    """Physical parameter validation bounds."""

    CONDUCTOR_TEMP_C: Tuple[float, float] = (-40, 250)
    AMBIENT_TEMP_C: Tuple[float, float] = (-50, 60)
    WIND_SPEED_MPS: Tuple[float, float] = (0, 50)
    WIND_ANGLE_DEG: Tuple[float, float] = (0, 90)
    SOLAR_RADIATION_WM2: Tuple[float, float] = (0, 1400)
    RELATIVE_HUMIDITY_PCT: Tuple[float, float] = (0, 100)


def validate_ambient_temperature(value: float) -> None:
    """Validate ambient temperature within physical bounds.

    Args:
        value: Temperature in Celsius

    Raises:
        ValueError: If outside valid range [-50, 60]°C
    """
    min_val, max_val = PhysicalBounds.AMBIENT_TEMP_C
    if not min_val <= value <= max_val:
        raise ValueError(
            f"Ambient temperature {value}°C outside valid range "
            f"[{min_val}, {max_val}]°C"
        )


def validate_conductor_temperature(
    value: float,
    conductor_type: str = "ACSR",
    is_emergency: bool = False
) -> None:
    """Validate conductor temperature for specific conductor type.

    Args:
        value: Temperature in Celsius
        conductor_type: Conductor type (ACSR, ACSS, etc.)
        is_emergency: Whether emergency rating applies

    Raises:
        ValueError: If outside valid range for conductor type
    """
    limits = {
        "AAC": (75, 100),
        "ACSR": (75, 100),
        "AAAC": (90, 120),
        "ACSS": (200, 250),
    }

    normal_max, emergency_max = limits.get(conductor_type, (75, 100))
    max_val = emergency_max if is_emergency else normal_max
    min_val = -40

    if not min_val <= value <= max_val:
        rating_type = "emergency" if is_emergency else "normal"
        raise ValueError(
            f"Conductor temperature {value}°C outside {rating_type} range "
            f"[{min_val}, {max_val}]°C for {conductor_type}"
        )


def validate_wind_speed(value: float) -> None:
    """Validate wind speed within physical bounds."""
    if not 0 <= value <= 50:
        raise ValueError(f"Wind speed {value} m/s outside valid range [0, 50]")


def validate_solar_radiation(value: float) -> None:
    """Validate solar radiation within physical bounds."""
    if not 0 <= value <= 1400:
        raise ValueError(
            f"Solar radiation {value} W/m² outside valid range [0, 1400]"
        )
```

### Pydantic Model Pattern

```python
from pydantic import BaseModel, Field, field_validator

class ThermalRatingInput(BaseModel):
    """Input parameters for thermal rating calculation."""

    ambient_temp_c: float = Field(
        ...,
        ge=-50,
        le=60,
        description="Ambient temperature in Celsius"
    )
    wind_speed_mps: float = Field(
        ...,
        ge=0,
        le=50,
        description="Wind speed in meters per second"
    )
    wind_angle_deg: float = Field(
        default=45,
        ge=0,
        le=90,
        description="Wind angle to conductor (0=parallel, 90=perpendicular)"
    )
    solar_radiation_wm2: float = Field(
        ...,
        ge=0,
        le=1400,
        description="Global solar radiation in W/m²"
    )
    current_a: float = Field(
        ...,
        ge=0,
        description="Line current in Amperes"
    )

    @field_validator('wind_angle_deg')
    @classmethod
    def validate_wind_angle(cls, v: float) -> float:
        """Normalize wind angle to [0, 90] range."""
        # Wind angle symmetry: 135° equivalent to 45°
        if v > 90:
            v = 180 - v
        if v < 0:
            v = abs(v)
        return min(v, 90)
```

## Sag and Clearance Limits

### Minimum Ground Clearance

Per NESC (National Electrical Safety Code):

| Voltage | Nominal | Min Clearance at Max Sag |
|---------|---------|--------------------------|
| 69 kV | 69 | 5.5 m (18 ft) |
| 138 kV | 138 | 6.1 m (20 ft) |
| 230 kV | 230 | 6.7 m (22 ft) |
| 345 kV | 345 | 7.3 m (24 ft) |
| 500 kV | 500 | 8.5 m (28 ft) |

**Note**: These are base values. Actual clearances depend on:
- Crossing type (road, railroad, navigable water)
- Terrain elevation
- Local regulations

### Sag Limits

```python
def validate_sag(
    sag_m: float,
    span_length_m: float,
    voltage_kv: float,
    ground_elevation_m: float,
    attachment_height_m: float
) -> None:
    """Validate sag doesn't violate clearance requirements.

    Args:
        sag_m: Maximum sag in meters
        span_length_m: Span length in meters
        voltage_kv: Nominal voltage
        ground_elevation_m: Ground elevation along span
        attachment_height_m: Conductor attachment height

    Raises:
        ClearanceViolation: If minimum clearance violated
    """
    min_clearance = get_minimum_clearance(voltage_kv)
    actual_clearance = attachment_height_m - sag_m - ground_elevation_m

    if actual_clearance < min_clearance:
        raise ClearanceViolation(
            f"Clearance {actual_clearance:.2f}m below minimum "
            f"{min_clearance:.2f}m required for {voltage_kv}kV"
        )
```

## Tension Limits

### Everyday Tension (EDT)

Typical limits as percentage of Rated Breaking Strength (RBS):

| Condition | % RBS | Description |
|-----------|-------|-------------|
| Initial unloaded | 20-25% | Installation tension |
| Final unloaded | 15-20% | After creep |
| Maximum working | 40% | Heavy loading |
| Emergency | 60% | Short-term overload |

### Aeolian Vibration

Critical self-damping tension (H/w ratio):

```python
def check_vibration_risk(
    tension_n: float,
    conductor_weight_n_per_m: float
) -> str:
    """Assess aeolian vibration risk based on H/w ratio.

    CIGRE recommendations for H/w limits.

    Returns:
        Risk level: 'low', 'moderate', 'high'
    """
    h_w_ratio = tension_n / conductor_weight_n_per_m

    if h_w_ratio < 1000:
        return "low"  # Self-damping sufficient
    elif h_w_ratio < 1400:
        return "moderate"  # Consider dampers
    else:
        return "high"  # Dampers required
```

## Error Messages

Always provide informative error messages:

```python
# Good: Clear context and guidance
raise ValueError(
    f"Conductor temperature {temp}°C exceeds emergency limit of 100°C "
    f"for ACSR conductor. Check thermal model inputs or consider "
    f"ACSS/HTLS conductor for higher ratings."
)

# Bad: No context
raise ValueError("Invalid temperature")
```

## Testing Validation

```python
import pytest

class TestPhysicalBounds:
    """Test physical parameter validation."""

    def test_ambient_temp_valid_range(self):
        """Accept temperatures within valid range."""
        for temp in [-50, -20, 0, 20, 40, 60]:
            validate_ambient_temperature(temp)  # Should not raise

    def test_ambient_temp_below_minimum(self):
        """Reject temperatures below -50°C."""
        with pytest.raises(ValueError, match=r"outside valid range"):
            validate_ambient_temperature(-51)

    def test_ambient_temp_above_maximum(self):
        """Reject temperatures above 60°C."""
        with pytest.raises(ValueError, match=r"outside valid range"):
            validate_ambient_temperature(61)

    def test_conductor_temp_emergency_mode(self):
        """Allow higher temps in emergency mode."""
        # Normal mode: 75°C max for ACSR
        with pytest.raises(ValueError):
            validate_conductor_temperature(90, "ACSR", is_emergency=False)

        # Emergency mode: 100°C max for ACSR
        validate_conductor_temperature(90, "ACSR", is_emergency=True)
```
