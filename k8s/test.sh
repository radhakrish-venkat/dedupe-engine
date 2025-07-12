#!/bin/bash

set -e

echo "🧪 Testing Deduplication Engine on Kubernetes..."

# Check if namespace exists
if ! kubectl get namespace dedupe-engine &> /dev/null; then
    echo "❌ dedupe-engine namespace not found. Please run deploy.sh first."
    exit 1
fi

echo "📊 Checking pod status..."
kubectl get pods -n dedupe-engine

echo "⏳ Waiting for all pods to be ready..."
kubectl wait --for=condition=ready pod -l app=cockroachdb -n dedupe-engine --timeout=300s
kubectl wait --for=condition=ready pod -l app=minio -n dedupe-engine --timeout=300s
kubectl wait --for=condition=ready pod -l app=data-storage-node -n dedupe-engine --timeout=300s
kubectl wait --for=condition=ready pod -l app=ingest-node -n dedupe-engine --timeout=300s

echo "🔍 Testing service connectivity..."

# Test CockroachDB
echo "Testing CockroachDB..."
kubectl exec -n dedupe-engine deployment/cockroachdb -- cockroach sql --insecure --host=localhost:26257 -e "SELECT version();"

# Test MinIO
echo "Testing MinIO..."
kubectl exec -n dedupe-engine deployment/minio -- mc alias set myminio http://localhost:9000 minioadmin minioadmin123
kubectl exec -n dedupe-engine deployment/minio -- mc mb myminio/dedupe-engine

echo "🧪 Running stream handler test job..."
kubectl apply -f k8s/stream-handler-job.yaml

echo "⏳ Waiting for test job to complete..."
kubectl wait --for=condition=complete job/stream-handler-test -n dedupe-engine --timeout=300s

echo "📋 Test job logs:"
kubectl logs job/stream-handler-test -n dedupe-engine

echo "✅ Testing completed successfully!"

echo ""
echo "📊 Final status:"
kubectl get pods -n dedupe-engine
echo ""
echo "🌐 Services:"
kubectl get svc -n dedupe-engine
echo ""
echo "📈 MinIO Erasure Coding Status:"
kubectl exec -n dedupe-engine deployment/minio -- mc admin info myminio 