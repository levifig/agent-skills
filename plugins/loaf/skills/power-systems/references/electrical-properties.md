# Electrical Properties Reference

Resistance, impedance, and catenary calculations for overhead conductors.

## Conductor Resistance

### DC Resistance

DC resistance at reference temperature (typically 20°C):

```python
def dc_resistance_at_temperature(
    r_dc_20c: float,
    temperature_c: float,
    alpha: float = 0.00403  # Aluminum
) -> float:
    """Calculate DC resistance at specified temperature.

    R(T) = R(20°C) × [1 + α × (T - 20)]

    Args:
        r_dc_20c: DC resistance at 20°C in Ω/km
        temperature_c: Conductor temperature in °C
        alpha: Temperature coefficient of resistance
            - Aluminum: 0.00403
            - Copper: 0.00393
            - Steel: 0.0045

    Returns:
        DC resistance in Ω/km at specified temperature
    """
    return r_dc_20c * (1 + alpha * (temperature_c - 20))
```

### AC Resistance

AC resistance accounts for skin effect and proximity effect:

```python
def ac_resistance(
    r_dc: float,
    frequency_hz: float,
    conductor_diameter_mm: float,
    conductor_type: str = "ACSR"
) -> float:
    """Calculate AC resistance including skin effect.

    For ACSR: R_ac ≈ R_dc × k_skin × k_proximity

    Args:
        r_dc: DC resistance at operating temperature
        frequency_hz: System frequency (50 or 60 Hz)
        conductor_diameter_mm: Overall conductor diameter
        conductor_type: Conductor construction type

    Returns:
        AC resistance in same units as r_dc
    """
    # Skin effect factor (approximate)
    # More accurate calculation requires conductor geometry
    if conductor_type == "ACSR":
        # For standard ACSR, skin effect factor ~1.02-1.10 at 50/60 Hz
        k_skin = 1.0 + 0.001 * frequency_hz * (conductor_diameter_mm / 25) ** 2
    else:
        k_skin = 1.0 + 0.0005 * frequency_hz * (conductor_diameter_mm / 25) ** 2

    # Proximity effect negligible for single conductor
    k_proximity = 1.0

    return r_dc * k_skin * k_proximity
```

### Temperature Coefficients

| Material | α (1/°C) | Reference |
|----------|----------|-----------|
| Aluminum (1350-H19) | 0.00403 | IEC 60889 |
| Aluminum alloy (6201) | 0.00347 | IEC 60104 |
| Copper (annealed) | 0.00393 | IEC 60028 |
| Steel (galvanized) | 0.0045 | Typical |

## Catenary Calculations

### Basic Catenary Equation

The shape of a conductor suspended between two points:

```
y(x) = c × cosh(x/c) - c

Where:
  c = H / w  (catenary constant)
  H = horizontal tension (N)
  w = weight per unit length (N/m)
  x = horizontal distance from lowest point
  y = vertical distance from lowest point
```

### Sag Calculation

```python
import math

def calculate_sag(
    span_length_m: float,
    horizontal_tension_n: float,
    weight_per_meter_n: float,
    use_parabolic: bool = True
) -> float:
    """Calculate conductor sag at mid-span.

    Args:
        span_length_m: Horizontal span length
        horizontal_tension_n: Horizontal component of tension
        weight_per_meter_n: Conductor weight per unit length (including ice if applicable)
        use_parabolic: Use parabolic approximation (valid for sag/span < 0.1)

    Returns:
        Sag in meters at mid-span
    """
    c = horizontal_tension_n / weight_per_meter_n  # Catenary constant

    if use_parabolic:
        # Parabolic approximation: D = wL²/(8H)
        sag = weight_per_meter_n * span_length_m ** 2 / (8 * horizontal_tension_n)
    else:
        # Exact catenary: D = c × (cosh(L/2c) - 1)
        sag = c * (math.cosh(span_length_m / (2 * c)) - 1)

    return sag


def sag_at_position(
    x_from_center_m: float,
    span_length_m: float,
    horizontal_tension_n: float,
    weight_per_meter_n: float
) -> float:
    """Calculate sag at any position along span.

    Args:
        x_from_center_m: Distance from mid-span (positive either direction)
        span_length_m: Total span length
        horizontal_tension_n: Horizontal tension
        weight_per_meter_n: Weight per meter

    Returns:
        Sag at specified position (0 at supports, max at center)
    """
    c = horizontal_tension_n / weight_per_meter_n
    half_span = span_length_m / 2

    # Sag relative to support level
    y_center = c * (math.cosh(half_span / c) - 1)  # Max sag
    y_at_x = c * (math.cosh(abs(x_from_center_m) / c) - 1)

    return y_center - y_at_x + y_center  # Sag from support level
```

### Conductor Length

```python
def conductor_length(
    span_length_m: float,
    sag_m: float,
    use_parabolic: bool = True
) -> float:
    """Calculate conductor length for given span and sag.

    Args:
        span_length_m: Horizontal span length
        sag_m: Mid-span sag
        use_parabolic: Use parabolic approximation

    Returns:
        Conductor arc length in meters
    """
    if use_parabolic:
        # L_arc ≈ L × (1 + 8D²/(3L²))
        return span_length_m * (1 + 8 * sag_m ** 2 / (3 * span_length_m ** 2))
    else:
        # Exact catenary requires numerical integration
        # or: L_arc = 2c × sinh(L/(2c))
        c = span_length_m ** 2 / (8 * sag_m)  # Approximate c from parabola
        return 2 * c * math.sinh(span_length_m / (2 * c))
```

