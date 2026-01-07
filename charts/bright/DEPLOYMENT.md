# Bright Helm Chart Deployment Guide

This guide provides detailed instructions for deploying Bright using Helm in various environments.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Environment-Specific Deployments](#environment-specific-deployments)
  - [Development](#development)
  - [Staging](#staging)
  - [Production](#production)
- [Configuration](#configuration)
- [Upgrading](#upgrading)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)

## Prerequisites

Before deploying Bright, ensure you have:

1. **Kubernetes Cluster**: v1.19 or higher
2. **Helm**: v3.2.0 or higher
3. **kubectl**: Configured to access your cluster
4. **Storage**: PersistentVolume provisioner (if using persistence)

## Quick Start

### 1. Install with Default Values

```bash
# From the repository root
helm install bright ./charts/bright

# Or with a custom namespace
helm install bright ./charts/bright --create-namespace --namespace bright
```

### 2. Verify Installation

```bash
# Check deployment status
kubectl get pods -l app.kubernetes.io/name=bright

# Check service
kubectl get svc -l app.kubernetes.io/name=bright

# View logs
kubectl logs -l app.kubernetes.io/name=bright --tail=50
```

### 3. Access the Application

```bash
# Port-forward to local machine
kubectl port-forward svc/bright 3000:3000

# Test the health endpoint
curl http://localhost:3000/health
```

**Production Best Practices:**

1. **High Availability**: Run 3+ replicas
2. **Resource Limits**: Set appropriate CPU/Memory limits
3. **Persistence**: Use fast SSD storage class
4. **Autoscaling**: Enable HPA for automatic scaling
5. **Monitoring**: Enable ServiceMonitor for Prometheus
6. **Security**: Enable Pod Security Policies

## Configuration

### Authentication

Enable authentication by setting a master key:

```bash
# Generate a secure key
export MASTER_KEY=$(openssl rand -base64 32)

# Install with authentication
helm install bright ./charts/bright \
  --set config.masterKey="$MASTER_KEY"

# Test with authentication
curl -H "Authorization: Bearer $MASTER_KEY" http://localhost:3000/health
```

### Storage Configuration

#### Use Existing PVC

```bash
# Create PVC first
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: bright-data
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 50Gi
  storageClassName: fast-ssd
EOF

# Install using existing PVC
helm install bright ./charts/bright \
  --set persistence.enabled=true \
  --set persistence.existingClaim=bright-data
```

#### Custom Storage Class

```bash
helm install bright ./charts/bright \
  --set persistence.enabled=true \
  --set persistence.storageClass=fast-ssd \
  --set persistence.size=100Gi
```

### Ingress Configuration

#### With cert-manager (TLS)

```bash
helm install bright ./charts/bright \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=search.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix \
  --set ingress.tls[0].secretName=bright-tls \
  --set ingress.tls[0].hosts[0]=search.example.com \
  --set-string ingress.annotations."cert-manager\.io/cluster-issuer"=letsencrypt-prod
```

### Resource Management

```bash
helm install bright ./charts/bright \
  --set resources.requests.cpu=500m \
  --set resources.requests.memory=1Gi \
  --set resources.limits.cpu=2000m \
  --set resources.limits.memory=4Gi
```

### Autoscaling

```bash
helm install bright ./charts/bright \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=3 \
  --set autoscaling.maxReplicas=10 \
  --set autoscaling.targetCPUUtilizationPercentage=70 \
  --set autoscaling.targetMemoryUtilizationPercentage=80
```

### Zero-Downtime Upgrades

1. Ensure you have multiple replicas running
2. Use Pod Disruption Budget
3. Use rolling update strategy (default)

```bash
helm upgrade bright ./charts/bright \
  --namespace production \
  --set replicaCount=3 \
  --set podDisruptionBudget.enabled=true \
  --set podDisruptionBudget.minAvailable=2 \
  --wait
```
