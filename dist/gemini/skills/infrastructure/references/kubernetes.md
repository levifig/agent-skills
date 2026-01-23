# Kubernetes Patterns

Best practices for Kubernetes deployments, services, and configuration.

## Pod Security Context

```yaml
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    runAsGroup: 1000
    fsGroup: 1000
  containers:
    - name: app
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities:
          drop: [ALL]
```

| Setting | Purpose |
|---------|---------|
| `runAsNonRoot: true` | Prevents running as root |
| `runAsUser: 1000` | Matches Dockerfile user |
| `allowPrivilegeEscalation: false` | Prevents privilege escalation |
| `readOnlyRootFilesystem: true` | Prevents filesystem modifications |
| `capabilities.drop: ALL` | Removes all Linux capabilities |

## Resource Management

```yaml
resources:
  requests:
    memory: "256Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"
```

| Workload Type | Memory Request | Memory Limit | CPU Request | CPU Limit |
|---------------|----------------|--------------|-------------|-----------|
| API Service | 256Mi | 512Mi | 100m | 500m |
| Background Worker | 512Mi | 1Gi | 200m | 1000m |
| Batch Job | 1Gi | 2Gi | 500m | 2000m |

**QoS Classes:** Guaranteed (requests==limits) > Burstable (requests<limits) > BestEffort (none)

## Health Probes

```yaml
livenessProbe:        # Restart if fails
  httpGet: {path: /health, port: 8000}
  initialDelaySeconds: 10
  periodSeconds: 30
  failureThreshold: 3

readinessProbe:       # Remove from service if fails
  httpGet: {path: /ready, port: 8000}
  initialDelaySeconds: 5
  periodSeconds: 10

startupProbe:         # For slow-starting apps
  httpGet: {path: /health, port: 8000}
  failureThreshold: 30
  periodSeconds: 5     # 30 * 5s = 150s max startup
```

## ConfigMaps and Secrets

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  LOG_LEVEL: "INFO"
  config.yaml: |
    database:
      pool_size: 10
---
apiVersion: v1
kind: Secret
metadata:
  name: app-secrets
type: Opaque
stringData:
  DATABASE_URL: "postgres://user:pass@host/db"
```

**Usage in pods:**

```yaml
envFrom:
  - configMapRef: {name: app-config}
  - secretRef: {name: app-secrets}
```

## Service Types

| Type | Use Case |
|------|----------|
| `ClusterIP` | Internal services (default) |
| `NodePort` | Dev/test external access |
| `LoadBalancer` | Production external access |

```yaml
apiVersion: v1
kind: Service
spec:
  type: ClusterIP
  selector: {app: app}
  ports:
    - port: 80
      targetPort: 8000
```

## Deployment Strategies

```yaml
# Rolling Update (default)
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 25%
      maxSurge: 25%
```

## Horizontal Pod Autoscaling

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: app
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target: {type: Utilization, averageUtilization: 70}
```

## Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
spec:
  podSelector:
    matchLabels: {app: app}
  policyTypes: [Ingress, Egress]
  ingress:
    - from:
        - podSelector: {matchLabels: {app: frontend}}
      ports: [{port: 8000}]
```

## StatefulSets vs Deployments

**Use StatefulSet when:**
- Stable network identity needed (pod-0, pod-1)
- Persistent storage per pod
- Ordered deployment/scaling
- Examples: databases, message queues

**Use Deployment when:**
- Stateless workloads
- Any pod can handle any request
- Examples: APIs, web servers

## Labels and Annotations

```yaml
metadata:
  labels:
    app.kubernetes.io/name: myapp
    app.kubernetes.io/component: api
    app.kubernetes.io/version: "1.0.0"
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8000"
```

## Checklist

- [ ] Security context configured
- [ ] runAsNonRoot: true
- [ ] Resource requests AND limits set
- [ ] Liveness and readiness probes defined
- [ ] Secrets not in ConfigMaps
- [ ] No `latest` image tags
- [ ] Labels follow Kubernetes conventions
