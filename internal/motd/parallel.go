package motd

import (
	"context"
	"fmt"
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
	Order    int           // Display order for a consistent output
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

			// Track timing if verbose mode is enabled
			var start time.Time
			if Verbose {
				start = time.Now()
			}

			// Get the information with timeout
			providerFunc := src.Provider.(func(context.Context) string)
			value := providerFunc(ctx)

			// Print timing debug info if verbose mode is enabled
			if Verbose {
				elapsed := time.Since(start)
				fmt.Printf("DEBUG: %s took %v\n", src.Key, elapsed)
			}

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

	// Sort results to maintain a consistent display order
	sort.Slice(results, func(i, j int) bool {
		return results[i].Order < results[j].Order
	})

	return results
}
