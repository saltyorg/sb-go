package table

import (
	"fmt"
	"io"
	"strings"

	aquatable "github.com/aquasecurity/table"
	runewidth "github.com/mattn/go-runewidth"
)

// Use types directly from aquasecurity/table
type (
	Alignment = aquatable.Alignment
	Dividers  = aquatable.Dividers
	Style     = aquatable.Style
)

// Table represents a table with support for colspan and rowspan
type Table struct {
	writer      io.Writer
	headers     []string
	rows        [][]string
	columnAlign []Alignment
	dividers    Dividers
	lineStyle   Style
	headerStyle Style
	padding     int
	borders     bool
	rowLines    bool
	colspans    map[int]int // map row index to colspan value
	headerCols  map[int]int // map header index to colspan value
}

// New creates a new Table
func New(w io.Writer) *Table {
	return &Table{
		writer:     w,
		dividers:   aquatable.UnicodeDividers,
		padding:    1,
		borders:    true,
		rowLines:   false,
		colspans:   make(map[int]int),
		headerCols: make(map[int]int),
	}
}

// SetHeaders sets the table headers
func (t *Table) SetHeaders(headers ...string) {
	t.headers = headers
}

// SetHeaderColSpans sets the colspan for a specific header by index
func (t *Table) SetHeaderColSpans(headerIndex int, colspan int) {
	t.headerCols[headerIndex] = colspan
}

// AddRow adds a row to the table
func (t *Table) AddRow(values ...string) {
	t.rows = append(t.rows, values)
}

// SetColSpans sets the colspan for a specific row by index
func (t *Table) SetColSpans(rowIndex int, colspan int) {
	t.colspans[rowIndex] = colspan
}

// SetAlignment sets the alignment for each column
func (t *Table) SetAlignment(aligns ...Alignment) {
	t.columnAlign = aligns
}

// SetDividers sets the divider style
func (t *Table) SetDividers(d Dividers) {
	t.dividers = d
}

// SetLineStyle sets the style for borders and dividers
func (t *Table) SetLineStyle(s Style) {
	t.lineStyle = s
}

// SetHeaderStyle sets the style for headers
func (t *Table) SetHeaderStyle(s Style) {
	t.headerStyle = s
}

// SetPadding sets the padding for cells
func (t *Table) SetPadding(p int) {
	t.padding = p
}

// SetBorders enables or disables borders
func (t *Table) SetBorders(enabled bool) {
	t.borders = enabled
}

// SetRowLines enables or disables lines between rows
func (t *Table) SetRowLines(enabled bool) {
	t.rowLines = enabled
}

