#!/usr/bin/env python3
"""
Validate physics values against known physical bounds.
Usage: validate-bounds.py --type <type> --value <value> [--unit <unit>]
       validate-bounds.py --check-all <json-file>

Types: conductor_temp, ambient_temp, wind_speed, solar_radiation, current, flux_density
"""

import argparse
import json
import sys
from dataclasses import dataclass
from typing import Optional


@dataclass
class Bounds:
    min_val: float
    max_val: float
    unit: str
    description: str


# Physical validation bounds from CIGRE/IEEE standards
BOUNDS = {
    'conductor_temp': Bounds(-40.0, 250.0, '°C', 'Conductor material limits'),
    'ambient_temp': Bounds(-50.0, 60.0, '°C', 'Operational temperature range'),
    'wind_speed': Bounds(0.0, 50.0, 'm/s', 'Extreme weather limit'),
    'solar_radiation': Bounds(0.0, 1400.0, 'W/m²', 'Max solar constant'),
    'current': Bounds(0.0, float('inf'), 'A', 'Must be non-negative'),
    'flux_density': Bounds(0.0, 2.0, 'T', 'Steel saturation limit'),
    'resistance': Bounds(0.0, float('inf'), 'Ω/km', 'Must be non-negative'),
    'sag': Bounds(0.0, 500.0, 'm', 'Reasonable sag range'),
    'tension': Bounds(0.0, 500000.0, 'N', 'Reasonable tension range'),
    'span_length': Bounds(10.0, 2000.0, 'm', 'Typical span range'),
}


def celsius_to_kelvin(c: float) -> float:
    return c + 273.15


def kelvin_to_celsius(k: float) -> float:
    return k - 273.15


def validate_value(param_type: str, value: float, unit: Optional[str] = None) -> tuple[bool, str]:
    """
    Validate a single value against bounds.
    Returns (is_valid, message)
    """
    if param_type not in BOUNDS:
        return False, f"Unknown parameter type: {param_type}. Valid types: {', '.join(BOUNDS.keys())}"

    bounds = BOUNDS[param_type]

    # Handle unit conversion for temperature
    if param_type in ['conductor_temp', 'ambient_temp'] and unit and unit.upper() == 'K':
        value = kelvin_to_celsius(value)

    if value < bounds.min_val:
        return False, f"{param_type} = {value} {bounds.unit} is below minimum ({bounds.min_val} {bounds.unit})"

    if value > bounds.max_val:
        return False, f"{param_type} = {value} {bounds.unit} exceeds maximum ({bounds.max_val} {bounds.unit})"

    return True, f"{param_type} = {value} {bounds.unit} is within valid range [{bounds.min_val}, {bounds.max_val}]"


def check_all_from_json(filepath: str) -> list[tuple[bool, str]]:
    """
    Validate all values from a JSON file.
    Expected format: {"conductor_temp": 85.0, "ambient_temp": 25.0, ...}
    """
    results = []

    try:
        with open(filepath) as f:
            data = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError) as e:
        return [(False, f"Error reading JSON file: {e}")]

    for param_type, value in data.items():
        if isinstance(value, dict):
            # Handle {"value": 85.0, "unit": "C"}
            val = value.get('value', 0)
            unit = value.get('unit')
            results.append(validate_value(param_type, val, unit))
        else:
            results.append(validate_value(param_type, value))

    return results


def print_bounds_table():
    """Print all bounds as a reference table."""
    print("Physical Validation Bounds Reference")
    print("=" * 70)
    print(f"{'Parameter':<20} {'Min':<12} {'Max':<12} {'Unit':<10} {'Description'}")
    print("-" * 70)
    for name, b in BOUNDS.items():
        max_str = str(b.max_val) if b.max_val != float('inf') else '∞'
        print(f"{name:<20} {b.min_val:<12} {max_str:<12} {b.unit:<10} {b.description}")


def main():
    parser = argparse.ArgumentParser(description='Validate physics values against known bounds')
    parser.add_argument('--type', '-t', help='Parameter type to validate')
    parser.add_argument('--value', '-v', type=float, help='Value to validate')
    parser.add_argument('--unit', '-u', help='Unit (e.g., C, K, m/s)')
    parser.add_argument('--check-all', '-c', help='JSON file with multiple values to check')
    parser.add_argument('--list-bounds', '-l', action='store_true', help='List all bounds')

    args = parser.parse_args()

    if args.list_bounds:
        print_bounds_table()
        sys.exit(0)

    if args.check_all:
        results = check_all_from_json(args.check_all)
        all_valid = True
        for is_valid, msg in results:
            status = "✓" if is_valid else "✗"
            print(f"{status} {msg}")
            if not is_valid:
                all_valid = False
        sys.exit(0 if all_valid else 1)

    if args.type and args.value is not None:
        is_valid, msg = validate_value(args.type, args.value, args.unit)
        status = "✓ VALID" if is_valid else "✗ INVALID"
        print(f"{status}: {msg}")
        sys.exit(0 if is_valid else 1)

    parser.print_help()
    sys.exit(1)


if __name__ == '__main__':
    main()
