# Thermal Models Reference

Heat balance calculations for overhead conductors per CIGRE TB 601 and IEEE 738.

## Heat Balance Equation

The steady-state conductor temperature is determined by:

```
P_J + P_solar = P_convection + P_radiation
```

Where:
- **P_J**: Joule heating (I²R losses)
- **P_solar**: Solar heat gain
- **P_convection**: Convective heat loss (natural + forced)
- **P_radiation**: Radiative heat loss

## CIGRE TB 601 Implementation

### Joule Heating (Section 3.2)

```python
def joule_heating_per_meter(
    current_a: float,
    resistance_ohm_per_m: float,
    temperature_c: float,
    alpha: float = 0.00403  # Aluminum temperature coefficient
) -> float:
    """Calculate Joule heating per unit length.

    CIGRE TB 601, Section 3.2

    Args:
        current_a: RMS current in Amperes
        resistance_ohm_per_m: DC resistance at 20°C per meter
        temperature_c: Conductor temperature in Celsius
        alpha: Temperature coefficient of resistance

    Returns:
        Joule heating in W/m
    """
    # Temperature-adjusted resistance
    r_t = resistance_ohm_per_m * (1 + alpha * (temperature_c - 20))
    return current_a ** 2 * r_t
```

### Solar Heat Gain (Section 3.3)

```python
def solar_heat_gain_per_meter(
    solar_radiation_wm2: float,
    diameter_m: float,
    absorptivity: float = 0.9
) -> float:
    """Calculate solar heat gain per unit length.

    CIGRE TB 601, Section 3.3

    Args:
        solar_radiation_wm2: Global solar radiation (0-1400 W/m²)
        diameter_m: Conductor overall diameter
        absorptivity: Solar absorptivity (typically 0.8-0.95)

    Returns:
        Solar heat gain in W/m
    """
    return absorptivity * solar_radiation_wm2 * diameter_m
```

### Forced Convection (Section 4.2.2)

```python
def forced_convection_loss_per_meter(
    wind_speed_mps: float,
    wind_angle_deg: float,
    conductor_temp_c: float,
    ambient_temp_c: float,
    diameter_m: float,
) -> float:
    """Calculate forced convection heat loss per unit length.

    CIGRE TB 601, Section 4.2.2
    Uses Nusselt number correlation for cylinder in crossflow.

    Args:
        wind_speed_mps: Wind speed in m/s
        wind_angle_deg: Angle between wind and conductor (0-90°)
        conductor_temp_c: Conductor surface temperature
        ambient_temp_c: Ambient air temperature
        diameter_m: Conductor diameter

    Returns:
        Convective heat loss in W/m
    """
    # Film temperature for property evaluation
    t_film = (conductor_temp_c + ambient_temp_c) / 2 + 273.15  # Kelvin

    # Air properties at film temperature
    rho = air_density(t_film)          # kg/m³
    mu = air_viscosity(t_film)         # Pa·s
    k = air_thermal_conductivity(t_film)  # W/(m·K)

    # Reynolds number
    re = rho * wind_speed_mps * diameter_m / mu

    # Nusselt number (CIGRE correlation)
    # B1, n1 depend on Reynolds number range
    if re < 2650:
        b1, n1 = 0.641, 0.471
    else:
        b1, n1 = 0.048, 0.800

    nu = b1 * re ** n1

    # Wind angle correction
    k_angle = 1.194 - math.cos(math.radians(wind_angle_deg)) + \
              0.194 * math.cos(math.radians(2 * wind_angle_deg)) + \
              0.368 * math.sin(math.radians(2 * wind_angle_deg))

    # Heat loss
    delta_t = conductor_temp_c - ambient_temp_c
    return math.pi * k * nu * k_angle * delta_t
```

### Natural Convection (Section 4.2.3)

```python
def natural_convection_loss_per_meter(
    conductor_temp_c: float,
    ambient_temp_c: float,
    diameter_m: float,
    elevation_m: float = 0
) -> float:
    """Calculate natural convection heat loss per unit length.

    CIGRE TB 601, Section 4.2.3
    Used when wind speed < 0.5 m/s.

    Note: IEEE 738 differs in low wind speed treatment.
    """
    # Grashof number
    delta_t = conductor_temp_c - ambient_temp_c
    t_film = (conductor_temp_c + ambient_temp_c) / 2 + 273.15

    g = 9.81  # m/s²
    beta = 1 / t_film  # Ideal gas approximation
    nu = air_kinematic_viscosity(t_film)

    gr = g * beta * abs(delta_t) * diameter_m ** 3 / nu ** 2

    # Prandtl number (≈ 0.71 for air)
    pr = 0.71

    # Rayleigh number
    ra = gr * pr

    # Nusselt number correlation (horizontal cylinder)
    nu_nat = 0.60 + 0.387 * ra ** (1/6) / (1 + (0.559 / pr) ** (9/16)) ** (8/27)

    k = air_thermal_conductivity(t_film)
    return math.pi * k * nu_nat * delta_t
```