// Render renders the table to the writer
func (t *Table) Render() {
	if len(t.headers) == 0 && len(t.rows) == 0 {
		return
	}

	// Determine the number of columns
	numCols := 0

	// Check header colspans to determine actual column count
	if len(t.headers) > 0 {
		headerColCount := 0
		for i := range t.headers {
			if cs, ok := t.headerCols[i]; ok {
				headerColCount += cs
			} else {
				headerColCount++
			}
		}
		numCols = headerColCount
	}

	// Also check rows for maximum column count
	for rowIdx, row := range t.rows {
		// Skip colspan rows in column counting
		if _, hasColspan := t.colspans[rowIdx]; hasColspan && len(row) == 1 {
			continue
		}
		if len(row) > numCols {
			numCols = len(row)
		}
	}

	// Calculate column widths
	colWidths := t.calculateColumnWidths(numCols)

	// Check if header is a full colspan
	headerIsFullColspan := false
	if len(t.headers) == 1 {
		if cs, ok := t.headerCols[0]; ok && cs == numCols {
			headerIsFullColspan = true
		}
	}

	// Render top border
	if t.borders {
		if headerIsFullColspan {
			t.writeLine(t.topBorderNoJunctions(colWidths))
		} else {
			t.writeLine(t.topBorder(colWidths))
		}
	}

	// Render headers
	if len(t.headers) > 0 {
		t.renderHeaders(colWidths, numCols)
		// After header, check if next row is a colspan row
		firstRowIsColspan := false
		if len(t.rows) > 0 {
			if cs, ok := t.colspans[0]; ok && len(t.rows[0]) == 1 && cs > 1 {
				firstRowIsColspan = true
			}
		}

		if headerIsFullColspan && !firstRowIsColspan {
			// Full colspan header followed by normal rows - use downward junctions
			t.writeLine(t.borderAfterColspan(colWidths))
		} else if headerIsFullColspan && firstRowIsColspan {
			// Full colspan header followed by colspan row - no junctions
			t.writeLine(t.borderNoJunctions(colWidths))
		} else {
			// Normal header - use cross junctions
			t.writeLine(t.middleBorder(colWidths, 0, numCols))
		}
	}

	// Render rows
	for rowIdx, row := range t.rows {
		colspan, hasColspan := t.colspans[rowIdx]

		if hasColspan && len(row) == 1 {
			// This is a colspan row - render as a single merged cell
			t.renderColspanRow(row[0], colWidths, colspan, numCols)
		} else {
			// Normal row
			t.renderRow(row, colWidths, numCols)
		}

		// Render row separator or bottom border
		if rowIdx < len(t.rows)-1 {
			if t.rowLines {
				// Check if next row is a colspan row
				nextRowIsColspan := false
				if nextColspan, ok := t.colspans[rowIdx+1]; ok && len(t.rows[rowIdx+1]) == 1 && nextColspan > 1 {
					nextRowIsColspan = true
				}

				// Check if current row is a colspan row
				currentRowIsColspan := hasColspan && len(row) == 1

				// Determine which border to use based on colspan context
				if currentRowIsColspan && nextRowIsColspan {
					// Between two colspan rows - no junctions
					t.writeLine(t.borderNoJunctions(colWidths))
				} else if currentRowIsColspan {
					// After a colspan row - use upward junctions (┴)
					t.writeLine(t.borderAfterColspan(colWidths))
				} else if nextRowIsColspan {
					// Before a colspan row - use downward junctions (┬)
					t.writeLine(t.borderBeforeColspan(colWidths))
				} else {
					// Between normal rows - use cross junctions (┼)
					t.writeLine(t.middleBorder(colWidths, 0, numCols))
				}
			}
		}
	}

	// Render bottom border
	if t.borders {
		t.writeLine(t.bottomBorder(colWidths))
	}
}

func (t *Table) calculateColumnWidths(numCols int) []int {
	widths := make([]int, numCols)

	// Check headers
	for i, header := range t.headers {
		if i < numCols {
			width := newANSI(header).Len()
			if width > widths[i] {
				widths[i] = width
			}
		}
	}

	// Check rows
	for rowIdx, row := range t.rows {
		// Skip colspan rows in width calculation
		if _, hasColspan := t.colspans[rowIdx]; hasColspan && len(row) == 1 {
			// For colspan rows, we don't update individual column widths
			// The total width is calculated based on the colspan
			continue
		}

		for i, cell := range row {
			if i < numCols {
				width := newANSI(cell).Len()
				if width > widths[i] {
					widths[i] = width
				}
			}
		}
	}

	// Add padding
	for i := range widths {
		widths[i] += 2 * t.padding
	}

	return widths
}

