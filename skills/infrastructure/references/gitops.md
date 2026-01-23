# GitOps Patterns

GitOps workflows with ArgoCD, Kustomize, and deployment strategies.

## Core Principles

1. **Declarative** - System described declaratively in Git
2. **Versioned** - Git as single source of truth
3. **Automated** - Approved changes applied automatically
4. **Auditable** - All changes traceable through commits

## Repository Structure

```
gitops-repo/
+-- apps/
|   +-- base/                # Base configurations
|   +-- overlays/            # Environment-specific
|       +-- dev/
|       +-- prod/
+-- clusters/                # Cluster configs
+-- infrastructure/          # Shared components
```

## ArgoCD Application

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/org/repo.git
    targetRevision: main
    path: apps/overlays/prod
  destination:
    server: https://kubernetes.default.svc
    namespace: my-app
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

## Kustomize Overlay

```yaml
# apps/overlays/prod/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: my-app-prod
resources:
  - ../../base
images:
  - name: my-app
    newTag: v1.2.3
replicas:
  - name: my-app
    count: 3
```

## Helm with ArgoCD

```yaml
spec:
  source:
    repoURL: https://charts.example.com
    chart: my-chart
    targetRevision: 2.1.0
    helm:
      valueFiles:
        - values-prod.yaml
```

## Deployment Strategies

### Blue/Green

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Rollout
spec:
  strategy:
    blueGreen:
      activeService: my-app-active
      previewService: my-app-preview
      autoPromotionEnabled: false
```

### Canary

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Rollout
spec:
  strategy:
    canary:
      steps:
        - setWeight: 10
        - pause: {duration: 5m}
        - setWeight: 50
        - analysis:
            templates:
              - templateName: success-rate
        - setWeight: 100
```

## Sync Policies

| Policy | Use Case |
|--------|----------|
| `prune: true` | Remove resources not in Git |
| `selfHeal: true` | Revert manual changes |
| `ApplyOutOfSyncOnly=true` | Sync only changed resources |

## Best Practices

- Use **sealed-secrets** or **external-secrets** for sensitive data
- Use **ApplicationSets** for multi-cluster deployments
- Set **sync waves** for dependency ordering (`argocd.argoproj.io/sync-wave: "1"`)
- Configure **resource hooks** for migrations (`argocd.argoproj.io/hook: PreSync`)

## GitOps Workflow

```
Code Change --> PR --> Review --> Merge to main
                                      |
                                      v
ArgoCD detects change --> Sync --> Kubernetes
                                      |
                                      v
                              Health checks pass
```

## Checklist

- [ ] Git repo as single source of truth
- [ ] Environment-specific overlays
- [ ] Automated sync enabled
- [ ] Secrets encrypted (sealed-secrets)
- [ ] Sync waves for dependencies
- [ ] Rollback strategy defined
