#!/usr/bin/env python3
"""
Unit conversion utilities for power systems calculations.
Usage: convert-units.py <value> <from-unit> <to-unit>
       convert-units.py --list

Supports: temperature, length, resistance, power, current, voltage
"""

import argparse
import sys
from typing import Callable


# Conversion functions
# Temperature: C, K, F
def c_to_k(c: float) -> float:
    return c + 273.15

def k_to_c(k: float) -> float:
    return k - 273.15

def c_to_f(c: float) -> float:
    return c * 9/5 + 32

def f_to_c(f: float) -> float:
    return (f - 32) * 5/9

# Length: m, ft, km, mi
def m_to_ft(m: float) -> float:
    return m * 3.28084

def ft_to_m(ft: float) -> float:
    return ft / 3.28084

def m_to_km(m: float) -> float:
    return m / 1000

def km_to_m(km: float) -> float:
    return km * 1000

def km_to_mi(km: float) -> float:
    return km * 0.621371

def mi_to_km(mi: float) -> float:
    return mi / 0.621371

# Resistance: ohm/km, ohm/mi, ohm/1000ft
def ohm_km_to_ohm_mi(r: float) -> float:
    return r * 1.60934

def ohm_mi_to_ohm_km(r: float) -> float:
    return r / 1.60934

def ohm_km_to_ohm_kft(r: float) -> float:
    return r * 0.3048

def ohm_kft_to_ohm_km(r: float) -> float:
    return r / 0.3048

# Power: W, kW, MW, hp
def w_to_kw(w: float) -> float:
    return w / 1000

def kw_to_w(kw: float) -> float:
    return kw * 1000

def kw_to_mw(kw: float) -> float:
    return kw / 1000

def mw_to_kw(mw: float) -> float:
    return mw * 1000

def kw_to_hp(kw: float) -> float:
    return kw * 1.34102

def hp_to_kw(hp: float) -> float:
    return hp / 1.34102

# Speed: m/s, ft/s, km/h, mph
def ms_to_fts(ms: float) -> float:
    return ms * 3.28084

def fts_to_ms(fts: float) -> float:
    return fts / 3.28084

def ms_to_kmh(ms: float) -> float:
    return ms * 3.6

def kmh_to_ms(kmh: float) -> float:
    return kmh / 3.6

def kmh_to_mph(kmh: float) -> float:
    return kmh * 0.621371

def mph_to_kmh(mph: float) -> float:
    return mph / 0.621371


# Conversion registry: (from_unit, to_unit) -> function
CONVERSIONS: dict[tuple[str, str], Callable[[float], float]] = {
    # Temperature
    ('C', 'K'): c_to_k,
    ('K', 'C'): k_to_c,
    ('C', 'F'): c_to_f,
    ('F', 'C'): f_to_c,
    ('K', 'F'): lambda k: c_to_f(k_to_c(k)),
    ('F', 'K'): lambda f: c_to_k(f_to_c(f)),

    # Length
    ('m', 'ft'): m_to_ft,
    ('ft', 'm'): ft_to_m,
    ('m', 'km'): m_to_km,
    ('km', 'm'): km_to_m,
    ('km', 'mi'): km_to_mi,
    ('mi', 'km'): mi_to_km,
    ('m', 'mi'): lambda m: km_to_mi(m_to_km(m)),
    ('mi', 'm'): lambda mi: km_to_m(mi_to_km(mi)),

    # Resistance
    ('ohm/km', 'ohm/mi'): ohm_km_to_ohm_mi,
    ('ohm/mi', 'ohm/km'): ohm_mi_to_ohm_km,
    ('ohm/km', 'ohm/kft'): ohm_km_to_ohm_kft,
    ('ohm/kft', 'ohm/km'): ohm_kft_to_ohm_km,

    # Power
    ('W', 'kW'): w_to_kw,
    ('kW', 'W'): kw_to_w,
    ('kW', 'MW'): kw_to_mw,
    ('MW', 'kW'): mw_to_kw,
    ('kW', 'hp'): kw_to_hp,
    ('hp', 'kW'): hp_to_kw,
    ('W', 'MW'): lambda w: kw_to_mw(w_to_kw(w)),
    ('MW', 'W'): lambda mw: kw_to_w(mw_to_kw(mw)),

    # Speed
    ('m/s', 'ft/s'): ms_to_fts,
    ('ft/s', 'm/s'): fts_to_ms,
    ('m/s', 'km/h'): ms_to_kmh,
    ('km/h', 'm/s'): kmh_to_ms,
    ('km/h', 'mph'): kmh_to_mph,
    ('mph', 'km/h'): mph_to_kmh,
    ('m/s', 'mph'): lambda ms: kmh_to_mph(ms_to_kmh(ms)),
    ('mph', 'm/s'): lambda mph: kmh_to_ms(mph_to_kmh(mph)),
}


