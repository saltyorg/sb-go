package spinners

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const liveTaskOutputLines = 8
const terminalCapabilitySettleDelay = 250 * time.Millisecond

// synchronizedOutputWriter asks compatible terminals to apply each renderer
// update atomically.
type synchronizedOutputWriter struct {
	writer io.Writer
	mu     sync.Mutex
}

func (w *synchronizedOutputWriter) Fd() uintptr {
	if file, ok := w.writer.(interface{ Fd() uintptr }); ok {
		return file.Fd()
	}
	return ^uintptr(0)
}

func (w *synchronizedOutputWriter) Read(output []byte) (int, error) {
	if reader, ok := w.writer.(io.Reader); ok {
		return reader.Read(output)
	}
	return 0, io.EOF
}

func (w *synchronizedOutputWriter) Close() error {
	return nil
}

func (w *synchronizedOutputWriter) Write(output []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if bytes.Contains(output, []byte(ansi.SetModeSynchronizedOutput)) {
		return w.writer.Write(output)
	}

	frame := make([]byte, 0, len(ansi.SetModeSynchronizedOutput)+len(output)+len(ansi.ResetModeSynchronizedOutput))
	frame = append(frame, ansi.SetModeSynchronizedOutput...)
	frame = append(frame, output...)
	frame = append(frame, ansi.ResetModeSynchronizedOutput...)
	written, err := w.writer.Write(frame)
	if err != nil {
		return 0, err
	}
	if written != len(frame) {
		return 0, io.ErrShortWrite
	}
	return len(output), nil
}

type taskOutputBuffer struct {
	lines   []string
	current []rune
	cursor  int
	parser  *ansi.Parser
}

func (b *taskOutputBuffer) WriteString(output string) {
	if b.parser == nil {
		b.parser = ansi.NewParser()
	}
	b.parser.SetHandler(ansi.Handler{
		Print: b.writeRune,
		Execute: func(char byte) {
			b.writeRune(rune(char))
		},
		HandleCsi: func(command ansi.Cmd, params ansi.Params) {
			if command.Final() != 'K' {
				return
			}
			mode, _, _ := params.Param(0, 0)
			switch mode {
			case 0:
				b.current = b.current[:min(b.cursor, len(b.current))]
			case 2:
				b.current = b.current[:0]
				b.cursor = 0
			}
		},
	})
	for i := range len(output) {
		b.parser.Advance(output[i])
	}
}

func (b *taskOutputBuffer) writeRune(char rune) {
	switch char {
	case '\r':
		b.cursor = 0
	case '\n':
		b.lines = append(b.lines, string(b.current))
		b.current = b.current[:0]
		b.cursor = 0
	case '\b':
		if b.cursor > 0 {
			b.cursor--
		}
	default:
		if b.cursor < len(b.current) {
			b.current[b.cursor] = char
		} else {
			b.current = append(b.current, char)
		}
		b.cursor++
	}
}

func (b *taskOutputBuffer) String() string {
	if len(b.lines) == 0 {
		return string(b.current)
	}
	if len(b.current) == 0 {
		return strings.Join(b.lines, "\n")
	}
	return strings.Join(b.lines, "\n") + "\n" + string(b.current)
}

func appendLiveTaskOutput(lines []string, depth int, output string) []string {
	output = strings.TrimSpace(output)
	if output == "" {
		return lines
	}
	outputLines := strings.Split(output, "\n")
	if len(outputLines) > liveTaskOutputLines {
		outputLines = outputLines[len(outputLines)-liveTaskOutputLines:]
	}
	prefix := strings.Repeat("  ", depth)
	for _, line := range outputLines {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, prefix+line)
		}
	}
	return lines
}

func getStyle(colorName string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorName))
}
