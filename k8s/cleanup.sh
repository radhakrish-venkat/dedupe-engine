#!/bin/bash

set -e

echo "🗑️ Cleaning up Kubernetes resources for dedupe-engine..."
kubectl delete namespace dedupe-engine --wait=true || true
kubectl delete pvc --all -n local-path-storage || true
kubectl delete pv --all || true
kubectl delete jobs --all -n local-path-storage || true
kubectl delete jobs --all -n dedupe-engine || true
kubectl delete configmap --all -n dedupe-engine || true
kubectl delete deployment --all -n dedupe-engine || true
kubectl delete service --all -n dedupe-engine || true
echo "✅ Cleanup complete!" 