package table

import (
	"bytes"
	"strings"
	"testing"

	aquatable "github.com/aquasecurity/table"
	runewidth "github.com/mattn/go-runewidth"
)

// verifyLineWidths checks that all lines have the same width
func verifyLineWidths(t *testing.T, output string) {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		return
	}

	// Get the expected width from the first line (using display width, not byte count)
	expectedWidth := runewidth.StringWidth(stripANSI(lines[0]))

	for i, line := range lines {
		stripped := stripANSI(line)
		actualWidth := runewidth.StringWidth(stripped)
		if actualWidth != expectedWidth {
			t.Errorf("Line %d has width %d, expected %d\nLine: %q\nStripped: %q",
				i, actualWidth, expectedWidth, line, stripped)
		}
	}
}

func TestBasicTable(t *testing.T) {
	buf := &bytes.Buffer{}
	table := New(buf)

	table.SetHeaders("Column 1", "Column 2")
	table.SetAlignment(aquatable.AlignLeft, aquatable.AlignLeft)
	table.SetDividers(aquatable.UnicodeRoundedDividers)
	table.SetBorders(true)

	table.AddRow("Row 1 Col 1", "Row 1 Col 2")
	table.AddRow("Row 2 Col 1", "Row 2 Col 2")

	table.Render()

	output := buf.String()
	if output == "" {
		t.Error("Expected non-empty output")
	}

	// Verify all lines have the same width
	verifyLineWidths(t, output)

	// Check that the output contains the headers
	if !strings.Contains(output, "Column 1") {
		t.Error("Expected output to contain 'Column 1'")
	}
	if !strings.Contains(output, "Column 2") {
		t.Error("Expected output to contain 'Column 2'")
	}

	// Check that the output contains the row data
	if !strings.Contains(output, "Row 1 Col 1") {
		t.Error("Expected output to contain 'Row 1 Col 1'")
	}
	if !strings.Contains(output, "Row 2 Col 2") {
		t.Error("Expected output to contain 'Row 2 Col 2'")
	}
}

func TestColspan(t *testing.T) {
	buf := &bytes.Buffer{}
	table := New(buf)

	table.SetHeaders("Main Header")
	table.SetHeaderColSpans(0, 2)
	table.SetAlignment(aquatable.AlignLeft, aquatable.AlignLeft)
	table.SetDividers(aquatable.UnicodeRoundedDividers)
	table.SetBorders(true)

	table.AddRow("Row 1 Col 1", "Row 1 Col 2")
	table.AddRow("Section Header")
	table.SetColSpans(1, 2)
	table.AddRow("Row 3 Col 1", "Row 3 Col 2")

	table.Render()

	output := buf.String()
	if output == "" {
		t.Error("Expected non-empty output")
	}

	// Verify all lines have the same width
	verifyLineWidths(t, output)

	// Check that headers are present
	if !strings.Contains(output, "Main Header") {
		t.Error("Expected output to contain 'Main Header'")
	}

	// Check that colspan row is present
	if !strings.Contains(output, "Section Header") {
		t.Error("Expected output to contain 'Section Header'")
	}
}

func TestHeaderColspan(t *testing.T) {
	buf := &bytes.Buffer{}
	table := New(buf)

	table.SetHeaders("Saltbox")
	table.SetHeaderColSpans(0, 2)
	table.SetHeaderStyle(aquatable.StyleBold)
	table.SetAlignment(aquatable.AlignLeft, aquatable.AlignLeft)
	table.SetDividers(aquatable.UnicodeRoundedDividers)
	table.SetBorders(true)

	table.AddRow("plex", "sb install plex")
	table.AddRow("sonarr", "sb install sonarr")

	table.Render()

	output := buf.String()
	if output == "" {
		t.Error("Expected non-empty output")
	}

	// Verify all lines have the same width
	verifyLineWidths(t, output)

	// Check that the header spans across both columns
	if !strings.Contains(output, "Saltbox") {
		t.Error("Expected output to contain 'Saltbox'")
	}
}

func TestStyling(t *testing.T) {
	buf := &bytes.Buffer{}
	table := New(buf)

	table.SetHeaders("Header")
	table.SetHeaderStyle(aquatable.StyleBold)
	table.SetLineStyle(aquatable.StyleBlue)
	table.SetDividers(aquatable.UnicodeRoundedDividers)
	table.SetBorders(true)

	table.AddRow("Test")

	table.Render()

	output := buf.String()
	if output == "" {
		t.Error("Expected non-empty output")
	}

	// Check for ANSI codes (basic check)
	if !strings.Contains(output, "\x1b[") {
		t.Error("Expected output to contain ANSI escape codes")
	}
}

func TestMultipleColspanSections(t *testing.T) {
	buf := &bytes.Buffer{}
	table := New(buf)

	table.SetHeaders("Saltbox")
	table.SetHeaderColSpans(0, 2)
	table.SetHeaderStyle(aquatable.StyleBold)
	table.SetAlignment(aquatable.AlignLeft, aquatable.AlignLeft)
	table.SetBorders(true)
	table.SetRowLines(true)
	table.SetDividers(aquatable.UnicodeRoundedDividers)
	table.SetLineStyle(aquatable.StyleBlue)
	table.SetPadding(1)

	// Saltbox section
	table.AddRow("plex", "sb install plex")
	table.AddRow("sonarr", "sb install sonarr")

	// Sandbox section with header
	rowIndex := 2
	table.AddRow("\x1b[1mSandbox\x1b[0m")
	table.SetColSpans(rowIndex, 2)
	_ = rowIndex // rowIndex is set but not used further in this test

	table.AddRow("sandbox-app1", "sb install sandbox-sandbox-app1")
	table.AddRow("sandbox-app2", "sb install sandbox-sandbox-app2")

	table.Render()

	output := buf.String()
	if output == "" {
		t.Error("Expected non-empty output")
	}

	// Verify all lines have the same width
	verifyLineWidths(t, output)

	// Verify both sections are present
	if !strings.Contains(output, "Saltbox") {
		t.Error("Expected output to contain 'Saltbox'")
	}
	if !strings.Contains(output, "Sandbox") {
		t.Error("Expected output to contain 'Sandbox'")
	}
	if !strings.Contains(output, "plex") {
		t.Error("Expected output to contain 'plex'")
	}
	if !strings.Contains(output, "sandbox-app1") {
		t.Error("Expected output to contain 'sandbox-app1'")
	}
}
