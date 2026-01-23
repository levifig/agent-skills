#!/usr/bin/env python3
"""Validate Kubernetes manifest for best practices.

Usage: validate-k8s-manifest.py <manifest.yaml>
"""

import sys
from pathlib import Path

import yaml


def validate_pod_spec(spec: dict, path: str) -> list[str]:
    """Validate pod spec for security and resource settings."""
    errors = []

    # Check security context
    security_context = spec.get("securityContext", {})
    if not security_context.get("runAsNonRoot"):
        errors.append(f"{path}: Missing runAsNonRoot: true")

    # Check containers
    containers = spec.get("containers", [])
    for i, container in enumerate(containers):
        container_path = f"{path}.containers[{i}]"

        # Check resources
        resources = container.get("resources", {})
        if not resources.get("requests"):
            errors.append(f"{container_path}: Missing resource requests")
        if not resources.get("limits"):
            errors.append(f"{container_path}: Missing resource limits")

        # Check probes
        if not container.get("livenessProbe"):
            errors.append(f"{container_path}: Missing livenessProbe")
        if not container.get("readinessProbe"):
            errors.append(f"{container_path}: Missing readinessProbe")

        # Check image tag
        image = container.get("image", "")
        if ":latest" in image or ":" not in image:
            errors.append(f"{container_path}: Using :latest or no tag for image")

        # Check container security context
        container_security = container.get("securityContext", {})
        if container_security.get("allowPrivilegeEscalation") is not False:
            errors.append(f"{container_path}: Missing allowPrivilegeEscalation: false")

    return errors


def validate_manifest(doc: dict) -> list[str]:
    """Validate a single Kubernetes manifest."""
    errors = []
    kind = doc.get("kind", "Unknown")
    name = doc.get("metadata", {}).get("name", "unnamed")
    path = f"{kind}/{name}"

    # Validate based on kind
    if kind == "Deployment":
        spec = doc.get("spec", {}).get("template", {}).get("spec", {})
        errors.extend(validate_pod_spec(spec, f"{path}.spec.template.spec"))

    elif kind == "StatefulSet":
        spec = doc.get("spec", {}).get("template", {}).get("spec", {})
        errors.extend(validate_pod_spec(spec, f"{path}.spec.template.spec"))

    elif kind == "CronJob":
        spec = (
            doc.get("spec", {})
            .get("jobTemplate", {})
            .get("spec", {})
            .get("template", {})
            .get("spec", {})
        )
        errors.extend(validate_pod_spec(spec, f"{path}.spec.jobTemplate.spec.template.spec"))

    elif kind == "Pod":
        spec = doc.get("spec", {})
        errors.extend(validate_pod_spec(spec, f"{path}.spec"))

    elif kind == "Secret":
        # Check if data contains sensitive keys in stringData (would be visible)
        string_data = doc.get("stringData", {})
        for key in string_data:
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

    with open(manifest_path) as f:
        docs = list(yaml.safe_load_all(f))

    for doc in docs:
        if doc is None:
            continue
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