func (t *Table) renderHeaders(colWidths []int, numCols int) {
	line := t.styledChar(t.dividers.NS)

	headerIdx := 0
	colIdx := 0

	for colIdx < numCols && headerIdx < len(t.headers) {
		header := t.headers[headerIdx]
		colspan := 1

		// Check if this header has a colspan
		if cs, ok := t.headerCols[headerIdx]; ok {
			colspan = cs
		}

		// Calculate total width for this header (including spanned columns)
		// The colWidths already include padding, and we need to account for
		// the separator characters that would normally appear between columns
		totalWidth := 0
		for i := 0; i < colspan && colIdx+i < numCols; i++ {
			totalWidth += colWidths[colIdx+i]
		}
		// When spanning multiple columns, we need to add space for the
		// separator characters that are "absorbed" by the span
		if colspan > 1 {
			separatorWidth := runewidth.StringWidth(stripANSI(t.styledChar(t.dividers.NS)))
			totalWidth += (colspan - 1) * separatorWidth
		}

		// Apply header style and alignment
		content := newANSI(header)
		if t.headerStyle != aquatable.StyleNormal {
			content = newANSI(fmt.Sprintf("\x1b[%dm%s\x1b[0m", t.headerStyle, header))
		}

		// Center align by default for headers, with padding
		paddedContent := t.addPadding(content, totalWidth, aquatable.AlignCenter)
		line += paddedContent.String() + t.styledChar(t.dividers.NS)

		colIdx += colspan
		headerIdx++
	}

	// Fill remaining columns if any
	for colIdx < numCols {
		line += strings.Repeat(" ", colWidths[colIdx]) + t.styledChar(t.dividers.NS)
		colIdx++
	}

	t.writeLine(line)
}

func (t *Table) renderRow(row []string, colWidths []int, numCols int) {
	line := t.styledChar(t.dividers.NS)

	for i := range numCols {
		var content ansiBlob
		if i < len(row) {
			content = newANSI(row[i])
		} else {
			content = newANSI("")
		}

		// Get alignment for this column
		align := aquatable.AlignLeft
		if i < len(t.columnAlign) {
			align = t.columnAlign[i]
		}

		// Add padding and align
		paddedContent := t.addPadding(content, colWidths[i], align)
		line += paddedContent.String() + t.styledChar(t.dividers.NS)
	}

	t.writeLine(line)
}

func (t *Table) renderColspanRow(content string, colWidths []int, colspan int, numCols int) {
	line := t.styledChar(t.dividers.NS)

	// Calculate total width for the colspan
	// The colWidths already include padding, and we need to account for
	// the separator characters that would normally appear between columns
	totalWidth := 0
	for i := 0; i < colspan && i < numCols; i++ {
		totalWidth += colWidths[i]
	}
	// When spanning multiple columns, we need to add space for the
	// separator characters that are "absorbed" by the span
	if colspan > 1 {
		separatorWidth := runewidth.StringWidth(stripANSI(t.styledChar(t.dividers.NS)))
		totalWidth += (colspan - 1) * separatorWidth
	}

	// Center align the content with padding
	cellContent := newANSI(content)
	paddedContent := t.addPadding(cellContent, totalWidth, aquatable.AlignCenter)
	line += paddedContent.String() + t.styledChar(t.dividers.NS)

	t.writeLine(line)
}

// stripANSI is a helper to remove ANSI codes from styled characters
func stripANSI(s string) string {
	result := ""
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		result += string(r)
	}
	return result
}

func (t *Table) addPadding(content ansiBlob, width int, align Alignment) ansiBlob {
	// Width already includes 2*padding from calculateColumnWidths
	// We need to: add padding, align content, then add padding again if needed

	// Available width for content after removing padding spaces
	contentWidth := max(width-(2*t.padding), 0)

	// Align the content within the available width
	aligned := t.alignCell(content, contentWidth, align)

	// Add padding on both sides
	paddingStr := strings.Repeat(" ", t.padding)
	return newANSI(paddingStr + aligned.String() + paddingStr)
}

func (t *Table) alignCell(content ansiBlob, width int, align Alignment) ansiBlob {
	padSize := width - content.Len()
	if padSize <= 0 {
		return content
	}

	switch align {
	case aquatable.AlignRight:
		return newANSI(strings.Repeat(" ", padSize) + content.String())
	case aquatable.AlignCenter:
		leftPad := padSize / 2
		rightPad := padSize - leftPad
		result := content.String()
		if leftPad > 0 {
			result = strings.Repeat(" ", leftPad) + result
		}
		if rightPad > 0 {
			result = result + strings.Repeat(" ", rightPad)
		}
		return newANSI(result)
	default: // aquatable.AlignLeft
		return newANSI(content.String() + strings.Repeat(" ", padSize))
	}
}

