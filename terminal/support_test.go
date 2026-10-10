package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestProgressErasePreservesCursorScreenAndHistory(t *testing.T) {
	for _, width := range []int{1, 2, 5} {
		for _, height := range []int{1, 3} {
			for x := range width {
				for y := range height {
					t.Run(fmt.Sprintf("%dx%d/%d,%d", width, height, x, y), func(t *testing.T) {
						cells := make([][]byte, height)
						want := make([][]byte, height)
						for row := range height {
							cells[row] = bytes.Repeat([]byte{'a'}, width)
							want[row] = bytes.Clone(cells[row])
							if row >= y {
								from := 0
								if row == y {
									from = x
								}
								for col := from; col < width; col++ {
									want[row][col] = ' '
								}
							}
						}
						cursorX, cursorY := x, y
						savedX, savedY := -1, -1
						archived := false
						parser := ansi.NewParser()
						parser.SetHandler(ansi.Handler{
							HandleEsc: func(command ansi.Cmd) {
								switch command.Final() {
								case '7':
									savedX, savedY = cursorX, cursorY
								case '8':
									cursorX, cursorY = savedX, savedY
								}
							},
							HandleCsi: func(command ansi.Cmd, params ansi.Params) {
								amount, _, _ := params.Param(0, 1)
								switch command.Final() {
								case 'B':
									cursorY = min(cursorY+amount, height-1)
								case 'C':
									cursorX = min(cursorX+amount, width-1)
								case 'K', 'J':
									for col := cursorX; col < width; col++ {
										cells[cursorY][col] = ' '
									}
									if command.Final() == 'J' {
										archived = archived || cursorX == 0 && cursorY == 0
										for row := cursorY + 1; row < height; row++ {
											for col := range width {
												cells[row][col] = ' '
											}
										}
									}
								}
							},
						})
						parser.Parse([]byte(progressEraseBelow(width, height)))
						if cursorX != x || cursorY != y {
							t.Errorf("cursor = (%d,%d), want (%d,%d)", cursorX, cursorY, x, y)
						}
						for row := range height {
							if !bytes.Equal(cells[row], want[row]) {
								t.Errorf("row %d = %q, want %q", row, cells[row], want[row])
							}
						}
						if archived {
							t.Error("erase archived progress rows into history")
						}
					})
				}
			}
		}
	}
}

func TestSynchronizedOutputWriterReportsOriginalBytes(t *testing.T) {
	for _, synchronized := range []bool{false, true} {
		t.Run(fmt.Sprintf("synchronized=%t", synchronized), func(t *testing.T) {
			payload := ansi.EraseScreenBelow
			if synchronized {
				payload = ansi.SetModeSynchronizedOutput + payload + ansi.ResetModeSynchronizedOutput
			}
			var output bytes.Buffer
			writer := synchronizedOutputWriter{writer: &output}
			written, err := writer.Write([]byte(payload))
			if err != nil || written != len(payload) {
				t.Fatalf("Write = (%d, %v), want (%d, nil)", written, err, len(payload))
			}
			if !bytes.Contains(output.Bytes(), []byte(progressEraseBelow(0, 0))) {
				t.Fatalf("erase was not adapted: %q", output.String())
			}
			writer.writer = shortProgressWriter{}
			written, err = writer.Write([]byte(payload))
			if written != 0 || !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("short Write = (%d, %v), want (0, %v)", written, err, io.ErrShortWrite)
			}
		})
	}
}

type shortProgressWriter struct{}

func (shortProgressWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }
