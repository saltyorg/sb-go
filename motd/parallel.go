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
	Provider InfoProvider
	Order    int // Display order for a consistent output
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

const defaultProviderTimeout = 30 * time.Second

// GetSystemInfo gathers all requested system information in parallel
func GetSystemInfo(ctx context.Context, sources []InfoSource, verbose bool) []Result {
	return getSystemInfo(ctx, sources, verbose, defaultProviderTimeout)
}

func getSystemInfo(ctx context.Context, sources []InfoSource, verbose bool, providerTimeout time.Duration) []Result {
	var wg sync.WaitGroup
	resultChan := make(chan Result, len(sources))
	results := make([]Result, 0, len(sources))

	// Launch goroutines for each information source
	for _, source := range sources {
		wg.Add(1)
		go func(src InfoSource) {
			defer wg.Done()

			providerCtx, cancel := context.WithTimeout(ctx, providerTimeout)
			defer cancel()

			// Track timing if verbose mode is enabled
			var start time.Time
			if verbose {
				start = time.Now()
			}

			providerResult := make(chan string, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						if verbose {
							fmt.Fprintf(os.Stderr, "PANIC in %s: %v\n", src.Key, r)
						}
						providerResult <- fmt.Sprintf("Error: panic occurred (%v)", r)
					}
				}()
				providerResult <- src.Provider(providerCtx, verbose)
			}()

			var value string
			select {
			case value = <-providerResult:
			case <-providerCtx.Done():
				if ctx.Err() != nil {
					value = fmt.Sprintf("Error: provider canceled (%v)", ctx.Err())
				} else {
					value = fmt.Sprintf("Error: provider timed out after %s", providerTimeout)
				}
			}

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