def normalize_unit(unit: str) -> str:
    """Normalize unit names to standard form."""
    unit = unit.strip()

    # Common aliases
    aliases = {
        'celsius': 'C', '°c': 'C', 'degc': 'C',
        'kelvin': 'K', '°k': 'K', 'degk': 'K',
        'fahrenheit': 'F', '°f': 'F', 'degf': 'F',
        'meter': 'm', 'meters': 'm', 'metre': 'm',
        'feet': 'ft', 'foot': 'ft',
        'kilometer': 'km', 'kilometers': 'km',
        'mile': 'mi', 'miles': 'mi',
        'watt': 'W', 'watts': 'W',
        'kilowatt': 'kW', 'kilowatts': 'kW',
        'megawatt': 'MW', 'megawatts': 'MW',
        'horsepower': 'hp',
    }

    return aliases.get(unit.lower(), unit)


def convert(value: float, from_unit: str, to_unit: str) -> tuple[float, str]:
    """
    Convert a value between units.
    Returns (converted_value, message)
    """
    from_unit = normalize_unit(from_unit)
    to_unit = normalize_unit(to_unit)

    if from_unit == to_unit:
        return value, f"{value} {from_unit} = {value} {to_unit} (same unit)"

    key = (from_unit, to_unit)
    if key not in CONVERSIONS:
        return 0, f"Unknown conversion: {from_unit} -> {to_unit}"

    result = CONVERSIONS[key](value)
    return result, f"{value} {from_unit} = {result:.6g} {to_unit}"


def list_conversions():
    """Print all available conversions."""
    print("Available Unit Conversions")
    print("=" * 50)

    categories = {
        'Temperature': ['C', 'K', 'F'],
        'Length': ['m', 'ft', 'km', 'mi'],
        'Resistance': ['ohm/km', 'ohm/mi', 'ohm/kft'],
        'Power': ['W', 'kW', 'MW', 'hp'],
        'Speed': ['m/s', 'ft/s', 'km/h', 'mph'],
    }

    for category, units in categories.items():
        print(f"\n{category}:")
        print(f"  Units: {', '.join(units)}")


def main():
    parser = argparse.ArgumentParser(description='Convert between units for power systems')
    parser.add_argument('value', nargs='?', type=float, help='Value to convert')
    parser.add_argument('from_unit', nargs='?', help='Source unit')
    parser.add_argument('to_unit', nargs='?', help='Target unit')
    parser.add_argument('--list', '-l', action='store_true', help='List available conversions')

    args = parser.parse_args()

    if args.list:
        list_conversions()
        sys.exit(0)

    if args.value is not None and args.from_unit and args.to_unit:
        result, msg = convert(args.value, args.from_unit, args.to_unit)
        print(msg)
        sys.exit(0 if 'Unknown' not in msg else 1)

    parser.print_help()
    sys.exit(1)


if __name__ == '__main__':
    main()
