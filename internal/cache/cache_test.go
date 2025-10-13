package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCache_SetGetRepoCache(t *testing.T) {
	// Create a temporary directory for test cache file
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "test_cache.json")

	// Create cache instance with custom file path
	cache := &Cache{
		data: make(map[string]any),
		file: cacheFile,
	}

	// Test data
	testRepoPath := "/test/repo/path"
	testData := map[string]any{
		"tags":    []string{"tag1", "tag2", "tag3"},
		"version": "1.0.0",
	}

	// Set repo cache
	cache.SetRepoCache(testRepoPath, testData)

	// Get repo cache
	retrieved, ok := cache.GetRepoCache(testRepoPath)
	if !ok {
		t.Fatal("Expected to retrieve cache, but it was not found")
	}

	// Verify tags
	tags, ok := retrieved["tags"].([]string)
	if !ok {
		t.Fatal("Expected tags to be []string")
	}

	expectedTags := testData["tags"].([]string)
	if len(tags) != len(expectedTags) {
		t.Errorf("Expected %d tags, got %d", len(expectedTags), len(tags))
	}

	// Verify version
	version, ok := retrieved["version"].(string)
	if !ok {
		t.Fatal("Expected version to be string")
	}

	if version != testData["version"] {
		t.Errorf("Expected version %s, got %s", testData["version"], version)
	}
}

func TestCache_GetRepoCache_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "test_cache.json")

	cache := &Cache{
		data: make(map[string]any),
		file: cacheFile,
	}

	// Try to get non-existent repo cache
	_, ok := cache.GetRepoCache("/nonexistent/repo")
	if ok {
		t.Error("Expected cache not to be found, but it was")
	}
}

func TestCache_LoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "test_cache.json")

	// Create test cache data
	testData := map[string]any{
		"/repo1": map[string]any{
			"tags": []any{"tag1", "tag2"},
		},
		"/repo2": map[string]any{
			"tags": []any{"tag3", "tag4"},
		},
	}

	// Write test data to file
	jsonData, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	if err := os.WriteFile(cacheFile, jsonData, 0644); err != nil {
		t.Fatalf("Failed to write test cache file: %v", err)
	}

	// Create cache and load from file
	cache := &Cache{
		data: make(map[string]any),
		file: cacheFile,
	}

	if err := cache.load(); err != nil {
		t.Fatalf("Failed to load cache: %v", err)
	}

	// Verify loaded data
	repo1, ok := cache.GetRepoCache("/repo1")
	if !ok {
		t.Error("Expected /repo1 to be loaded")
	}

	tags, ok := repo1["tags"].([]any)
	if !ok {
		t.Fatal("Expected tags to be []any")
	}

	if len(tags) != 2 {
		t.Errorf("Expected 2 tags for /repo1, got %d", len(tags))
	}
}

func TestCache_SaveToFile(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "test_cache.json")

	cache := &Cache{
		data: make(map[string]any),
		file: cacheFile,
	}

	// Add test data
	testData := map[string]any{
		"tags": []string{"tag1", "tag2"},
	}
	cache.SetRepoCache("/test/repo", testData)

	// Verify file was created
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		t.Fatal("Cache file was not created")
	}

	// Read and verify file contents
	fileData, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("Failed to read cache file: %v", err)
	}

	var loadedData map[string]any
	if err := json.Unmarshal(fileData, &loadedData); err != nil {
		t.Fatalf("Failed to unmarshal cache file: %v", err)
	}

	if _, ok := loadedData["/test/repo"]; !ok {
		t.Error("Expected /test/repo to be in saved cache")
	}
}

func TestCache_LoadFromFile_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "nonexistent_cache.json")

	cache := &Cache{
		data: make(map[string]any),
		file: cacheFile,
	}

	// Loading non-existent file should not return error
	if err := cache.load(); err != nil {
		t.Errorf("Expected no error when loading non-existent file, got: %v", err)
	}

	// Cache should be empty
	if len(cache.data) != 0 {
		t.Error("Expected cache to be empty when file doesn't exist")
	}
}

func TestCache_CheckCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "test_cache.json")

	cache := &Cache{
		data: make(map[string]any),
		file: cacheFile,
	}

	// Set up test repo cache
	testRepoPath := "/test/repo"
	testData := map[string]any{
		"tags": []any{"tag1", "tag2", "tag3"},
	}
	cache.SetRepoCache(testRepoPath, testData)

	tests := []struct {
		name            string
		repoPath        string
		tags            []string
		expectedFound   bool
		expectedMissing []string
	}{
		{
			name:            "All tags present",
			repoPath:        testRepoPath,
			tags:            []string{"tag1", "tag2"},
			expectedFound:   true,
			expectedMissing: []string{},
		},
		{
			name:            "Some tags missing",
			repoPath:        testRepoPath,
			tags:            []string{"tag1", "tag4"},
			expectedFound:   false,
			expectedMissing: []string{"tag4"},
		},
		{
			name:            "All tags missing",
			repoPath:        testRepoPath,
			tags:            []string{"tag4", "tag5"},
			expectedFound:   false,
			expectedMissing: []string{"tag4", "tag5"},
		},
		{
			name:            "Non-existent repo",
			repoPath:        "/nonexistent",
			tags:            []string{"tag1"},
			expectedFound:   true,
			expectedMissing: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, missing := cache.CheckCache(tt.repoPath, tt.tags)

			if found != tt.expectedFound {
				t.Errorf("Expected found=%v, got %v", tt.expectedFound, found)
			}

			if len(missing) != len(tt.expectedMissing) {
				t.Errorf("Expected %d missing tags, got %d", len(tt.expectedMissing), len(missing))
			}

			// Verify missing tags match
			for _, expectedTag := range tt.expectedMissing {
				foundMissing := slices.Contains(missing, expectedTag)
				if !foundMissing {
					t.Errorf("Expected missing tag %s not found in result", expectedTag)
				}
			}
		})
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "concurrent_cache.json")

	cache := &Cache{
		data: make(map[string]any),
		file: cacheFile,
	}

	// Test concurrent writes and reads
	done := make(chan bool)

	// Writer goroutines
	for i := range 10 {
		go func(id int) {
			testData := map[string]any{
				"tags": []string{"tag1", "tag2"},
			}
			cache.SetRepoCache("/test/repo", testData)
			done <- true
		}(i)
	}

	// Reader goroutines
	for i := range 10 {
		go func(id int) {
			_, _ = cache.GetRepoCache("/test/repo")
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range 20 {
		<-done
	}

	// Verify cache is still valid
	data, ok := cache.GetRepoCache("/test/repo")
	if !ok {
		t.Error("Expected cache to exist after concurrent access")
	}

	if data == nil {
		t.Error("Expected cache data to be non-nil")
	}
}
