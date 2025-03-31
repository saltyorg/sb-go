package motd

import (
	"context"
	"sort"
	"sync"
	"time"
)

// InfoProvider defines a function type that provides information with timeout support
type InfoProvider func(ctx context.Context) string

// InfoSource represents a source of system information
type InfoSource struct {
	Key      string        // The label for the information (e.g., "Distribution:")
	Provider interface{}   // The function that retrieves the information (can be string or []string)
	Timeout  time.Duration // How long to wait before timing out
	Order    int           // Display order for consistent output
}

// Result stores the output of a single information function
type Result struct {
	Key   string
	Value string
	Order int // For maintaining display order
}

// MultiStringResult stores results that contain multiple lines
type MultiStringResult struct {
	Key    string
	Values []string
	Order  int
}

// GetSystemInfo gathers all requested system information in parallel
func GetSystemInfo(sources []InfoSource) []Result {
	var wg sync.WaitGroup
	resultChan := make(chan Result, len(sources))
	results := make([]Result, 0, len(sources))

	// Launch goroutines for each information source
	for _, source := range sources {
		wg.Add(1)
		go func(src InfoSource) {
			defer wg.Done()

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), src.Timeout)
			defer cancel()

			// Get the information with timeout
			providerFunc := src.Provider.(func(context.Context) string)
			value := providerFunc(ctx)

			resultChan <- Result{
				Key:   src.Key,
				Value: value,
				Order: src.Order,
			}
		}(source)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for r := range resultChan {
		results = append(results, r)
	}

	// Sort results by order to maintain consistent display order
	sort.Slice(results, func(i, j int) bool {
		return results[i].Order < results[j].Order
	})

	return results
}

// GetMultilineSystemInfo gathers information that returns multiple lines
func GetMultilineSystemInfo(source InfoSource) MultiStringResult {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), source.Timeout)
	defer cancel()

	// This is a type assertion that requires the provider to return []string
	providerFunc := func(ctx context.Context) []string {
		if provider, ok := source.Provider.(func(context.Context) []string); ok {
			return provider(ctx)
		}
		return []string{"Error: invalid provider type"}
	}

	return MultiStringResult{
		Key:    source.Key,
		Values: providerFunc(ctx),
		Order:  source.Order,
	}
}

// GetDiskInfoParallel gathers disk usage information in parallel (special case)
func GetDiskInfoParallel(ctx context.Context) []string {
	diskChan := make(chan []string, 1)

	go func() {
		diskChan <- GetDiskUsage()
	}()

	select {
	case result := <-diskChan:
		return result
	case <-ctx.Done():
		return []string{"Disk information timed out"}
	}
}

// GetMemoryInfoParallel gathers memory usage information in parallel
func GetMemoryInfoParallel(ctx context.Context) []string {
	memoryChan := make(chan []string, 1)

	go func() {
		memoryChan <- GetMemoryUsage()
	}()

	select {
	case result := <-memoryChan:
		return result
	case <-ctx.Done():
		return []string{"Memory information timed out"}
	}
}
