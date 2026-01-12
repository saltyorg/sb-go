package motd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/saltyorg/sb-go/internal/executor"
)

// AvailableBannerTypes contains all valid box types for the boxes command
var AvailableBannerTypes = []string{
	"ada-box", "ada-cmt", "bear", "boxquote", "boy", "c", "c-cmt", "c-cmt2", "caml", "capgirl", "cat", "cc",
	"columns", "diamonds", "dog", "f90-box", "f90-cmt", "face", "fence", "girl", "headline", "html", "html-cmt",
	"ian_jones", "important", "important2", "important3", "java-cmt", "javadoc", "jstone", "lisp-cmt", "mouse",
	"normand", "nuke", "parchment", "peek", "pound-cmt", "right", "santa", "scroll", "scroll-akn", "shell",
	"simple", "spring", "stark1", "stark2", "stone", "sunset", "tex-box", "tex-cmt", "twisted", "underline",
	"unicornsay", "unicornthink", "vim-box", "vim-cmt", "weave", "whirly", "xes",
}

// IsValidFont checks if the given font exists in the figlet directory
func IsValidFont(font string) bool {
	if font == "term" {
		// Special built-in font
		return true
	}

	fontDir := "/usr/share/figlet"

	// First check in the font directory
	validExtensions := []string{".flf", ".tlf"}
	for _, ext := range validExtensions {
		fontPath := filepath.Join(fontDir, font+ext)
		if _, err := os.Stat(fontPath); err == nil {
			return true
		}
	}

	// Then check in the current directory
	for _, ext := range validExtensions {
		fontPath := font + ext
		if _, err := os.Stat(fontPath); err == nil {
			return true
		}
	}

	return false
}

// ListAvailableFonts returns all available fonts in the figlet directory
func ListAvailableFonts() []string {
	fonts := []string{"term"} // Add the special built-in font
	fontDir := "/usr/share/figlet"

	entries, err := os.ReadDir(fontDir)
	if err != nil {
		return fonts
	}

	// Extract unique base names of all font files
	fontMap := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			fileName := entry.Name()
			extension := filepath.Ext(fileName)
			if extension == ".flf" || extension == ".tlf" {
				baseName := strings.TrimSuffix(fileName, extension)
				fontMap[baseName] = true
			}
		}
	}

	// Convert map to slice
	for font := range fontMap {
		fonts = append(fonts, font)
	}

	return fonts
}

// GenerateBanner generates the banner using toilet and boxes
func GenerateBanner(title, font, boxType string) string {
	// First check if toilet is installed
	if _, err := exec.LookPath("toilet"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: toilet command not found, using fallback banner\n")
		return fmt.Sprintf("--- %s ---\n", title)
	}

	// If no box type or "none", just use toilet
	if boxType == "" || boxType == "none" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := executor.Run(ctx, "toilet",
			executor.WithArgs("-f", font, title),
			executor.WithOutputMode(executor.OutputModeCombined),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating banner: %v\n", err)
			fmt.Fprintf(os.Stderr, "Command output: %s\n", string(result.Combined))
			return fmt.Sprintf("--- %s ---\n", title)
		}
		return string(result.Combined)
	}

	// Check if boxes is installed
	if _, err := exec.LookPath("boxes"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: boxes command not found, using toilet only\n")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := executor.Run(ctx, "toilet",
			executor.WithArgs("-f", font, title),
			executor.WithOutputMode(executor.OutputModeCombined),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating banner: %v\n", err)
			fmt.Fprintf(os.Stderr, "Command output: %s\n", string(result.Combined))
			return fmt.Sprintf("--- %s ---\n", title)
		}
		return string(result.Combined)
	}

	// Use toilet and pipe to boxes
	// First get toilet output
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	toiletResult, err := executor.Run(ctx, "toilet",
		executor.WithArgs("-f", font, title),
		executor.WithOutputMode(executor.OutputModeCapture),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running toilet: %v\n", err)
		return fmt.Sprintf("--- %s ---\n", title)
	}

	// Pipe toilet output to boxes
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var boxesOutput bytes.Buffer
	boxesResult, err := executor.Run(ctx, "boxes",
		executor.WithArgs("-d", boxType, "-a", "hc", "-p", "h8"),
		executor.WithStdin(bytes.NewReader(toiletResult.Stdout)),
		executor.WithStdout(&boxesOutput),
		executor.WithOutputMode(executor.OutputModeCapture),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running boxes: %v\n", err)
		if len(boxesResult.Stderr) > 0 {
			fmt.Fprintf(os.Stderr, "Boxes stderr: %s\n", string(boxesResult.Stderr))
		}
		return fmt.Sprintf("--- %s ---\n", title)
	}

	return boxesOutput.String()
}

// GenerateBannerFromFile processes the content of a file with toilet
func GenerateBannerFromFile(content string, toiletArgs string) string {
	if _, err := exec.LookPath("toilet"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: toilet command not found. Cannot process banner file.\n")
		return content // Fallback to raw content if toilet isn't installed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := splitShellArgs(toiletArgs)
	result, err := executor.Run(ctx, "toilet",
		executor.WithArgs(args...),
		executor.WithStdin(strings.NewReader(content)),
		executor.WithOutputMode(executor.OutputModeCombined),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating banner from file: %v\n", err)
		fmt.Fprintf(os.Stderr, "Command output: %s\n", string(result.Combined))
		return content // Fallback to raw content on error
	}

	return string(result.Combined)
}

func splitShellArgs(input string) []string {
	var args []string
	var buf strings.Builder
	inQuote := rune(0)
	escaped := false

	flush := func() {
		if buf.Len() > 0 {
			args = append(args, buf.String())
			buf.Reset()
		}
	}

	for _, r := range input {
		if escaped {
			buf.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			continue
		}

		if inQuote != 0 {
			if r == inQuote {
				inQuote = 0
				continue
			}
			buf.WriteRune(r)
			continue
		}

		switch {
		case r == '"' || r == '\'':
			inQuote = r
		case unicode.IsSpace(r):
			flush()
		default:
			buf.WriteRune(r)
		}
	}

	flush()
	return args
}
