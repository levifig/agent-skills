#!/usr/bin/env python3
"""Validate Kubernetes manifest for best practices.

Usage: validate-k8s-manifest.py <manifest.yaml>
"""

import sys
import re
from pathlib import Path


def validate_pod_spec(text: str, path: str) -> list[str]:
    """Validate pod spec for security and resource settings."""
    errors = []

    if not re.search(r"(?m)^\s*runAsNonRoot:\s*true\s*(?:#.*)?$", text):
        errors.append(f"{path}: Missing runAsNonRoot: true")

    if "containers:" not in text:
        return errors

    container_path = f"{path}.containers"
    for required in ["resources:", "requests:", "limits:", "livenessProbe:", "readinessProbe:"]:
        if required not in text:
            errors.append(f"{container_path}: Missing {required.rstrip(':')}")

    for image in re.findall(r"(?m)^\s*image:\s*([^\s#]+)", text):
        if ":latest" in image or ":" not in image:
            errors.append(f"{container_path}: Using :latest or no tag for image")

    if not re.search(r"(?m)^\s*allowPrivilegeEscalation:\s*false\s*(?:#.*)?$", text):
        errors.append(f"{container_path}: Missing allowPrivilegeEscalation: false")

    return errors


def field_value(text: str, field: str, default: str = "Unknown") -> str:
    match = re.search(rf"(?m)^\s*{re.escape(field)}:\s*([^\s#]+)", text)
    if not match:
        return default
    return match.group(1).strip().strip('"\'')


def secret_string_data_keys(text: str) -> list[str]:
    match = re.search(r"(?ms)^stringData:\s*\n(?P<body>(?:^[ \t]+[A-Za-z0-9_.-]+:\s*.*\n?)+)", text)
    if not match:
        return []
    return re.findall(r"(?m)^\s+([A-Za-z0-9_.-]+):", match.group("body"))


def manifest_documents(text: str) -> list[str]:
    return [doc.strip() for doc in re.split(r"(?m)^---\s*$", text) if doc.strip()]


def validate_manifest(doc: str) -> list[str]:
    """Validate a single Kubernetes manifest."""
    errors = []
    kind = field_value(doc, "kind")
    name = field_value(doc, "name", "unnamed")
    path = f"{kind}/{name}"

    if kind in {"Deployment", "StatefulSet"}:
        errors.extend(validate_pod_spec(doc, f"{path}.spec.template.spec"))
    elif kind == "CronJob":
        errors.extend(validate_pod_spec(doc, f"{path}.spec.jobTemplate.spec.template.spec"))
    elif kind == "Pod":
        errors.extend(validate_pod_spec(doc, f"{path}.spec"))
    elif kind == "Secret":
        for key in secret_string_data_keys(doc):
            if any(sensitive in key.upper() for sensitive in ["PASSWORD", "KEY", "TOKEN"]):
                errors.append(f"{path}: Sensitive data in stringData (use data with base64)")
    return errors


def main():
    if len(sys.argv) < 2:
        print("Usage: validate-k8s-manifest.py <manifest.yaml>")
        sys.exit(1)

    manifest_path = Path(sys.argv[1])
    if not manifest_path.exists():
        print(f"Error: File not found: {manifest_path}")
        sys.exit(1)

    print(f"Validating: {manifest_path}")
    print("=" * 50)

    all_errors = []

    docs = manifest_documents(manifest_path.read_text())
    for doc in docs:
        errors = validate_manifest(doc)
        all_errors.extend(errors)

    if all_errors:
        for error in all_errors:
            print(f"❌ {error}")
        print("=" * 50)
        print(f"Found {len(all_errors)} issue(s)")
        sys.exit(1)
    else:
        print("✓ All checks passed!")
        sys.exit(0)


if __name__ == "__main__":
    main()
