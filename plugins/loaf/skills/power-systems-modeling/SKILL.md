---
name: power-systems-modeling
description: >-
  Covers thermal rating models (CIGRE TB 601, IEEE 738), conductor physics, and
  electrical properties for overhead transmission lines. Use when implementing
  thermal calculations, validating conductors, or computing sag and resistance.
  Not for infras...
user-invocable: false
allowed-tools: 'Read, Write, Edit, Glob, Grep, Bash(python:*)'
version: 2.0.0-dev.19
---

# Power Systems Reference

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Available Scripts
- Default Standard
- Unit Conventions
- Standard Citations
- Testing Tolerances
- Related Skills

Follows [foundations](../foundations/SKILL.md) code quality and TDD principles.

Domain knowledge for overhead transmission line physics, thermal ratings, and mechanical analysis.

## Critical Rules

- Always validate physical parameters at function boundaries; raise `ValueError` for out-of-range values
- Use CIGRE TB 601 as the default thermal rating standard; document deviations in code comments
- Always cite standard section numbers in code comments (e.g., `CIGRE TB 601, Section 4.2.3`)
- Use SI units internally (temperatures in Kelvin); convert to Celsius only for display
- Use `pytest.approx()` with `rel=1e-3` for thermal calculation tolerances

## Verification

- Confirm all physical parameters are validated against bounds before computation
- Verify standard citations are present in code comments for thermal formulas
- Run `scripts/validate-bounds.py` to check parameter values against physical limits

## Quick Reference

| Parameter | Valid Range |
|-----------|-------------|
| Conductor temperature | -40°C to 250°C |
| Ambient temperature | -50°C to 60°C |
| Wind speed | 0 to 50 m/s |
| Solar radiation | 0 to 1400 W/m² |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Thermal Models | [references/thermal-models.md](references/thermal-models.md) | CIGRE TB 601, IEEE 738 thermal rating implementations |
| Conductor Limits | [references/conductor-limits.md](references/conductor-limits.md) | Physical validation bounds and parameter constraints |
| Electrical Properties | [references/electrical-properties.md](references/electrical-properties.md) | Resistance, sag, catenary formulas and calculations |
| Standards Reference | [references/standards-reference.md](references/standards-reference.md) | Industry standards summary (CIGRE, IEEE, IEC, EN) |

## Available Scripts

| Script | Usage | Description |
|--------|-------|-------------|
| `scripts/validate-bounds.py` | `validate-bounds.py -t conductor_temp -v 85` | Validate physics values against bounds |
| `scripts/convert-units.py` | `convert-units.py 25 C K` | Convert between units (temp, length, power, speed) |
| `scripts/check-standard-refs.sh` | `check-standard-refs.sh <dir>` | Check for proper CIGRE/IEEE citations in code |

## Default Standard

**CIGRE TB 601** is the default thermal rating standard. Document any deviations in code comments. IEEE 738 differs in treatment of low wind speeds.

## Unit Conventions

- **Internal**: SI units, temperatures in Kelvin
- **Display**: Celsius for temperatures
- **Document**: Always include units in variable names or docstrings

## Standard Citations

Always cite standard section numbers in code comments:

```python
# CIGRE TB 601, Section 4.2.3: Natural convection heat loss
# Formula: P_n = pi * D * lambda * Nu * (T_s - T_a)
```

## Testing Tolerances

Thermal calculations: use `pytest.approx()` with `rel=1e-3` (0.1% accuracy).

## Related Skills

- `database-patterns` — For persisting physics results
- [foundations/code-style](../foundations/references/code-style.md) — For Python conventions in physics code
