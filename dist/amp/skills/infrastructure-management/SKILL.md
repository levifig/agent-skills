---
name: infrastructure-management
description: >-
  Covers Docker, Kubernetes, GitOps, CI/CD pipelines, and container security.
  Use when writing Dockerfiles, configuring K8s manifests, setting up CI/CD, or
  managing deployments. Provides patterns for infrastructure as code. Not for
  application code (use development skills), database schema (use
  database-design), or security audits (use security-compliance).
version: 2.0.0-dev.22
---

# Infrastructure

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Available Scripts
- CI Failure Triage

Infrastructure patterns for containerization, orchestration, CI/CD pipelines, and deployment automation.

## Critical Rules

### Always

- Use multi-stage builds to minimize image size
- Run containers as non-root user (UID 1000)
- Include health checks in all services
- Pin specific image versions (no `:latest`)
- Set resource requests AND limits
- Use `npm ci` / `pip-sync` in CI (not install)
- Commit lockfiles to version control

### Never

- Commit secrets to version control
- Use `:latest` tags in production
- Skip security scanning in CI
- Deploy without rollback capability
- Store state in containers
- Run as root in production

## Verification

### After Editing Infrastructure Files

**Kubernetes Manifests:**
- If `kubectl` is available, validate with dry-run:
  - Client-side: `kubectl apply --dry-run=client -f {manifest}`
  - Server-side (if cluster access): `kubectl apply --dry-run=server -f {manifest}`
- Check for common issues:
  - Missing resource limits/requests (CPU, memory)
  - Missing liveness/readiness probes
  - Using `:latest` tag (prefer specific versions)
  - Privileged containers (security concern)
  - No namespace specified
- If `kubesec` is available: `kubesec scan {manifest}`

**Dockerfiles:**
- Check for:
  - Non-root USER specified
  - Specific image versions (no `:latest`)
  - HEALTHCHECK instruction
  - No secrets in ENV instructions
  - Using COPY instead of ADD for local files

**Terraform Files:**
- If `terraform` is available:
  - Check format: `terraform fmt -check -recursive .`
  - Validate: `terraform validate`
  - Plan: `terraform plan -out=/tmp/tfplan`
- If `infracost` is available: `infracost breakdown --path /tmp/tfplan`
- If `tfsec` is available: `tfsec .`

### Before Committing

- Validate Kubernetes manifests with kubectl dry-run
- Check Dockerfile best practices
- Run terraform fmt and validate if applicable
- Review security scan results
- Ensure no secrets are committed

## Quick Reference

| Layer | Technologies |
|-------|--------------|
| Containers | Docker, BuildKit, multi-stage builds |
| Orchestration | Kubernetes, Helm, Kustomize |
| GitOps | ArgoCD, Flux, Argo Rollouts |
| CI/CD | GitHub Actions, GitLab CI |
| Registries | GHCR, ECR, GCR, DockerHub |

## Topics

| Topic | Reference File | Use When |
|-------|----------------|----------|
| Docker | `references/docker.md` | Writing Dockerfiles, optimizing builds, adding health checks |
| Kubernetes | `references/kubernetes.md` | Creating deployments, services, probes, resource limits |
| GitOps | `references/gitops.md` | Setting up ArgoCD, Kustomize, sync policies |
| CI/CD | `references/ci-cd.md` | Building GitHub Actions workflows, caching, secrets |
| Troubleshooting | `references/troubleshooting.md` | Debugging CI failures, version conflicts, cache issues |

## Available Scripts

| Script | Usage | Description |
|--------|-------|-------------|
| `scripts/check-dockerfile.sh` | `check-dockerfile.sh <file>` | Validate Dockerfile best practices |
| `scripts/validate-k8s-manifest.py` | `validate-k8s-manifest.py <file>` | Check K8s manifest for required fields |

## CI Failure Triage

```
CI Failed
+-- Same code passes locally?
|   +-- YES --> Check environment differences
|   |   +-- Python/Node version
|   |   +-- Environment variables
|   |   +-- File permissions
|   |   +-- Installed dependencies
|   +-- NO --> Fix the actual bug
+-- Flaky (sometimes passes)?
|   +-- Check for race conditions, shared state, timeouts
+-- Always fails in CI?
    +-- Check runner resources (memory, timeout)
    +-- Check external service access
    +-- Check CI-specific config
```
