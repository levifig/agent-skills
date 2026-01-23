---
name: power-systems
description: >-
  Use for electrical power systems engineering. Covers thermal rating models
  (CIGRE TB 601, IEEE 738), conductor temperature limits, catenary physics (sag,
  tension, clearance), electrical properties (resistance, reactance), and
  standards compliance (CIGRE, IEEE, IEC). Activate when working with
  transmission line calculations, thermal modeling, or conductor physics
  validation.
agent: backend-dev
allowed-tools: 'Read, Write, Edit, Glob, Grep, Bash(python:*)'
---

# Power Systems Reference

Follows [foundations principles](../foundations/SKILL.md).

Domain knowledge for overhead transmission line physics, thermal ratings, and mechanical analysis.

## When to Use This Skill

- Implementing thermal models or conductor temperature calculations
- Working with catenary physics (sag, tension, clearance)
- Validating electrical properties (resistance, reactance, impedance)
- Referencing industry standards (CIGRE, IEEE, IEC)
- Applying physical bounds validation
- Reviewing or writing physics algorithms
- Implementing conductor property calculations

## Key Reference Files

| File | Content |
|------|---------|
| `reference/thermal-models.md` | CIGRE TB 601, IEEE 738 thermal rating implementations |
| `reference/conductor-limits.md` | Physical validation bounds and parameter constraints |
| `reference/electrical-properties.md` | Resistance, sag, catenary formulas and calculations |
| `reference/standards-reference.md` | Industry standards summary (CIGRE, IEEE, IEC, EN) |

## Available Scripts

| Script | Usage | Description |
|--------|-------|-------------|
| `scripts/validate-bounds.py` | `validate-bounds.py -t conductor_temp -v 85` | Validate physics values against bounds |
| `scripts/convert-units.py` | `convert-units.py 25 C K` | Convert between units (temp, length, power, speed) |
| `scripts/check-standard-refs.sh` | `check-standard-refs.sh <dir>` | Check for proper CIGRE/IEEE citations in code |

## Quick Reference

### Default Standard

**CIGRE TB 601** is the default thermal rating standard. Document any deviations in code comments:

```python
# Note: Using CIGRE 601 natural convection formula (Section 4.2.3)
# IEEE 738 differs in treatment of low wind speeds
```

### Physical Bounds (Quick Check)

| Parameter | Valid Range |
|-----------|-------------|
| Conductor temperature | -40°C to 250°C |
| Ambient temperature | -50°C to 60°C |
| Wind speed | 0 to 50 m/s |
| Solar radiation | 0 to 1400 W/m² |

### Unit Conventions

- **Internal**: SI units, temperatures in Kelvin
- **Display**: Celsius for temperatures
- **Document**: Always include units in variable names or docstrings

## Code Patterns

### Parameter Validation

Always validate physical parameters at function boundaries:

```python
def calculate_conductor_temperature(
    current_a: float,
    ambient_temp_c: float,
    wind_speed_mps: float,
) -> float:
    """Calculate steady-state conductor temperature.

    Args:
        current_a: Line current in Amperes (0 to rated × 2)
        ambient_temp_c: Ambient temperature in Celsius (-50 to 60)
        wind_speed_mps: Wind speed in m/s (0 to 50)

    Raises:
        ValueError: If parameters outside physical bounds
    """
    if not -50 <= ambient_temp_c <= 60:
        raise ValueError(f"Ambient temperature {ambient_temp_c}°C outside valid range [-50, 60]")
    # ... implementation
```

### Standard Citations

Always cite standard section numbers in code comments:

```python
# CIGRE TB 601, Section 4.2.3: Natural convection heat loss
# Formula: P_n = pi * D * lambda * Nu * (T_s - T_a)
```

### Testing Patterns

Use `pytest.approx()` with appropriate tolerances:

```python
def test_thermal_model():
    result = calculate_ampacity(...)
    # Thermal calculations: 1e-3 tolerance (0.1% accuracy)
    assert result == pytest.approx(expected, rel=1e-3)
```

## Related Skills

- `database-patterns` — For persisting physics results
- [foundations/code-style](../foundations/reference/code-style.md) — For Python conventions in physics code