### Radiative Heat Loss (Section 4.3)

```python
def radiative_heat_loss_per_meter(
    conductor_temp_c: float,
    ambient_temp_c: float,
    diameter_m: float,
    emissivity: float = 0.9
) -> float:
    """Calculate radiative heat loss per unit length.

    CIGRE TB 601, Section 4.3
    Stefan-Boltzmann law for gray body radiation.

    Args:
        conductor_temp_c: Conductor surface temperature
        ambient_temp_c: Ambient/sky temperature
        diameter_m: Conductor diameter
        emissivity: Surface emissivity (typically 0.8-0.95)

    Returns:
        Radiative heat loss in W/m
    """
    sigma = 5.67e-8  # Stefan-Boltzmann constant, W/(m²·K⁴)
    t_s = conductor_temp_c + 273.15  # Kelvin
    t_a = ambient_temp_c + 273.15    # Kelvin

    return math.pi * diameter_m * emissivity * sigma * (t_s ** 4 - t_a ** 4)
```

## IEEE 738 Differences

Key differences from CIGRE TB 601:

| Aspect | CIGRE TB 601 | IEEE 738 |
|--------|--------------|----------|
| Natural convection threshold | 0.5 m/s | 0.61 m/s (2 ft/s) |
| Film temperature | Average | Ambient |
| Convection correlation | Different B1, n1 | Morgan correlation |
| Default emissivity | 0.9 | 0.5 (new conductor) |

When deviating from CIGRE:
```python
# Note: Using IEEE 738 Morgan correlation for convection
# per project specification requirement
```

## Steady-State Solution

```python
def solve_conductor_temperature(
    current_a: float,
    ambient_temp_c: float,
    wind_speed_mps: float,
    wind_angle_deg: float,
    solar_radiation_wm2: float,
    conductor: ConductorProperties,
    tolerance: float = 0.1,  # °C
    max_iterations: int = 100
) -> float:
    """Solve for steady-state conductor temperature.

    Uses Newton-Raphson iteration on heat balance equation.

    Returns:
        Conductor temperature in °C

    Raises:
        ConvergenceError: If solution doesn't converge
        ValueError: If parameters outside physical bounds
    """
    # Validate inputs
    validate_physical_bounds(ambient_temp_c, wind_speed_mps, solar_radiation_wm2)

    # Initial guess: ambient + 30°C
    t_c = ambient_temp_c + 30

    for i in range(max_iterations):
        # Heat gains
        p_joule = joule_heating_per_meter(current_a, conductor.r_dc_20c, t_c)
        p_solar = solar_heat_gain_per_meter(solar_radiation_wm2, conductor.diameter)

        # Heat losses
        if wind_speed_mps > 0.5:
            p_conv = forced_convection_loss_per_meter(...)
        else:
            p_conv = natural_convection_loss_per_meter(...)
        p_rad = radiative_heat_loss_per_meter(...)

        # Heat balance residual
        residual = (p_joule + p_solar) - (p_conv + p_rad)

        if abs(residual) < tolerance:
            return t_c

        # Newton-Raphson update
        # ... derivative calculation and update

    raise ConvergenceError(f"Temperature solution did not converge after {max_iterations} iterations")
```

## Dynamic Thermal Rating

For transient analysis (CIGRE TB 601, Chapter 5):

```python
def transient_temperature(
    initial_temp_c: float,
    current_a: float,
    ambient_conditions: List[AmbientCondition],
    conductor: ConductorProperties,
    time_step_s: float = 60
) -> List[float]:
    """Calculate transient conductor temperature.

    CIGRE TB 601, Chapter 5
    Uses thermal mass and time constant.

    Args:
        initial_temp_c: Starting conductor temperature
        current_a: Applied current
        ambient_conditions: Time series of ambient conditions
        conductor: Conductor thermal properties
        time_step_s: Time step for integration

    Returns:
        List of conductor temperatures at each time step
    """
    # Thermal mass per unit length
    m_c = conductor.thermal_mass_per_meter  # J/(m·K)

    temperatures = [initial_temp_c]
    t_c = initial_temp_c

    for ambient in ambient_conditions:
        # Steady-state target
        t_ss = solve_conductor_temperature(current_a, ambient, conductor)

        # Time constant
        tau = m_c / derivative_heat_balance(t_c, ambient, conductor)

        # Exponential approach
        t_c = t_ss + (t_c - t_ss) * math.exp(-time_step_s / tau)
        temperatures.append(t_c)

    return temperatures
```

## References

- CIGRE TB 601 (2014): "Guide for Thermal Rating Calculations of Overhead Lines"
- CIGRE TB 207 (2002): "Thermal Behaviour of Overhead Conductors"
- IEEE 738-2012: "Calculating the Current-Temperature Relationship of Bare Overhead Conductors"
