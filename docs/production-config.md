# Production Configuration Guide

## Customer-Scale Deduplication Engine Configuration

This guide helps you configure the deduplication engine for different customer scales and workloads.

## Architecture Overview

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Stream Handler │    │   Ingest Node   │    │ Data Storage    │
│                 │    │                 │    │ Node            │
│ • File Reading  │───▶│ • Chunking      │───▶│ • Metadata DB   │
│ • gRPC Client   │    │ • Deduplication │    │ • Object Store  │
│ • Backup Stream │    │ • Hybrid Cache  │    │ • Chunk Storage │
```

### Hybrid Deduplication Layers

1. **Hot Cache** (In-Memory): Frequently accessed chunks
2. **Local Store** (RocksDB): Persistent local storage
3. **Central DB** (CockroachDB): Distributed metadata

## Configuration by Customer Scale

### Small Business (1M-10M unique chunks)

```go
config := &dedupe.Config{
    HotCacheSize:    10000,  // 10K hot chunks in memory (~2MB)
    RocksDBPath:     "/data/dedupe/local",
    EnableCentralDB: true,
    SyncToCentralDB: true,
    BatchSize:       1000,
}
```

**Resource Requirements:**
- **Memory**: 4GB RAM (2GB for hot cache + 2GB for RocksDB cache)
- **Disk**: 10GB for RocksDB storage
- **Network**: Low latency to CockroachDB

**Expected Performance:**
- **Hit Rate**: 95%+ (most chunks in local RocksDB)
- **Latency**: <1ms for hot data, <10ms for local data
- **Throughput**: 10K-50K chunks/second

### Medium Business (10M-100M unique chunks)

```go
config := &dedupe.Config{
    HotCacheSize:    50000,  // 50K hot chunks in memory (~10MB)
    RocksDBPath:     "/data/dedupe/local",
    EnableCentralDB: true,
    SyncToCentralDB: true,
    BatchSize:       5000,
}
```

**Resource Requirements:**
- **Memory**: 16GB RAM (10GB for hot cache + 6GB for RocksDB cache)
- **Disk**: 100GB for RocksDB storage
- **Network**: High bandwidth to CockroachDB

**Expected Performance:**
- **Hit Rate**: 90%+ (most chunks in local RocksDB)
- **Latency**: <1ms for hot data, <20ms for local data
- **Throughput**: 50K-200K chunks/second

### Large Enterprise (100M-1B+ unique chunks)

```go
config := &dedupe.Config{
    HotCacheSize:    100000, // 100K hot chunks in memory (~20MB)
    RocksDBPath:     "/data/dedupe/local",
    EnableCentralDB: true,
    SyncToCentralDB: true,
    BatchSize:       10000,
}
```

**Resource Requirements:**
- **Memory**: 64GB+ RAM (20GB for hot cache + 44GB for RocksDB cache)
- **Disk**: 1TB+ for RocksDB storage (SSD recommended)
- **Network**: High bandwidth, low latency to CockroachDB

**Expected Performance:**
- **Hit Rate**: 85%+ (most chunks in local RocksDB)
- **Latency**: <1ms for hot data, <50ms for local data
- **Throughput**: 200K-1M chunks/second

## Performance Optimization

### Memory Configuration

```go
// RocksDB memory configuration
db, err := pebble.Open(dbPath, &pebble.Options{
    // Cache size: 50% of available RAM for RocksDB
    Cache: pebble.NewCache(32 << 30), // 32GB cache
    
    // Compression for space efficiency
    Compression: pebble.SnappyCompression,
    
    // Write buffer size
    MemTableSize: 64 << 20, // 64MB
    
    // Level 0 compaction threshold
    L0CompactionThreshold: 4,
    L0StopWritesThreshold: 12,
})
```

### Hot Cache Sizing

```go
// Hot cache size based on workload
func calculateHotCacheSize(totalChunks int) int {
    switch {
    case totalChunks < 1000000:
        return 10000  // 1% of total
    case totalChunks < 10000000:
        return 50000  // 0.5% of total
    case totalChunks < 100000000:
        return 100000 // 0.1% of total
    default:
        return 200000 // 0.02% of total
    }
}
```

## Monitoring and Metrics

### Key Metrics to Monitor

1. **Hit Rates**
   - Hot cache hit rate (target: >20%)
   - Local store hit rate (target: >70%)
   - Overall hit rate (target: >90%)

2. **Latency**
   - Hot cache access: <1ms
   - Local store access: <10ms
   - Central DB access: <100ms

3. **Throughput**
   - Chunks processed per second
   - Deduplication ratio
   - Storage efficiency

### Example Monitoring Code

```go
// Monitor performance every minute
func (e *HybridDedupeEngine) monitorPerformance() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := e.GetStats()
        hitRate := e.GetHitRate()
        
        log.Printf("Performance - Hit Rate: %.2f%%, Hot: %d, Local: %d, Central: %d, Misses: %d",
            hitRate, stats.HotCacheHits, stats.LocalStoreHits, stats.CentralDBHits, stats.Misses)
        
        // Alert if hit rate drops below threshold
        if hitRate < 80 {
            log.Printf("WARNING: Low hit rate detected: %.2f%%", hitRate)
        }
    }
}
```

## Deployment Options

### Option 1: Docker Compose (Development/Testing)

```yaml
# docker-compose.yml
version: '3.8'
services:
  ingest-node:
    image: dedupe-engine:latest
    environment:
      - HOT_CACHE_SIZE=50000
      - ROCKSDB_PATH=/data/dedupe/local
      - ENABLE_CENTRAL_DB=true
      - SYNC_TO_CENTRAL_DB=true
    volumes:
      - dedupe-data:/data/dedupe
    deploy:
      resources:
        limits:
          memory: 16G
        reservations:
          memory: 8G
    ulimits:
      nofile:
        soft: 65536
        hard: 65536

