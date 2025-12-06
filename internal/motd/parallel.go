package motd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// InfoProvider defines a function type that provides information with timeout support
type InfoProvider func(ctx context.Context, verbose bool) string

// InfoSource represents a source of system information
type InfoSource struct {
	Key      string // The label for the information (e.g., "Distribution:")
	Provider any    // The function that retrieves the information (can be string or []string)
	Order    int    // Display order for a consistent output
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
func GetSystemInfo(sources []InfoSource, verbose bool) []Result {
	var wg sync.WaitGroup
	resultChan := make(chan Result, len(sources))
	results := make([]Result, 0, len(sources))

	// Launch goroutines for each information source
	for _, source := range sources {
		wg.Add(1)
		go func(src InfoSource) {
			defer wg.Done()

			// Recover from panics to prevent goroutine crashes from hanging the entire MOTD.
			// Without this, a panic in any provider would prevent wg.Done() from being called,
			// causing the wait group to hang indefinitely and no output to be displayed.
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in %s: %v\n", src.Key, r)
					}
					resultChan <- Result{
						Key:   src.Key,
						Value: fmt.Sprintf("Error: panic occurred (%v)", r),
						Order: src.Order,
					}
				}
			}()

			ctx := context.Background()

			// Track timing if verbose mode is enabled
			var start time.Time
			if verbose {
				start = time.Now()
			}

			// Get the information with timeout
			providerFunc, ok := src.Provider.(func(context.Context, bool) string)
			if !ok {
				if verbose {
					fmt.Printf("ERROR: Invalid provider type for %s\n", src.Key)
				}
				resultChan <- Result{
					Key:   src.Key,
					Value: "Error: Invalid provider",
					Order: src.Order,
				}
				return
			}
			value := providerFunc(ctx, verbose)

			// Print timing debug info if verbose mode is enabled
			if verbose {
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
