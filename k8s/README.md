# Kubernetes Deployment Guide

This guide explains how to deploy the Deduplication Engine on Kubernetes with multi-node MinIO using erasure coding.

## 🏗️ Architecture

The Kubernetes deployment includes:

- **MinIO**: Distributed object storage with erasure coding (4 drives, 2 parity)
- **CockroachDB**: Distributed SQL database for metadata
- **Data Storage Node**: gRPC service for chunk storage
- **Ingest Node**: gRPC service for backup processing
- **Stream Handler**: Client for file processing (deployed as Job)

## 📋 Prerequisites

- Kubernetes cluster (tested with kind)
- kubectl configured
- Docker for building images
- At least 2 nodes for proper erasure coding

## 🚀 Quick Deployment

### 1. Build and Deploy

```bash
# Deploy everything
./k8s/deploy.sh

# Test the deployment
./k8s/test.sh
```

### 2. Manual Deployment

```bash
# Create namespace
kubectl apply -f k8s/namespace.yaml

# Deploy databases
kubectl apply -f k8s/cockroachdb-deployment.yaml
kubectl apply -f k8s/cockroachdb-service.yaml
kubectl apply -f k8s/minio-*.yaml

# Deploy applications
kubectl apply -f k8s/data-storage-deployment.yaml
kubectl apply -f k8s/ingest-deployment.yaml
kubectl apply -f k8s/services.yaml
```

## 🔧 Configuration

### MinIO Erasure Coding

The MinIO configuration uses:
- **4 data drives** with **2 parity drives**
- **50% storage efficiency** (can lose 2 drives without data loss)
- **Automatic data distribution** across drives

```yaml
# From k8s/minio-configmap.yaml
MINIO_ERASURE_CODING_DRIVES=4
MINIO_ERASURE_CODING_PARITY=2
```

### Storage Configuration

- **Persistent Volumes**: 10Gi each for MinIO drives
- **Storage Class**: Uses local storage for development
- **Data Persistence**: Survives pod restarts

### Resource Limits

```yaml
resources:
  requests:
    memory: "256Mi"
    cpu: "250m"
  limits:
    memory: "1Gi"
    cpu: "500m"
```

## 🧪 Testing

### 1. Run Test Job

```bash
kubectl apply -f k8s/stream-handler-job.yaml
kubectl logs job/stream-handler-test -n dedupe-engine
```

### 2. Manual Testing

```bash
# Port forward to access services
kubectl port-forward svc/ingest-node 50051:50051 -n dedupe-engine
kubectl port-forward svc/minio 9000:9000 -n dedupe-engine

# Test with local client
./stream-handler -file test-file.txt -ingest-addr localhost:50051
```

### 3. Verify Erasure Coding

```bash
# Check MinIO status
kubectl exec -n dedupe-engine deployment/minio -- mc admin info myminio

# Check drive status
kubectl exec -n dedupe-engine deployment/minio -- mc admin heal myminio
```

## 📊 Monitoring

### Check Pod Status

```bash
kubectl get pods -n dedupe-engine
kubectl describe pod <pod-name> -n dedupe-engine
```

### View Logs

```bash
# Application logs
kubectl logs -f deployment/ingest-node -n dedupe-engine
kubectl logs -f deployment/data-storage-node -n dedupe-engine

# Database logs
kubectl logs -f deployment/cockroachdb -n dedupe-engine
kubectl logs -f deployment/minio -n dedupe-engine
```

### Service Endpoints

```bash
kubectl get svc -n dedupe-engine
```

## 🔍 Troubleshooting

### Common Issues

1. **Image Pull Errors**
   ```bash
   # Build image locally
   docker build -f Dockerfile.k8s -t dedupe-engine:latest .
   ```

2. **Storage Issues**
   ```bash
   # Check PVC status
   kubectl get pvc -n dedupe-engine
   kubectl describe pvc <pvc-name> -n dedupe-engine
   ```

3. **Service Connectivity**
   ```bash
   # Test service connectivity
   kubectl exec -n dedupe-engine deployment/ingest-node -- nslookup data-storage-node
   ```

### Health Checks

```bash
# Check all pods are ready
kubectl wait --for=condition=ready pod -l app=ingest-node -n dedupe-engine

# Check service endpoints
kubectl get endpoints -n dedupe-engine
```

## 🗑️ Cleanup

```bash
# Remove everything
kubectl delete namespace dedupe-engine

# Or remove individual components
kubectl delete -f k8s/
```

## 📈 Scaling

### Horizontal Pod Autoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: ingest-node-hpa
  namespace: dedupe-engine
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ingest-node
  minReplicas: 1
  maxReplicas: 5
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

### Multi-Node MinIO

For production, deploy MinIO across multiple nodes:

```yaml
# StatefulSet for distributed MinIO
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: minio-distributed
spec:
  replicas: 4
  serviceName: minio-distributed
  # ... configuration for distributed deployment
```

## 🔒 Security

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: dedupe-engine-network-policy
  namespace: dedupe-engine
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: dedupe-engine
    ports:
    - protocol: TCP
      port: 50051
    - protocol: TCP
      port: 50052
```

### Secrets Management

```bash
# Create secrets for production
kubectl create secret generic minio-credentials \
  --from-literal=access-key=your-access-key \
  --from-literal=secret-key=your-secret-key \
  -n dedupe-engine
```

## 📚 Additional Resources

- [MinIO Erasure Coding](https://docs.min.io/docs/minio-erasure-code-quickstart-guide.html)
- [CockroachDB on Kubernetes](https://www.cockroachlabs.com/docs/stable/orchestrate-cockroachdb-with-kubernetes.html)
- [Kubernetes Best Practices](https://kubernetes.io/docs/concepts/configuration/overview/)

## 🎯 Performance Tuning

### Resource Optimization

```yaml
# Increase resources for production
resources:
  requests:
    memory: "1Gi"
    cpu: "500m"
  limits:
    memory: "2Gi"
    cpu: "1000m"
```

### Storage Optimization

```yaml
# Use SSD storage class
storageClassName: ssd-storage
```

### Network Optimization

```yaml
# Use NodePort or LoadBalancer for external access
spec:
  type: LoadBalancer
``` 