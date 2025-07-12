#!/bin/bash

set -e

echo "🚀 Deploying Deduplication Engine to Kubernetes..."

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed. Please install kubectl first."
    exit 1
fi

# Check if kind cluster is running
if ! kubectl cluster-info &> /dev/null; then
    echo "❌ Kubernetes cluster is not accessible. Please ensure your kind cluster is running."
    exit 1
fi

echo "📋 Creating namespace..."
kubectl apply -f namespace.yaml

echo "🗄️ Deploying CockroachDB..."
kubectl apply -f cockroachdb-deployment.yaml
kubectl apply -f cockroachdb-service.yaml

echo "☁️ Deploying MinIO with erasure coding..."
kubectl apply -f minio-storageclass.yaml
kubectl apply -f minio-pvc.yaml
kubectl apply -f minio-configmap.yaml
kubectl apply -f minio-deployment.yaml
kubectl apply -f minio-service.yaml

echo "⏳ Waiting for databases to be ready..."
kubectl wait --for=condition=ready pod -l app=cockroachdb -n dedupe-engine --timeout=300s
kubectl wait --for=condition=ready pod -l app=minio -n dedupe-engine --timeout=300s

echo "🔧 Deploying application services..."
kubectl apply -f data-storage-deployment.yaml
kubectl apply -f ingest-deployment.yaml
kubectl apply -f services.yaml

echo "⏳ Waiting for application services to be ready..."
kubectl wait --for=condition=ready pod -l app=data-storage-node -n dedupe-engine --timeout=300s
kubectl wait --for=condition=ready pod -l app=ingest-node -n dedupe-engine --timeout=300s

echo "✅ Deployment completed successfully!"
echo ""
echo "📊 Service Status:"
kubectl get pods -n dedupe-engine
echo ""
echo "🌐 Service Endpoints:"
kubectl get svc -n dedupe-engine
echo ""
echo "🧪 To test the deployment, run:"
echo "   kubectl apply -f stream-handler-job.yaml"
echo ""
echo "📋 To view logs:"
echo "   kubectl logs -f deployment/ingest-node -n dedupe-engine"
echo "   kubectl logs -f deployment/data-storage-node -n dedupe-engine"
echo ""
echo "🗑️ To clean up:"
echo "   kubectl delete namespace dedupe-engine" 