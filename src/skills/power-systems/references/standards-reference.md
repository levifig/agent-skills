# Standards Reference

Summary of industry standards for overhead transmission line analysis.

## Primary Standards

### CIGRE TB 601 (2014)
**"Guide for Thermal Rating Calculations of Overhead Lines"**

| Chapter | Topic | Key Content |
|---------|-------|-------------|
| 2 | Scope | Rating methodologies overview |
| 3 | Heat Gains | Joule heating, solar radiation, corona |
| 4 | Heat Losses | Convection (forced/natural), radiation |
| 5 | Transient Ratings | Time constants, dynamic rating |
| 6 | Sag-Temperature | Change of state, ruling span |
| 7 | Clearances | Minimum distances, blow-out |
| 8 | Monitoring | Real-time thermal rating |

**Default Standard**: Use CIGRE 601 unless project specifies otherwise.

### IEEE 738-2012
**"Calculating the Current-Temperature Relationship of Bare Overhead Conductors"**

Key differences from CIGRE 601:

| Aspect | IEEE 738 | CIGRE 601 |
|--------|----------|-----------|
| Convection threshold | 0.61 m/s | 0.5 m/s |
| Film temperature | Ambient | Average |
| Nusselt correlation | Morgan | McAdams |
| Default emissivity | 0.5 (new) | 0.9 (aged) |
| Geographic focus | North America | International |

**Usage**: Specify IEEE 738 for North American projects or regulatory compliance.

### CIGRE TB 207 (2002)
**"Thermal Behaviour of Overhead Conductors"**

Predecessor to TB 601. Contains:
- Historical conductor properties data
- Detailed heat transfer theory
- Experimental validation data

**Usage**: Reference for theoretical background; prefer TB 601 for calculations.

## Secondary Standards

### IEC Standards

| Standard | Title | Application |
|----------|-------|-------------|
| IEC 61597 | Overhead electrical conductors - Calculation methods | Conductor mechanical properties |
| IEC 60826 | Design criteria for overhead transmission lines | Structural design |
| IEC 60889 | Hard-drawn aluminium wire for conductors | Aluminum properties |
| IEC 60104 | Aluminium-magnesium-silicon alloy wire | Alloy conductor properties |
| IEC 60028 | International standard of resistance for copper | Copper properties |

### Regional Standards

**North America:**
- NESC (National Electrical Safety Code) - Clearances, grounding
- ASCE Manual 74 - Loading criteria

**Europe:**
- EN 50341 - Overhead electrical lines (>1kV AC)
- EN 50182 - Conductors for overhead lines

## Standard Selection Guide

```python
def select_thermal_standard(
    project_region: str,
    regulatory_requirement: str | None = None
) -> str:
    """Select appropriate thermal rating standard.

    Args:
        project_region: Geographic region
        regulatory_requirement: Specific regulatory standard if required

    Returns:
        Standard name (e.g., "CIGRE_601", "IEEE_738")
    """
    if regulatory_requirement:
        return regulatory_requirement

    region_defaults = {
        "north_america": "IEEE_738",
        "europe": "CIGRE_601",
        "asia": "CIGRE_601",
        "australia": "CIGRE_601",
        "south_america": "CIGRE_601",
    }

    return region_defaults.get(project_region.lower(), "CIGRE_601")
```

## Code Documentation Pattern

Always cite specific standard sections:

```python
def calculate_convective_heat_loss(
    wind_speed: float,
    temperature_diff: float,
    diameter: float
) -> float:
    """Calculate convective heat loss per unit length.

    Implementation follows CIGRE TB 601, Section 4.2.

    For forced convection (wind > 0.5 m/s):
        Uses Nusselt number correlation from Section 4.2.2
        Nu = B1 × Re^n1 (Eq. 4.5)

    For natural convection (wind ≤ 0.5 m/s):
        Uses correlation from Section 4.2.3
        Nu = 0.60 + 0.387 × Ra^(1/6) × F(Pr) (Eq. 4.12)

    Note: IEEE 738 uses different correlation coefficients.
    See IEEE_738_convection() for that implementation.
    """
    pass
```

## Standard Conflicts

When standards conflict, document the choice:

```python
# Standard conflict: Natural convection threshold
#
# CIGRE TB 601, Section 4.2: Threshold at 0.5 m/s
# IEEE 738-2012, Section 4: Threshold at 0.61 m/s (2 ft/s)
#
# Decision: Using CIGRE 601 threshold (0.5 m/s) per project specification.
# This is more conservative for thermal rating calculations.

NATURAL_CONVECTION_THRESHOLD_MPS = 0.5
```

## Conductor Property Standards

### Typical Conductor Types

| Type | Standard | Construction |
|------|----------|--------------|
| ACSR | ASTM B232 | Aluminum/Steel stranded |
| AAC | ASTM B231 | All-aluminum stranded |
| AAAC | ASTM B399 | Aluminum alloy stranded |
| ACSS | ASTM B856 | Aluminum/Steel, annealed Al |

### Standard Conductor Names

ASTM naming convention for ACSR:

| Name | Al Area (kcmil) | Stranding | Application |
|------|-----------------|-----------|-------------|
| Penguin | 266.8 | 6/1 | Distribution |
| Partridge | 266.8 | 26/7 | Subtransmission |
| Hawk | 477.0 | 26/7 | Transmission |
| Drake | 795.0 | 26/7 | Heavy transmission |
| Cardinal | 954.0 | 54/7 | Extra heavy |
| Bluebird | 1590.0 | 84/19 | Maximum capacity |

## Compliance Checklist

When implementing physics algorithms:

- [ ] Identify applicable standard (CIGRE 601, IEEE 738, etc.)
- [ ] Cite section numbers in code comments
- [ ] Document any deviations from standard
- [ ] Use standard's recommended default values
- [ ] Validate against standard's example calculations
- [ ] Note geographic/regulatory applicability

## References

### Primary Documents

1. **CIGRE TB 601** (2014): "Guide for Thermal Rating Calculations of Overhead Lines"
   - Working Group B2.43
   - ISBN: 978-2-85873-284-0

2. **IEEE 738-2012**: "IEEE Standard for Calculating the Current-Temperature
   Relationship of Bare Overhead Conductors"
   - DOI: 10.1109/IEEESTD.2013.6692858

3. **CIGRE TB 207** (2002): "Thermal Behaviour of Overhead Conductors"
   - Task Force 22.12
   - Historical reference

### Supporting Documents

1. **IEC 61597** (1995): "Overhead electrical conductors - Calculation methods
   for stranded bare conductors"

2. **EN 50341-1** (2012): "Overhead electrical lines exceeding AC 1 kV -
   Part 1: General requirements - Common specifications"

3. **NESC** (2017): "National Electrical Safety Code"
   - IEEE/ANSI C2-2017
