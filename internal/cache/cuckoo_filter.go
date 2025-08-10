package cache

import (
	"crypto/rand"
	"encoding/binary"
	"hash/fnv"
	"math"
)

// CuckooFilter implements a proper Cuckoo filter with collision resolution
type CuckooFilter struct {
	buckets         []Bucket
	numBuckets      int
	bucketSize      int
	maxKicks        int
	fingerprintSize int
	loadFactor      float64
}

// Bucket represents a bucket in the Cuckoo filter
type Bucket struct {
	fingerprints []uint32
	occupied     uint32 // Bitmap of occupied slots
}

// NewCuckooFilter creates a new Cuckoo filter with proper configuration
func NewCuckooFilter(capacity int, falsePositiveRate float64) *CuckooFilter {
	// Calculate optimal parameters based on capacity and false positive rate
	bucketSize := 4 // 4 fingerprints per bucket (optimal for space efficiency)

	// Calculate number of buckets needed
	// Formula: numBuckets = capacity / (bucketSize * loadFactor)
	// Load factor of 0.85 provides better space efficiency and lower false positives
	loadFactor := 0.85
	numBuckets := int(math.Ceil(float64(capacity) / (float64(bucketSize) * loadFactor)))

	// Ensure we have at least 1 bucket
	if numBuckets < 1 {
		numBuckets = 1
	}

	// Calculate fingerprint size based on false positive rate
	// Formula: fingerprintSize = log2(1/falsePositiveRate) + log2(2*bucketSize)
	// Add extra bits for better accuracy
	fingerprintSize := int(math.Ceil(math.Log2(1/falsePositiveRate) + math.Log2(2*float64(bucketSize)) + 2))

	// Ensure minimum fingerprint size
	if fingerprintSize < 12 {
		fingerprintSize = 12
	}
	if fingerprintSize > 32 {
		fingerprintSize = 32
	}

	// Create buckets
	buckets := make([]Bucket, numBuckets)
	for i := range buckets {
		buckets[i] = Bucket{
			fingerprints: make([]uint32, bucketSize),
			occupied:     0,
		}
	}

	return &CuckooFilter{
		buckets:         buckets,
		numBuckets:      numBuckets,
		bucketSize:      bucketSize,
		maxKicks:        500, // Maximum number of kicks before giving up
		fingerprintSize: fingerprintSize,
		loadFactor:      loadFactor,
	}
}

// Add adds a fingerprint to the Cuckoo filter
func (cf *CuckooFilter) Add(fingerprint string) bool {
	fp := cf.getFingerprint(fingerprint)

	// Try to insert in primary bucket
	if cf.insertToBucket(fp, cf.getPrimaryBucket(fingerprint)) {
		return true
	}

	// Try to insert in secondary bucket
	secondaryBucket := cf.getSecondaryBucket(fp, cf.getPrimaryBucket(fingerprint))
	if cf.insertToBucket(fp, secondaryBucket) {
		return true
	}

	// Both buckets are full, need to kick out existing fingerprints
	return cf.kickout(fp, cf.getPrimaryBucket(fingerprint))
}

// Contains checks if a fingerprint might be in the filter
func (cf *CuckooFilter) Contains(fingerprint string) bool {
	fp := cf.getFingerprint(fingerprint)
	primaryBucket := cf.getPrimaryBucket(fingerprint)
	secondaryBucket := cf.getSecondaryBucket(fp, primaryBucket)

	// Check primary bucket
	if cf.bucketContains(fp, primaryBucket) {
		return true
	}

	// Check secondary bucket
	return cf.bucketContains(fp, secondaryBucket)
}

// Remove removes a fingerprint from the filter
func (cf *CuckooFilter) Remove(fingerprint string) bool {
	fp := cf.getFingerprint(fingerprint)
	primaryBucket := cf.getPrimaryBucket(fingerprint)
	secondaryBucket := cf.getSecondaryBucket(fp, primaryBucket)

	// Try to remove from primary bucket
	if cf.removeFromBucket(fp, primaryBucket) {
		return true
	}

	// Try to remove from secondary bucket
	return cf.removeFromBucket(fp, secondaryBucket)
}

// Size returns the number of fingerprints in the filter
func (cf *CuckooFilter) Size() int {
	total := 0
	for _, bucket := range cf.buckets {
		total += cf.countOccupiedSlots(bucket.occupied)
	}
	return total
}

// Capacity returns the maximum number of fingerprints the filter can hold
func (cf *CuckooFilter) Capacity() int {
	return cf.numBuckets * cf.bucketSize
}

// LoadFactor returns the current load factor
func (cf *CuckooFilter) LoadFactor() float64 {
	return float64(cf.Size()) / float64(cf.Capacity())
}

// getFingerprint creates a fingerprint from the input string
func (cf *CuckooFilter) getFingerprint(fingerprint string) uint32 {
	// Use a better hash function for more uniform distribution
	h := fnv.New64a()
	h.Write([]byte(fingerprint))
	hash := h.Sum64()

	// Use both halves of the 64-bit hash for better distribution
	upper := uint32(hash >> 32)
	lower := uint32(hash & 0xFFFFFFFF)

	// Combine upper and lower parts
	combined := upper ^ lower

	// Extract fingerprint of desired size
	mask := uint32((1 << cf.fingerprintSize) - 1)
	fp := combined & mask

	// Ensure fingerprint is not zero (reserved for empty slots)
	if fp == 0 {
		fp = 1
	}

	return fp
}