func (t *Table) topBorder(colWidths []int) string {
	line := t.styledChar(t.dividers.ES)
	for i, width := range colWidths {
		line += strings.Repeat(t.styledChar(t.dividers.EW), width)
		if i < len(colWidths)-1 {
			line += t.styledChar(t.dividers.ESW)
		}
	}
	line += t.styledChar(t.dividers.SW)
	return line
}

func (t *Table) topBorderNoJunctions(colWidths []int) string {
	line := t.styledChar(t.dividers.ES)
	totalWidth := 0
	for _, width := range colWidths {
		totalWidth += width
	}
	// Add separator widths for all columns except first
	for i := 1; i < len(colWidths); i++ {
		totalWidth += runewidth.StringWidth(stripANSI(t.styledChar(t.dividers.NS)))
	}
	line += strings.Repeat(t.styledChar(t.dividers.EW), totalWidth)
	line += t.styledChar(t.dividers.SW)
	return line
}

func (t *Table) middleBorder(colWidths []int, startCol int, endCol int) string {
	line := t.styledChar(t.dividers.NES)
	for i, width := range colWidths {
		line += strings.Repeat(t.styledChar(t.dividers.EW), width)
		if i < len(colWidths)-1 {
			line += t.styledChar(t.dividers.ALL)
		}
	}
	line += t.styledChar(t.dividers.NSW)
	return line
}

func (t *Table) borderNoJunctions(colWidths []int) string {
	line := t.styledChar(t.dividers.NES)
	totalWidth := 0
	for _, width := range colWidths {
		totalWidth += width
	}
	// Add separator widths for all columns except first
	for i := 1; i < len(colWidths); i++ {
		totalWidth += runewidth.StringWidth(stripANSI(t.styledChar(t.dividers.NS)))
	}
	line += strings.Repeat(t.styledChar(t.dividers.EW), totalWidth)
	line += t.styledChar(t.dividers.NSW)
	return line
}

func (t *Table) borderAfterColspan(colWidths []int) string {
	// After a colspan row - use downward junctions (┬)
	line := t.styledChar(t.dividers.NES)
	for i, width := range colWidths {
		line += strings.Repeat(t.styledChar(t.dividers.EW), width)
		if i < len(colWidths)-1 {
			line += t.styledChar(t.dividers.ESW) // ┬ downward junction
		}
	}
	line += t.styledChar(t.dividers.NSW)
	return line
}

func (t *Table) borderBeforeColspan(colWidths []int) string {
	// Before a colspan row - use upward junctions (┴)
	line := t.styledChar(t.dividers.NES)
	for i, width := range colWidths {
		line += strings.Repeat(t.styledChar(t.dividers.EW), width)
		if i < len(colWidths)-1 {
			line += t.styledChar(t.dividers.NEW) // ┴ upward junction
		}
	}
	line += t.styledChar(t.dividers.NSW)
	return line
}

func (t *Table) bottomBorder(colWidths []int) string {
	line := t.styledChar(t.dividers.NE)
	for i, width := range colWidths {
		line += strings.Repeat(t.styledChar(t.dividers.EW), width)
		if i < len(colWidths)-1 {
			line += t.styledChar(t.dividers.NEW)
		}
	}
	line += t.styledChar(t.dividers.NW)
	return line
}

func (t *Table) styledChar(char string) string {
	if t.lineStyle != aquatable.StyleNormal {
		return fmt.Sprintf("\x1b[%dm%s\x1b[0m", t.lineStyle, char)
	}
	return char
}

func (t *Table) writeLine(line string) {
	fmt.Fprintf(t.writer, "%s\n", line)
}
