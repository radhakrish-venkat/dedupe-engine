#!/bin/bash

# Hybrid Deduplication Engine Kubernetes Deployment Script
# This script deploys the hybrid deduplication engine with RocksDB and in-memory cache

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="dedupe-engine"
DEPLOYMENT_NAME="hybrid-dedupe-ingest"
IMAGE_TAG="${IMAGE_TAG:-latest}"
REPLICAS="${REPLICAS:-3}"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if kubectl is available
check_kubectl() {
    if ! command -v kubectl &> /dev/null; then
        print_error "kubectl is not installed or not in PATH"
        exit 1
    fi
    
    if ! kubectl cluster-info &> /dev/null; then
        print_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    print_success "kubectl is available and connected to cluster"
}

# Function to create namespace
create_namespace() {
    print_status "Creating namespace: $NAMESPACE"
    
    if kubectl get namespace $NAMESPACE &> /dev/null; then
        print_warning "Namespace $NAMESPACE already exists"
    else
        kubectl create namespace $NAMESPACE
        kubectl label namespace $NAMESPACE name=dedupe-engine
        print_success "Namespace $NAMESPACE created"
    fi
}

# Function to create service account
create_service_account() {
    print_status "Creating service account"
    
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dedupe-engine-sa
  namespace: $NAMESPACE
  labels:
    app: hybrid-dedupe-ingest
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: dedupe-engine-role
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dedupe-engine-binding
subjects:
- kind: ServiceAccount
  name: dedupe-engine-sa
  namespace: $NAMESPACE
roleRef:
  kind: ClusterRole
  name: dedupe-engine-role
  apiGroup: rbac.authorization.k8s.io
EOF
    
    print_success "Service account and RBAC created"
}

# Function to create secrets
create_secrets() {
    print_status "Creating secrets"
    
    # MinIO secrets (replace with your actual values)
    kubectl create secret generic minio-secret \
        --namespace=$NAMESPACE \
        --from-literal=access-key=minioadmin \
        --from-literal=secret-key=minioadmin \
        --dry-run=client -o yaml | kubectl apply -f -
    
    print_success "Secrets created"
}

# Function to deploy storage classes
deploy_storage() {
    print_status "Deploying storage classes and PVC"
    
    kubectl apply -f k8s/hybrid-dedupe-pvc.yaml
    
    # Wait for PVC to be bound
    print_status "Waiting for PVC to be bound..."
    kubectl wait --for=condition=Bound pvc/dedupe-pvc -n $NAMESPACE --timeout=300s
    
    print_success "Storage deployed"
}

# Function to deploy configuration
deploy_config() {
    print_status "Deploying configuration"
    
    kubectl apply -f k8s/hybrid-dedupe-configmap.yaml
    
    print_success "Configuration deployed"
}

# Function to deploy the application
deploy_application() {
    print_status "Deploying hybrid deduplication engine"
    
    # Update image tag in deployment
    sed "s|dedupe-engine:latest|dedupe-engine:$IMAGE_TAG|g" k8s/hybrid-dedupe-deployment.yaml | \
    kubectl apply -f -
    
    print_success "Application deployment created"
}

# Function to deploy services
deploy_services() {
    print_status "Deploying services"
    
    kubectl apply -f k8s/hybrid-dedupe-service.yaml
    
    print_success "Services deployed"
}

# Function to deploy autoscaling
deploy_autoscaling() {
    print_status "Deploying autoscaling"
    
    kubectl apply -f k8s/hybrid-dedupe-hpa.yaml
    
    print_success "Autoscaling deployed"
}

# Function to deploy network policies
deploy_network_policies() {
    print_status "Deploying network policies"
    
    kubectl apply -f k8s/hybrid-dedupe-networkpolicy.yaml
    
    print_success "Network policies deployed"
}

# Function to wait for deployment
wait_for_deployment() {
    print_status "Waiting for deployment to be ready..."
    
    kubectl rollout status deployment/$DEPLOYMENT_NAME -n $NAMESPACE --timeout=600s
    
    print_success "Deployment is ready"
}

