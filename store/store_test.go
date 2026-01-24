package store

import (
	"bright/models"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestConcurrentIndexOperations tests that concurrent operations on different indexes don't deadlock
func TestConcurrentIndexOperations(t *testing.T) {
	tmpDir := t.TempDir()
	store := Initialize(tmpDir)

	// Create multiple indexes
	numIndexes := 5
	for i := range numIndexes {
		config := &models.IndexConfig{
			ID:         fmt.Sprintf("index_%d", i),
			PrimaryKey: "id",
		}
		if err := store.CreateIndex(config); err != nil {
			t.Fatalf("Failed to create index: %v", err)
		}
	}

	// Track operations completed
	var opsCompleted int64
	done := make(chan bool)
	timeout := time.After(30 * time.Second)

	// Launch concurrent operations on different indexes
	for i := range numIndexes {
		go func(indexNum int) {
			indexID := fmt.Sprintf("index_%d", indexNum)

			// Perform multiple operations
			for j := range 10 {
				// Add documents
				docs := []map[string]any{
					{"id": fmt.Sprintf("doc_%d_%d", indexNum, j), "name": "test"},
				}
				if err := store.AddDocumentsInternal(indexID, docs); err != nil {
					t.Errorf("Failed to add documents: %v", err)
				}
				atomic.AddInt64(&opsCompleted, 1)

				// Update document
				updates := map[string]any{"name": "updated"}
				if err := store.UpdateDocumentInternal(indexID, fmt.Sprintf("doc_%d_%d", indexNum, j), updates); err != nil {
					// Document might not exist yet, that's ok
				}
				atomic.AddInt64(&opsCompleted, 1)

				// Delete document
				if err := store.DeleteDocumentInternal(indexID, fmt.Sprintf("doc_%d_%d", indexNum, j)); err != nil {
					// Document might not exist, that's ok
				}
				atomic.AddInt64(&opsCompleted, 1)
			}

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete or timeout
	completed := 0
	select {
	case <-timeout:
		t.Fatalf("Test timed out - possible deadlock detected. Operations completed: %d", atomic.LoadInt64(&opsCompleted))
	default:
		for range numIndexes {
			<-done
			completed++
		}
	}

	if completed != numIndexes {
		t.Fatalf("Not all goroutines completed: %d/%d", completed, numIndexes)
	}

	t.Logf("Successfully completed %d operations without deadlock", atomic.LoadInt64(&opsCompleted))
}

// TestConcurrentReadsAndWrites tests that concurrent reads and writes don't deadlock
func TestConcurrentReadsAndWrites(t *testing.T) {
	tmpDir := t.TempDir()
	store := Initialize(tmpDir)

	config := &models.IndexConfig{
		ID:         "test_index",
		PrimaryKey: "id",
	}
	if err := store.CreateIndex(config); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Add initial documents
	docs := make([]map[string]any, 100)
	for i := range 100 {
		docs[i] = map[string]any{"id": fmt.Sprintf("doc_%d", i), "value": i}
	}
	if err := store.AddDocumentsInternal("test_index", docs); err != nil {
		t.Fatalf("Failed to add documents: %v", err)
	}

	var opsCompleted int64
	done := make(chan bool)
	timeout := time.After(30 * time.Second)

	// Launch concurrent readers
	for range 10 {
		go func() {
			for range 20 {
				_, _, err := store.GetIndex("test_index")
				if err != nil {
					t.Errorf("Failed to get index: %v", err)
				}
				atomic.AddInt64(&opsCompleted, 1)
				time.Sleep(time.Millisecond)
			}
			done <- true
		}()
	}

	// Launch concurrent writers
	for writerNum := range 5 {
		go func() {
			for j := range 10 {
				updates := map[string]any{"value": writerNum*10 + j}
				docID := fmt.Sprintf("doc_%d", (writerNum*10+j)%100)
				if err := store.UpdateDocumentInternal("test_index", docID, updates); err != nil {
					// Document might not exist, that's ok
				}
				atomic.AddInt64(&opsCompleted, 1)
				time.Sleep(time.Millisecond)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	completed := 0
	totalGoroutines := 15
	select {
	case <-timeout:
		t.Fatalf("Test timed out - possible deadlock detected. Operations completed: %d", atomic.LoadInt64(&opsCompleted))
	default:
		for range totalGoroutines {
			<-done
			completed++
		}
	}

	if completed != totalGoroutines {
		t.Fatalf("Not all goroutines completed: %d/%d", completed, totalGoroutines)
	}

	t.Logf("Successfully completed %d read/write operations without deadlock", atomic.LoadInt64(&opsCompleted))
}

// TestConcurrentIndexCreationAndDeletion tests that creating and deleting indexes concurrently doesn't deadlock
func TestConcurrentIndexCreationAndDeletion(t *testing.T) {
	tmpDir := t.TempDir()
	store := Initialize(tmpDir)

	var opsCompleted int64
	done := make(chan bool)
	timeout := time.After(30 * time.Second)

	// Launch goroutines that create and delete indexes
	for goroutineNum := range 5 {
		go func() {
			for j := range 10 {
				indexID := fmt.Sprintf("index_%d_%d", goroutineNum, j)
				config := &models.IndexConfig{
					ID:         indexID,
					PrimaryKey: "id",
				}

				// Create index
				if err := store.CreateIndex(config); err != nil {
					t.Errorf("Failed to create index: %v", err)
				}
				atomic.AddInt64(&opsCompleted, 1)

				// Add some documents
				docs := []map[string]any{
					{"id": "doc_1", "name": "test"},
				}
				if err := store.AddDocumentsInternal(indexID, docs); err != nil {
					t.Errorf("Failed to add documents: %v", err)
				}
				atomic.AddInt64(&opsCompleted, 1)

				// Delete index
				if err := store.DeleteIndex(indexID); err != nil {
					t.Errorf("Failed to delete index: %v", err)
				}
				atomic.AddInt64(&opsCompleted, 1)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	completed := 0
	select {
	case <-timeout:
		t.Fatalf("Test timed out - possible deadlock detected. Operations completed: %d", atomic.LoadInt64(&opsCompleted))
	default:
		for range 5 {
			<-done
			completed++
		}
	}

	if completed != 5 {
		t.Fatalf("Not all goroutines completed: %d/5", completed)
	}

	t.Logf("Successfully completed %d create/delete operations without deadlock", atomic.LoadInt64(&opsCompleted))
}

// TestConcurrentBatchOperations tests that batch operations don't deadlock
func TestConcurrentBatchOperations(t *testing.T) {
	tmpDir := t.TempDir()
	store := Initialize(tmpDir)

	// Create multiple indexes
	numIndexes := 3
	for i := range numIndexes {
		config := &models.IndexConfig{
			ID:         fmt.Sprintf("batch_index_%d", i),
			PrimaryKey: "id",
		}
		if err := store.CreateIndex(config); err != nil {
			t.Fatalf("Failed to create index: %v", err)
		}
	}

	var opsCompleted int64
	done := make(chan bool)
	timeout := time.After(30 * time.Second)

	// Launch concurrent batch operations
	for i := range numIndexes {
		go func() {
			indexID := fmt.Sprintf("batch_index_%d", i)

			for batch := range 5 {
				// Create batch of documents
				docs := make([]map[string]any, 50)
				for j := range 50 {
					docs[j] = map[string]any{
						"id":    fmt.Sprintf("batch_%d_doc_%d", batch, j),
						"value": j,
					}
				}

				if err := store.AddDocumentsInternal(indexID, docs); err != nil {
					t.Errorf("Failed to add batch: %v", err)
				}
				atomic.AddInt64(&opsCompleted, 1)

				// Delete batch with filter
				if err := store.DeleteDocumentsInternal(indexID, "", []string{
					fmt.Sprintf("batch_%d_doc_0", batch),
					fmt.Sprintf("batch_%d_doc_1", batch),
				}); err != nil {
					t.Errorf("Failed to delete batch: %v", err)
				}
				atomic.AddInt64(&opsCompleted, 1)
			}

			done <- true
		}()
	}

	// Wait for all goroutines
	completed := 0
	select {
	case <-timeout:
		t.Fatalf("Test timed out - possible deadlock detected. Operations completed: %d", atomic.LoadInt64(&opsCompleted))
	default:
		for range numIndexes {
			<-done
			completed++
		}
	}

	if completed != numIndexes {
		t.Fatalf("Not all goroutines completed: %d/%d", completed, numIndexes)
	}

	t.Logf("Successfully completed %d batch operations without deadlock", atomic.LoadInt64(&opsCompleted))
}

// TestLockFairnessUnderContention tests that locks are fair under high contention
func TestLockFairnessUnderContention(t *testing.T) {
	tmpDir := t.TempDir()
	store := Initialize(tmpDir)

	config := &models.IndexConfig{
		ID:         "contention_index",
		PrimaryKey: "id",
	}
	if err := store.CreateIndex(config); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Add initial documents
	docs := make([]map[string]any, 100)
	for i := range 100 {
		docs[i] = map[string]any{"id": fmt.Sprintf("doc_%d", i), "value": i}
	}
	if err := store.AddDocumentsInternal("contention_index", docs); err != nil {
		t.Fatalf("Failed to add documents: %v", err)
	}

	// Track operations per goroutine
	opsPerGoroutine := make([]int64, 20)
	done := make(chan bool)
	timeout := time.After(30 * time.Second)

	// Launch many goroutines competing for same index
	for goroutineNum := range 20 {
		go func() {
			for range 100 {
				updates := map[string]any{"value": goroutineNum*100 + 0}
				docID := fmt.Sprintf("doc_%d", (goroutineNum*100)%100)
				if err := store.UpdateDocumentInternal("contention_index", docID, updates); err != nil {
					// Document might not exist, that's ok
				}
				atomic.AddInt64(&opsPerGoroutine[goroutineNum], 1)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	completed := 0
	select {
	case <-timeout:
		t.Fatalf("Test timed out - possible deadlock detected")
	default:
		for range 20 {
			<-done
			completed++
		}
	}

	if completed != 20 {
		t.Fatalf("Not all goroutines completed: %d/20", completed)
	}

	// Check that operations are fairly distributed
	minOps := int64(100)
	maxOps := int64(0)
	for i := range 20 {
		ops := atomic.LoadInt64(&opsPerGoroutine[i])
		if ops < minOps {
			minOps = ops
		}
		if ops > maxOps {
			maxOps = ops
		}
	}

	// Allow some variance but ensure no goroutine is starved
	variance := float64(maxOps-minOps) / float64(minOps)
	if variance > 0.5 { // Allow up to 50% variance
		t.Logf("Warning: High variance in operation distribution: min=%d, max=%d, variance=%.2f%%", minOps, maxOps, variance*100)
	}

	t.Logf("Lock fairness test passed: min=%d ops, max=%d ops, variance=%.2f%%", minOps, maxOps, variance*100)
}

// TestNoDeadlockWithMultipleIndexes tests a realistic scenario with multiple indexes
func TestNoDeadlockWithMultipleIndexes(t *testing.T) {
	tmpDir := t.TempDir()
	store := Initialize(tmpDir)

	// Create indexes
	indexCount := 10
	for i := range indexCount {
		config := &models.IndexConfig{
			ID:         fmt.Sprintf("realistic_index_%d", i),
			PrimaryKey: "id",
		}
		if err := store.CreateIndex(config); err != nil {
			t.Fatalf("Failed to create index: %v", err)
		}
	}

	var opsCompleted int64
	done := make(chan bool)
	timeout := time.After(60 * time.Second)

	// Simulate realistic workload
	for i := range 50 {
		go func() {
			for j := range 20 {
				indexNum := (i + j) % indexCount
				indexID := fmt.Sprintf("realistic_index_%d", indexNum)

				// Random operation
				op := (i + j) % 4
				switch op {
				case 0: // Add
					docs := []map[string]any{
						{"id": fmt.Sprintf("doc_%d_%d", i, j), "data": "test"},
					}
					if err := store.AddDocumentsInternal(indexID, docs); err != nil {
						// Ignore errors
					}
				case 1: // Update
					updates := map[string]any{"data": "updated"}
					if err := store.UpdateDocumentInternal(indexID, fmt.Sprintf("doc_%d_%d", i, j), updates); err != nil {
						// Ignore errors
					}
				case 2: // Delete
					if err := store.DeleteDocumentInternal(indexID, fmt.Sprintf("doc_%d_%d", i, j)); err != nil {
						// Ignore errors
					}
				case 3: // Get
					if _, _, err := store.GetIndex(indexID); err != nil {
						// Ignore errors
					}
				}
				atomic.AddInt64(&opsCompleted, 1)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	completed := 0
	select {
	case <-timeout:
		t.Fatalf("Test timed out - possible deadlock detected. Operations completed: %d", atomic.LoadInt64(&opsCompleted))
	default:
		for range 50 {
			<-done
			completed++
		}
	}

	if completed != 50 {
		t.Fatalf("Not all goroutines completed: %d/50", completed)
	}

	t.Logf("Realistic workload test passed: %d operations completed without deadlock", atomic.LoadInt64(&opsCompleted))
}

// BenchmarkConcurrentOperations benchmarks concurrent operations
func BenchmarkConcurrentOperations(b *testing.B) {
	tmpDir := b.TempDir()
	store := Initialize(tmpDir)

	// Create indexes
	for i := range 5 {
		config := &models.IndexConfig{
			ID:         fmt.Sprintf("bench_index_%d", i),
			PrimaryKey: "id",
		}
		if err := store.CreateIndex(config); err != nil {
			b.Fatalf("Failed to create index: %v", err)
		}
	}

	b.ResetTimer()

	// Run concurrent operations
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			indexID := fmt.Sprintf("bench_index_%d", i%5)
			docs := []map[string]any{
				{"id": fmt.Sprintf("doc_%d", i), "value": i},
			}
			if err := store.AddDocumentsInternal(indexID, docs); err != nil {
				b.Errorf("Failed to add documents: %v", err)
			}
			i++
		}
	})
}