// getPrimaryBucket gets the primary bucket index for a fingerprint
func (cf *CuckooFilter) getPrimaryBucket(fingerprint string) int {
	h := fnv.New64a()
	h.Write([]byte(fingerprint))
	hash := h.Sum64()
	return int(hash % uint64(cf.numBuckets))
}

// getSecondaryBucket gets the secondary bucket index for a fingerprint
func (cf *CuckooFilter) getSecondaryBucket(fp uint32, primaryBucket int) int {
	// Use XOR with fingerprint to get secondary bucket
	secondaryBucket := primaryBucket ^ int(fp)
	if secondaryBucket < 0 {
		secondaryBucket = -secondaryBucket
	}
	return secondaryBucket % cf.numBuckets
}

// insertToBucket tries to insert a fingerprint into a bucket
func (cf *CuckooFilter) insertToBucket(fp uint32, bucketIndex int) bool {
	bucket := &cf.buckets[bucketIndex]

	// Find an empty slot
	for i := 0; i < cf.bucketSize; i++ {
		if (bucket.occupied & (1 << i)) == 0 {
			bucket.fingerprints[i] = fp
			bucket.occupied |= (1 << i)
			return true
		}
	}

	return false
}

// bucketContains checks if a bucket contains a fingerprint
func (cf *CuckooFilter) bucketContains(fp uint32, bucketIndex int) bool {
	bucket := cf.buckets[bucketIndex]

	for i := 0; i < cf.bucketSize; i++ {
		if (bucket.occupied&(1<<i)) != 0 && bucket.fingerprints[i] == fp {
			return true
		}
	}

	return false
}

// removeFromBucket removes a fingerprint from a bucket
func (cf *CuckooFilter) removeFromBucket(fp uint32, bucketIndex int) bool {
	bucket := &cf.buckets[bucketIndex]

	for i := 0; i < cf.bucketSize; i++ {
		if (bucket.occupied&(1<<i)) != 0 && bucket.fingerprints[i] == fp {
			bucket.occupied &= ^(1 << i)
			bucket.fingerprints[i] = 0
			return true
		}
	}

	return false
}

// kickout performs the kickout process when both buckets are full
func (cf *CuckooFilter) kickout(fp uint32, bucketIndex int) bool {
	// Start with the primary bucket
	currentBucket := bucketIndex
	currentFp := fp

	for kicks := 0; kicks < cf.maxKicks; kicks++ {
		// Find a random slot in the current bucket to kick out
		bucket := &cf.buckets[currentBucket]
		slot := cf.findRandomOccupiedSlot(bucket)

		if slot == -1 {
			// No occupied slots found (shouldn't happen)
			return false
		}

		// Kick out the existing fingerprint
		kickedFp := bucket.fingerprints[slot]
		bucket.fingerprints[slot] = currentFp

		// Try to insert the kicked fingerprint in its alternative bucket
		altBucket := cf.getSecondaryBucket(kickedFp, currentBucket)

		if cf.insertToBucket(kickedFp, altBucket) {
			return true
		}

		// Continue with the kicked fingerprint
		currentBucket = altBucket
		currentFp = kickedFp
	}

	// Max kicks reached, insertion failed
	return false
}

// findRandomOccupiedSlot finds a random occupied slot in a bucket
func (cf *CuckooFilter) findRandomOccupiedSlot(bucket *Bucket) int {
	occupiedSlots := make([]int, 0, cf.bucketSize)

	for i := 0; i < cf.bucketSize; i++ {
		if (bucket.occupied & (1 << i)) != 0 {
			occupiedSlots = append(occupiedSlots, i)
		}
	}

	if len(occupiedSlots) == 0 {
		return -1
	}

	// Return a random occupied slot
	randomIndex := cf.getRandomInt(len(occupiedSlots))
	return occupiedSlots[randomIndex]
}

// countOccupiedSlots counts the number of occupied slots in a bucket
func (cf *CuckooFilter) countOccupiedSlots(occupied uint32) int {
	count := 0
	for i := 0; i < cf.bucketSize; i++ {
		if (occupied & (1 << i)) != 0 {
			count++
		}
	}
	return count
}

// getRandomInt generates a random integer in [0, max)
func (cf *CuckooFilter) getRandomInt(max int) int {
	var buf [8]byte
	rand.Read(buf[:])
	val := binary.BigEndian.Uint64(buf[:])
	return int(val % uint64(max))
}

// GetStats returns statistics about the Cuckoo filter
func (cf *CuckooFilter) GetStats() map[string]interface{} {
	size := cf.Size()
	capacity := cf.Capacity()
	loadFactor := cf.LoadFactor()

	return map[string]interface{}{
		"size":             size,
		"capacity":         capacity,
		"load_factor":      loadFactor,
		"num_buckets":      cf.numBuckets,
		"bucket_size":      cf.bucketSize,
		"fingerprint_size": cf.fingerprintSize,
		"max_kicks":        cf.maxKicks,
	}
}
