package motd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Prepare the command
	var cmdStr string

	// If no box type or "none", just use toilet
	if boxType == "" || boxType == "none" {
		cmdStr = fmt.Sprintf("toilet -f %s '%s'", font, title)
	} else {
		// Check if boxes is installed
		if _, err := exec.LookPath("boxes"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: boxes command not found, using toilet only\n")
			cmdStr = fmt.Sprintf("toilet -f %s '%s'", font, title)
		} else {
			cmdStr = fmt.Sprintf("toilet -f %s '%s' | boxes -d %s -a hc -p h8",
				font, title, boxType)
		}
	}

	// Run the command
	cmd := exec.Command("bash", "-c", cmdStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating banner: %v\n", err)
		fmt.Fprintf(os.Stderr, "Command output: %s\n", string(output))
		return fmt.Sprintf("--- %s ---\n", title)
	}

	return string(output)
}