volumes:
  dedupe-data:
    driver: local
```

### Option 2: Kubernetes (Production)

#### Quick Deployment

```bash
# Deploy the hybrid deduplication engine
./k8s/deploy-hybrid-dedupe.sh deploy

# Check status
./k8s/deploy-hybrid-dedupe.sh status

# View logs
./k8s/deploy-hybrid-dedupe.sh logs

# Show endpoints
./k8s/deploy-hybrid-dedupe.sh endpoints
```

#### Manual Deployment

```bash
# 1. Create namespace and RBAC
kubectl apply -f k8s/hybrid-dedupe-pvc.yaml
kubectl apply -f k8s/hybrid-dedupe-configmap.yaml

# 2. Deploy the application
kubectl apply -f k8s/hybrid-dedupe-deployment.yaml
kubectl apply -f k8s/hybrid-dedupe-service.yaml

# 3. Deploy autoscaling
kubectl apply -f k8s/hybrid-dedupe-hpa.yaml

# 4. Deploy network policies
kubectl apply -f k8s/hybrid-dedupe-networkpolicy.yaml
```

#### Kubernetes Configuration Files

The Kubernetes deployment includes:

1. **Deployment** (`hybrid-dedupe-deployment.yaml`)
   - 3 replicas with rolling updates
   - Resource limits and requests
   - Health checks and probes
   - Security context

2. **Services** (`hybrid-dedupe-service.yaml`)
   - LoadBalancer for external access
   - ClusterIP for internal communication
   - gRPC, metrics, and health endpoints

3. **Storage** (`hybrid-dedupe-pvc.yaml`)
   - 100GB persistent volume claim
   - Fast SSD storage class
   - Encrypted storage

4. **Configuration** (`hybrid-dedupe-configmap.yaml`)
   - Hybrid dedupe settings
   - Logging configuration
   - Metrics and health check config

5. **Autoscaling** (`hybrid-dedupe-hpa.yaml`)
   - CPU and memory-based scaling
   - Custom metrics for deduplication load
   - 3-20 replica range

6. **Network Policies** (`hybrid-dedupe-networkpolicy.yaml`)
   - Secure communication
   - Service-to-service access control

#### Environment Variables

```bash
# Deployment configuration
HOT_CACHE_SIZE=50000          # Hot cache entries
ROCKSDB_PATH=/data/dedupe/local # RocksDB storage path
ENABLE_CENTRAL_DB=true        # Enable CockroachDB
SYNC_TO_CENTRAL_DB=true       # Sync to central DB
BATCH_SIZE=5000               # Batch size for DB operations

# Service configuration
GRPC_PORT=50051               # gRPC service port
METRICS_PORT=8080             # Metrics endpoint port
HEALTH_PORT=8081              # Health check port

# Database configuration
COCKROACHDB_ADDR=cockroachdb-service:26257
COCKROACHDB_DATABASE=dedupe_engine
COCKROACHDB_USER=root
COCKROACHDB_SSL_MODE=disable

# Storage configuration
STORAGE_NODE_ADDR=data-storage-service:50052
MINIO_ENDPOINT=minio-service:9000
MINIO_BUCKET=dedupe-chunks
MINIO_USE_SSL=false

# Performance tuning
GOMAXPROCS=4                 # Number of CPU cores
GOGC=100                     # Garbage collection tuning
```

## Cost Optimization

### Storage Costs (Cloud)

| Storage Type | Cost per GB/month | Use Case |
|--------------|------------------|----------|
| **Memory** | $0.10-0.50 | Hot cache (frequently accessed) |
| **SSD** | $0.02-0.10 | RocksDB local storage |
| **HDD** | $0.01-0.05 | Long-term archival |

### Recommended Configuration

```go
// Cost-optimized configuration
config := &dedupe.Config{
    HotCacheSize:    50000,  // Balance between performance and cost
    RocksDBPath:     "/data/dedupe/local",
    EnableCentralDB: true,
    SyncToCentralDB: true,
    BatchSize:       5000,   // Reduce network calls
}
```

This configuration provides:
- **90%+ hit rate** for most workloads
- **Cost-effective** storage usage
- **Scalable** performance
- **Production-ready** reliability

## Troubleshooting

### Common Issues

1. **Low Hit Rate**
   - Increase hot cache size
   - Check RocksDB performance
   - Monitor network latency to CockroachDB

2. **High Memory Usage**
   - Reduce hot cache size
   - Optimize RocksDB cache settings
   - Check for memory leaks

3. **Slow Performance**
   - Use SSD storage for RocksDB
   - Increase CPU/memory resources
   - Optimize network configuration

4. **Storage Issues**
   - Check PVC binding
   - Verify storage class configuration
   - Monitor disk space usage

### Monitoring Commands

```bash
# Check pod status
kubectl get pods -n dedupe-engine -l app=hybrid-dedupe-ingest

# View logs
kubectl logs -n dedupe-engine -l app=hybrid-dedupe-ingest

# Check resource usage
kubectl top pods -n dedupe-engine

# Monitor HPA
kubectl get hpa -n dedupe-engine

# Check storage
kubectl get pvc -n dedupe-engine
```
