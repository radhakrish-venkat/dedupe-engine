package cache

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestCuckooFilterBasic(t *testing.T) {
	cf := NewCuckooFilter(1000, 0.01)

	// Test basic add and contains
	fingerprints := []string{
		"abc123",
		"def456",
		"ghi789",
		"jkl012",
	}

	// Add fingerprints
	for _, fp := range fingerprints {
		if !cf.Add(fp) {
			t.Errorf("Failed to add fingerprint: %s", fp)
		}
	}

	// Check that all added fingerprints are found
	for _, fp := range fingerprints {
		if !cf.Contains(fp) {
			t.Errorf("Fingerprint not found: %s", fp)
		}
	}

	// Check that non-existent fingerprints are not found
	nonExistent := []string{
		"xyz999",
		"mno888",
		"pqr777",
	}

	for _, fp := range nonExistent {
		if cf.Contains(fp) {
			t.Errorf("False positive for fingerprint: %s", fp)
		}
	}
}

func TestCuckooFilterRemove(t *testing.T) {
	cf := NewCuckooFilter(1000, 0.01)

	// Add fingerprints
	fingerprints := []string{"abc123", "def456", "ghi789"}
	for _, fp := range fingerprints {
		cf.Add(fp)
	}

	// Remove one fingerprint
	if !cf.Remove("def456") {
		t.Error("Failed to remove fingerprint")
	}

	// Check that removed fingerprint is not found
	if cf.Contains("def456") {
		t.Error("Removed fingerprint still found")
	}

	// Check that other fingerprints are still found
	if !cf.Contains("abc123") || !cf.Contains("ghi789") {
		t.Error("Other fingerprints not found after removal")
	}
}

func TestCuckooFilterCapacity(t *testing.T) {
	capacity := 100
	cf := NewCuckooFilter(capacity, 0.01)

	// Add fingerprints up to capacity
	for i := 0; i < capacity; i++ {
		fp := fmt.Sprintf("fingerprint_%d", i)
		if !cf.Add(fp) {
			t.Errorf("Failed to add fingerprint at index %d", i)
		}
	}

	// Check size
	if cf.Size() != capacity {
		t.Errorf("Expected size %d, got %d", capacity, cf.Size())
	}

	// Check load factor
	expectedLoadFactor := float64(capacity) / float64(cf.Capacity())
	if cf.LoadFactor() != expectedLoadFactor {
		t.Errorf("Expected load factor %f, got %f", expectedLoadFactor, cf.LoadFactor())
	}
}

func TestCuckooFilterFalsePositiveRate(t *testing.T) {
	capacity := 10000
	cf := NewCuckooFilter(capacity, 0.01) // 1% false positive rate

	// Add fingerprints
	addedFingerprints := make([]string, capacity)
	for i := 0; i < capacity; i++ {
		fp := fmt.Sprintf("added_fp_%d_%d", i, rand.Int())
		addedFingerprints[i] = fp
		cf.Add(fp)
	}

	// Test with non-existent fingerprints
	falsePositives := 0
	totalTests := 10000

	for i := 0; i < totalTests; i++ {
		fp := fmt.Sprintf("test_fp_%d_%d", i, rand.Int())

		// Ensure this fingerprint wasn't added
		found := false
		for _, added := range addedFingerprints {
			if added == fp {
				found = true
				break
			}
		}

		if !found && cf.Contains(fp) {
			falsePositives++
		}
	}

	falsePositiveRate := float64(falsePositives) / float64(totalTests)

	// Allow some tolerance (actual rate should be close to 0.01)
	if falsePositiveRate > 0.02 { // 2% tolerance
		t.Errorf("False positive rate too high: %f (expected ~0.01)", falsePositiveRate)
	}

	t.Logf("False positive rate: %f (%d/%d)", falsePositiveRate, falsePositives, totalTests)
}

func TestCuckooFilterCollisionHandling(t *testing.T) {
	// Create a small filter to force collisions
	cf := NewCuckooFilter(10, 0.01)

	// Add many fingerprints to force collisions
	successfulAdds := 0
	for i := 0; i < 100; i++ {
		fp := fmt.Sprintf("collision_test_%d", i)
		if cf.Add(fp) {
			successfulAdds++
		}
	}

	// Should be able to add at least some fingerprints even with collisions
	if successfulAdds == 0 {
		t.Error("No fingerprints could be added due to collisions")
	}

	t.Logf("Successfully added %d fingerprints with collisions", successfulAdds)
}

func TestCuckooFilterConcurrentAccess(t *testing.T) {
	cf := NewCuckooFilter(1000, 0.01)

	// Test concurrent adds
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				fp := fmt.Sprintf("concurrent_%d_%d", id, j)
				cf.Add(fp)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify that some fingerprints were added
	if cf.Size() == 0 {
		t.Error("No fingerprints were added in concurrent test")
	}

	t.Logf("Added %d fingerprints in concurrent test", cf.Size())
}

func TestCuckooFilterStats(t *testing.T) {
	cf := NewCuckooFilter(1000, 0.01)

	// Add some fingerprints
	for i := 0; i < 100; i++ {
		cf.Add(fmt.Sprintf("stats_test_%d", i))
	}

	stats := cf.GetStats()

	// Check required stats
	requiredStats := []string{"size", "capacity", "load_factor", "num_buckets", "bucket_size", "fingerprint_size", "max_kicks"}
	for _, stat := range requiredStats {
		if _, exists := stats[stat]; !exists {
			t.Errorf("Missing stat: %s", stat)
		}
	}

	// Check specific values
	if stats["size"] != 100 {
		t.Errorf("Expected size 100, got %v", stats["size"])
	}

	if stats["bucket_size"] != 4 {
		t.Errorf("Expected bucket size 4, got %v", stats["bucket_size"])
	}

	t.Logf("Stats: %+v", stats)
}

func BenchmarkCuckooFilterAdd(b *testing.B) {
	cf := NewCuckooFilter(100000, 0.01)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fp := fmt.Sprintf("benchmark_fp_%d", i)
		cf.Add(fp)
	}
}

func BenchmarkCuckooFilterContains(b *testing.B) {
	cf := NewCuckooFilter(100000, 0.01)

	// Pre-populate with fingerprints
	for i := 0; i < 10000; i++ {
		fp := fmt.Sprintf("benchmark_fp_%d", i)
		cf.Add(fp)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fp := fmt.Sprintf("benchmark_fp_%d", i%10000)
		cf.Contains(fp)
	}
}

func BenchmarkCuckooFilterRemove(b *testing.B) {
	cf := NewCuckooFilter(100000, 0.01)

	// Pre-populate with fingerprints (limit to avoid memory issues)
	maxFingerprints := 10000
	if b.N > maxFingerprints {
		maxFingerprints = b.N
	}

	for i := 0; i < maxFingerprints; i++ {
		fp := fmt.Sprintf("benchmark_fp_%d", i)
		cf.Add(fp)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fp := fmt.Sprintf("benchmark_fp_%d", i%maxFingerprints)
		cf.Remove(fp)
	}
}