## Change of State

Calculating sag at different temperatures/loads:

### Ruling Span Method

```python
def ruling_span(spans: list[float]) -> float:
    """Calculate equivalent ruling span for a line section.

    Used for change-of-state calculations on multi-span sections.

    Args:
        spans: List of span lengths in meters

    Returns:
        Ruling span in meters
    """
    sum_cubed = sum(s ** 3 for s in spans)
    sum_linear = sum(spans)
    return math.sqrt(sum_cubed / sum_linear)
```

### State Change Equation

```python
def change_of_state(
    initial_tension_n: float,
    initial_temp_c: float,
    final_temp_c: float,
    span_length_m: float,
    conductor: ConductorProperties,
    alpha_expansion: float = 23e-6  # Aluminum, 1/°C
) -> float:
    """Calculate tension at new temperature using catenary equation of state.

    Solves the cubic state equation:
    H2³ + a×H2² + b×H2 + c = 0

    Args:
        initial_tension_n: Initial horizontal tension
        initial_temp_c: Initial temperature
        final_temp_c: Final temperature
        span_length_m: Span length
        conductor: Conductor mechanical properties
        alpha_expansion: Coefficient of thermal expansion

    Returns:
        Final horizontal tension in Newtons
    """
    # Weight per unit length
    w = conductor.weight_per_meter_n

    # Cross-sectional area and modulus
    a = conductor.area_m2
    e = conductor.modulus_pa

    # Temperature change
    delta_t = final_temp_c - initial_temp_c

    # State equation coefficients
    # ... complex derivation omitted for brevity

    # Solve cubic equation
    # Return positive real root
    pass  # See CIGRE TB 601, Section 6 for full derivation
```

## Transmission Line Parameters

### Positive Sequence Impedance

```python
def positive_sequence_impedance(
    r_ac_ohm_per_km: float,
    gmr_m: float,
    gmd_m: float,
    frequency_hz: float = 60
) -> complex:
    """Calculate positive sequence impedance per unit length.

    Z1 = R + jωL
    L = (μ₀/2π) × ln(GMD/GMR)

    Args:
        r_ac_ohm_per_km: AC resistance
        gmr_m: Geometric Mean Radius of conductor (or bundle)
        gmd_m: Geometric Mean Distance between phases
        frequency_hz: System frequency

    Returns:
        Complex impedance in Ω/km
    """
    mu_0 = 4 * math.pi * 1e-7  # H/m

    # Inductance per meter
    l_per_m = (mu_0 / (2 * math.pi)) * math.log(gmd_m / gmr_m)

    # Inductive reactance per km
    omega = 2 * math.pi * frequency_hz
    x_l = omega * l_per_m * 1000  # Convert to per km

    return complex(r_ac_ohm_per_km, x_l)
```

### Bundle GMR

```python
def bundle_gmr(
    single_gmr_m: float,
    bundle_count: int,
    bundle_spacing_m: float
) -> float:
    """Calculate equivalent GMR for bundled conductors.

    GMR_bundle = (GMR × d^(n-1))^(1/n)

    Args:
        single_gmr_m: GMR of single subconductor
        bundle_count: Number of subconductors (2, 3, 4, etc.)
        bundle_spacing_m: Spacing between subconductors

    Returns:
        Equivalent bundle GMR in meters
    """
    if bundle_count == 1:
        return single_gmr_m

    # d = spacing for 2-bundle, or radius for 3+ (regular polygon)
    d = bundle_spacing_m

    return (single_gmr_m * d ** (bundle_count - 1)) ** (1 / bundle_count)
```

## Pi-Model Parameters

For transmission line modeling:

```python
@dataclass
class TransmissionLinePiModel:
    """Pi-model parameters for transmission line segment."""

    resistance_ohm: float       # Series resistance
    reactance_ohm: float        # Series reactance
    susceptance_siemens: float  # Shunt susceptance (total, split Y/2 each end)
    conductance_siemens: float  # Shunt conductance (usually negligible)

    @classmethod
    def from_line_parameters(
        cls,
        length_km: float,
        r_per_km: float,
        x_per_km: float,
        b_per_km: float,
        g_per_km: float = 0
    ) -> "TransmissionLinePiModel":
        """Create Pi-model from per-unit-length parameters."""
        return cls(
            resistance_ohm=r_per_km * length_km,
            reactance_ohm=x_per_km * length_km,
            susceptance_siemens=b_per_km * length_km,
            conductance_siemens=g_per_km * length_km
        )
```

## Physical Constants

```python
# Permeability of free space
MU_0 = 4 * math.pi * 1e-7  # H/m

# Permittivity of free space
EPSILON_0 = 8.854e-12  # F/m

# Standard gravity
G = 9.80665  # m/s²

# Speed of light
C = 299792458  # m/s
```
