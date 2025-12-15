package cache

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/saltyorg/sb-go/internal/constants"
)

// Cache is a thread-safe structure for storing and persisting cached data.
// It maintains an in-memory map (data) protected by a read-write mutex (mu)
// and saves the cache contents to a file (file) for persistence.
type Cache struct {
	data map[string]any
	mu   sync.RWMutex
	file string
}

// NewCache initializes and returns a new Cache instance.
// It sets up the cache with an empty data map and a fixed file path defined in constants.
// After initialization, it attempts to load any existing cache data from the file.
// Returns the Cache instance or an error if loading the cache fails.
func NewCache() (*Cache, error) {
	return NewCacheWithFile(constants.SaltboxCacheFile)
}

// NewCacheWithFile initializes and returns a new Cache instance with a custom file path.
// This is primarily useful for testing to avoid interfering with the actual cache file.
// It sets up the cache with an empty data map and the specified file path.
// After initialization, it attempts to load any existing cache data from the file.
// Returns the Cache instance or an error if loading the cache fails.
func NewCacheWithFile(filePath string) (*Cache, error) {
	c := &Cache{
		data: make(map[string]any),
		file: filePath,
	}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

// GetRepoCache retrieves cached data for a specific repository, identified by repoPath.
// It returns the repository's cache data (as a map) and a boolean indicating whether the cache exists.
func (c *Cache) GetRepoCache(repoPath string) (map[string]any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	repoCache, ok := c.data[repoPath].(map[string]any)
	return repoCache, ok
}

// SetRepoCache updates the cache for a specific repository with new data.
// It locks the cache for writing, updates the repository's entry, and then saves the entire cache to the file.
// Note: Save errors are not returned as cache operations are non-critical to application functionality.
func (c *Cache) SetRepoCache(repoPath string, repoCache map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[repoPath] = repoCache
	_ = c.save()
}

// CheckCache verifies whether all provided tags exist in the cached data for a given repository.
// It returns a boolean indicating if all tags are present and a slice of any missing tags.
// If no cache exists for the repository or if the tags cannot be parsed, it assumes the cache is missing.
func (c *Cache) CheckCache(repoPath string, tags []string) (bool, []string) {
	repoCache, ok := c.GetRepoCache(repoPath)
	if !ok || repoCache == nil {
		return true, []string{}
	}

	cachedTags, ok := repoCache["tags"].([]any)
	if !ok {
		return true, []string{}
	}

	// Build a set of cached tags for a quick lookup.
	cachedTagSet := make(map[string]bool)
	for _, tag := range cachedTags {
		if strTag, ok := tag.(string); ok {
			cachedTagSet[strTag] = true
		}
	}

	// Identify any tags missing from the cache.
	var missingTags []string
	for _, tag := range tags {
		if _, ok := cachedTagSet[tag]; !ok {
			missingTags = append(missingTags, tag)
		}
	}

	return len(missingTags) == 0, missingTags
}

// load reads the cache data from the file specified in the Cache struct.
// If the file does not exist, the cache remains empty (no error is returned).
// On success, it will unmarshal the JSON data into the cache's internal map.
func (c *Cache) load() error {
	if _, err := os.Stat(c.file); os.IsNotExist(err) {
		return nil // File doesn't exist; start with an empty cache.
	}

	data, err := os.ReadFile(c.file)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return json.Unmarshal(data, &c.data)
}

// save serializes the current cache data to JSON and writes it to the file specified in the Cache struct.
// The caller must hold a lock on c.mu before calling this method.
// The file is written with permissions 0644.
func (c *Cache) save() error {
	data, err := json.Marshal(c.data)
	if err != nil {
		return err
	}

	return os.WriteFile(c.file, data, 0644)
}