# Function to show deployment status
show_status() {
    print_status "Deployment status:"
    
    echo ""
    echo "Pods:"
    kubectl get pods -n $NAMESPACE -l app=hybrid-dedupe-ingest
    
    echo ""
    echo "Services:"
    kubectl get services -n $NAMESPACE -l app=hybrid-dedupe-ingest
    
    echo ""
    echo "PVC:"
    kubectl get pvc -n $NAMESPACE
    
    echo ""
    echo "HPA:"
    kubectl get hpa -n $NAMESPACE
    
    echo ""
    echo "Network Policies:"
    kubectl get networkpolicies -n $NAMESPACE
}

# Function to show logs
show_logs() {
    print_status "Recent logs from deployment:"
    
    kubectl logs -n $NAMESPACE -l app=hybrid-dedupe-ingest --tail=50
}

# Function to show metrics endpoints
show_endpoints() {
    print_status "Service endpoints:"
    
    echo ""
    echo "External LoadBalancer:"
    kubectl get service hybrid-dedupe-service -n $NAMESPACE -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "Pending..."
    
    echo ""
    echo "Internal service:"
    kubectl get service hybrid-dedupe-internal -n $NAMESPACE -o jsonpath='{.spec.clusterIP}'
    
    echo ""
    echo "Ports:"
    echo "- gRPC: 50051"
    echo "- Metrics: 8080"
    echo "- Health: 8081"
}

# Function to cleanup
cleanup() {
    print_warning "Cleaning up deployment..."
    
    kubectl delete -f k8s/hybrid-dedupe-networkpolicy.yaml --ignore-not-found
    kubectl delete -f k8s/hybrid-dedupe-hpa.yaml --ignore-not-found
    kubectl delete -f k8s/hybrid-dedupe-service.yaml --ignore-not-found
    kubectl delete -f k8s/hybrid-dedupe-deployment.yaml --ignore-not-found
    kubectl delete -f k8s/hybrid-dedupe-configmap.yaml --ignore-not-found
    kubectl delete -f k8s/hybrid-dedupe-pvc.yaml --ignore-not-found
    kubectl delete secret minio-secret -n $NAMESPACE --ignore-not-found
    kubectl delete serviceaccount dedupe-engine-sa -n $NAMESPACE --ignore-not-found
    kubectl delete namespace $NAMESPACE --ignore-not-found
    
    print_success "Cleanup completed"
}

# Main deployment function
deploy() {
    print_status "Starting hybrid deduplication engine deployment"
    print_status "Configuration:"
    echo "  Namespace: $NAMESPACE"
    echo "  Image Tag: $IMAGE_TAG"
    echo "  Replicas: $REPLICAS"
    echo ""
    
    check_kubectl
    create_namespace
    create_service_account
    create_secrets
    deploy_storage
    deploy_config
    deploy_application
    deploy_services
    deploy_autoscaling
    deploy_network_policies
    wait_for_deployment
    
    print_success "Hybrid deduplication engine deployed successfully!"
    echo ""
    show_status
    echo ""
    show_endpoints
}

# Main script logic
case "${1:-deploy}" in
    deploy)
        deploy
        ;;
    status)
        show_status
        ;;
    logs)
        show_logs
        ;;
    endpoints)
        show_endpoints
        ;;
    cleanup)
        cleanup
        ;;
    *)
        echo "Usage: $0 {deploy|status|logs|endpoints|cleanup}"
        echo ""
        echo "Commands:"
        echo "  deploy    - Deploy the hybrid deduplication engine"
        echo "  status    - Show deployment status"
        echo "  logs      - Show recent logs"
        echo "  endpoints - Show service endpoints"
        echo "  cleanup   - Clean up all resources"
        echo ""
        echo "Environment variables:"
        echo "  IMAGE_TAG - Docker image tag (default: latest)"
        echo "  REPLICAS  - Number of replicas (default: 3)"
        exit 1
        ;;
esac
