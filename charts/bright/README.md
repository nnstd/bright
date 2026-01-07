# Bright Helm Chart

This Helm chart deploys Bright, a high-performance full-text search database built with Go, Fiber, and Bleve.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+
- PV provisioner support in the underlying infrastructure (if persistence is enabled)

## Installing the Chart

To install the chart with the release name `bright`:

```bash
helm install bright ./charts/bright
```

The command deploys Bright on the Kubernetes cluster with default configuration. The [Parameters](#parameters) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `bright` deployment:

```bash
helm uninstall bright
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Parameters

### Global parameters

| Name                      | Description                                     | Value                    |
| ------------------------- | ----------------------------------------------- | ------------------------ |
| `replicaCount`           | Number of Bright replicas to deploy             | `1`                      |
| `nameOverride`           | String to partially override bright.name        | `""`                     |
| `fullnameOverride`       | String to fully override bright.fullname        | `""`                     |

### Image parameters

| Name                      | Description                                     | Value                    |
| ------------------------- | ----------------------------------------------- | ------------------------ |
| `image.registry`         | Bright image registry                           | `docker.io`              |
| `image.repository`       | Bright image repository                         | `bright/bright`          |
| `image.tag`              | Bright image tag (defaults to chart appVersion) | `""`                     |
| `image.pullPolicy`       | Bright image pull policy                        | `IfNotPresent`           |
| `imagePullSecrets`       | Docker registry secret names as an array        | `[]`                     |

### Application configuration

| Name                      | Description                                     | Value                    |
| ------------------------- | ----------------------------------------------- | ------------------------ |
| `config.port`            | Application port                                | `"3000"`                 |
| `config.masterKey`       | Master key for authentication                   | `""`                     |
| `config.logLevel`        | Log level (debug, info, warn, error)            | `"info"`                 |
| `config.dataPath`        | Data path for persistent storage                | `"/data"`                |

### Service parameters

| Name                      | Description                                     | Value                    |
| ------------------------- | ----------------------------------------------- | ------------------------ |
| `service.type`           | Kubernetes service type                         | `ClusterIP`              |
| `service.port`           | Service HTTP port                               | `3000`                   |
| `service.portName`       | Service port name                               | `http`                   |
| `service.annotations`    | Service annotations                             | `{}`                     |

### Ingress parameters

| Name                      | Description                                     | Value                    |
| ------------------------- | ----------------------------------------------- | ------------------------ |
| `ingress.enabled`        | Enable ingress controller resource              | `false`                  |
| `ingress.className`      | Ingress class name                              | `""`                     |
| `ingress.annotations`    | Ingress annotations                             | `{}`                     |
| `ingress.hosts`          | Ingress hosts configuration                     | See values.yaml          |
| `ingress.tls`            | Ingress TLS configuration                       | `[]`                     |

### Persistence parameters

| Name                           | Description                                     | Value                    |
| ------------------------------ | ----------------------------------------------- | ------------------------ |
| `persistence.enabled`         | Enable persistence using PVC                    | `true`                   |
| `persistence.existingClaim`   | Name of existing PVC                            | `""`                     |
| `persistence.storageClass`    | PVC Storage Class                               | `""`                     |
| `persistence.accessModes`     | PVC Access Modes                                | `["ReadWriteOnce"]`      |
| `persistence.size`            | PVC Storage Request                             | `8Gi`                    |
| `persistence.annotations`     | PVC annotations                                 | `{}`                     |

### Resource parameters

| Name                      | Description                                     | Value                    |
| ------------------------- | ----------------------------------------------- | ------------------------ |
| `resources.limits.cpu`   | CPU resource limits                             | `1000m`                  |
| `resources.limits.memory`| Memory resource limits                          | `1Gi`                    |
| `resources.requests.cpu` | CPU resource requests                           | `100m`                   |
| `resources.requests.memory`| Memory resource requests                      | `128Mi`                  |

### Autoscaling parameters

| Name                                          | Description                                     | Value        |
| --------------------------------------------- | ----------------------------------------------- | ------------ |
| `autoscaling.enabled`                        | Enable autoscaling                              | `false`      |
| `autoscaling.minReplicas`                    | Minimum number of replicas                      | `1`          |
| `autoscaling.maxReplicas`                    | Maximum number of replicas                      | `10`         |
| `autoscaling.targetCPUUtilizationPercentage` | Target CPU utilization percentage               | `80`         |

### Security parameters

| Name                                    | Description                                     | Value                    |
| --------------------------------------- | ----------------------------------------------- | ------------------------ |
| `podSecurityContext.fsGroup`           | Group ID for the pod                            | `2000`                   |
| `podSecurityContext.runAsNonRoot`      | Run as non-root user                            | `true`                   |
| `podSecurityContext.runAsUser`         | User ID for the container                       | `1000`                   |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation                  | `false`                  |
| `securityContext.capabilities.drop`    | Linux capabilities to drop                      | `["ALL"]`                |
| `securityContext.readOnlyRootFilesystem` | Mount root filesystem as read-only            | `true`                   |

### Service Account parameters

| Name                                | Description                                     | Value        |
| ----------------------------------- | ----------------------------------------------- | ------------ |
| `serviceAccount.create`            | Create service account                          | `true`       |
| `serviceAccount.automount`         | Automount service account token                 | `true`       |
| `serviceAccount.annotations`       | Service account annotations                     | `{}`         |
| `serviceAccount.name`              | Service account name                            | `""`         |

## Configuration and installation details

### Setting up authentication

To enable authentication, set the master key:

```bash
helm install bright ./charts/bright \
  --set config.masterKey="your-secret-key"
```

### Enabling Ingress

To enable ingress:

```bash
helm install bright ./charts/bright \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=bright.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix
```

### Persistence

By default, persistence is enabled with an 8Gi volume. To use an existing PVC:

```bash
helm install bright ./charts/bright \
  --set persistence.existingClaim=my-pvc
```

To disable persistence:

```bash
helm install bright ./charts/bright \
  --set persistence.enabled=false
```

### Resource Management

Configure resource limits and requests:

```bash
helm install bright ./charts/bright \
  --set resources.limits.cpu=2000m \
  --set resources.limits.memory=2Gi \
  --set resources.requests.cpu=500m \
  --set resources.requests.memory=512Mi
```

### Autoscaling

Enable autoscaling:

```bash
helm install bright ./charts/bright \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=10
```

## Examples

### Development Setup

```bash
helm install bright-dev ./charts/bright \
  --set persistence.enabled=false \
  --set resources.requests.cpu=50m \
  --set resources.requests.memory=64Mi
```

### Production Setup

```bash
helm install bright-prod ./charts/bright \
  --set replicaCount=3 \
  --set config.masterKey="prod-secret-key" \
  --set persistence.size=50Gi \
  --set persistence.storageClass=fast-ssd \
  --set resources.limits.cpu=2000m \
  --set resources.limits.memory=4Gi \
  --set resources.requests.cpu=1000m \
  --set resources.requests.memory=2Gi \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=search.example.com \
  --set autoscaling.enabled=true
```

## Troubleshooting

### Check pod status

```bash
kubectl get pods -l app.kubernetes.io/name=bright
```

### View logs

```bash
kubectl logs -l app.kubernetes.io/name=bright
```

### Check service

```bash
kubectl get svc -l app.kubernetes.io/name=bright
```

## License

Copyright © 2024 Bright Project
