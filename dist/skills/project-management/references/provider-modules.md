# Provider Modules

A provider module maps the stable `project-management/v1` semantic contract onto one tracker through a connection the harness already exposes. Adding a provider never changes the Flow ceremonies or gives Loaf a provider client.

## Module Shape

Each module is one ordinary skill directory:

```text
skills/<provider>/
├── SKILL.md
├── capabilities.json
└── references/            # optional, Markdown only
```

The directory and frontmatter names use the provider slug. `capabilities.json` uses schema `loaf-provider-capabilities/v1`, binds to `project-management/v1`, declares `connection: harness-native`, and maps every common operation. The module must be independently addable; core Flow manifests, ceremony skills, and templates do not list provider names.

## Capability Semantics

For every common operation, declare:

- the exact native semantic, such as a GitHub Issue, GitLab work item, or Gitea issue;
- `availability: runtime` when the harness connection may expose it, or `availability: unsupported` when the provider has no faithful native behavior;
- the maximum honest fidelity: `exact`, `advisory`, `manual`, or `unsupported`;
- ordered `before`, `execute`, and `after` capabilities for reads, mutation, and authoritative readback.

An unavailable native concept stays `unsupported`. Do not emulate hierarchy in comments, encode workflow state in labels and call it exact, or silently flatten provider semantics. A provider may use a truthful advisory mapping only when the result envelope reports that fidelity.

## Connection Boundary

The module discovers only connections already exposed by the harness. It never installs or authenticates a connector, requests provider credentials, calls provider HTTP APIs itself, proxies traffic through Loaf, or stores ongoing local-to-native mappings.

## Contributor Verification

Provider validation is data-driven. A synthetic conforming provider must pass without editing `flow-contract.json` or any ceremony skill. Validation rejects unknown operation IDs, omitted common operations, overstated fidelity, provider-name mismatches, missing runtime discovery, and mutation mappings without authoritative readback.
